# KVDB — System Architecture

**Hackathon:** Kill It and Prove It — 24-Hour Engineering Hackathon  
**Deliverable:** 5 — Architecture Diagram and Robustness Proof  
**Database engine:** Go (`./kvdb`)  
**Torture harness:** Python (`./torture`)

---

## 1. What KVDB does

KVDB is a small, embedded key-value database. It stores records in one
authoritative file called `data.db`.

The database uses an **append-only log**:

- A new `put` adds a new record to the end of the file.
- A `delete` adds a delete record; it does not immediately remove old bytes.
- An in-memory index called **KeyDir** points to the latest record for each key.
- On restart, KVDB scans the file and rebuilds KeyDir.
- Compaction creates a new file containing only live records, then replaces
  the old file safely.

**Important rule:** KVDB must not report success before the write has passed
the file synchronization step (`File.Sync()`).

---

## 2. High-level architecture

This view shows the main parts of the system and how they connect.

```text
          CLIENTS AND TESTING
   +-------------------------------+
   | CLI commands                  |
   | Python Torture Harness        |
   | Serve mode (JSON-Lines IPC)   |
   +---------------+---------------+
                   |
                   v
          +-------------------+
          |   Go KVDB Engine  |
          |                   |
          | Request handling |
          | Lock management   |
          | Write / Read      |
          | Recovery          |
          | Compaction        |
          +----+---------+----+
               |         |
          +----v---+  +--v----------------+
          | KeyDir |  | Append-only log   |
          | (RAM)  |  | data.db           |
          +--------+  +-------------------+
                           |
                    +------v-------+
                    | Compaction   |
                    | temp file    |
                    +--------------+
```

### Main components

| Component | Responsibility |
|---|---|
| CLI | Provides `put`, `get`, `del`, `batch`, `dump`, and `check` commands. |
| Serve Daemon | Accepts JSON-Lines requests and sends responses to clients. |
| Python Torture Harness | Runs random operations, kills the engine, restarts it, and checks the recovered data. |
| Go KVDB Engine | Executes database operations and manages storage. |
| Sidecar Lock | `data.db.lock` with exclusive `flock(LOCK_EX)` prevents another process from opening the same database concurrently. |
| `sync.RWMutex` | Coordinates access to shared in-process engine state. |
| KeyDir | In-memory hash index that maps each key to its latest record location and metadata. |
| `data.db` | Authoritative persistent database file. |
| Compaction Temp File | Holds the replacement database while compaction is in progress. |

---

## 3. Low-level architecture

This view shows the internal write, recovery, and compaction paths.

```text
                         GO KVDB ENGINE
  +----------------------------------------------------------+
  |                                                          |
  |  Request                                                  |
  |     |                                                    |
  |     v                                                    |
  |  Lock + Input Validation                                 |
  |     |                                                    |
  |     v                                                    |
  |  Quota Guard: valid_eof + frame_size <= max_bytes         |
  |     |                                                    |
  |     v                                                    |
  |  UABE Frame Encoder                                       |
  |     |                                                    |
  |     v                                                    |
  |  WriteAt(data.db, valid_eof)                              |
  |     |                                                    |
  |     v                                                    |
  |  File.Sync()                                              |
  |     |                                                    |
  |     v                                                    |
  |  Update KeyDir + Advance Sequence                        |
  |     |                                                    |
  |     v                                                    |
  |  Return Success / ACK                                    |
  |                                                          |
  +----------------------------------------------------------+
```

### Write and commit sequence

1. The client sends a `put`, `delete`, or `batch` request.
2. The engine validates the key and value limits.
3. The engine acquires the exclusive in-process write lock.
4. The UABE encoder builds the frame in memory.
5. The Quota Guard checks that the frame fits within `max_bytes`.
6. The writer appends the complete frame at `valid_eof` using `WriteAt`.
7. The engine calls `File.Sync()` and waits for it to return successfully.
8. The engine applies the operation to KeyDir and advances `valid_eof` and
   `next_seq_no`.
9. The lock is released and the caller receives success. Serve mode emits
   the ACK only after the engine reports success.

**Crash behaviour**

- If the process dies before the frame is complete, recovery does not accept
  the incomplete frame.
- If the frame is complete and passes validation, recovery can accept it.
- If `File.Sync()` succeeds but the process dies before the ACK reaches the
  client, the operation may still be present after restart.
- An operation that was acknowledged must be present after restart.

---

## 4. On-disk file format

The authoritative file is `data.db`.

```text
data.db
|
+-- File Header (32 bytes)
|
+-- UABE Frame 1
|   +-- Frame Header (28 bytes)
|   +-- Payload (one or more operation records)
|   +-- Payload CRC32C (4 bytes)
|
+-- UABE Frame 2
|   +-- Frame Header (28 bytes)
|   +-- Payload
|   +-- Payload CRC32C (4 bytes)
|
+-- UABE Frame 3
    +-- Frame Header (28 bytes)
    +-- Payload
    +-- Payload CRC32C (4 bytes)
```

All integer fields use **little-endian** byte order. Checksums use **CRC32C
(Castagnoli)**.

### File header — 32 bytes

| Field | Size | Description |
|---|---:|---|
| Magic | 8 bytes | `KVDB2026` |
| Version | 4 bytes | Format version `1` |
| Generation | 8 bytes | Compaction generation |
| Reserved flags | 8 bytes | Currently `0` |
| Header CRC32C | 4 bytes | Checksum over the first 28 bytes |

### UABE frame header — 28 bytes

| Field | Size | Description |
|---|---:|---|
| Frame magic | 4 bytes | `0x55414245` (`UABE`) |
| Header CRC32C | 4 bytes | Checksum over header bytes 8–27 |
| Payload length | 8 bytes | Payload size |
| Mutation sequence | 8 bytes | Global sequence number |
| Operation count | 4 bytes | Number of operations, from 1 to 10,000 |

### Operation record

| Field | Size | Description |
|---|---:|---|
| Operation type | 1 byte | `0x01` = PUT, `0x02` = DEL |
| Key length | 2 bytes | Unsigned 16-bit integer; 1–1,024 bytes |
| Value length | 4 bytes | Unsigned 32-bit integer; 0–1,048,576 bytes |
| Key | Variable | Raw key bytes |
| Value | Variable | Raw value bytes; zero length for DEL |

Each frame ends with a 4-byte payload CRC32C checksum.

---

## 5. Recovery after a crash

KeyDir exists only in memory. When KVDB restarts, it rebuilds KeyDir from
the authoritative file.

```text
             OPEN DATABASE
                   |
                   v
          Acquire sidecar lock
                   |
                   v
          Validate 32-byte header
                   |
                   v
          Scan frames from byte 32
                   |
                   v
       Validate frame header + CRC32C
                   |
             +-----+-----+
             |           |
           Valid       Invalid /
             |         incomplete
             v           |
       Apply operations  |
       to KeyDir         |
             |           |
             v           v
       Scan next     Stop at last
         frame       valid frame
                         |
                         v
                  Truncate invalid tail
                  and File.Sync()
                         |
                         v
                    Engine ready
```

### Recovery steps

1. Open the sidecar lock file and acquire exclusive `flock`.
2. Check whether `data.db` exists and has at least 32 bytes.
3. If a new file is needed, write the 32-byte header and synchronize it.
4. Read and validate the existing file header.
5. Start scanning at byte offset 32 using a 64 KB buffered reader.
6. Read each 28-byte frame header and validate its magic and header CRC32C.
7. Check the payload length and operation-count limits.
8. Read the complete payload and its 4-byte CRC32C trailer.
9. Apply operations only after the complete frame passes validation.
10. Update `valid_eof` and `next_seq_no`, then continue scanning.
11. Stop at clean EOF or the first incomplete or invalid frame.
12. If invalid bytes remain after the last valid frame, truncate to
    `valid_eof` and call `File.Sync()`.
13. Clean up an orphan compaction temp file when safe and bring the engine
    online.

---

## 6. Crash-safe compaction

Compaction removes obsolete records and reduces database file size.

**Trigger conditions:**

- `dead_bytes > 50%`
- `total_size > 10MB`

Both conditions must be true.

```text
       OLD data.db
            |
            v
     Read live records
       using KeyDir
            |
            v
   Write data.db.compact.tmp
            |
            v
      File.Sync() temp
            |
            v
     Atomic rename to
        data.db
            |
            v
    Sync parent directory
            |
            v
    Reopen database and
       rebuild KeyDir
```

### Compaction steps

1. Acquire the exclusive engine write lock.
2. Create `data.db.compact.tmp` with generation `old_gen + 1`.
3. Copy live records from the old database using KeyDir.
4. Call `File.Sync()` on the temporary file and close it.
5. Atomically rename the temporary file over `data.db`.
6. Open and synchronize the parent directory.
7. Reopen the authoritative database file and rebuild KeyDir.
8. Release the exclusive lock.

**Crash behaviour**

- Before rename: the old `data.db` remains authoritative; the temp file can
  be cleaned up on the next open.
- After rename: the replacement file is authoritative. Startup validates
  its header and frames.

---

## 7. Concurrency and disk-full protection

### Concurrency

KVDB uses two levels of locking:

- **Process-level:** exclusive `flock(LOCK_EX)` on `data.db.lock` prevents
  another process from operating on the same database.
- **In-process:** `sync.RWMutex` coordinates access to shared state. Writers
  use the exclusive lock.

### Disk-full protection

Before writing a frame, the Quota Guard checks:

`valid_eof + frame_size <= max_bytes`

If the next frame exceeds the configured limit, the write must fail cleanly.
Previously committed data must remain readable and recoverable.

---

## 8. The three correctness guarantees

### 1. Durability

The engine calls `File.Sync()` before publishing the mutation and returning
success. Therefore, once an operation is acknowledged, its frame has passed
the durability barrier.

### 2. Batch atomicity

A batch is encoded as one UABE frame. Recovery validates the complete frame
before applying its operations. A partial or invalid frame is not replayed.

### 3. Recoverability

KeyDir is rebuilt from validated frames in `data.db`. Incomplete tail data
is discarded. Compaction writes and synchronizes a replacement before
renaming it over the authoritative file.

---

## 9. How the design is tested

The Python Torture Harness:

1. Starts KVDB in Serve mode as a child process.
2. Sends random operations, including concurrent writes and batches.
3. Records every successful ACK in its own independent oracle log.
4. Kills the database process using `SIGKILL` at random times.
5. Restarts KVDB and runs integrity checks.
6. Compares the recovered database with the independent oracle.
7. Repeats the test using a reproducible random seed.

The harness also targets large values, large batches, recovery crashes,
compaction crashes, and disk-full behaviour.

---

## 10. Summary

KVDB has one authoritative append-only database file, a rebuildable
in-memory index, explicit write synchronization, validated recovery, and
copy-on-write compaction.

**The central promise is simple: an acknowledged operation must survive a
process crash. The torture harness repeatedly tries to prove that promise
wrong.**

> **Important submission requirement:** The hackathon brief also requires a
> separate scenario-to-mechanism proof table covering all 32 scenarios in
> Groups A–F. Each row must include the disk state at the crash, the mechanism,
> the code location, and the test or verification evidence. This document
> explains the architecture but does not replace that table.

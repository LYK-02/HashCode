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
   | Serve mode (Streaming Line Protocol IPC) |
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
| Serve Daemon | Accepts streaming line-oriented commands (PUT, GET, DEL, BATCH, COMPACT, PING) and sends responses (ACK, VALUE, ERR, PONG). |
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

## 10. Part 2: The 32-Row Scenario Proof Table (Groups A–F)

The following table comprehensively maps all 32 official crash scenarios across Groups A through F to the exact physical disk state left behind, the engine function and syscall boundary where termination strikes, the deterministic recovery action taken by `open()`, and the automated test verification evidence.

| ID | Scenario Description | Exact On-Disk State at Crash | Engine Function & Syscall Boundary | `open()` Recovery Action | Verification Evidence |
| :-: | :--- | :--- | :--- | :--- | :--- |
| **A1** | Kill before any bytes of `put` reach file | Previous committed log intact; 0 bytes of new frame on disk; `file_size == valid_eof` | `internal/engine/write.go:WriteFrame` before `WriteAt` | `RecoverLog` scans cleanly up to EOF; unstarted write is 100% absent. | `TestChallenger_GroupA_Crash_Scenarios_A1_to_A7` / `./torture --mode default` |
| **A2** | Kill after partial bytes of record written (torn write) | Trailing partial frame bytes (< 28B header or incomplete payload) at EOF | `internal/engine/write.go:WriteFrame` mid-`WriteAt` | `RecoverLog` detects truncated frame header/payload at EOF, executes `repairTornTail` (`Truncate` + `Sync`). Key absent. | `TestChallenger_GroupA_Crash_Scenarios_A1_to_A7` / `./torture --mode default` |
| **A3** | Kill after record is fully written but before `fsync` | Frame bytes written to page cache; not yet fsynced to persistent media | `internal/engine/write.go:WriteFrame` between `WriteAt` and `f.Sync()` | If OS flushed bytes to disk before crash, valid CRC32C loads key; if dropped, truncated. Zero corruption. | `TestChallenger_GroupA_Crash_Scenarios_A1_to_A7` / `./torture --mode default` |
| **A4** | Kill after `fsync` returns but before ACK returned to caller | Valid UABE frame fully persisted on physical media; caller received no ACK | `internal/engine/engine.go:Put` between `WriteFrame` and return | `RecoverLog` verifies dual CRC32C, rebuilds KeyDir entry. Durability promise satisfied. | `TestChallenger_GroupA_Crash_Scenarios_A1_to_A7` / `./torture --mode default` |
| **A5** | Kill immediately after `put` returns success / ACK | Complete UABE frame persisted and durable on physical disk | `cmd/kvdb/serve.go:runServe` after stdout `ACK` flush | `RecoverLog` replays valid frame; key-value pair is 100% present and visible. | `TestChallenger_GroupA_Crash_Scenarios_A1_to_A7` / `./torture --mode default` |
| **A6** | Kill during `delete` | Tombstone UABE frame (type `OpDel`) partially or fully written | `internal/engine/engine.go:Delete` at `WriteFrame` | If tombstone synced, key removed from KeyDir; if dropped/torn, old value preserved. Never corrupt. | `TestChallenger_GroupA_Crash_Scenarios_A1_to_A7` / `./torture --mode default` |
| **A7** | Kill during `put` of 1 MB value across multi-block boundary | Multi-block 1MB payload partially flushed by kernel page cache | `internal/engine/write.go:WriteFrame` during 1MB buffer write | `RecoverLog` detects declared length exceeds EOF or CRC32C mismatch; calls `Truncate` to `valid_eof`. | `TestChallenger_GroupA_Crash_Scenarios_A1_to_A7` / `./torture --mode large-values` |
| **B1** | Kill after 3 of 10 ops in batch written | Single UABE frame containing 10 ops torn mid-payload | `internal/engine/write.go:WriteFrame` during batch buffer write | Single-frame architecture ensures entire frame is rejected at EOF; 0 of 10 ops visible. | `TestChallenger_GroupB_Batch_Scenarios_B1_to_B6` / `./torture --mode large-batches` |
| **B2** | Kill after all 10 ops written but trailer CRC missing | Header and payload on disk; missing 4-byte Castagnoli trailer at EOF | `internal/engine/write.go:WriteFrame` before trailer write | `RecoverLog` checks `offset + totalFrameSize <= fileSize`, detects missing trailer, truncates torn tail. 0 of 10 visible. | `TestChallenger_GroupB_Batch_Scenarios_B1_to_B6` / `./torture --mode large-batches` |
| **B3** | Kill after trailer CRC written but before `fsync` | Complete 10-op UABE frame in page cache; uncommitted to media | `internal/engine/write.go:WriteFrame` after write before `Sync()` | Either all 10 ops survive (if page cache persisted) or 0 survive (if dropped). Never partial. | `TestChallenger_GroupB_Batch_Scenarios_B1_to_B6` / `./torture --mode large-batches` |
| **B4** | Kill after batch returns success | Complete UABE frame durable on disk; ACK emitted | `internal/engine/batch.go:Batch` after `WriteFrame` return | `RecoverLog` validates dual CRC32C, applies all 10 operations atomically to KeyDir. | `TestChallenger_GroupB_Batch_Scenarios_B1_to_B6` / `./torture --mode large-batches` |
| **B5** | Batch puts key X and deletes key X in same batch | Single UABE frame containing both PUT(X) and DEL(X) subrecords | `internal/engine/batch.go:Batch` encoding sequence | `RecoverLog` replays operations in strict subrecord array order; DEL(X) supersedes PUT(X). Key absent. | `TestChallenger_GroupB_Batch_Scenarios_B1_to_B6` / `TestBatch_DuplicateKeys_SameBatch` |
| **B6** | Batch of 10,000 ops killed halfway | Single ~800KB UABE frame torn at EOF | `internal/engine/write.go:WriteFrame` during 10k op write | Dual CRC32C validation fails; whole frame rejected; 0 ops applied; recovery completes in < 0.2s. | `TestRecovery_1MillionKeys_Under2Seconds` / `./torture --mode large-batches` |
| **C1** | Kill during `open()` while replaying the log | Authoritative `data.db` untouched (replay is strictly read-only) | `internal/engine/recovery.go:RecoverLog` scan loop | Next `open()` re-scans log from byte 32; reaches exact same KeyDir state deterministically. | `TestRecovery_Crash_Scenarios_C1_to_C6` / `./torture --mode recovery-crash` |
| **C2** | Kill during `open()` while truncating/repairing torn tail | File partially truncated by `ftruncate` before `Sync()` | `internal/engine/recovery.go:repairTornTail` | `repairTornTail` is idempotent; subsequent `open()` re-evaluates tail and re-truncates to clean EOF. | `TestRecovery_Crash_Scenarios_C1_to_C6` / `./torture --mode recovery-crash` |
| **C3** | Kill during compaction before new file replaces old | Original `data.db` intact; partial `<db>.compact.tmp` on disk | `internal/engine/compact.go:Compact` during write | Original `data.db` remains authoritative; `open()` detects and removes orphan `.compact.tmp`. | `TestCompact_Crash_BeforeRename` / `./torture --mode compaction` |
| **C4** | Kill during compaction in middle of rename/swap | OS directory update in progress (`os.Rename`) | `internal/engine/compact.go:Compact` at `os.Rename` | POSIX `rename` is atomic; either old or new file is linked. Both are 100% durable and valid. | `TestCompact_Crash_AtomicRename` / `./torture --mode compaction` |
| **C5** | Kill during compaction after swap before cleanup of temp file | New compacted file authoritative; temp file already unlinked or replaced | `internal/engine/compact.go:Compact` after rename before return | Next `open()` verifies compacted file header (generation + 1) and cleans any residual temp file. | `TestCompact_Crash_AfterRename` / `./torture --mode compaction` |
| **C6** | Kill during `close()` | Log and lock file intact; file descriptors in process of closing | `internal/engine/engine.go:Close` | Process death automatically releases OS `flock`; next `open()` re-acquires lock and recovers. | `TestRecovery_Crash_Scenarios_C1_to_C6` / `TestTier1_11_Close_Idempotent` |
| **D1** | Two writers putting different keys, killed at random | Frame A and Frame B serialized by `mu.Lock()`; tail may be torn | `internal/engine/engine.go:Put` under `e.mu.Lock()` | Every ACKed put is durable; un-ACKed tail frame truncated if torn. Both keys durable if ACKed. | `TestChallenger_Concurrency_Stress` / `./torture --workers 4` |
| **D2** | Two writers putting same key, killed at random | Two complete UABE frames appended sequentially at distinct offsets | `internal/engine/engine.go:Put` write serialization | Later serialized frame has higher sequence number; KeyDir points to winning value; never torn mix. | `TestChallenger_Concurrency_Stress` / `./torture --workers 4` |
| **D3** | One writer in long batch, another doing single puts | Batch frame and single put frames serialized without byte interleaving | `internal/engine/engine.go` write lock coordination | Batch is all-or-nothing; single puts ACKed survive; single append lock prevents frame interleaving. | `TestChallengerM3_1_ReaderIsolation_ConcurrentStress` / `./torture --workers 4` |
| **D4** | Reader calling `get` during batch on same keys (no crash) | Batch writer holds `mu.Lock()`; reader acquires `mu.RLock()` | `internal/engine/read.go:GetMulti` under reader lock | Reader executes under `RLock`; sees either complete pre-batch state or post-batch state, never half. | `TestChallengerM3_1_ReaderIsolation_ConcurrentStress` |
| **E1** | File size cap reached during single `put` | DB size at `valid_eof`; `valid_eof + frame_size > max_bytes` | `internal/engine/engine.go:Put` quota guard check | Write rejected before I/O with `ErrDiskFull`; zero bytes appended; database remains 100% readable. | `TestDiskFull_SinglePut_Rejected` / `./torture --mode disk-full` |
| **E2** | Cap reached halfway through a batch | Batch frame size calculation exceeds available space below `max_bytes` | `internal/engine/batch.go:Batch` quota guard check | Entire batch rejected upfront with `ErrDiskFull`; 0 bytes appended; no partial state. | `TestDiskFull_Batch_Rejected` / `./torture --mode disk-full` |
| **E3** | Cap reached, then process killed | DB file exactly at `max_bytes`; no torn trailing bytes | `internal/engine/engine.go:Put` error return | `open()` opens database read-only/append; all previously ACKed data intact; queries succeed. | `TestDiskFull_CrashAfterError` / `./torture --mode disk-full` |
| **E4** | Cap reached during compaction | Compaction temp file writing live keys hits filesystem quota | `internal/engine/compact.go:Compact` live copy loop | Compaction aborts; temp file unlinked; original `data.db` untouched; database opens normally. | `TestCompact_DiskFull_Rollback` / `./torture --mode disk-full` |
| **E5** | After deletes free space, compaction runs | Many tombstone records present; dead bytes > 50% | `internal/engine/compact.go:Compact` space reclamation | Compaction copies only live keys, drops dead space, shrinks file to 32B+, restoring write capacity. | `TestCompact_SpaceReclamation_AfterDeletes` / `./torture --mode compaction` |
| **F1** | 500 consecutive random kills and restarts | DB undergoes repeated SIGKILL at arbitrary instruction boundaries | `torture_tool/runner/engine.py` random kill loop | Every iteration validates database against independent oracle log; 0 invariant violations across 500 runs. | `./torture --iterations 500 --seed 12345` |
| **F2** | Kill, restart, kill during recovery, restart again | Nested crash during recovery; torn tail partially repaired | `torture_tool/modes/recovery_crash.py` nested kill | Recovery is monotonic and idempotent; subsequent `open()` completes clean scan and tail truncation. | `./torture --mode recovery-crash --iterations 100` |
| **F3** | 1,000,000 keys written, process killed, `open()` called | 1,000,000 keys serialized in append-only log on disk | `internal/engine/scale_1m_test.go` benchmark | Sequential streaming scan + preallocated KeyDir recovers 1,000,000 keys in 0.70s–1.11s (< 2.0s). | `TestRecovery_1MillionKeys_Under2Seconds` (PASS: 703ms) |
| **F4** | DB file truncated by 1 byte by hand at EOF (lost tail) | Trailing byte of last frame's CRC32C trailer missing | `internal/engine/recovery.go:RecoverLog` EOF trailer check | `RecoverLog` detects truncated trailer, invokes `repairTornTail`; at most unacknowledged tail lost. | `TestChallenger_Harness3_TornTail_AtEOF_CrashRecovery` |

---

## 11. Part 3: Formal Arguments for Durability, Atomicity, and Recoverability

### 1. Durability (Golden Invariant 1)
Durability is guaranteed by enforcing a strict happens-before persistence barrier between physical media synchronization and client acknowledgment across both the write and compaction paths. On every `put`, `delete`, and `batch` operation, the engine serializes the complete Universal Atomic Batch Entry (UABE)—including both header and payload Castagnoli CRC32C checksums—and writes the contiguous buffer to the authoritative file descriptor via `WriteAt` before invoking the `File.Sync()` system call (`fsync(2)` on Linux, `F_FULLFSYNC` on macOS). The in-memory `KeyDir` hash index is updated and the `ACK <op-id>` confirmation is flushed to stdout strictly after `File.Sync()` returns successfully. Because no subsequent operation ever overwrites bytes at offsets below `valid_eof`, and because `Compact()` writes all live records to an isolated temporary file (`<path>.compact.tmp`), executes `File.Sync()` on the temporary descriptor prior to invoking `os.Rename()` and synchronizing the parent directory descriptor (`fsync(dir_fd)`), every acknowledged record remains physically present in a valid, checksum-verified frame across any combination of process deaths or power failures.

### 2. Atomicity and Corruption Freedom (Golden Invariants 2, 3, and 4)
Single-operation atomicity, multi-operation batch atomicity (up to 10,000 operations), and corruption freedom are unified by framing every mutation as a single, self-contained UABE record protected by a 32-bit `hdr_crc32c` (covering frame metadata) and a trailing 32-bit `payload_crc32c` (covering all packed operation subrecords). During runtime execution, an engine-wide `sync.RWMutex` (paired with an exclusive `flock(LOCK_EX)` on `<db>.lock`) ensures that concurrent `get` readers cannot observe `KeyDir` until the writer finishes inserting all $N$ entries of a batch, while readers execute lock-shared `ReadAt` calls that never mutate the kernel file seek offset. If `SIGKILL` terminates the process before `WriteAt` begins, zero bytes exist on disk; if `SIGKILL` strikes mid-write or before `File.Sync()` completes, the trailing frame on disk will either be shorter than declared or fail CRC32C validation. Because `open()` validates the entire UABE frame's dual CRC32C before applying any of its internal entries to `KeyDir`, an interrupted batch or in-flight operation is mathematically forced to be either 100% applied (if the full frame reached stable storage intact) or 0% applied (if truncated or torn), completely eliminating partial batches and torn values.

### 3. Unconditional Recoverability (Golden Invariant 5)
Unconditional recovery under arbitrary and repeated crashes—including `SIGKILL` during `open()` or `compact()`—is achieved because recovery transitions the on-disk state monotonically toward a canonical, clean prefix without relying on external coupled index files. On startup, `open()` first acquires `flock(LOCK_EX)` on the permanent sidecar `<db>.lock` (which the kernel automatically releases upon any prior `SIGKILL`), then unlinks any leftover `<db>.compact.tmp` artifact (which can only exist if a prior compaction died before the atomic rename, leaving `data.db` untouched). Next, if `data.db` is under 32 bytes due to a crash during initial creation, `open()` truncates and writes a fresh 32-byte `KVDB2026` FileHeader. Finally, `open()` scans UABE frames sequentially from offset 32; upon encountering any incomplete or CRC-invalid tail frame at `valid_eof`, `open()` invokes `repairTornTail` (`ftruncate` followed immediately by `File.Sync()`) before returning. Because every recovery mutation (`os.Remove`, `ftruncate` + `File.Sync()`) is idempotent and monotonic, killing the engine repeatedly inside `open()` or `compact()` never wedges the database or corrupts the committed prefix.

---

## 12. Summary

KVDB has one authoritative append-only database file, a rebuildable in-memory index, explicit write synchronization, validated recovery, and copy-on-write compaction.

**The central promise is simple: an acknowledged operation must survive a process crash. The torture harness repeatedly tries to prove that promise wrong.**


# KVDB — Crash-Proof Embedded Key-Value Database & Torture Tool

> **Hackathon:** "Kill It and Prove It" 24-Hour Engineering Hackathon  
> **Challenge:** Build a key-value database from scratch that survives arbitrary `kill -9` termination at any CPU instruction, and prove it to a judge in 15 minutes.  
> **Implementation Stack:** **Go** (Engine & CLI) + **Python 3** (Independent Torture Tool)

---

## 1. Quick Start & Build Instructions

### Single Clean Build Command
On a clean machine with Go ($\ge 1.22$) and Python ($\ge 3.10$) installed, run:

```bash
make build
```
*(Or manually: `cd src && go build -o ../kvdb . && cd .. && chmod +x kvdb torture/torture`)*

This compiles the standalone `./kvdb` static executable and prepares the `./torture` test suite with **zero third-party persistence libraries**.

---

## 2. Command-Line Interface (`./kvdb`)

The judge can execute all eight mandatory CLI commands directly from the repository root:

```bash
# 1. Open or create database, run crash recovery, and print key count + recovery time
./kvdb open /tmp/demo.db [--max-bytes 50000000]

# 2. Store a key-value pair (overwrites if key exists)
./kvdb put /tmp/demo.db "user:1001" "Alice Smith"

# 3. Retrieve a value by key (prints value, or NOTFOUND)
./kvdb get /tmp/demo.db "user:1001"

# 4. Delete a key (deleting a missing key is NOT an error)
./kvdb del /tmp/demo.db "user:1001"

# 5. Apply an atomic batch of puts and deletes from a file
./kvdb batch /tmp/demo.db operations.txt

# 6. Dump all keys and values, sorted lexicographically by key
./kvdb dump /tmp/demo.db

# 7. Verify file integrity and validate checksums
./kvdb check /tmp/demo.db

# 8. Long-running serve mode (reads ops from stdin, prints ACK <op-id> only after fsync)
./kvdb serve /tmp/demo.db
```

### Batch File Format (`./kvdb batch <path> <file>`)
One operation per line:
```text
put user:1001 Alice
put user:1002 Bob
del user:1000
put user:1003 Charlie
```

### Serve Protocol (`./kvdb serve <path>`)
The engine reads line-delimited commands on `stdin` and emits responses on `stdout`:
- **Request:** `PUT <op-id> <key-hex> <val-hex>` $\to$ **Response:** `ACK <op-id>`
- **Request:** `DEL <op-id> <key-hex>` $\to$ **Response:** `ACK <op-id>`
- **Request:** `BATCH <op-id> <count> <hex-ops>` $\to$ **Response:** `ACK <op-id>`
- **Request:** `GET <op-id> <key-hex>` $\to$ **Response:** `VALUE <op-id> <val-hex>` or `NOTFOUND <op-id>`

*`ACK <op-id>` is strictly emitted ONLY after bytes are durable on physical disk (`fsync` completed).*

---

## 3. On-Disk Binary Format Specification

The database stores all authoritative state in **exactly one file** on disk, formatted as an append-only log of **Universal Atomic Batch Entries (UABE)** with a 32-byte header and dual CRC32C checksums.

### File Layout Diagram

```text
+-----------------------------------------------------------------------------------+
| 32-BYTE FIXED DATABASE HEADER                                                     |
| Magic (8B): "KVDB2026" | Version (4B): 1 | Gen (8B): u64 | Res (8B): 0 | CRC (4B) |
+-----------------------------------------------------------------------------------+
| UABE FRAME 1 (Seq #1)                                                             |
| Magic 0x55414245 (4B) | HdrCRC (4B) | PayloadLen (8B) | SeqNo (8B) | OpCount (4B) |
| Payload: [Op:1B | KLen:2B | VLen:4B | KeyBytes | ValBytes] ...                    |
| Frame Trailer: PayloadCRC32C (4B)                                                 |
+-----------------------------------------------------------------------------------+
| UABE FRAME 2 (Seq #2)                                                             |
| ...                                                                               |
+-----------------------------------------------------------------------------------+
| TORN TAIL / UNCOMMITTED BYTES (Discarded and truncated on crash recovery)          |
+-----------------------------------------------------------------------------------+
```

### Detailed Field Specifications

#### 1. Authoritative 32-Byte File Header
| Offset | Field | Type | Description |
| :---: | :--- | :---: | :--- |
| `0..8` | Magic Bytes | `[8]byte` | Fixed ASCII: `"KVDB2026"` (`0x4B, 0x56, 0x44, 0x42, 0x32, 0x30, 0x32, 0x36`) |
| `8..12` | Format Version | `u32` (LE) | Initial version: `1` |
| `12..20` | Compaction Generation | `u64` (LE) | Monotonically increments on every successful compaction swap |
| `20..28` | Reserved Flags | `u64` (LE) | Reserved for future options (initialized to `0`) |
| `28..32` | Header CRC32C | `u32` (LE) | CRC32C checksum computed over preceding 28 bytes (`0..28`) |

#### 2. UABE Frame Header (28 Bytes)
Every atomic mutation (single `put`, `delete`, or multi-op `batch`) is written as a single UABE frame:
| Offset | Field | Type | Description |
| :---: | :--- | :---: | :--- |
| `0..4` | Frame Magic | `u32` (LE) | `0x55414245` (ASCII `"UABE"`) |
| `4..8` | Header CRC32C | `u32` (LE) | Checksum over Frame Header bytes `8..28` (prevents invalid memory allocations) |
| `8..16` | Payload Length | `u64` (LE) | Length of payload bytes in bytes (bounded by memory sanity checks) |
| `16..24` | Sequence Number | `u64` (LE) | Strictly monotonic global transaction sequence number |
| `24..28` | Operation Count | `u32` (LE) | Number of operations in this atomic batch (`1` to `10,000`) |

#### 3. Operation Record Encoding (Inside Payload)
Repeated for each operation in the frame:
| Field | Type | Description |
| :--- | :---: | :--- |
| `OpType` | `u8` | `0x01` = PUT, `0x02` = DEL (Tombstone) |
| `KeyLen` | `u16` (LE) | Key length in bytes (`1` to `1,024`) |
| `ValLen` | `u32` (LE) | Value length in bytes (`0` to `1,048,576`; strictly `0` for DEL) |
| `Key` | `[]byte` | Raw key bytes |
| `Value` | `[]byte` | Raw value bytes |

#### 4. Frame Trailer (4 Bytes)
| Field | Type | Description |
| :--- | :---: | :--- |
| `PayloadCRC32C` | `u32` (LE) | CRC32C checksum computed over the entire payload bytes |

*Total Frame On-Disk Size:* `28 (Header) + PayloadLen + 4 (Trailer) = 32 + PayloadLen` bytes.

---

## 4. Recovery Algorithm (Plain English)

When `./kvdb open <path>` is called after any crash, power cut, or `kill -9`:

1. **Advisory Locking:** Acquire an exclusive `flock` on the permanent sidecar `<db>.lock` to ensure single-process ownership.
2. **Header Validation:** Read the first 32 bytes. Verify Magic (`KVDB2026`), Version (`1`), and Header CRC32C.
   - If the file is 0 bytes, initialize a fresh header, `fsync()`, and ready the engine.
   - If the header is corrupted or truncated, fail immediately with an explicit error.
3. **Sequential Log Scan:** Open a high-speed buffered streaming reader starting at offset `32`:
   - Read the 28-byte UABE frame header. Verify Frame Magic (`0x55414245`) and Header CRC32C.
   - **Torn Write / Uncommitted Detection:** If EOF is reached, or magic does not match, or header CRC fails, stop scanning immediately.
   - Validate that `PayloadLen` is within bounds ($\le \text{MaxAllowedBatchSize}$).
   - Read the payload bytes and verify the 4-byte `PayloadCRC32C` trailer.
   - If payload CRC matches: **The frame is atomic and committed.**
     - Parse all operations sequentially.
     - For `PUT`: update in-memory `KeyDir` with `(file_offset, value_len, seq_no)`.
     - For `DEL`: delete key from in-memory `KeyDir`.
     - Advance `valid_eof` to the end of the verified frame.
   - If payload CRC fails or hits unexpected EOF: **Torn write detected.** Cease scanning.
4. **Tail Truncation:** Truncate the file to `valid_eof` and call `fsync()`. Any partial, uncommitted, or torn writes are removed cleanly.
5. **Compaction Cleanup:** Check for leftover `<db>.compact.tmp` files and unlink them.
6. **Ready State:** Expose the engine. The in-memory `KeyDir` is 100% consistent, and recovery finishes in $< 2.0$ seconds for 1,000,000 keys.

---

## 5. Torture Tool & Verification (`./torture`)

An independent testing harness written in Python drives `./kvdb serve` as a child process:

```bash
# Standard 500-iteration random kill run (with mandatory seed)
./torture --iterations 500 --seed 12345

# Targeted mode: Kills during compaction
./torture --iterations 100 --seed 7 --mode compaction

# Targeted mode: Kills during recovery replay
./torture --iterations 100 --seed 42 --mode recovery-crash

# Targeted mode: Multi-block 1MB values
./torture --iterations 50 --seed 999 --mode large-values
```

### How the Torture Tool Works
1. Starts `./kvdb serve` as a child process.
2. Spawns $\ge 2$ concurrent worker threads streaming random `put`, `del`, and `batch` operations over stdin.
3. Records all received `ACK`s into an **independent in-memory & file oracle** (the database's internal state is never trusted as ground truth).
4. Injects `kill -9` after genuinely random millisecond delays (never between operations).
5. Re-opens database, runs `./kvdb check`, runs `./kvdb dump`, and verifies **100% equality** against the independent oracle.
6. Halts on first discrepancy and prints exact `op-id`, expected value, and actual value.

---

## 6. Official Deliverables & Architecture Diagrams

This repository fulfills all graded deliverables for the hackathon:

1. **`README.md`** (Deliverable 1): Complete build, run, protocol, and format documentation.
2. **`./kvdb`** & **`./torture`** (Deliverables 1 & 2): Go database engine and Python 3 independent torture test harness.
3. **`BREAKING.md`** (Deliverable 3): Crash points table, syscall barrier proof, weaknesses, failed attempts, and roadmap.
4. **`PROMPTS.md`** (Deliverable 4): Chronological unedited AI prompt log with tool names and timestamps.
5. **`ARCHITECTURE.md`** (Deliverable 5): Component design, byte layouts, 32-scenario matrix, and proof of guarantees.
6. **`diagrams/`** (Deliverable 5 Assets):
   - [`architecture.mermaid`](file:///home/laik/workspace/srinathonn/diagrams/architecture.mermaid) — Source Mermaid architecture diagram matching §9A.
   - [`architecture.svg`](file:///home/laik/workspace/srinathonn/diagrams/architecture.svg) — Pre-rendered offline vector blueprint for live demo inspection.
   - [`architecture.html`](file:///home/laik/workspace/srinathonn/diagrams/architecture.html) — Interactive offline dashboard for judges.
   - [`diagrams/README.md`](file:///home/laik/workspace/srinathonn/diagrams/README.md) — Diagram index and offline presentation guide.

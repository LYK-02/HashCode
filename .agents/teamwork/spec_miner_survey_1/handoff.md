# Comprehensive Specification Mining Report: Go Storage Engine & CLI (`kvdb`)

> **Agent Identity:** `spec_miner_survey_1`  
> **Target Package:** Go Key-Value Storage Engine & judge-facing CLI `./kvdb`  
> **Source Documents Audited:** 45 Markdown files in `docs/` (`docs/engine/`, `docs/common/`, `docs/torture_tool/`), `.agents/rules/01_invariants.md` through `10_code_quality_and_practices.md`, `README.md`, `ARCHITECTURE.md`, `BREAKING.md`, and `ORIGINAL_REQUEST.md`.  
> **Date:** 2026-11-06  

---

## 1. Observation

### 1.1 Verbatim Authority Quotes and File Locations

1. **Stack Decision:**
   - `docs/DESIGN_DECISIONS.md:3-7`:  
     > "Selected stack: Go engine + Python torture tool... Use Go for the engine and CLI, Python 3 for the external torture tool. The latest supplied Antigravity Engine Final Plan explicitly assigns Go to `src/` and Python to `torture/`."
   - `docs/README.md:22`:  
     > "This documentation chooses Go + Python to keep one coherent implementation plan aligned with the latest Antigravity plan. Do not mix Go and Rust implementation instructions."

2. **The Core Invariant & Durability Promise:**
   - `.agents/rules/01_invariants.md:5-15`:
     > "1. Acknowledged writes are durable: Every operation (`put`, `delete`, `batch`) that returned success (`ACK`) before the crash is present in the database.  
     > 2. Unacknowledged writes are atomic: Every operation that had not returned success at the moment of the crash is either fully present or fully absent.  
     > 3. Batches are all-or-nothing: Every batch is treated as one atomic operation. A batch is never partially present.  
     > 4. Zero corruption: No key ever holds garbage, torn values, or data from a different key.  
     > 5. Guaranteed recovery: `open()` never fails or crashes on a database file produced by this engine, regardless of how or when it was killed."
   - `docs/engine/05_WRITE_AND_COMMIT_PROTOCOL.md:21-25`:
     > "A write call returning nil error does not mean durable. A successful fsync is the barrier. Never update KeyDir or return success before it... Emit ACK <op-id> and flush stdout."

3. **On-Disk File Header (32 Bytes):**
   - `docs/common/04_SHARED_DATA_FORMAT.md:17-18`:
     > "All persisted multi-byte integers are little-endian. The authoritative file begins with a fixed 32-byte header. Header offsets: `0..8` magic bytes `KVDB2026`; `8..12` format version (`u32`, initially 1); `12..20` compaction generation (`u64`); `20..28` reserved flags (`u64`, zero); `28..32` header CRC32C (`u32`) over bytes `0..28`."
   - `docs/engine/03_BINARY_FILE_FORMAT.md:17-18`:
     > "The database file is a 32-byte fixed header followed by contiguous UABE frames. All integers are little-endian. Header: bytes 0–7 magic `KVDB2026`; 8–11 version `1`; 12–19 compaction generation; 20–27 reserved zero; 28–31 CRC32C over first 28 bytes."

4. **UABE Frame Structure (28-byte Header + Payload + 4-byte Trailer):**
   - `docs/common/04_SHARED_DATA_FORMAT.md:21`:
     > "Each UABE frame has a 28-byte fixed header, payload, and 4-byte trailer. Frame header offsets relative to frame: `0..4` magic `0x55414245` (`UABE`, u32); `4..8` header CRC32C over bytes `8..28`; `8..16` payload length (`u64`); `16..24` sequence number (`u64`); `24..28` operation count (`u32`). Payload starts at frame offset 28. Trailer is the four-byte CRC32C of the entire payload. Total frame size is `32 + payload_len`."
   - `docs/engine/04_RECORD_ENCODING_AND_CHECKSUMS.md:17`:
     > "Payload is a concatenation of exactly `op_count` operation subrecords. Each starts with `op_type:u8`, `key_len:u16`, `value_len:u32`, then `key[key_len]`, then `value[value_len]`. PUT type is 0x01; DEL type is 0x02. Key length is 1–1,024. Value length is 0–1,048,576. DEL requires value length zero."
   - `docs/engine/04_RECORD_ENCODING_AND_CHECKSUMS.md:29`:
     > "CRC32C uses the Castagnoli polynomial. Do not accidentally use IEEE CRC32. In Go, `hash/crc32` with `crc32.Castagnoli` is the intended standard-library primitive."

5. **Locking & Single File:**
   - `docs/engine/11_CONCURRENCY_AND_LOCKING.md:25`:
     > "Cross-process lock: permanent `<db>.lock` sidecar, exclusive flock from open through close. Lock the sidecar before recovery, compaction, or temp cleanup. Do not unlink or rename the lock file: flock attaches to inode, and replacing the path can split ownership across two inodes."
   - `docs/engine/02_ENGINE_ARCHITECTURE.md:21`:
     > "`Engine` owns `mu sync.RWMutex`, `db *os.File`, `lock *os.File`, canonical path, parent directory path/descriptor, `maxBytes`, `validEOF`, `nextSeqNo`, compaction generation, and `keyDir map[string]KeyDirEntry`."
   - `docs/engine/02_ENGINE_ARCHITECTURE.md:33`:
     > "Lock order must be fixed: process flock first, then in-process mutex. Never acquire these in reverse order."

6. **Torn Tail Repair vs. Middle Corruption:**
   - `docs/engine/08_CRASH_RECOVERY.md:25`:
     > "At the first invalid/incomplete trailing frame, truncate file to the frame's start offset and sync the file. A one-byte truncation at EOF therefore discards only the incomplete last frame... Do not advance validEOF until repair sync succeeds."
   - `docs/engine/09_TORN_WRITE_HANDLING.md:25`:
     > "Tail policy: only a malformed suffix at EOF is treated as a torn write and truncated. If invalid data occurs before later bytes, do not blindly truncate all later data and claim acknowledged data is safe. Report middle-of-file corruption as an integrity error unless the format has a proven resynchronization scheme."

7. **Compaction Crash Safety:**
   - `docs/engine/10_COMPACTION.md:21`:
     > "Algorithm: (1) create `<db>.compact.tmp` in the same directory; (2) write a valid header with incremented compaction generation; (3) iterate live keys in deterministic key order; (4) encode current values into valid UABE frames; (5) write all bytes; (6) sync temp file; (7) close temp descriptor and check close result; (8) atomically rename temp over data path; (9) sync parent directory; (10) open/retain the new database descriptor and rebuild/swap KeyDir/EOF/sequence state; (11) release lock."

8. **Scale and Limits:**
   - `.agents/rules/01_invariants.md:35-36`:
     > "Recovery Target: `open()` must finish recovery in under 2.0 seconds with 1,000,000 keys. Data Limits: Keys 1–1,024 bytes, values 0–1 MB, batch size up to 10,000 operations, total database keys $\ge 1,000,000$."

9. **CLI Interface Commands:**
   - `docs/common/05_CLI_AND_PROTOCOL_SPEC.md:17`:
     > "The judge-facing commands, invoked from repository root, are:  
     > `./kvdb open <path> [--max-bytes N]`  
     > `./kvdb put <path> <key> <value>`  
     > `./kvdb get <path> <key>`  
     > `./kvdb del <path> <key>`  
     > `./kvdb batch <path> <file>`  
     > `./kvdb dump <path>`  
     > `./kvdb check <path>`  
     > `./kvdb serve <path>`"

10. **Banned Dependencies:**
    - `.agents/rules/02_banned_libraries.md:6-14`:
      > "NO third-party database, storage engine, or persistence libraries (SQLite, LevelDB, RocksDB, LMDB, BoltDB, Badger, etc.). Standard library only. NO network access (100% offline)."

---

## 2. Logic Chain

1. **Storage Subsystem Invariant:**
   - Standard OS `write()` system calls buffer data in kernel page cache. A process termination via `kill -9` or kernel crash before sync leaves writes unpersisted or torn.
   - Therefore, a durable storage engine must strictly invoke `file.Sync()` (`fsync`) and verify return code `nil` *before* (a) updating the in-memory lookup index `KeyDir`, and (b) emitting the success `ACK <op-id>` on `stdout`.
   - By structuring all atomic mutations—whether single `put`, `del`, or multi-op `batch`—as a single contiguous UABE frame, atomicity is guaranteed at the frame envelope layer without requiring a separate two-phase commit marker.

2. **Frame Checksumming and Defense in Depth:**
   - Recovery must parse frames sequentially from offset 32.
   - If memory allocations were performed based directly on untrusted `payload_len` from disk, a single byte corruption or malicious disk image could trigger an immediate Out-Of-Memory panic (e.g. `payload_len = 0xFFFFFFFFFFFFFFFF`).
   - Hence, the 28-byte UABE header is split into an independently checksummed header: bytes 8..27 (payload length, sequence number, operation count) are checksummed into `Header CRC32C` at bytes 4..8.
   - The scanner validates `Frame Magic == 0x55414245` and `Header CRC32C` *before* allocating any memory. Furthermore, `payload_len` is capped against maximum allowable batch size ($10,000 \times (7 + 1024 + 1048576)$).
   - Only after reading the payload is the trailer `PayloadCRC32C` validated. If it fails at EOF, it is classified as a torn write and truncated; if in the middle of valid frames, it indicates catastrophic non-tail corruption and fails closed.

3. **In-Memory Index (`KeyDir`) and Sequential Recovery:**
   - Because no secondary persistent index is maintained on disk, `data.db` is the sole source of truth.
   - On `open()`, the engine scans from byte 32 to `validEOF`. For each valid frame, operations are applied sequentially to `keyDir map[string]KeyDirEntry`.
   - Each `KeyDirEntry` stores only `(fileOffset uint64, valueLen uint32, seqNo uint64)`. The absolute value offset is calculated deterministically as:
     $$\text{valueOffset} = \text{frameStart} + 28 + \text{payloadOffset} + 7 + \text{keyLen}$$
   - This keeps memory footprint bounded for 1,000,000 keys ($\sim 60\text{ MB}$ in Go) and allows recovery to execute in $\le 2.0$ seconds via sequential scanning and 64KB+ read buffering.

4. **Crash-Safe Compaction:**
   - Log compaction cannot append in place. It must rebuild live entries into `<db>.compact.tmp`.
   - The temporary file must be fully written, `Sync()`-ed, closed, atomically renamed over `data.db` via `os.Rename`, and followed immediately by `parentDir.Sync()` to ensure directory entry persistence across OS crashes.
   - During compaction, the writer lock `mu.Lock()` prevents concurrent mutations from creating torn sequence numbers.
   - If a crash occurs before `os.Rename`, the original database is intact; on restart, the orphan `.compact.tmp` is unlinked. If a crash occurs after `os.Rename`, the new compacted file is already complete and durable.

---

## 3. Features Discovered

| # | Category | Feature | Description | Inputs | Outputs | Error Behavior | Discovered Via |
|---|----------|---------|-------------|--------|---------|----------------|----------------|
| 1 | Lifecycle | Database File Header | 32-byte fixed header at offset 0 identifying file format and compaction generation. | File descriptor at offset 0. | 32 bytes on disk. | Header < 32 bytes or bad magic/CRC returns `CORRUPT_HEADER`. | `docs/engine/03_BINARY_FILE_FORMAT.md:17` |
| 2 | Lifecycle | Process-Level Exclusive Lock | Permanent sidecar file `<db>.lock` acquired via `unix.Flock(fd, unix.LOCK_EX\|unix.LOCK_NB)`. | Database path string. | Open lock file descriptor held for lifetime. | Returns `LOCKED` if another process holds flock; released on `kill -9`. | `docs/engine/11_CONCURRENCY_AND_LOCKING.md:25` |
| 3 | Codec | UABE Frame Encoder | Serializes single operations or atomic batches into 28B header + payload + 4B trailer. | List of 1–10,000 operations (`put`/`del`). | Contiguous byte slice `32 + payload_len`. | Returns `INVALID_ARGUMENT`, `KEY_TOO_LARGE`, `VALUE_TOO_LARGE`, `BATCH_TOO_LARGE`. | `docs/common/04_SHARED_DATA_FORMAT.md:21` |
| 4 | Codec | Dual CRC32C Validation | Header CRC32C over bytes 8..27, Trailer CRC32C over payload bytes using Castagnoli polynomial. | Encoded byte buffers. | `uint32` Little-Endian checksums. | Mismatch rejects frame; never panics. | `docs/engine/04_RECORD_ENCODING_AND_CHECKSUMS.md:29` |
| 5 | Storage | Position-Independent Append | Writes complete frame at `validEOF` using full write loop (`WriteAt` / `Pwrite`). | `[]byte` frame buffer, offset `validEOF`. | Number of bytes written; updates `validEOF`. | Short write/EIO/ENOSPC triggers rollback to `lastSyncedEOF` and truncation. | `docs/engine/09_TORN_WRITE_HANDLING.md:17` |
| 6 | Storage | Sync Barrier Before ACK | Mandatory `file.Sync()` prior to index publication and output emission. | `*os.File` | OS fsync completion. | Sync failure triggers rollback; if rollback fails, engine poisoned (`ENGINE_POISONED`). | `docs/engine/05_WRITE_AND_COMMIT_PROTOCOL.md:21` |
| 7 | Storage | Logical Quota Enforcement | Configurable `max_bytes` checked via overflow-safe arithmetic `validEOF + frameSize <= maxBytes`. | `max_bytes uint64`, `frameSize uint64`. | Boolean accept / reject. | Returns `DISK_FULL` without mutating disk or index; file remains readable. | `docs/engine/12_DISK_FULL_AND_ERROR_HANDLING.md:17` |
| 8 | Index | In-Memory KeyDir | Hash map mapping keys to `(fileOffset, valueLen, seqNo)`. | Key string / slice. | Value locator. | Non-existent key returns `NOTFOUND` (distinct from 0-length value). | `docs/engine/13_IN_MEMORY_INDEX.md:17` |
| 9 | Mutation | PUT Operation | Appends UABE frame with `op_type=0x01`; updates KeyDir locator to new frame value bytes. | `key []byte` (1..1024), `value []byte` (0..1MB). | Success (nil error) + index updated. | Oversized key/val returns error; disk full returns `DISK_FULL`. | `docs/engine/06_PUT_GET_DELETE.md:17` |
| 10 | Mutation | DEL Operation (Tombstone) | Appends UABE frame with `op_type=0x02` and `value_len=0`; deletes key from KeyDir. | `key []byte` (1..1024). | Success; key removed from live KeyDir. | Deleting missing key is valid and durable. | `docs/engine/06_PUT_GET_DELETE.md:25` |
| 11 | Mutation | Atomic Batch Operation | Encodes 1..10,000 PUT/DEL subrecords into a single UABE frame; atomic commit. | File or slice of operations. | All applied or none applied. | Fails if any subrecord invalid; killed batch leaves zero partial keys. | `docs/engine/07_BATCH_ATOMICITY.md:17` |
| 12 | Query | GET Operation | Look up KeyDir locator under read lock; reads value bytes using `ReadAt` without seek cursor. | `key []byte`. | Copied `[]byte` value, `found bool`, `err`. | Key missing returns `found=false`; read error returns `IO_READ`. | `docs/engine/06_PUT_GET_DELETE.md:21` |
| 13 | Maintenance | Crash-Safe Compaction | Rewrites live keys to `<db>.compact.tmp`, syncs, closes, renames, syncs parent directory, swaps index. | Engine instance. | Shrunk database file with incremented generation. | ENOSPC/kill during temp build leaves original `data.db` intact. | `docs/engine/10_COMPACTION.md:21` |
| 14 | Recovery | Startup Replay & Tail Truncation | Scans frames from offset 32; on EOF torn write, truncates to last valid frame and syncs. | Authoritative `data.db`. | Populated `KeyDir`, verified `validEOF`. | Middle corruption fails closed (`CORRUPT_FRAME`); torn tail truncated cleanly. | `docs/engine/08_CRASH_RECOVERY.md:21` |
| 15 | Concurrency | Process & Thread Synchronization | `flock` on lock sidecar + in-process `sync.RWMutex`; reader blocked during batch. | Goroutines / threads. | Thread-safe data access. | Reader observes pre-batch or post-batch state; never partial state. | `docs/engine/11_CONCURRENCY_AND_LOCKING.md:17` |
| 16 | CLI | `open` Command | Opens/creates database, runs recovery, prints key count and recovery elapsed time. | `<path> [--max-bytes N]`. | stdout: `keys: <count>, recovery: <time>`. | Nonzero exit on lock or corruption. | `docs/common/05_CLI_AND_PROTOCOL_SPEC.md:17` |
| 17 | CLI | `put` Command | Stores key and value; overwrites if existing. | `<path> <key> <value>`. | Exit code 0 on success. | Nonzero exit on error (e.g. disk full). | `docs/common/05_CLI_AND_PROTOCOL_SPEC.md:17` |
| 18 | CLI | `get` Command | Prints value of key, or `NOTFOUND`. | `<path> <key>`. | stdout: value bytes, or `NOTFOUND`. | Exit code 0 if found/notfound; nonzero on I/O. | `docs/common/05_CLI_AND_PROTOCOL_SPEC.md:17` |
| 19 | CLI | `del` Command | Removes key from database; deleting missing key is not an error. | `<path> <key>`. | Exit code 0. | Nonzero exit on I/O. | `docs/common/05_CLI_AND_PROTOCOL_SPEC.md:17` |
| 20 | CLI | `batch` Command | Reads batch file with `put k v` / `del k` per line; executes atomically. | `<path> <file>`. | Exit code 0. | Fails without applying any operation if syntax/validation error occurs. | `docs/common/05_CLI_AND_PROTOCOL_SPEC.md:17` |
| 21 | CLI | `dump` Command | Dumps all live key/value pairs sorted lexicographically by key. | `<path>`. | stdout: sorted pairs deterministically. | Nonzero exit on I/O. | `docs/common/05_CLI_AND_PROTOCOL_SPEC.md:17` |
| 22 | CLI | `check` Command | Validates database header and all frames sequentially. | `<path>`. | stdout: `OK` or detailed error reason. | Nonzero exit on corruption. | `docs/common/05_CLI_AND_PROTOCOL_SPEC.md:17` |
| 23 | CLI | `serve` Command | Long-running stdin/stdout multiplexed protocol engine. | `<path>`. | stdout: `ACK <id>`, `VALUE <id> <hex>`, `ERR <id> ...`. | Protocol lines flushed immediately; diagnostics on stderr. | `docs/engine/15_SERVE_MODE.md:17` |

---

## 4. Edge Cases

| # | Feature | Input / Condition | Observed / Required Behavior |
|---|---------|-------------------|-----------------------------|
| 1 | Database Creation | Database file 0 bytes on open. | Initializes fresh 32-byte header (`KVDB2026`, version 1, generation 0, reserved 0, CRC32C), calls `Sync()`, syncs directory, sets `validEOF=32`. |
| 2 | Header Corruption | File < 32 bytes after prior usage, or header CRC mismatch. | Rejects open with `CORRUPT_HEADER`. Never blindly overwrites or zeroes an invalid header that might contain acknowledged data. |
| 3 | Zero-Length Value | `PUT key ""` (empty value). | Valid. Encodes UABE frame with `value_len=0`. KeyDir stores locator with length 0. `GET` returns `found=true`, empty slice (distinct from `NOTFOUND`). |
| 4 | Zero-Length Key | `PUT "" value` (empty key). | Rejected with `INVALID_ARGUMENT` or `KEY_TOO_LARGE` (key length must be 1..1024 bytes). |
| 5 | Key Boundary | Key exactly 1,024 bytes; Key 1,025 bytes. | 1,024 bytes accepted; 1,025 bytes rejected before disk append with `KEY_TOO_LARGE`. |
| 6 | Value Boundary | Value exactly 1,048,576 bytes (1 MiB); 1 MiB + 1 byte. | 1 MiB accepted; 1 MiB + 1 byte rejected before disk append with `VALUE_TOO_LARGE`. |
| 7 | Delete Non-Zero Value | DEL subrecord with `value_len > 0`. | Decoder rejects frame with `CORRUPT_FRAME`; encoder enforces `value_len == 0`. |
| 8 | Delete Missing Key | `DEL missing_key`. | Officially valid; appends durable tombstone frame, updates sequence number, returns success. |
| 9 | Duplicate Keys in Batch | Batch with `PUT k v1`, then `PUT k v2`, then `DEL k`. | Processed strictly in payload order. Last operation wins (key deleted). |
| 10 | Empty Batch | `BATCH` with 0 operations. | Rejected as invalid; no frame written to disk. |
| 11 | Batch Size Limit | Batch with 10,000 operations; 10,001 operations. | 10,000 operations accepted; 10,001 operations rejected with `BATCH_TOO_LARGE`. |
| 12 | Corrupted Frame Magic | Frame magic byte `!= 0x55414245` at EOF. | Classified as torn tail. Truncates file to `validEOF` (start of corrupt frame), calls `Sync()`, recovery finishes cleanly. |
| 13 | Header CRC Mismatch | Corrupted payload length field in frame header at EOF. | Header CRC fails before allocating memory. Scan halts; file truncated to `validEOF` and synced. |
| 14 | Payload CRC Mismatch | Trailer CRC32C mismatch at EOF (e.g. killed during multi-block write). | Classified as torn write. Truncates to frame start and syncs. |
| 15 | Non-Tail Corruption | Invalid frame or CRC mismatch followed by additional valid frames. | Must NOT silently discard later frames. Fails closed with integrity error (`CORRUPT_FRAME`). |
| 16 | One-Byte Truncation (F4) | File truncated by 1 byte at EOF. | Recovery detects short frame/trailer, truncates to start of that frame, all prior ACKed frames preserved. |
| 17 | Kill During fsync | `kill -9` while kernel is executing `file.Sync()`. | Frame may or may not be valid on restart. If valid, replayed; if torn, truncated. No partial or corrupted state. |
| 18 | Kill After fsync, Before ACK | Data flushed to physical media, process killed before writing `ACK` to stdout. | Operation recovered and present in database. Oracle correctly models in-flight operations as uncertain (present or absent). |
| 19 | Disk Full on Batch (E2) | Batch requires $N$ bytes; `validEOF + N > maxBytes`. | Pre-check fails with `DISK_FULL` before writing a single byte. Zero batch operations applied to disk or KeyDir. |
| 20 | Compaction Crash Before Rename | `kill -9` while `<db>.compact.tmp` is being written or synced. | Original `<db>` remains 100% intact. Orphan temp unlinked on subsequent `open()`. |
| 21 | Compaction Crash After Rename | `kill -9` after `os.Rename` succeeds but before `parentDir.Sync()`. | New file is already authoritative inode at path. Next `open()` recovers the complete new file. |
| 22 | Concurrent Reader During Batch | Reader `GET` executing while writer is committing 10,000-op batch. | Reader acquires `mu.RLock()` which blocks until writer releases `mu.Lock()`. Reader sees either pre-batch or post-batch state, never an intermediate slice. |
| 23 | Sidecar Lock Contention | Process 2 attempts `open` while Process 1 is active. | `flock` fails with `EWOULDBLOCK` / `EAGAIN`. Process 2 returns error (`LOCKED`) and immediately exits; does not corrupt database. |
| 24 | Lock Holder Killed (`kill -9`) | Process holding `flock` is killed abruptly. | Kernel automatically closes open file descriptors and releases `flock`. Next process acquires lock cleanly without manual cleanup. |
| 25 | Sidecar File Invariant | Attempt to unlink/recreate `<db>.lock`. | Prohibited. `flock` binds to the underlying inode. Lock file is never unlinked or replaced. |

---

## 5. Phase-by-Phase Engine Implementation Breakdown (`docs/engine/PHASES.md`)

The engine must proceed strictly through phases **E0** to **E10**. No phase may be marked complete without automated test evidence.

| Phase | Title | Primary Focus & Implementations | Strict Exit Gates |
| :---: | :--- | :--- | :--- |
| **E0** | Contract and package setup | Freeze `04_SHARED_DATA_FORMAT.md` & `05_CLI_AND_PROTOCOL_SPEC.md`. Create Go module boundaries (`src/engine`, `src/storage`, `src/index`, `src/recovery`, `src/compaction`, `cmd/kvdb`). Freeze Little-Endian, dual CRC32C, error codes, sequence numbers, `max_bytes` semantics. | Golden fixtures match Python parser. Package builds offline with zero external dependencies. |
| **E1** | Codec and file header | Implement 32-byte DB header codec (Castagnoli CRC32C). Implement 28-byte UABE header codec. Implement PUT/DEL subrecords. Implement 4-byte payload CRC32C trailer. Strict bounds checking (key $\le 1024$, val $\le 1\text{MB}$, ops $\le 10000$, DEL val=0). | Hex fixtures match Python; boundary & malformed tests pass; zero panics or unbounded memory allocations on fuzzing. Codec has zero filesystem side effects. |
| **E2** | Engine lifecycle & locks | Canonicalize path. Open permanent `<db>.lock` sidecar with exclusive `flock`. Open/create data file; write 32B header on creation; sync file and directory. Initialize `sync.RWMutex`, KeyDir, `validEOF`, sequence number. Clean `Close()`. | Two processes cannot open same DB simultaneously. Killing lock holder allows new opener to acquire lock. Initial header survives close/reopen. |
| **E3** | Append & single operations | Implement mutation path: validate $\to$ encode frame in memory $\to$ acquire mutex $\to$ check quota $\to$ loop write at `validEOF` $\to$ `file.Sync()` $\to$ update KeyDir $\to$ advance `validEOF`/seq $\to$ release mutex $\to$ emit ACK. PUT overwrites; GET reads via `ReadAt`; DEL appends tombstone. | PUT/GET/DEL survive process restart. ACK emitted strictly after `Sync()`. Empty and 1MB values work. Overwrite returns newest value. No mixed values across kills. |
| **E4** | Atomic batch | Validate all ops pre-disk. Encode into single UABE frame. Preserve input order and duplicate key semantics. Max 10,000 ops. Single append and sync. All operations applied to KeyDir under single writer lock. | Recovery replays full batch or none. Reader sees pre- or post-batch state. Killed batch leaves zero partial keys. Every batch ACK = 1 durable frame. |
| **E5** | Recovery & tail repair | Sequential scan from byte 32. Read 28B header, verify magic & header CRC. Validate bounds. Read payload & trailer, verify payload CRC. Parse subrecords, update KeyDir, advance `validEOF`. On invalid/incomplete final frame at EOF: truncate to frame start, sync truncation. Middle corruption fails closed. | Truncation at every byte boundary handled cleanly. 1-byte EOF truncation drops only last uncommitted frame. Recovery kill during replay/truncation is idempotent. |
| **E6** | Serve & CLI | Implement all 8 commands (`open`, `put`, `get`, `del`, `batch`, `dump`, `check`, `serve`). Stdout protocol-only; diagnostics on stderr. Request IDs echoed. Flush stdout after every response. Maintain persistent engine instance in serve mode. | Commands match specification. Judge can send batches manually. Zero stdout banner pollution. Offline build works from clean clone. |
| **E7** | Error & disk-full hardening | Quota precheck (`validEOF + frameSize <= maxBytes`). Actual ENOSPC/EIO error handling. Rollback partial append to `lastSyncedEOF` and sync. Poison engine if rollback fails. Stable error codes. Zero KeyDir mutation on error. | E1–E5 disk-full tests pass. Failed PUT/BATCH preserves prior state. Kill after quota rejection preserves all ACKed data. No append at uncertain offset. |
| **E8** | Compaction | Exclusive writer lock. Create `<db>.compact.tmp` in same directory. Write header with incremented generation. Write live KeyDir entries in deterministic key order. Sync temp. Close temp. `os.Rename` over DB. Sync parent directory. Swap engine file descriptors and KeyDir. Handle orphan temp on open. | Old or new complete DB survives kill at every barrier. Compaction shrinks delete-heavy DB. Subsequent writes succeed. Temp ENOSPC leaves original intact. |
| **E9** | Scale & performance | Preallocate KeyDir map. Use bounded 64KB+ read buffers. Eliminate unnecessary allocations/copies in recovery. Sequential scanning. Benchmark 1,000,000 keys on target hardware. | Recovery of 1,000,000 keys in $< 2.0$ seconds. `check` and `dump` hashes prove correctness. Compaction and crash tests pass after optimization. |
| **E10** | Release verification | Verify all deliverables: `./kvdb`, `./torture`, `README.md`, `ARCHITECTURE.md`, `BREAKING.md`, `PROMPTS.md`. Castagnoli CRC32C confirmed. 32-scenario matrix completed. Offline clean build. | All 8 CLI commands work. 500-iteration torture passes with 0 invariant violations. 15-minute demo rehearsed. |

---

## 6. Exact Binary Specification Deep-Dive

### 6.1 Authoritative 32-Byte File Header
All fields are Little-Endian. Total size is exactly 32 bytes:

| Byte Range | Field Name | Type | Value / Constraint | Description |
| :---: | :--- | :---: | :---: | :--- |
| `0..8` | Magic Bytes | `[8]byte` | `"KVDB2026"` (`0x4B, 0x56, 0x44, 0x42, 0x32, 0x30, 0x32, 0x36`) | Fixed ASCII file identifier |
| `8..12` | Format Version | `uint32` | `1` | Database binary format version |
| `12..20` | Compaction Gen | `uint64` | Monotonic counter | Incremented on every completed compaction swap |
| `20..28` | Reserved Flags | `uint64` | `0` | Reserved for future options |
| `28..32` | Header CRC32C | `uint32` | CRC32C over bytes `0..28` | Castagnoli CRC32C of preceding 28 bytes |

### 6.2 28-Byte UABE Frame Header
Every mutation (single PUT, single DEL, or multi-op BATCH) begins with this fixed 28-byte header:

| Byte Range | Field Name | Type | Value / Constraint | Description |
| :---: | :--- | :---: | :---: | :--- |
| `0..4` | Frame Magic | `uint32` | `0x55414245` (`"UABE"`) | Fixed frame marker (`0x45, 0x42, 0x41, 0x55` in LE) |
| `4..8` | Header CRC32C | `uint32` | CRC32C over bytes `8..28` | Checksum protecting `PayloadLen`, `SeqNo`, `OpCount` |
| `8..16` | Payload Length | `uint64` | $0 < \text{len} \le \text{MaxPayload}$ | Total byte count of operation subrecords |
| `16..24` | Sequence Number | `uint64` | Monotonically increasing | Global sequence number assigned to this frame |
| `24..28` | Operation Count | `uint32` | $1 \le \text{count} \le 10,000$ | Number of operation subrecords in payload |

### 6.3 Operation Subrecord Encoding (Inside Payload)
Payload is composed of exactly `OpCount` consecutive subrecords:

| Byte Offset | Field Name | Type | Constraint | Description |
| :---: | :--- | :---: | :---: | :--- |
| `0..1` | `OpType` | `uint8` | `0x01` (PUT) or `0x02` (DEL) | Subrecord operation type |
| `1..3` | `KeyLen` | `uint16` (LE) | $1 \le \text{KeyLen} \le 1,024$ | Key byte count |
| `3..7` | `ValLen` | `uint32` (LE) | $0 \le \text{ValLen} \le 1,048,576$; strictly `0` for DEL | Value byte count |
| `7 .. 7+KeyLen` | `Key` | `[]byte` | Opaque raw bytes | Raw key byte array |
| `7+KeyLen .. 7+KeyLen+ValLen` | `Value` | `[]byte` | Opaque raw bytes (empty if `ValLen==0`) | Raw value byte array |

### 6.4 4-Byte Frame Trailer
Immediately follows the payload:

| Byte Offset | Field Name | Type | Description |
| :---: | :--- | :---: | :--- |
| `0..4` | `PayloadCRC32C` | `uint32` (LE) | Castagnoli CRC32C computed over the exact payload bytes (`0..PayloadLen`) |

**Total Frame On-Disk Size:**  
$$\text{TotalFrameSize} = 28 + \text{PayloadLen} + 4 = 32 + \text{PayloadLen}\text{ bytes}$$

---

## 7. Durability, Concurrency, and CLI Specifications

### 7.1 Durability Sequence & Write Ordering
Every mutation must execute this 10-step sequence:
1. **Input Validation:** Check key (1–1024B), value (0–1MB), batch op count (1–10000).
2. **In-Memory Frame Construction:** Serialize complete frame into memory buffer and compute dual Castagnoli CRCs.
3. **Lock Acquisition:** Acquire engine writer lock (`mu.Lock()`).
4. **Quota Precheck:** Check `validEOF + frameSize <= maxBytes` (using checked arithmetic).
5. **Full Write Loop:** Write all bytes at `validEOF` using `file.WriteAt()` in a loop handling short writes.
6. **Physical Sync Barrier:** Execute `file.Sync()` (`fsync`). If sync fails, trigger rollback and poison engine.
7. **KeyDir Publication:** Apply all operations to in-memory `KeyDir` in payload order.
8. **State Advancement:** Advance `validEOF += frameSize` and increment `nextSeqNo`.
9. **Lock Release:** Release `mu.Unlock()`.
10. **ACK Emission:** Emit `ACK <op-id>\n` to stdout and flush buffer.

### 7.2 Locking Architecture
- **Process Mutual Exclusion:** Open `<db>.lock` with `os.O_CREATE | os.O_RDWR` and execute `syscall.Flock(fd, syscall.LOCK_EX | syscall.LOCK_NB)`. Lock descriptor is retained for process lifetime. Never unlinked or recreated.
- **In-Process Synchronization:** Single `sync.RWMutex` protecting all engine fields (`keyDir`, `validEOF`, `nextSeqNo`, `db`). Lock hierarchy: OS `flock` $\to$ engine `sync.RWMutex`.

### 7.3 CLI Commands & Grammar
Invoked from repository root via `./kvdb`:
1. `./kvdb open <path> [--max-bytes N]` — Recovers DB, prints: `keys: <N>, recovery: <X.XXXs>`.
2. `./kvdb put <path> <key> <value>` — Persists key-value; overwrites if exists.
3. `./kvdb get <path> <key>` — Prints raw value to stdout, or prints `NOTFOUND`.
4. `./kvdb del <path> <key>` — Deletes key; succeeds even if key does not exist.
5. `./kvdb batch <path> <file>` — Reads text file with lines: `put <key> <val>` or `del <key>`. All-or-nothing atomic.
6. `./kvdb dump <path>` — Prints all live key/value pairs sorted lexicographically by key (`<key> <val>` or deterministic escaped format).
7. `./kvdb check <path>` — Verifies 32B header and all UABE frames; prints `OK` or error reason with byte offset.
8. `./kvdb serve <path>` — Long-running multiplexed mode over stdin/stdout.

### 7.4 Serve Protocol Grammar
Line-oriented text protocol over stdin/stdout:
- **Requests (stdin):**
  - `PUT <op-id> <key-hex> <val-hex>`
  - `DEL <op-id> <key-hex>`
  - `BATCH <op-id> <count> <hex-encoded-ops>`
  - `GET <op-id> <key-hex>`
  - `PING` (optional heartbeat)
- **Responses (stdout):**
  - `ACK <op-id>` (strictly emitted after physical `Sync()` and index publication)
  - `VALUE <op-id> <val-hex>`
  - `NOTFOUND <op-id>`
  - `ERR <op-id> <ERROR_CODE> <safe-message>`
- **Formatting Constraints:**
  - `stdout` is protocol-only; NO debug logs, banners, or progress text.
  - All diagnostics and error logging routed to `stderr`.
  - Every response line flushed immediately (`writer.Flush()`).

---

## 8. Caveats

1. **No Third-Party Storage Libraries:** Zero SQLite, RocksDB, BoltDB, or external persistence engines are permitted. Only Go standard library (`os`, `sync`, `hash/crc32`, `encoding/binary`, `syscall`).
2. **Offline Execution:** All builds and test runs must execute with network access disabled (100% offline).
3. **Recovery Target:** Official target is $\le 2.0$ seconds for 1,000,000 keys. Stretch targets like $< 0.4\text{s}$ are optional and must not compromise parsing bounds checks.
4. **Directory Sync Portability:** Compaction requires parent directory `fsync` after `os.Rename`. On Linux, this is achieved by `d, err := os.Open(parentDir); d.Sync(); d.Close()`.

---

## 9. Conclusion

The specification across `docs/` and `.agents/rules/` provides a comprehensive, mathematically closed, and bit-level deterministic contract for the Go storage engine.
- The 32-byte header and UABE envelope unify single mutations and atomic batches into a single crash-safe data model with dual Castagnoli CRC32C validation.
- Durability is guaranteed by strict write ordering (`payload` $\to$ `fsync` $\to$ `KeyDir` $\to$ `ACK`).
- Recovery is provably idempotent, handling torn tails via truncation to `validEOF` and failing closed on non-tail corruption.
- Compaction guarantees zero data loss via temporary file staging, directory syncing, and atomic rename.

All specifications are fully probed, documented, and ready for immediate implementation by the engine development team.

---

## 10. Verification Method

To independently verify this specification report:
1. **Header & Frame Structure Verification:**
   - Inspect `docs/engine/03_BINARY_FILE_FORMAT.md` lines 17–23 and `docs/common/04_SHARED_DATA_FORMAT.md` lines 17–25. Compare against Section 6 of this report.
2. **Phase Exit Gates Verification:**
   - Inspect `docs/engine/PHASES.md` lines 8–292 to confirm phases E0 through E10 match Section 5 of this report.
3. **Locking & Durability Verification:**
   - Inspect `docs/engine/05_WRITE_AND_COMMIT_PROTOCOL.md` lines 20–30 and `docs/engine/11_CONCURRENCY_AND_LOCKING.md` lines 17–33.
4. **CLI & Protocol Verification:**
   - Inspect `docs/common/05_CLI_AND_PROTOCOL_SPEC.md` lines 16–36 and `README.md` lines 24–70.

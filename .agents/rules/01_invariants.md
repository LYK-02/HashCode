# The Core Invariant & Engine Constraints

> **Authority:** "Kill It and Prove It" Hackathon Guide (Sections 1, 2, 4, 5)

## 1. The Core Invariant
After a crash at any point in time, and after `open()` has completed recovery:
1. **Acknowledged writes are durable:** Every operation (`put`, `delete`, `batch`) that returned success (`ACK`) before the crash is present in the database.
2. **Unacknowledged writes are atomic:** Every operation that had not returned success at the moment of the crash is either fully present or fully absent.
3. **Batches are all-or-nothing:** Every batch is treated as one atomic operation. A batch is never partially present.
4. **Zero corruption:** No key ever holds garbage, torn values, or data from a different key.
5. **Guaranteed recovery:** `open()` never fails or crashes on a database file produced by this engine, regardless of how or when it was killed.

## 2. Definition of "Returned Success"
When `put`, `delete`, or `batch` returns to the caller or emits an `ACK`, it has made a promise that the data is durable on physical disk (`fsync` completed). Returning success before physical durability is a fatal invariant violation.

## 3. Definition of "Crash"
Immediate termination via `kill -9` (or `TerminateProcess`) with zero warning and no chance to run cleanup code. Crash can happen at ANY instruction:
- In the middle of a `write()` system call (torn write).
- In the middle of `fsync()`.
- In the middle of renaming/swapping files during compaction.
- In the middle of recovery code during the next `open()`.

## 4. Mandatory Storage Constraints
- **Stack:** Go (Engine & CLI in `src/`) + Python 3 (Independent Torture Tool in `torture/`).
- **Single Authoritative Data File:** Exactly one data file on disk with a fixed 32-byte header (`KVDB2026`) and repeating UABE frames (`0x55414245`). A permanent sidecar lock file (`<db>.lock`) with exclusive `flock` ensures process-level mutual exclusion.
- **Write Ordering (UABE Frame Model):**
  1. Encode atomic UABE frame in memory (Frame Header + Operations Payload + Trailer CRC32C).
  2. Check quota: `valid_eof + frame_size <= max_bytes`.
  3. Write complete frame to file via position-independent write (`file.WriteAt` / `syscall.Pwrite`).
  4. Call `file.Sync()` (`syscall.Fsync`) as a physical durability barrier.
  5. Apply operations to in-memory `KeyDir` in payload order.
  6. Advance `valid_eof` and return success / emit `ACK <op-id>` to caller.
- **Compaction:** Must shrink the file after 1,000,000 puts followed by 1,000,000 deletes. Compaction itself must be crash-safe (write temp $\to$ `fsync(temp)` $\to$ `os.Rename` $\to$ `fsync(dir)` $\to$ swap in-memory descriptor).
- **Disk Full Simulation (`--max-bytes`):** Database must accept configurable maximum file size. Writes that exceed the cap must fail cleanly with `ErrDiskFull`; database remains readable, recoverable, and existing data is never corrupted.
- **Recovery Target:** `open()` must finish recovery in under 2.0 seconds with 1,000,000 keys.
- **Data Limits:** Keys 1–1,024 bytes, values 0–1 MB, batch size up to 10,000 operations, total database keys $\ge 1,000,000$.


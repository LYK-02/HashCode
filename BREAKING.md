# Deliverable 3: "How I Would Break This" (`BREAKING.md`)

> **Target:** Maximum 2 Pages  
> **Purpose:** Critical adversarial self-audit detailing precise failure boundaries, syscall ordering, honest architectural weaknesses, and lessons from torture testing.

---

## 1. Crash Points Table

The following table documents the critical failure boundaries in the engine's write and recovery paths:

| # | Exact Location / Function | Physical Disk State at Crash | Action Taken by `open()` on Next Recovery |
| :-: | :--- | :--- | :--- |
| **1** | `storage/log.go:WriteFrame` before `pwrite` completes | Previous committed log intact; 0 or partial bytes in OS buffer; uncommitted | Scanner validates preceding valid frame; stops at EOF; partial uncommitted bytes truncated. |
| **2** | `storage/log.go:WriteFrame` during 1MB payload write | Partial record bytes on disk; Trailer CRC32C missing; no ACK sent | Scanner detects missing/invalid Trailer CRC at EOF; safely truncates file to `valid_eof`. Key absent. |
| **3** | `storage/log.go:WriteFrame` after `pwrite` but before `Sync()` | Entire UABE frame in volatile OS page cache; not yet fsynced to platter | If bytes persisted, valid CRC loads key; if dropped, file truncated to last fsync. Zero corruption. |
| **4** | `storage/log.go:WriteFrame` after `Sync()` returns before `ACK` | UABE frame is 100% durable on disk media; caller did not receive ACK | Scanner verifies valid Frame and Trailer CRC; applies key to `KeyDir`. Acknowledged or unacknowledged write survives durably. |
| **5** | `engine/engine.go:Delete` after tombstone write before `Sync()` | Tombstone frame in OS buffer; old value on disk | Unacknowledged delete; if dropped on crash, old value remains visible; if durable, key absent. Never corrupt. |
| **6** | `engine/engine.go:Batch` after 5 of 10 operations encoded | Frame in userspace RAM; 0 bytes written to disk | Entire batch absent; previous committed state remains authoritative. |
| **7** | `compaction/compact.go:Run` during temp file rewrite | Incomplete `<path>.compact.tmp` on disk; original database untouched | Original file is authoritative. `open()` detects orphan `.compact.tmp` and unlinks it. |
| **8** | `compaction/compact.go:Run` after temp `Sync()` before `Rename` | `<path>.compact.tmp` fully synced to disk; original database untouched | `Rename` not reached; original database file remains authoritative; orphan temp unlinked. |
| **9** | `compaction/compact.go:Swap` during `os.Rename` syscall | Filesystem directory inode update in progress | Atomic rename: directory points either to old or new file; both are fully synced and complete. |
| **10** | `compaction/compact.go:Swap` after `Rename` before directory `fsync` | New file authoritative; directory metadata dirty in VFS cache | On ext4/XFS, new file opens; if metadata reverted on power loss, old file opens. Both are consistent. |
| **11** | `recovery/recover.go:ReplayLog` during tail truncation `os.Truncate` | Database file truncated halfway through torn tail | Replay is idempotent; next `open()` re-scans from byte 0 to last valid frame and completes truncation. |
| **12** | `storage/log.go:CheckQuota` on `--max-bytes` violation | Write aborted before I/O; `valid_eof` unchanged | No bytes written; `ErrDiskFull` returned; file remains readable, uncorrupted, and recoverable. |

---

## 2. The Ordering Argument

Our architecture strictly guarantees the Core Invariant through a precise, non-reorderable sequence of system calls:

```
1. pwrite(fd, frame_bytes, valid_eof)   -> Write complete UABE frame (Payload + Trailer CRC)
2. fsync(fd)                            -> Force dirty file pages to physical media
3. KeyDir.Apply(frame.Ops)              -> Update in-memory index in payload order
4. Emit "ACK <op-id>"                   -> Signal durability promise to caller
```

### Why This Sequence Guarantees Correctness:
1. **The Syscall Durability Barrier:** By placing `syscall.Fsync` (or Go `file.Sync()`) strictly after `pwrite` and strictly before emitting `ACK`, we eliminate phantom durability. An `ACK` is a contract that physical media holds the bits.
2. **Atomic Single-Frame Batches:** Rather than writing separate records followed by a vulnerable commit marker, the entire batch (up to 10,000 operations) is enclosed in a single UABE frame ending with a `PayloadCRC32C` trailer. A crash anywhere before the trailer is fsynced causes the entire frame to fail verification on recovery. Atomicity is absolute.
3. **Compaction Swap Ordering:** Compaction writes live keys to a temporary file, calls `Sync()` on the temporary file descriptor, closes it, and calls `os.Rename(temp, target)`. Crucially, we then call `fsync` on the **parent directory descriptor**. While `rename` is atomic in memory on POSIX filesystems, directory sync guarantees that the directory inode entry itself is durable against storage power cuts.

---

## 3. Known Weaknesses (Honest Architectural Limitations)

1. **Recovery Time is $O(\text{Total Log Size})$:**
   Because our engine relies on a single authoritative append-only log without a separate persisted B-tree, recovery requires sequentially scanning every UABE frame from byte 32 to EOF. If 5,000,000 writes occur without compaction, replaying all frames will exceed the 2.0-second recovery limit. Crash-safe compaction must run periodically to bound the log.
2. **Go Garbage Collection & In-Memory KeyDir Overhead:**
   Maintaining 1,000,000 keys in Go's in-memory `map[string]KeyDirEntry` consumes approximately 120–160 MB of heap memory. While easily within standard laptop limits, large GC mark-and-sweep phases under heavy concurrent writes can introduce latency spikes during `serve` mode.
3. **Compaction Disk Space Headroom Requirement:**
   Because compaction follows an out-of-place copy-on-write pattern to ensure atomicity, the filesystem must have sufficient free disk space to store both the existing database file and the temporary compacted file simultaneously. If the disk is at 95% capacity, compaction will fail with `ErrDiskFull`.

---

## 4. What We Tried That Failed

1. **Attempt 1: Separate Commit Marker Records (Discarded):**
   Initially, we tested writing individual operation records followed by a separate 8-byte `COMMIT` record. The torture tool revealed that under rapid millisecond kills, crashes between the last operation and the commit marker required complex backward-seeking rollback logic, and multi-threaded writers caused interleaved uncommitted records. We abandoned this design in favor of the **Universal Atomic Batch Entry (UABE)** format, where every mutation (single or batch) is a single, self-contained, dual-checksummed frame.
2. **Attempt 2: Advisory `flock` on the Authoritative Data File (Discarded):**
   Locking the database file itself created a race condition during compaction: when `os.Rename` swapped the new file over the old path, processes opening the path acquired locks on different inodes. We resolved this by introducing a **permanent sidecar lock file** (`<db>.lock`) that is never renamed or deleted.
3. **Attempt 3: Raw Byte Streaming Over Unix Pipes (Discarded):**
   In early prototypes of `serve` mode, writing raw binary keys containing newlines broke line-oriented readers. We adopted strict hex-encoding for binary keys and values in `serve` mode, ensuring clean, unambiguous protocol framing.

---

## 5. If We Had 24 More Hours

1. **Hardware-Accelerated CRC32C via SIMD / Assembly:**
   Implement Go assembly routines utilizing SSE4.2 / ARMv8 CRC32 instructions to double recovery scanning throughput, guaranteeing sub-0.5s recovery for 1,000,000 keys.
2. **Concurrent Background Compaction Worker:**
   Implement non-blocking double-buffered compaction where reads and writes continue appending to the active log while a background goroutine rewrites historical frames, swapping with minimal lock contention.
3. **Embedded Fixed-Size Index Checkpoint:**
   Periodically append a serialized KeyDir snapshot trailer to the log, allowing `open()` to skip sequential scanning of historical log segments and start scanning only from the last snapshot offset.


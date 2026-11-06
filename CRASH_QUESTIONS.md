# "Crash-Here" Defense Answer Bank (`CRASH_QUESTIONS.md`)

> **Event:** "Kill It and Prove It" Hackathon — Oral Defense Protocol (Minutes 8 to 12)  
> **Value:** 20 Points (Up to 7 per question across 3 random code lines).

---

## The Three-Part Answer Template
Whenever the judge points at a line in the write or recovery paths and asks:
> *"The process dies right here. What is on disk? What does the next open() do?"*

Structure your answer strictly in three sentences:
1. **Physical Disk State:** State precisely what bytes reached the storage media (fsynced) vs. what remained in OS volatile cache.
2. **Detection Mechanism:** Explain which validation check (`CRC32`, `Magic`, `Commit Marker`, or `EOF`) detects the crash state during startup scan.
3. **Recovery Action:** Describe the exact recovery procedure (truncate to last committed offset, repair torn tail, or ignore temp file).

---

## Standard Crash Points & Model Answers

### Point 1: Crash during record payload write (Torn Write - Scenario A2)
* **Code Location:** `internal/engine/write.go:WriteFrame` during `WriteAt` before `File.Sync()`.
* **What is on Disk:** Partial UABE frame bytes (header or partial payload) may have reached OS buffer; uncommitted and un-fsynced. The 4-byte Castagnoli Trailer CRC32C is completely missing or corrupted.
* **What `open()` Does:** The recovery scanner reads the 28-byte header, attempts to read payload bytes, and computes Castagnoli CRC32C. Checksum fails (or hits unexpected EOF). Because the frame trailer CRC is invalid, `open()` safely calls `repairTornTail`, truncating the file back to `valid_eof`. Zero corruption; key is absent.

---

### Point 2: Crash after payload write but before `File.Sync()` (Scenario A3, B2)
* **Code Location:** `internal/engine/write.go:WriteFrame` between `WriteAt` and `f.Sync()`.
* **What is on Disk:** The UABE frame bytes are in OS page cache but not yet forced to physical storage media.
* **What `open()` Does:** If bytes persisted to media before power cut, `open()` validates the Castagnoli CRC trailer and advances `valid_eof`. If bytes were lost in volatile RAM, EOF is reached early, trailer is missing, and file is truncated to last fsynced frame. Uncommitted in-flight write is safely dropped.

---

### Point 3: Crash after `File.Sync()` before returning success (Scenario A4, B4)
* **Code Location:** `internal/engine/write.go:WriteFrame` after `f.Sync()` returns 0 but before emitting `ACK` over stdout.
* **What is on Disk:** The entire UABE frame (Header + Payload + Castagnoli Trailer CRC) is 100% physically durable on disk media.
* **What `open()` Does:** The recovery scanner verifies the header CRC and trailer CRC. Since the frame is 100% complete, it applies the record into `KeyDir`. Even though the client crashed before receiving the ACK, durability promises are preserved (Scenario A4).

---

### Point 4: Crash during compaction swap (Scenario C4)
* **Code Location:** `internal/engine/compact.go:Compact` during `os.Rename(tmp, authoritative)`.
* **What is on Disk:** The temporary file `<path>.compact.tmp` was fully written and fsynced. On POSIX filesystems (ext4/btrfs/xfs), `rename` is atomic.
* **What `open()` Does:** If rename did not complete, the old file is intact and authoritative. If rename completed, the new file is authoritative. `open()` checks for leftover `.compact.tmp` files and unlinks them. In either case, all live data is preserved.

---

### Point 5: Crash during `open()` recovery replay (Scenario C1, C2, F2)
* **Code Location:** `internal/engine/recovery.go:repairTornTail` while calling `ftruncate`.
* **What is on Disk:** The database file is at its torn state or partially truncated.
* **What `open()` Does:** Recovery is fully idempotent. When `open()` runs a second time, it scans from byte 32 up to the last valid UABE frame and re-applies truncation. Replay of replay is identical.


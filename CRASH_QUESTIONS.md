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
* **Code Location:** `write_record_payload()` before the first `fsync()`.
* **What is on Disk:** Partial record bytes (magic byte or partial key/value) may have hit disk or remain in page cache; uncommitted and un-fsynced. Commit marker is completely absent.
* **What `open()` Does:** The recovery scanner reads the record header, attempts to read `key_len + val_len` bytes, and calculates the CRC32. The CRC checksum fails (or hit unexpected EOF). Because no commit marker follows, `open()` safely truncates the file back to the last valid commit marker offset. Zero corruption; key is absent.

---

### Point 2: Crash after record payload `fsync` but before commit marker (Scenario A3, B2)
* **Code Location:** Between `fsync(payload)` and writing the `COMMIT` marker opcode.
* **What is on Disk:** The record body (key, value, CRC) is durable on physical disk, but the commit marker has not been written.
* **What `open()` Does:** The recovery scanner validates the record CRC32 successfully, but searches for the subsequent `COMMIT` marker opcode. Because the commit marker is absent, the operation was never acknowledged to the caller. Recovery treats it as an uncommitted in-flight transaction, truncates or ignores the uncommitted record, and restores the database to the previous committed state.

---

### Point 3: Crash after commit marker `fsync` before returning success (Scenario A4, B4)
* **Code Location:** After `fsync(commit_marker)` but before emitting `ACK` over stdout.
* **What is on Disk:** Both the record payload and the commit marker are physically durable on disk.
* **What `open()` Does:** The recovery scanner verifies the record CRC and confirms the valid `COMMIT` marker. The transaction is durable! `open()` applies the record into the in-memory hash index. Even though the caller crashed before hearing the ACK, durability promises are preserved (Scenario A4).

---

### Point 4: Crash during compaction swap (Scenario C4)
* **Code Location:** In `swap_compaction_file()` during atomic rename `rename(tmp_file, data_file)`.
* **What is on Disk:** The filesystem directory entry points either to the old data file or the newly compacted file. On Linux POSIX filesystems (ext4/btrfs/xfs), `rename` is atomic.
* **What `open()` Does:** If rename did not complete, the old file is intact and authoritative. If rename completed, the new file is authoritative. `open()` checks for leftover `.compact.tmp` files and unlinks them. All data remains 100% consistent; nothing is lost.

---

### Point 5: Crash during `open()` recovery replay (Scenario C1, C2, F2)
* **Code Location:** Inside the recovery loop while truncating a corrupted tail.
* **What is on Disk:** The database file is partially truncated or at its previous uncommitted state.
* **What `open()` Does:** Recovery is fully idempotent. When `open()` runs a second time, it scans from byte 0 up to the last valid commit marker and re-applies truncation if needed. The resulting in-memory state is identical.


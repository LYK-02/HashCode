# Phase T8 Implementation Plan: Targeted Maintenance Modes: Recovery, Compaction, Disk-Full

> Canonical Phase: **T8**  
> Component: Maintenance State Stress, Compaction Race Injections, Quota Boundary Testing  
> Status: **Completed & Verified**  
> Related Docs: `docs/torture_tool/12_RECOVERY_CRASH_TESTING.md`, `docs/torture_tool/13_COMPACTION_CRASH_TESTING.md`, `docs/torture_tool/14_DISK_FULL_TESTING.md`

---

## 1. Objective and Scope

Implement three specialized adversarial test modes targeting maintenance routines and edge failure conditions:
1. `--mode recovery-crash`: Kills the engine while it is performing crash recovery (log replay and post-recovery truncation of torn tails).
2. `--mode compaction`: Kills the engine while it is compacting the database file (during temp-file writing and atomic directory swap).
3. `--mode disk-full`: Validates engine behavior when `--max-bytes` quota is exceeded or simulated disk full occurs, verifying rejection of writes and recovery of space after compaction.

---

## 2. Dependencies and Pre-Conditions

- **Phase T5 & T6:** Core crash loop and verifier active.
- **Engine Capabilities:** Support for `./kvdb open` with recovery, online/offline compaction, and `--max-bytes` quota enforcement.
- **Runtime Environment:** Python standard library `os`, `signal`, `time`, `shutil`.

---

## 3. Detailed Technical Specifications

### 3.1 Recovery-Crash Mode (`--mode recovery-crash`)
- **Objective:** Prove that an interrupted recovery never causes catastrophic data loss or permanent corruption (Scenarios B1–B4).
- **Procedure:**
  1. Populate a database with a large history of records ($50,000+$ mutations) to create a multi-megabyte log.
  2. Append an uncommitted/torn tail record to simulate an uncommitted transaction.
  3. Invoke `./kvdb open <db_path>` (which replays records and truncates the torn tail).
  4. Send `SIGKILL` **during** the open/replay operation (using microsecond-level timing).
  5. Restart `./kvdb open <db_path>` again and let it run to completion.
  6. Execute `kvdb check` and `kvdb dump`; verify state against oracle.
  7. Repeat nested kills up to 5 times sequentially on the same database file.

### 3.2 Compaction-Crash Mode (`--mode compaction`)
- **Objective:** Prove that killing during compaction preserves either the full original database or the fully compacted database (Scenarios C1–C4).
- **Procedure:**
  1. Generate heavy overwrite and delete workloads on a small key space (creating $90\%$ obsolete garbage data).
  2. Measure initial file size $S_{\text{initial}}$.
  3. Trigger compaction (either via CLI `./kvdb compact <db_path>` or through serve mode).
  4. Fire `SIGKILL` at seeded delays:
     - Boundary 1: While temporary compaction file (`data.db.compact` or similar) is being written.
     - Boundary 2: While temporary compaction file is being synced.
     - Boundary 3: Exactly around the atomic file rename / swap.
  5. Restart engine:
     - Verify orphaned temporary compaction files are cleaned up or ignored.
     - Verify database file is 100% valid.
     - If kill occurred after swap: verify file size $S_{\text{new}} \ll S_{\text{initial}}$.
     - Verify all live keys match oracle bit-for-bit.

### 3.3 Disk-Full Mode (`--mode disk-full`)
- **Objective:** Prove that when disk space / quota is exhausted, pending writes are rejected cleanly with `ERR` and never leave corrupted or half-written records (Scenarios E1–E5).
- **Procedure:**
  1. Start `./kvdb serve <db_path> --max-bytes <small_quota>` (e.g. 500 KiB or 1 MiB).
  2. Write data up to the threshold until engine begins rejecting requests with `ERR <op_id> QUOTA_EXCEEDED`.
  3. Send `SIGKILL` immediately after rejection.
  4. Restart and verify:
     - All previously ACKed writes remain intact.
     - The rejected write left zero partial artifacts or corrupt records in the file.
     - `kvdb check` passes cleanly.
  5. Issue deletions followed by compaction, and verify subsequent writes succeed once space is reclaimed.

---

## 4. Phase Breakdown & Subtasks

### Subtask T8.1: Recovery Crash Harness
- Implement `torture_tool/modes/recovery_crash.py`:
  - Workload pre-loader generating substantial replay logs.
  - Subprocess timer targeting `open` execution window.
  - Multi-cycle nested crash executor.

### Subtask T8.2: Compaction Crash Harness
- Implement `torture_tool/modes/compaction_crash.py`:
  - Garbage generator (dense PUT/DEL churn).
  - Compaction trigger and synchronized kill scheduler.
  - Post-crash compaction artifact inspector (verifying no leaked `.tmp` or `.compact` locks).

### Subtask T8.3: Disk-Full Quota Harness
- Implement `torture_tool/modes/disk_full.py`:
  - Quota calculation utilities.
  - Rejection response verification (`ERR` handling).
  - Space reclamation and deletion verification.

---

## 5. Failure Modes and Edge Cases

| Case | Scenario | Expected Handling |
|---|---|---|
| **M8.1** | Replay truncation killed mid-truncate | Next recovery must detect shortened file and complete cleanly without panics. |
| **M8.2** | Orphaned compaction file | Engine startup removes or safely overwrites leftover temp files. |
| **M8.3** | Quota rejection marks key in index | Oracle verifies rejected key is absent; flags bug if rejected key is present in dump. |
| **M8.4** | Compaction corrupts active keys | Deep state verification checks every single live key after compaction. |

---

## 6. Verification and Completion Criteria

- [x] 100 iterations of `--mode recovery-crash` pass with zero check or dump failures.
- [x] 100 iterations of `--mode compaction` pass; verify database shrinks and live data is intact.
- [x] 100 iterations of `--mode disk-full` pass; verify clean `ERR` responses and no corrupted records.


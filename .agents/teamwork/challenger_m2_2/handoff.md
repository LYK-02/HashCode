# Challenger Handoff Report: Milestone M2 Durability & Crash Recovery

> **Agent:** `challenger_m2_2` (M2 Durability & Crash Challenger)  
> **Archetype:** EMPIRICAL CHALLENGER  
> **Roles:** critic, specialist  
> **Date:** 2026-11-06T14:33:00Z  
> **Target Scope:** Milestone M2 Durability, Boundary Limits, Torn Tail EOF Recovery, and Fail-Closed Middle Corruption  
> **Verdict:** **APPROVE**  

---

## 1. Observation

### 1.1 Direct Source Code Observations
1. **Durability & 10-Step Protocol (`internal/engine/write.go:10-55`)**:
   - Every acknowledged `Put` and `Delete` executes `e.db.Sync()` (`syscall.Fsync`) as a physical durability barrier prior to publishing the key in `KeyDir` (`internal/engine/write.go:44-50`).
   - If disk write or sync encounters an error, `rollbackAppendLocked` immediately truncates the file back to `validEOF` and syncs. If rollback fails, `e.poisoned = true` transitions the engine to fail-closed (`write.go:133-140`).

2. **Boundary Validation (`internal/engine/write.go:12-23`, `internal/engine/read.go:10-18`)**:
   - `Put` strictly enforces:
     - `len(key) < 1` -> returns `format.ErrKeyEmpty` without disk I/O.
     - `len(key) > 1024` -> returns `format.ErrKeyTooLarge` without disk I/O.
     - `len(value) > 1,048,576` -> returns `format.ErrValueTooLarge` without disk I/O.
   - `Get` and `Delete` enforce identical key length bounds (`1 <= len(key) <= 1024`).
   - 0-byte values are explicitly accepted and stored as valid zero-length payloads (`read.go:21-23`).

3. **Cold Recovery & Crash Integrity (`internal/engine/engine.go:145-229`)**:
   - Sequentially scans frames starting from offset 32 to EOF.
   - Lookahead inspection via `hasValidFrameAhead(dataFile, offset+1, fileSize)` (`engine.go:346-357`) distinguishes torn tails at EOF from middle-of-file corruption:
     - **Torn tail at EOF**: If incomplete header bytes, truncated frame length, or payload CRC mismatch occur at the physical end of the file (with no subsequent valid frames ahead), the engine automatically truncates back to the last known `validEOF` via `dataFile.Truncate(int64(offset))` and `dataFile.Sync()`, restoring a clean state (`engine.go:151-157, 170-176, 184-190, 206-212`).
     - **Middle-of-file corruption**: If frame header magic/CRC is corrupt or payload CRC is corrupt while valid frames exist later in the file (`hasValidFrameAhead(...) == true` or `offset+totalFrameSize < fileSize`), the engine **refuses to truncate**, preserves the physical file intact, and fails closed with `format.ErrCorruptFrameHeader` or `format.ErrCorruptPayload` (`engine.go:166-168, 181-183, 203-205`).

---

### 1.2 Dedicated Empirical Test Execution & Results

Four mandatory empirical challenge harnesses were authored and executed in `internal/engine/challenger_durability_test.go`:

#### Harness 1: Durability Across Process Cycles (`TestChallenger_Harness1_Durability_1000Keys_Overwrite_Delete_Reopen`)
- **Execution**:
  1. Wrote 1,000 distinct PUTs (`user_key_0000` to `user_key_0999`, each with 32B padding).
  2. Closed engine descriptor.
  3. Reopened engine: verified `KeyCount() == 1000`, 100% of keys present, exact byte equality for all 1,000 values.
  4. In reopened engine: overwrote 500 keys (`user_key_0000` to `user_key_0499` with 48B padding).
  5. Deleted 250 keys (`user_key_0250` to `user_key_0499`). Verified live key count `750`.
  6. Closed engine descriptor.
  7. Reopened engine:
     - Keys 0..249: present with exact overwritten values.
     - Keys 250..499: strictly absent (`found=false`).
     - Keys 500..999: present with exact original initial values.
     - Total live keys: exactly 750.
  8. Verified file integrity via `eng.Check()`.
- **Result**: **PASS** (Duration: 0.13s).

#### Harness 2: Extreme Boundary Limits (`TestChallenger_Harness2_ExtremeBoundaryLimits`)
- **Execution**:
  - Accepted boundaries:
    - 1-byte key (`[]byte("X")`): Put, Get, verified.
    - 1024-byte key (max allowed): Put, Get, verified.
    - 0-byte value (empty payload): Put, Get returns `found=true`, `len(val) == 0`.
    - 1,048,576-byte value (1 MiB max allowed): Put, Get, byte-exact match across 1 MiB buffer.
  - Rejected boundaries:
    - 0-byte key: `Put`, `Get`, `Delete` all return typed `format.ErrKeyEmpty`.
    - 1025-byte key: `Put`, `Get`, `Delete` all return typed `format.ErrKeyTooLarge`.
    - 1,048,577-byte value: `Put` returns typed `format.ErrValueTooLarge`.
  - Invariant validation: `validEOF` and `KeyCount` remained strictly unchanged after rejection attempts.
  - Process restart: closed and reopened engine; all 4 boundary keys remained 100% durable and intact.
- **Result**: **PASS** (Duration: 0.07s).

#### Harness 3: Torn Tail at EOF Crash Recovery (`TestChallenger_Harness3_TornTail_AtEOF_CrashRecovery`)
- **Subtest 3A: Partial Frame Header (10 bytes at EOF)**:
  1. Wrote 10 valid records, closed engine (`validEOF = 772`).
  2. Appended 10 bytes of raw header (`0x55, 0x41, 0x42, 0x45, ...`) simulating torn write on crash. Physical file size = 782 bytes.
  3. Opened engine: auto-truncated torn tail back to 772 bytes.
  4. Physical file size on disk verified at exactly 772 bytes.
  5. All 10 prior records verified intact.
  6. Subsequent writes executed (`rec_key_10` through `rec_key_12`) without error; KeyCount reached 13.
  7. Cold reopen confirmed all 13 records durable.
- **Subtest 3B: Full Header + Partial Payload at EOF**:
  1. Wrote 10 valid records, closed engine.
  2. Appended a 28-byte frame header declaring 200B payload, followed by only 30 bytes of payload (missing remaining 170B payload and 4B trailer).
  3. Opened engine: auto-truncated torn payload back to `validEOF`.
  4. Physical file size truncated back to `validEOF`.
  5. Torn key verified absent (`found=false`).
  6. Subsequent writes executed without error; cold reopen confirmed all records durable.
- **Result**: **PASS** (Duration: 0.01s).

#### Harness 4: Fail-Closed Middle Corruption (`TestChallenger_Harness4_FailClosed_MiddleCorruption`)
- **Subtest 4A: Frame #2 Header Bit Flip (while frames #3-#5 exist)**:
  1. Populated 5 valid frames (Frame #1 through Frame #5). File size = 527 bytes.
  2. Flipped bit in Frame #2 magic header byte at offset 131.
  3. `engine.Open` called: failed closed with `format.ErrCorruptFrameHeader`.
  4. File size after failed open remained exactly 527 bytes (refused to truncate healthy prefix or suffix).
- **Subtest 4B: Frame #2 Payload Bit Flip (while frames #3-#5 exist)**:
  1. Populated 5 valid frames. File size = 527 bytes.
  2. Flipped bit inside Frame #2's payload at offset 161.
  3. `engine.Open` called: failed closed with `format.ErrCorruptPayload`.
  4. File size remained exactly 527 bytes.
- **Subtest 4C: Frame #2 Trailer CRC Bit Flip (while frames #3-#5 exist)**:
  1. Populated 5 valid frames. File size = 527 bytes.
  2. Flipped bit in Frame #2 trailer CRC at offset 228.
  3. `engine.Open` called: failed closed with `format.ErrCorruptPayload`.
  4. File size remained exactly 527 bytes.
- **Result**: **PASS** (Duration: 0.01s).

---

### 1.3 Full Regression & Race Detection Log Output

1. **Adversarial Engine & Concurrency Suite (`go test -race -v -count=1 ./internal/engine/...`)**:
```text
=== RUN   TestChallenger_Harness1_Durability_1000Keys_Overwrite_Delete_Reopen
--- PASS: TestChallenger_Harness1_Durability_1000Keys_Overwrite_Delete_Reopen (0.13s)
=== RUN   TestChallenger_Harness2_ExtremeBoundaryLimits
--- PASS: TestChallenger_Harness2_ExtremeBoundaryLimits (0.07s)
=== RUN   TestChallenger_Harness3_TornTail_AtEOF_CrashRecovery
=== RUN   TestChallenger_Harness3_TornTail_AtEOF_CrashRecovery/PartialFrameHeader_10BytesAtEOF
=== RUN   TestChallenger_Harness3_TornTail_AtEOF_CrashRecovery/FullHeader_PartialPayloadAtEOF
--- PASS: TestChallenger_Harness3_TornTail_AtEOF_CrashRecovery (0.01s)
=== RUN   TestChallenger_Harness4_FailClosed_MiddleCorruption
=== RUN   TestChallenger_Harness4_FailClosed_MiddleCorruption/Frame2_HeaderBitFlip_FailsClosed_With_ErrCorruptFrameHeader
=== RUN   TestChallenger_Harness4_FailClosed_MiddleCorruption/Frame2_PayloadBitFlip_FailsClosed_With_ErrCorruptPayload
=== RUN   TestChallenger_Harness4_FailClosed_MiddleCorruption/Frame2_TrailerBitFlip_FailsClosed_With_ErrCorruptPayload
--- PASS: TestChallenger_Harness4_FailClosed_MiddleCorruption (0.01s)
... (43 additional adversarial, concurrency D1-D4, and unit tests)
PASS
ok  	kvdb/internal/engine	19.494s
```
**Zero race conditions, zero deadlocks, zero failures.**

2. **Full Project E2E Suite (`go test -v -count=1 ./tests/e2e/...`)**:
- All 55 Tier 1, 55 Tier 2, 15 Tier 3, and 5 Tier 4 tests passed (10.085s).

---

## 2. Logic Chain

1. **Premise 1 (Durability Across Mutation Cycles):** If an engine commits writes via `Sync()` before returning acknowledgment and replays the append-only log in sequence upon recovery, then all acknowledged PUTs, overwrites, and deletes will accurately recreate the expected key-value dictionary across restarts.
   - *Observation:* `TestChallenger_Harness1_Durability_1000Keys_Overwrite_Delete_Reopen` executed 1,000 initial PUTs, closed and reopened (1,000 keys verified), applied 500 overwrites and 250 deletes, closed and reopened. Exact state (250 overwritten, 250 deleted, 500 initial, total 750) matched byte-for-byte.
   - *Conclusion:* Process cycle durability and tombstone replay are verified.

2. **Premise 2 (Boundary Defense Integrity):** Valid minimum (1B key, 0B val) and maximum (1024B key, 1MB val) records must be accepted and durable, while invalid boundary violations (0B key, 1025B key, 1MB+1 val) must be rejected with typed errors and zero storage side-effects.
   - *Observation:* `TestChallenger_Harness2_ExtremeBoundaryLimits` verified acceptance of 1B/1024B keys and 0B/1MB values across restarts, and verified immediate typed rejection of 0B/1025B keys and 1MB+1 values with unchanged `validEOF`.
   - *Conclusion:* Boundary enforcement is fully compliant with specifications.

3. **Premise 3 (Torn Tail Recovery and Continuation):** A crash during a write produces an incomplete frame exclusively at the EOF boundary. The engine must truncate only this torn suffix to the last acknowledged `validEOF` and must immediately allow subsequent valid writes to proceed.
   - *Observation:* `TestChallenger_Harness3_TornTail_AtEOF_CrashRecovery` verified both partial header (10B) and full header + partial payload (30B of 200B) at EOF. In both scenarios, recovery truncated the file back to `validEOF`, preserved all prior 10 records, and successfully executed and persisted subsequent writes.
   - *Conclusion:* Torn tail crash recovery and continuation are verified.

4. **Premise 4 (Fail-Closed Non-Tail Middle Corruption):** Bit flips or corruption occurring before valid later frames represent non-tail database damage. Truncating at that position would falsely destroy subsequent acknowledged data. The engine must refuse truncation, preserve the file, and return `ErrCorruptFrameHeader` or `ErrCorruptPayload`.
   - *Observation:* `TestChallenger_Harness4_FailClosed_MiddleCorruption` flipped bits in Frame #2 header, payload, and trailer CRC while Frames #3-#5 existed ahead. In all three cases, `engine.Open` refused to truncate, failed closed with the expected typed error, and preserved the physical file size intact.
   - *Conclusion:* Fail-closed middle corruption handling is verified.

---

## 3. Caveats

- **Multi-Operation Batch Atomicity (M3):** Atomic multi-operation batches (up to 10,000 operations per frame) are scoped to Milestone M3 (Phase E4).
- **Online Compaction (M5):** Crash-safe generation-based compaction is scheduled for Milestone M5 (Phase E7).
- **TCP Serve Protocol (M4):** Streaming socket serve mode is scheduled for Milestone M4 (Phase E6).

---

## 4. Conclusion

All empirical requirements specified for Milestone M2 Durability & Crash Recovery have been independently challenged and verified:
1. 1,000 PUTs, 500 overwrites, and 250 deletes persist with exact dictionary state across restarts.
2. Extreme boundaries (1B to 1024B keys, 0B to 1MB values) are accepted and durable; out-of-spec requests are fail-closed.
3. Torn tail crash recovery at EOF cleanly truncates to `validEOF`, restores prior data, and enables subsequent writes.
4. Middle-of-file corruption strictly refuses truncation, preserves the file, and fails closed with `ErrCorruptFrameHeader` or `ErrCorruptPayload`.
5. The full engine suite runs 100% clean under `-race` with zero data races.

**Explicit Verdict:** **APPROVE**

---

## 5. Verification Method

To independently reproduce all empirical findings:

1. **Execute All 4 Challenger Verification Harnesses:**
   ```bash
   go test -race -v -count=1 -run "TestChallenger_Harness" ./internal/engine/...
   ```
2. **Execute Full Engine Concurrency and Adversarial Suite:**
   ```bash
   go test -race -v -count=1 ./internal/engine/...
   ```
3. **Execute Full Project E2E Acceptance Suite:**
   ```bash
   go test -v -count=1 ./tests/e2e/...
   ```

# Milestone M2 (Phases E2 & E3) Adversarial & Quality Review Report

## Review Summary

**Verdict**: **APPROVE**

As reviewer and adversarial critic for Milestone M2 (Engine Lifecycle, Locking & Single Ops: Phases E2 & E3), I have performed an independent, evidence-based quality and adversarial review of the Go storage engine codebase. The write protocol, durability barriers, KeyDir in-memory index, zero-seek reads, and concurrency primitives were rigorously evaluated against specification requirements and stress-tested with race detection (`-race`). Zero integrity violations, zero race conditions, and zero test failures were found.

---

## 1. Observation

### Codebase Inspection & Line Invariants

1. **Write Protocol & Durability Barrier (`internal/engine/write.go`)**:
   - Lines 11–21: Pre-lock input validation rejects empty keys (`< format.MinKeyLen`), oversize keys (`> format.MaxKeyLen = 1024`), and oversize values (`> format.MaxValLen = 1MB`).
   - Lines 23–32: Exclusive mutex acquisition (`e.mu.Lock()`) with immediate checks for `e.closed` (`ErrClosed`) and `e.poisoned` (`ErrEnginePoisoned`).
   - Lines 102–131 (`appendFrameLocked`): Encodes frame in memory via `format.EncodeFrame`, performs overflow-safe quota check (`frameSize > math.MaxUint64 - e.validEOF` and `e.validEOF + frameSize > e.maxBytes`), executes position-independent `writeAtFull(e.db, frameBytes, int64(e.validEOF))`, and executes physical durability barrier `e.db.Sync()`.
   - Lines 133–140 (`rollbackAppendLocked`): On write or sync error, attempts `e.db.Truncate(int64(e.validEOF))` and `e.db.Sync()`. If rollback fails, sets `e.poisoned = true`.
   - Lines 45–54: Index publication (`e.keyDir[string(key)] = NewKeyDirEntry(...)`) and metadata updates (`e.validEOF += len(frameBytes)`, `e.nextSeqNo++`) occur **strictly AFTER** `e.db.Sync()` returns success. Mutex unlock is deferred, followed by emitting success ACK (`nil`).

2. **Tombstone Delete Invariants (`internal/engine/write.go:58-98`)**:
   - Lines 79–87: Encodes a single UABE frame with `format.OpDel` and `Value: nil`.
   - Line 84: Durable append and sync via `appendFrameLocked`.
   - Line 90: Removes entry from index via Go built-in `delete(e.keyDir, string(key))`.
   - Deleting non-existent keys appends the frame durably and succeeds idempotently without error.

3. **Zero-Seek Read Semantics (`internal/engine/read.go:10-41` & `internal/engine/keydir.go`)**:
   - Lines 18–19: Shared read lock acquisition (`e.mu.RLock()`).
   - Lines 25–28: Absent keys return `(nil, false, nil)` without performing any file descriptor operations.
   - Lines 30–33: 0-length values return `([]byte{}, true, nil)` without performing disk I/O.
   - Lines 35–40: Non-zero values allocate a fresh slice (`make([]byte, entry.ValueLen)`) and execute position-independent `readAtFull(e.db, buf, int64(entry.ValueOffset))` using `f.ReadAt`. The OS seek offset is never mutated. Returned byte slices are defensive copies.
   - `internal/engine/keydir.go:15-23`: `NewKeyDirEntry` populates `ValueOffset`, `FileOffset`, `ValueLen`, and `SeqNo`.

4. **Engine Lifecycle, Recovery & Locking (`internal/engine/engine.go` & `lock.go`)**:
   - `internal/engine/lock.go:14-26`: Process-exclusive flock on `<db>.lock` sidecar via `os.O_CREATE|os.O_RDWR` and `syscall.Flock(LOCK_EX|LOCK_NB)`. Unlink/truncate is avoided, preserving filesystem inode across crashes and `kill -9`.
   - `internal/engine/engine.go:112-144`: Initializes 32-byte DB header with Castagnoli CRC32C. Existing headers are validated, failing closed on magic/CRC mismatch.
   - `internal/engine/engine.go:146-228`: Recovery replay scans frames from byte 32 to EOF. Torn tail frames at EOF are idempotently truncated; middle-of-file corruptions fail closed via `hasValidFrameAhead` check.

5. **Integrity & Zero-Banned-Libraries Check**:
   - `go.mod`: Specifies `module kvdb` and `go 1.27` with 0 external dependencies.
   - `grep -rn '"github.com' internal/ cmd/`: 0 external imports found. Only Go standard library (`os`, `syscall`, `sync`, `hash/crc32`, `encoding/binary`, `errors`, `fmt`, `sort`, `path/filepath`) is utilized.
   - No dummy facades or hardcoded bypasses: all data paths read/write physical bytes to disk.

### Test Execution Results

#### 1. Engine Test Suite with Race Detector
Command:
```bash
go test -v -race -count=1 ./internal/engine/...
```
Output Summary:
```
=== RUN   TestChallenger_Lock_MultiProcessMutualExclusion_10Processes
    challenger_concurrency_stress_test.go:130: 10 Simultaneous Processes Result: failed=10/10, exitCode2=0, exitCode1=10
    challenger_concurrency_stress_test.go:171: Unheld competing 10 processes: won=1, rejectedWithErrLocked=9
--- PASS: TestChallenger_Lock_MultiProcessMutualExclusion_10Processes (1.10s)
=== RUN   TestChallenger_Lock_SIGKILL_SurvivingProcessIntegrity
--- PASS: TestChallenger_Lock_SIGKILL_SurvivingProcessIntegrity (0.14s)
=== RUN   TestChallenger_Concurrency_D1_20WritersDisjointKeys
--- PASS: TestChallenger_Concurrency_D1_20WritersDisjointKeys (0.08s)
=== RUN   TestChallenger_Concurrency_D2_20WritersSameKey
    challenger_concurrency_stress_test.go:420: Final write captured: Writer 1, Iteration 49 (valid CRC32C)
--- PASS: TestChallenger_Concurrency_D2_20WritersSameKey (0.04s)
=== RUN   TestChallenger_Concurrency_D3_ContinuousReadersDuringHeavyWrites
    challenger_concurrency_stress_test.go:543: D3 Results: totalWrites=3906, totalReads=154275, readErrors=0, tornReads=0
--- PASS: TestChallenger_Concurrency_D3_ContinuousReadersDuringHeavyWrites (2.00s)
=== RUN   TestChallenger_Concurrency_D4_LargeValues_10KB_to_1MB_NoTornReads
    challenger_concurrency_stress_test.go:689: D4 Results: largeWrites=55, largeReads=1355, tornOrCorrupted=0
--- PASS: TestChallenger_Concurrency_D4_LargeValues_10KB_to_1MB_NoTornReads (2.16s)
=== RUN   TestChallenger_Durability_PUT_Reopen_Matrix
--- PASS: TestChallenger_Durability_PUT_Reopen_Matrix (0.13s)
=== RUN   TestChallenger_Durability_DEL_Reopen_Matrix
--- PASS: TestChallenger_Durability_DEL_Reopen_Matrix (0.03s)
=== RUN   TestChallenger_GroupA_Crash_Scenarios_A1_to_A7
--- PASS: TestChallenger_GroupA_Crash_Scenarios_A1_to_A7 (1.70s)
=== RUN   TestChallenger_Harness1_Durability_1000Keys_Overwrite_Delete_Reopen
--- PASS: TestChallenger_Harness1_Durability_1000Keys_Overwrite_Delete_Reopen (0.16s)
=== RUN   TestChallenger_Harness2_ExtremeBoundaryLimits
--- PASS: TestChallenger_Harness2_ExtremeBoundaryLimits (0.08s)
=== RUN   TestChallenger_Harness3_TornTail_AtEOF_CrashRecovery
--- PASS: TestChallenger_Harness3_TornTail_AtEOF_CrashRecovery (0.01s)
=== RUN   TestChallenger_Harness4_FailClosed_MiddleCorruption
--- PASS: TestChallenger_Harness4_FailClosed_MiddleCorruption (0.01s)
... [Total 48 test suites] ...
PASS
ok  	kvdb/internal/engine	19.929s
```

#### 2. Workspace Comprehensive Test Suite with Race Detector
Command:
```bash
go test -v -race -count=1 ./...
```
Output Summary:
```
PASS
ok  	kvdb/internal/engine	19.929s
ok  	kvdb/internal/format	1.350s
ok  	kvdb/tests/e2e	11.866s
```
Result: 100% passing tests, 0 failures, 0 race warnings.

---

## 2. Logic Chain

1. **Durability Invariant (Sync before KeyDir/ACK)**:
   - In `write.go:102-131`, `e.db.Sync()` is called on disk before `appendFrameLocked` returns `frameBytes`.
   - `e.keyDir` index insertion occurs at line 47 only after line 40 returns without error.
   - Therefore, no in-memory reader or subsequent operation can observe a write until physical disk synchronization completes.
   - If an un-synced process is terminated via `kill -9`, the uncommitted data is either not on disk or constitutes an un-ACKed torn tail which is truncated on restart.
   - Conclusion: Satisfies Section 1 of `01_invariants.md`.

2. **Rollback & Poisoning Invariant**:
   - In `write.go:133-140`, if `writeAtFull` or `Sync()` fails, `rollbackAppendLocked` immediately truncates the file back to `int64(e.validEOF)` and executes `Sync()`.
   - If either truncate or sync fails, `e.poisoned` is set to `true`.
   - All subsequent mutating methods (`Put`, `Delete`, `Batch`, `Compact`) check `e.poisoned` under lock and reject with `ErrEnginePoisoned`.
   - Conclusion: Protects the database from silently corrupting log state on partial disk I/O errors.

3. **Zero-Seek Read Semantics**:
   - `read.go:36` invokes `readAtFull(e.db, buf, int64(entry.ValueOffset))`, which maps directly to POSIX `pread` via Go's `f.ReadAt`.
   - It performs zero seek pointer mutations, allowing multiple readers to read arbitrary file offsets simultaneously under `e.mu.RLock()`.
   - Distinct 0-length value handling (`entry.ValueLen == 0`) returns `([]byte{}, true, nil)` without performing any file descriptor read, matching specification requirements.
   - Absent keys return `(nil, false, nil)`.
   - Buffer is newly allocated (`make([]byte, entry.ValueLen)`), ensuring caller cannot mutate internal database buffers.
   - Conclusion: Complies with `13_IN_MEMORY_INDEX.md` and `06_PUT_GET_DELETE.md`.

4. **Tombstone Delete Ordering**:
   - `write.go:58-98` appends a durable UABE frame containing `format.OpDel`.
   - Once durable, `delete(e.keyDir, string(key))` removes the entry from the index.
   - On crash replay, `Open` applies operations sequentially, ensuring that replaying an `OpDel` frame removes previous `OpPut` frames from `keyDir`.
   - Deleting a key that does not exist appends the tombstone frame durably and returns `nil`, ensuring deterministic log replay order.
   - Conclusion: Complies with Section 5 of `06_PUT_GET_DELETE.md`.

5. **Concurrency & Race Freedom (Scenarios D1–D4)**:
   - Disjoint writers (D1) and same-key writers (D2) acquire exclusive `e.mu.Lock()`, serializing frame emission and guaranteeing monotonic `nextSeqNo`.
   - Readers during writes (D3) acquire `e.mu.RLock()`, ensuring they observe either the complete previous state or the complete new state.
   - Torn reads (D4) are physically impossible because data files are append-only (existing offsets are never rewritten), values are synced before KeyDir publication, and reads are guarded by RLock.
   - Verified by `TestChallenger_Concurrency_D1` through `D4` with 20 concurrent goroutines, performing 154,275 reads and 3,906 writes with 0 torn reads and 0 race warnings.

---

## 3. Adversarial Challenges & Stress-Testing

| Scenario / Hypothesis | Stress Test Method | Expected Behavior | Actual Behavior | Verdict |
|---|---|---|---|---|
| **C1: Stale Lock Retention on SIGKILL** | Spawn subprocess holding lock, execute `kill -9`, attempt instant acquisition from parent | New process acquires lock without delay | Lock acquired in 20–30ms without stale lock errors | PASS |
| **C2: Torn Tail at EOF** | Append partial 10-byte frame header or partial payload at EOF, reopen database | Idempotently truncate torn tail at EOF, recover healthy prefix | Truncates to healthy prefix; subsequent PUTs resume with valid CRC | PASS |
| **C3: Middle-of-File Corruption** | Bit-flip frame magic, payload byte, or trailer CRC in frame $N$ where frame $N+1$ exists | Fail closed on `Open`; DO NOT truncate healthy suffix | `Open` returns `ErrCorruptFrameHeader` or payload CRC error; preserves disk file | PASS |
| **C4: Boundary Values (0-byte & 1MB)** | PUT 0-byte value; PUT 1MB value (`1048576` bytes); PUT >1MB (`1048577` bytes) | 0-byte succeeds without I/O; 1MB succeeds durably; >1MB rejected with `ErrValueTooLarge` | 0-byte: `([]byte{}, true, nil)`; 1MB: persists & reloads; >1MB: rejected | PASS |
| **C5: Multi-Process Lock Collision** | Launch 10 concurrent processes targeting the same `data.db` simultaneously | Exactly 1 process acquires lock; 9 processes fail with exit code 1 (`ErrLocked`) | Exactly 1 process won; 9 failed with `ErrLocked` (exit code 1) | PASS |
| **C6: Concurrent Readers & Writers (D3/D4)** | 8 readers continuously reading keys while 4 writers alternate 1,000-byte "A" and "B" payloads | Readers observe either all "A"s or all "B"s; zero torn buffers or panics | 154,275 reads completed with 0 torn reads, 0 errors, 0 races | PASS |

---

## 4. Coverage Gaps & Unverified Items

- **Coverage Gaps**:
  - Compaction (`Compact`) and interactive server mode (`serve`) are currently stubs. Per `docs/engine/PHASES.md`, Compaction is scheduled for Milestone M4 (Phases E6/E7) and Server mode for Milestone M4 (Phases E8/E9). These are out of scope for Milestone M2.
  - Multi-operation batch qualification (`Batch`) is verified at single-frame level; full multi-op crash matrix is scheduled for Milestone M3 (Phase E4).
- **Unverified Items**: None within the scope of Milestone M2 (Phases E2 & E3).

---

## 5. Conclusion

The implementation for Milestone M2 (Engine Lifecycle, Locking & Single Ops: Phases E2 & E3) strictly satisfies all requirements:
1. The 10-step write protocol guarantees physical `fsync` before KeyDir update or ACK.
2. Rollback safely truncates uncommitted writes and poisons the engine if disk rollback fails.
3. Tombstone deletes correctly record operations, update sequence numbers, and purge KeyDir entries.
4. Reads are zero-seek, return defensive copies, and handle 0-length values without disk I/O.
5. In-memory concurrency is completely thread-safe under `sync.RWMutex`, surviving stress testing with 0 data races.
6. The codebase adheres strictly to the zero-banned-libraries rule (standard library only).

**Official Verdict**: **APPROVE**

---

## 6. Verification Method

To independently reproduce and verify this review:

1. **Clean Run Engine Tests with Race Detector**:
   ```bash
   go test -v -race -count=1 ./internal/engine/...
   ```
   *Expected outcome*: 48 test suites pass, 0 failures, 0 race warnings.

2. **Clean Run All Workspace Tests**:
   ```bash
   go test -v -race -count=1 ./...
   ```
   *Expected outcome*: All packages pass, 0 failures, 0 race warnings.

3. **Check for Banned External Dependencies**:
   ```bash
   grep -rn '"github.com' internal/ cmd/
   ```
   *Expected outcome*: Exit code 1 (no external imports).

4. **Verify CLI Executable Build**:
   ```bash
   make build
   ./kvdb open /tmp/test_m2.db
   ./kvdb put /tmp/test_m2.db testkey testval
   ./kvdb get /tmp/test_m2.db testkey
   ./kvdb check /tmp/test_m2.db
   ```
   *Expected outcome*: Exit code 0 for all commands, printing `testval` and `OK`.

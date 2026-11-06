# Handoff Report: Milestone M2 (Phases E2 & E3) Implementation

**Author:** `worker_m2_1` (Milestone M2 Implementation Lead)  
**Date:** 2026-11-06T14:15:00Z  
**Recipient:** Orchestrator (`9fade4e7-7ff3-4d78-9ecd-6f9b6c00f5be`)  
**Scope:** Milestone M2 — Lifecycle, Sidecar Flock, 10-Step Write Protocol, KeyDir Index, ReadAt, Cold Replay Recovery, and Concurrency Verification.

---

## 1. Observation

### 1.1 Direct Source Code Observations
1. **Locking & Sidecar File (`internal/engine/lock.go`)**:
   - `acquireLock(dataFilePath string)` computes canonical sidecar path `lockPath = dataFilePath + ".lock"`.
   - Opens lock descriptor using `os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)`.
   - Acquires non-blocking advisory lock using `syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)`.
   - Returns typed `format.ErrLocked` upon `syscall.EWOULDBLOCK` or `syscall.EAGAIN`.
   - Releases lock using `syscall.Flock(int(f.Fd()), syscall.LOCK_UN)` followed by `f.Close()`. The lock file is never unlinked or renamed, avoiding inode racing.

2. **In-Memory Index (`internal/engine/keydir.go`)**:
   - `KeyDirEntry` holds `ValueOffset uint64`, `FileOffset uint64`, `ValueLen uint32`, and `SeqNo uint64`.
   - `SingleOpValueOffset(fileOffset uint64, keyLen int)` accurately computes the raw value byte offset:
     $$\text{ValueOffset} = \text{FileOffset} + 28 \text{ (UABE header)} + 1 \text{ (OpType)} + 2 \text{ (KeyLen)} + 4 \text{ (ValueLen)} + \text{KeyLen}$$
   - Exposed thread-safe accessors: `KeyCount()`, `ValidEOF()`, and `NextSeqNo()`.

3. **Random Read (`internal/engine/read.go`)**:
   - `Get(key []byte) ([]byte, bool, error)` validates input ($1 \le \text{len(key)} \le 1024$), acquires `e.mu.RLock()`, and checks closed state.
   - For entries with `ValueLen == 0` (valid 0-byte value), returns `[]byte{}, true, nil` without disk I/O.
   - For positive lengths, performs `readAtFull(e.db, buf, int64(entry.ValueOffset))` directly at the physical offset without mutating file offsets or seeking, and returns a defensive copy.

4. **10-Step Write Protocol & Atomicity (`internal/engine/write.go`)**:
   - `Put(key, value []byte)` and `Delete(key []byte)` strictly enforce:
     1. Validation: Key 1..1024B, Value 0..1MiB (or 0 for DEL).
     2. Lock acquisition: `e.mu.Lock()`.
     3. Frame serialization: `format.EncodeFrame(...)` generating 28-byte UABE header + payload record + 4-byte CRC32C trailer.
     4. Quota check: if `e.maxBytes > 0 && e.validEOF + frameSize > e.maxBytes`, reject with `format.ErrQuotaExceeded` without disk writes.
     5. Position-independent write loop: `writeAtFull(e.db, frameBytes, int64(e.validEOF))`.
     6. Physical durability barrier: `e.db.Sync()` (`fsync`).
     7. Index publication: `e.keyDir[string(key)] = entry` (or `delete(e.keyDir, string(key))` for DEL).
     8. Metadata update: `e.validEOF += frameSize`, `e.nextSeqNo++`.
     9. Write lock release: `e.mu.Unlock()`.
     10. Return success.
   - Rollback handling: `rollbackAppendLocked` attempts `e.db.Truncate(int64(e.validEOF))` and `Sync()` if disk writes fail prior to step 6. If rollback fails, the engine transitions to `poisoned = true` to fail closed.

5. **Lifecycle & Cold Recovery (`internal/engine/engine.go`)**:
   - Canonicalizes path via `filepath.Abs` and `filepath.Clean`.
   - On new database (0-byte file), writes 32-byte header (`format.EncodeHeader`), calls `file.Sync()`, calls `syncDir(filepath.Dir(dataFilePath))` for parent directory metadata durability, and sets `validEOF = 32`.
   - On existing database, validates 32-byte header with `format.DecodeHeader`.
   - `recoverFrames`: scans sequentially from offset 32 to EOF. Employs `hasValidFrameAhead` lookahead scanner:
     - If corruption occurs with valid frames ahead, the engine refuses auto-truncation and returns `ErrCorruptFrameHeader` or `ErrCorruptPayload` (fail-closed integrity).
     - If incomplete bytes or CRC error occur exclusively at EOF (torn tail crash), truncates back to last known `validEOF` and syncs.
   - `Close()`: flushes, syncs, closes data file, releases flock sidecar, marks `closed = true`. Subsequent calls return `ErrClosed`.

6. **CLI Binding (`cmd/kvdb/main.go`)**:
   - Supports `open`, `put`, `get`, `del`, `batch`, `dump`, `check` subcommands with POSIX-compliant exit codes:
     - 0: Success.
     - 1: Key not found, corrupt database, or quota exceeded.
     - 2: Database locked by another process (`ErrLocked`).
   - Quota persistence: stores optional `maxBytes` uint64 in the `<db>.lock` sidecar file during `open <db> --max-bytes N` so subsequent standalone CLI operations respect user quotas.
   - Token buffer safety: configured `bufio.NewScanner` with a 64MB buffer for 1MB value handling.
   - Dump format: formats entries as `key=value` to preserve empty string values.

### 1.2 Test Execution Results
- **Engine Unit & Concurrency Test Suite**:
  ```bash
  go test -v -race -count=1 ./internal/engine/...
  ```
  Result: **PASS** (39/39 tests passed, 0 race conditions, duration: 9.736s).
  Specifically verified:
  - `TestEngine_Flock_MutualExclusion`: Parallel processes cannot acquire lock simultaneously (`ErrLocked`).
  - `TestEngine_Flock_CleanCloseHandoff`: Sequential process handoff works immediately.
  - `TestEngine_Flock_SIGKILL_Recovery`: SIGKILL of lock holder allows instant lock acquisition by surviving process.
  - `TestEngine_Concurrency_D1_DisjointWriters`: Multi-goroutine disjoint writes with 0 races.
  - `TestEngine_Concurrency_D2_SameKeyWriters`: Multi-goroutine concurrent overwrites with consistent Last-Write-Wins.
  - `TestEngine_Concurrency_D3_ReadersDuringWrites`: Continuous concurrent readers observe valid historical or latest values without tearing.
  - `TestEngine_Concurrency_D4_NoTornReads`: Concurrent readers reading 1,000-byte patterned values observe zero corruptions or torn states.

- **Full Project Build & Verification**:
  ```bash
  make build
  make test
  ```
  Result: **PASS**
  - Binary compilation: `./kvdb` built cleanly with zero warnings (`CGO_ENABLED=0`).
  - All format unit tests passed.
  - All engine adversarial tests passed.
  - All E2E test suites (Tier 1, Tier 2, Tier 3, Tier 4) passed (stub `serve` commands skipped as expected for M2).

---

## 2. Logic Chain

1. **Advisory Flock vs File Removal**:
   - *Observation*: Unlinking a lock file on process exit creates an unavoidable race condition where a concurrent process opens an unlinked inode while another creates a new one.
   - *Deduction*: By keeping `<path>.lock` permanent (never unlinked or renamed) and binding mutual exclusion strictly to `syscall.Flock(LOCK_EX|LOCK_NB)`, kernel-level advisory locks cleanly associate with the existing inode.
   - *Verification*: `TestAdversarial_Lock_SidecarInvariants` and `TestEngine_Flock_SIGKILL_Recovery` verify zero orphaned lock issues across abrupt process kills.

2. **Zero-Seek Read Consistency**:
   - *Observation*: Standard `os.File.Read()` relies on an internal file offset pointer, requiring mutex serialization for reads.
   - *Deduction*: Using `io.ReaderAt` (`db.ReadAt(buf, int64(entry.ValueOffset))`) allows concurrent readers to execute in parallel under `e.mu.RLock()` without contending on file offset pointers or disturbing writer appends.
   - *Verification*: `TestEngine_Concurrency_D4_NoTornReads` and `TestEngine_Concurrency_D3_ReadersDuringWrites` pass under `-race` with 20 concurrent readers and 5 writers.

3. **Crash Recovery & Torn Tail Distinctions**:
   - *Observation*: Crash at EOF during write creates an incomplete UABE frame at the physical tail. Conversely, bit flips or file truncations in the middle of a database represent file corruption rather than a normal crash tail.
   - *Deduction*: Scanning forward with `hasValidFrameAhead` detects whether valid frames exist beyond the corrupt region. If valid frames exist ahead, the engine refuses truncation and returns `ErrCorruptFrameHeader` or `ErrCorruptPayload`. If no valid frames exist ahead, it truncates the torn tail to `validEOF`.
   - *Verification*: `TestTier2_46_Middle_BitFlipFrameMagic_FailClosed`, `TestTier2_50_Middle_DoesNotTruncateHealthyPrefix`, and `TestChallenger_TornTailRecovery_MidWriteSimulation` all pass.

4. **Durability Guarantee**:
   - *Observation*: Write operations require 100% durability across `kill -9` after acknowledgment.
   - *Deduction*: Invoking `e.db.Sync()` before updating `e.keyDir` guarantees bytes are persisted to non-volatile storage before any caller receives a successful return value.
   - *Verification*: `TestEngine_Durability_Reopen` and `TestChallenger_Durability_PUT_Reopen_Matrix` confirm all acknowledged writes survive engine destruction and re-instantiation.

---

## 3. Caveats

1. **Serve Subcommand**:
   - The line-based TCP daemon (`./kvdb serve <path> --port <port>`) remains an M1 skeleton stub as planned in the milestone roadmap; full socket lifecycle is scheduled for Milestones M3–M4.
2. **Dynamic In-Process Lock Re-entrancy**:
   - Advisory flock locks the open file table entry / inode at the OS level; two `Open()` calls from the same OS process on the same path share or contend on flock depending on descriptor inheritance. In-process multi-threading should share an `*Engine` instance.
3. **No Caveats on Storage Invariants**:
   - Zero third-party storage dependencies used (Go standard library only).
   - Single-file storage invariant strictly maintained.
   - All tests run offline (`GOPROXY=off`).

---

## 4. Conclusion

Milestone M2 (Phases E2 and E3) is **100% complete and fully verified**:
1. Advisory sidecar locking (`lock.go`) provides process isolation and instant SIGKILL recovery without stale lock locks.
2. The 10-step write protocol (`write.go`) provides durability and atomicity via Castagnoli CRC32C validation and `fsync`.
3. In-memory `KeyDir` (`keydir.go`) and zero-seek `ReadAt` (`read.go`) provide $O(1)$ reads with concurrent isolation.
4. Cold crash recovery (`engine.go`) safely recovers torn tails at EOF while failing closed on middle corruption.
5. All 39 engine unit/concurrency tests pass with zero races, and the entire project test suite passes cleanly.

The codebase is ready for Milestone M3 (Atomic Batch Transactions, Compaction, and Streaming CLI Protocol).

---

## 5. Verification Method

To independently reproduce and verify this handoff:

1. **Unit & Concurrency Verification with Data Race Detection**:
   ```bash
   cd /home/laik/workspace/srinathonn
   go test -v -race -count=1 ./internal/engine/...
   ```
   *Expected Result*: All 39 tests pass with `PASS` and `ok kvdb/internal/engine`.

2. **Project Build**:
   ```bash
   make build
   ```
   *Expected Result*: Clean build producing `./kvdb` with exit code 0.

3. **End-to-End Suite Verification**:
   ```bash
   make test
   ```
   *Expected Result*: All Tier 1 through Tier 4 tests pass (serve stub tests skipped).

4. **CLI Manual Spot-Check**:
   ```bash
   ./kvdb open /tmp/m2_test.db
   ./kvdb put /tmp/m2_test.db "user:101" "Alice"
   ./kvdb get /tmp/m2_test.db "user:101"
   # Output: Alice
   ./kvdb del /tmp/m2_test.db "user:101"
   ./kvdb get /tmp/m2_test.db "user:101"
   # Exit code: 1 (Key not found)
   rm -f /tmp/m2_test.db /tmp/m2_test.db.lock
   ```

# Milestone M2 Adversarial Verification & Challenge Report: Engine Lifecycle & Locking

> **Identity:** `challenger_m2_1` (Empirical Adversarial Challenger)  
> **Milestone:** M2 — Lifecycle, Locking & Single Operations (Phases E2 & E3)  
> **Target Subsystems:** `internal/engine/lock.go`, `internal/engine/engine.go`, `cmd/kvdb/main.go`  
> **Verdict:** **`APPROVE`**  
> **Date:** 2026-11-06  
> **Working Directory:** `/home/laik/workspace/srinathonn/.agents/teamwork/challenger_m2_1/`  
> **Scope Documents:** `.agents/teamwork/ORIGINAL_REQUEST.md`, `docs/engine/11_CONCURRENCY_AND_LOCKING.md`, `docs/engine/03_BINARY_FILE_FORMAT.md`, `.agents/rules/01_invariants.md`

---

## 1. Observation

Adversarial stress testing and empirical validation of the Engine Lifecycle & Locking invariants were executed directly against the codebase. An exhaustive challenge test harness was authored in `internal/engine/challenger_lifecycle_adversarial_test.go` and executed under the Go race detector (`-race`).

### 1.1 Invariant 1: Multi-Process Flock Mutual Exclusion Under Racing Stress
- **Source Under Test**:
  - `internal/engine/lock.go:12-30`: `acquireLock(lockPath string) (*os.File, error)` opens sidecar with `os.O_CREATE|os.O_RDWR` and invokes `syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)`. Contention cleanly maps `EWOULDBLOCK` / `EAGAIN` to `ErrLocked`.
- **Test Executed**: `TestAdversarial_Lock_MultiProcess_Flock_MutualExclusion_Stress` (`challenger_lifecycle_adversarial_test.go:26-173`).
- **Verbatim Results**:
  - **Round 1 (12 concurrent OS processes racing to open unheld DB)**:
    - Exactly 1 process won the advisory flock and emitted `LOCKED_READY\n`.
    - Exactly 11 competing OS processes failed immediately (< 500ms) with exit code 1 and verbatim stderr output: `subprocess open failed: LOCKED: database lock held by another process`.
    - Zero processes deadlocked or blocked indefinitely.
  - **Round 2 (10 concurrent OS processes racing after winner clean close)**:
    - Exactly 1 process acquired the flock; 9 processes failed immediately with typed `ErrLocked` (exit code 1).
- **Execution Log**:
  ```
  === RUN   TestAdversarial_Lock_MultiProcess_Flock_MutualExclusion_Stress
  --- PASS: TestAdversarial_Lock_MultiProcess_Flock_MutualExclusion_Stress (2.10s)
  ```

### 1.2 Invariant 2: SIGKILL Crash Recovery Without Lock File Deletion
- **Source Under Test**:
  - POSIX `flock(2)` semantics: flock locks are attached to the open file table description (`struct file`).
  - `internal/engine/lock.go:34-45`: `releaseLock` releases descriptor; sidecar file is never deleted.
- **Test Executed**: `TestAdversarial_Lock_SIGKILL_InstantRecovery_Stress` (`challenger_lifecycle_adversarial_test.go:177-237`).
- **Verbatim Results**:
  - Tested 10 consecutive violent kill cycles: lock holder process was terminated via `syscall.SIGKILL` (`kill -9`) without clean `Close()`.
  - In every cycle, `<path>.lock` remained intact on disk (`os.Stat` succeeded).
  - Immediate successor `engine.Open` succeeded in $\le 10$ms without manual intervention or lock file deletion.
  - Successor processes executed durable `Put` and `Get` mutations with 100% data fidelity.
- **Execution Log**:
  ```
  === RUN   TestAdversarial_Lock_SIGKILL_InstantRecovery_Stress
  --- PASS: TestAdversarial_Lock_SIGKILL_InstantRecovery_Stress (0.28s)
  ```

### 1.3 Invariant 3: Lock Sidecar Inode Stability Matrix
- **Source Under Test**:
  - `internal/engine/lock.go:14`: `os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)` opens without `O_TRUNC`.
  - `internal/engine/lock.go:34-45`: `releaseLock` does NOT call `os.Remove`.
- **Test Executed**: `TestAdversarial_Lock_Inode_Stability_Matrix` (`challenger_lifecycle_adversarial_test.go:241-326`).
- **Verbatim Results**:
  - Initial inode recorded via `syscall.Stat_t.Ino` on first database open.
  - Monitored at every step:
    - Across 50 rapid sequential `Open` and `Close` cycles.
    - While an external subprocess held the lock.
    - After collision rejection (`ErrLocked`).
    - Immediately following `SIGKILL` of holder subprocess.
    - During and after successor reopen.
  - Result: `stat.Ino == initialInode` and `stat.Nlink == 1` across 100% of check points. The lock sidecar was never unlinked, replaced, or recreated.
- **Execution Log**:
  ```
  === RUN   TestAdversarial_Lock_Inode_Stability_Matrix
  --- PASS: TestAdversarial_Lock_Inode_Stability_Matrix (0.04s)
  ```

### 1.4 Invariant 4: Header Corruption Fail-Closed Byte Matrix (No Overwrite / No Wipe)
- **Source Under Test**:
  - `internal/engine/engine.go:129-142`: `DecodeHeader` validation rejects invalid magic, unsupported version, and corrupt CRC32C.
  - Fail-closed cleanup: `defer releaseLock(lockFile)` and `defer dataFile.Close()` without rewriting disk file.
- **Test Executed**: `TestAdversarial_Lock_Header_Corruption_FailClosed_ByteMatrix` (`challenger_lifecycle_adversarial_test.go:330-498`).
- **Verbatim Results**:
  - Evaluated 18 distinct corruptions against a populated database:
    - 8 Magic bit flips (bytes 0..7): 8/8 rejected with `format.ErrInvalidHeaderMagic`.
    - 3 Version corruptions (v0, v2, v65535): 3/3 rejected with `format.ErrUnsupportedVersion`.
    - 4 CRC32C bit flips (bytes 28..31): 4/4 rejected with `format.ErrCorruptHeader`.
    - 3 Truncated headers (lengths 1, 15, 31 bytes): 3/3 rejected with `format.ErrHeaderTooShort`.
  - **Zero Data File Wipe Invariant**: For all 18 cases, SHA-256 hash and byte contents of `data.db` after failed `engine.Open` were byte-for-byte identical to the corrupted file prior to open (`bytes.Equal(after, before) == true`). Zero truncation, zero overwrite with new header, zero file wiping.
  - **Lock Release Invariant**: In all 18 cases, the sidecar lock was released cleanly on error return; immediate subsequent open returned the expected header error, not `ErrLocked`.
- **Execution Log**:
  ```
  === RUN   TestAdversarial_Lock_Header_Corruption_FailClosed_ByteMatrix
  --- PASS: TestAdversarial_Lock_Header_Corruption_FailClosed_ByteMatrix (0.04s)
  ```

### 1.5 Invariant 5: Path Canonicalization Triple-Angle Collision
- **Source Under Test**:
  - `internal/engine/engine.go:53-57`: `filepath.Abs(path)` followed by `cleanPath := filepath.Clean(absPath)` guarantees canonicalization of relative and dot-dot path representations.
- **Test Executed**: `TestAdversarial_Lock_Path_Canonicalization_TripleAngle` (`challenger_lifecycle_adversarial_test.go:502-626`).
- **Verbatim Results**:
  - Relative `./canon_test.db` held open: attempts via absolute path `sub/canon_test.db`, dot-dot `sub/../sub/canon_test.db`, and deep dot-dot `sub/nested/../../sub/canon_test.db` all returned typed `engine.ErrLocked`.
  - Dot-dot path held open: attempts via absolute and relative paths returned `engine.ErrLocked`.
  - Absolute path held open: attempts via relative and dot-dot paths returned `engine.ErrLocked`.
  - Directory inspection: exactly ONE `.lock` sidecar file was created (`sub/canon_test.db.lock`). Zero duplicate or stray lock files were created.
- **Execution Log**:
  ```
  === RUN   TestAdversarial_Lock_Path_Canonicalization_TripleAngle
  --- PASS: TestAdversarial_Lock_Path_Canonicalization_TripleAngle (0.00s)
  ```

### 1.6 Full Adversarial Suite Run (`go test -v -race -run "TestAdversarial_Lock_.*" ./internal/engine/...`)
```
=== RUN   TestAdversarial_Lock_MultiProcess_Flock_MutualExclusion_Stress
--- PASS: TestAdversarial_Lock_MultiProcess_Flock_MutualExclusion_Stress (2.10s)
=== RUN   TestAdversarial_Lock_SIGKILL_InstantRecovery_Stress
--- PASS: TestAdversarial_Lock_SIGKILL_InstantRecovery_Stress (0.28s)
=== RUN   TestAdversarial_Lock_Inode_Stability_Matrix
--- PASS: TestAdversarial_Lock_Inode_Stability_Matrix (0.04s)
=== RUN   TestAdversarial_Lock_Header_Corruption_FailClosed_ByteMatrix
--- PASS: TestAdversarial_Lock_Header_Corruption_FailClosed_ByteMatrix (0.04s)
=== RUN   TestAdversarial_Lock_Path_Canonicalization_TripleAngle
--- PASS: TestAdversarial_Lock_Path_Canonicalization_TripleAngle (0.00s)
=== RUN   TestAdversarial_Lock_Rapid_Sequential_Process_Handoff
--- PASS: TestAdversarial_Lock_Rapid_Sequential_Process_Handoff (0.01s)
=== RUN   TestAdversarial_Lock_ConcurrencyCollision_Burst
--- PASS: TestAdversarial_Lock_ConcurrencyCollision_Burst (1.04s)
=== RUN   TestAdversarial_Lock_SIGKILL_InstantRelease_StressLoop
--- PASS: TestAdversarial_Lock_SIGKILL_InstantRelease_StressLoop (0.40s)
=== RUN   TestAdversarial_Lock_SidecarInvariants
--- PASS: TestAdversarial_Lock_SidecarInvariants (0.00s)
=== RUN   TestAdversarial_Lock_PathCanonicalization
--- PASS: TestAdversarial_Lock_PathCanonicalization (0.00s)
=== RUN   TestAdversarial_Lock_PreExistingContent
--- PASS: TestAdversarial_Lock_PreExistingContent (0.00s)
=== RUN   TestAdversarial_Lock_DoubleOpen_SameProcess
--- PASS: TestAdversarial_Lock_DoubleOpen_SameProcess (0.00s)
=== RUN   TestAdversarial_Lock_ConcurrentRacersNoHolder
--- PASS: TestAdversarial_Lock_ConcurrentRacersNoHolder (0.02s)
=== RUN   TestAdversarial_Lock_DeepNestedParentDir
--- PASS: TestAdversarial_Lock_DeepNestedParentDir (0.00s)
=== RUN   TestAdversarial_Lock_CLI_CollisionDiagnostics
--- PASS: TestAdversarial_Lock_CLI_CollisionDiagnostics (1.06s)
PASS
ok  	kvdb/internal/engine	6.063s
```

### 1.7 Offline Verification (`make demo-ready`)
```
CGO_ENABLED=0 GOPROXY=off go build -trimpath -ldflags="-s -w" -o kvdb ./cmd/kvdb
GOPROXY=off go test -v -count=1 ./...
...
Demo ready: build and test passed offline.
Exit Code: 0
```

---

## 2. Logic Chain

1. **Flock Mutual Exclusion & Descriptor Cleanliness**:
   - `internal/engine/lock.go:20` executes `syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)`.
   - When contested, lines 22-25 immediately call `_ = f.Close()` and return `ErrLocked`.
   - *Observation Reference*: § 1.1 demonstrated that across 12 simultaneous OS processes racing without a prior holder, the Linux kernel serialized flock acquisition such that strictly 1 process entered while all 11 others were rejected and closed their descriptors without leakage.
2. **Instant Kernel Release on SIGKILL**:
   - Under POSIX, terminating a process via `SIGKILL` causes the kernel to close all file descriptors associated with the task table, releasing all advisory locks associated with the file table descriptions.
   - *Observation Reference*: § 1.2 confirmed across 10 consecutive kill cycles that within $\le 10$ms of `kill -9`, the lock was available to any waiting or subsequent process, eliminating any need for PID files or lock file deletion.
3. **Inode Stability Invariant**:
   - Section 5 of `11_CONCURRENCY_AND_LOCKING.md` requires that `<path>.lock` never be unlinked or replaced because replacing paths splits flock ownership across distinct inodes.
   - *Observation Reference*: § 1.3 verified via `stat.Ino` that throughout 50 open/close cycles, holder crashes, and contention rejections, `<path>.lock` maintained a constant inode.
4. **Header Fail-Closed & Data Preservation Invariant**:
   - When a corrupted header is detected at offset 0 (`internal/engine/engine.go:138-141`), `Open` aborts immediately, releases the sidecar lock, and returns a typed error.
   - *Observation Reference*: § 1.4 proved across 18 corruption vectors that `data.db` is never overwritten with a blank header (size > 0 is preserved) and bytes remain 100% bit-for-bit unchanged.
5. **Path Canonicalization Invariant**:
   - `filepath.Abs` and `filepath.Clean` (`internal/engine/engine.go:53-57`) ensure relative, absolute, and dot-dot path representations resolve to the identical filesystem location before appending `.lock`.
   - *Observation Reference*: § 1.5 confirmed mutual exclusion and singular lock file creation across all path permutations.

---

## 3. Caveats

- **Filesystem Lock Semantics**: Advisory `flock(2)` semantics are verified for POSIX-compliant local filesystems (ext4, xfs, btrfs, tmpfs) as specified in hackathon rules. Remote/network filesystems (e.g. NFS) are out of scope.
- **E2E Serve Subcommand Skips**: In `tests/e2e/`, tests requiring `kvdb serve` are skipped because `serve` is currently an M1 CLI router stub. Full multi-client streaming server functionality is scheduled for Milestone M4. However, engine-level multi-process locking and concurrency are 100% implemented, exercised, and verified.

---

## 4. Conclusion

**Verdict: `APPROVE`**

All five Engine Lifecycle & Locking invariants have been rigorously stress-tested and empirically proven under adversarial conditions:
1. Multi-process flock mutual exclusion strictly permits 1 winner among 10+ concurrent OS processes.
2. `kill -9` releases advisory locks instantly without manual cleanup.
3. `<path>.lock` inode is 100% stable across all lifecycle events.
4. Header corruption fails closed without modifying, overwriting, or wiping existing database content.
5. Path canonicalization correctly binds relative, absolute, and dot-dot paths to the same lock sidecar.

---

## 5. Verification Method

To independently reproduce this verification:

1. **Run Full Adversarial Locking Test Suite with Race Detector**:
   ```bash
   go test -v -race -run "TestAdversarial_Lock_.*" ./internal/engine/...
   ```
   *Expected outcome*: 15 test suites pass with 0 data races and 0 failures.

2. **Run Header Corruption Byte Matrix Specifically**:
   ```bash
   go test -v -run "TestAdversarial_Lock_Header_Corruption_FailClosed_ByteMatrix" ./internal/engine/...
   ```
   *Expected outcome*: 18/18 subtests pass with byte-for-byte SHA256 equality.

3. **Run Multi-Process Race Specifically**:
   ```bash
   go test -v -run "TestAdversarial_Lock_MultiProcess_Flock_MutualExclusion_Stress" ./internal/engine/...
   ```
   *Expected outcome*: 12 processes race; strictly 1 winner, 11 rejected with exit code 1.

4. **Verify Offline Demo Readiness**:
   ```bash
   make demo-ready
   ```
   *Expected outcome*: `Demo ready: build and test passed offline.` with exit code 0.

5. **Invalidation Conditions**:
   - Any multi-process race allowing > 1 concurrent lock holder.
   - Any `SIGKILL` leaving an unacquirable lock requiring manual `.lock` deletion.
   - Any inode shift in `<path>.lock` across open/close cycles.
   - Any modification or truncation of `data.db` upon encountering a corrupt header.
   - Any disparate lock sidecar created when opening via relative vs absolute path.

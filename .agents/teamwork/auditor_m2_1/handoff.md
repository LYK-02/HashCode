# Forensic Audit Report — Milestone M2

**Work Product**: `internal/engine/` and repository state for Milestone M2 (Phases E2 & E3)  
**Profile**: General Project (Integrity Forensics) — Mode: Demo  
**Auditor**: `auditor_m2_1`  
**Date**: 2026-11-06  
**Verdict**: **CLEAN** (Zero Integrity Violations Detected)

---

## Executive Summary

A forensic integrity audit was conducted across the storage engine package (`internal/engine/`), binary format codec (`internal/format/`), CLI executable (`cmd/kvdb/`), and test suites (`internal/engine/`, `tests/e2e/`). Every claim made in Worker M2's handoff report was empirically checked and verified. No dummy facades, no hardcoded test values, no banned third-party storage libraries, and no mocked system calls were found. The implementation strictly adheres to standard-library-only execution, non-blocking process-level `flock` locking on a permanent sidecar file, synchronous physical `fsync` barriers prior to index updates, and mathematically verified Castagnoli CRC32C framing.

---

## Phase Results

| Check Item | Result | Forensic Assessment |
|---|:---:|---|
| **1. Genuine Implementation Audit** | **PASS** | `Open`, `Close`, `Put`, `Get`, `Delete`, `Batch`, and locking execute authentic business logic. No hardcoded return values, lookup tables keyed to test inputs, mock responses, or dummy facades. |
| **2. Banned Library & Tool Delegations** | **PASS** | `go.mod` specifies 0 external dependencies (`module kvdb`, `go 1.27`). Authoritative `go list` confirms 100% standard library imports across all packages and test suites. Zero third-party persistence or WAL engines. |
| **3. Durability & Syscall Authenticity** | **PASS** | `syscall.Flock(int(f.Fd()), syscall.LOCK_EX\|syscall.LOCK_NB)` is genuinely invoked on `.lock` sidecar. `e.db.Sync()` is synchronously executed on every mutation before in-memory index publication and caller ACK. Castagnoli CRC32C polynomial (`0x82F63B78`) is calculated over actual headers and payloads. |
| **4. Pre-populated Artifact Detection** | **PASS** | Zero pre-existing `.log`, `*result*`, or `*output*` files found in workspace prior to audit test executions. |
| **5. Static & Runtime Race Inspection** | **PASS** | Build clean with zero warnings. `go test -v -race ./internal/engine/...` passed 100% (19.55s). `go test -v -race ./...` passed 100% across all packages with zero race warnings. |
| **6. Adversarial Stress & Crash Recovery** | **PASS** | Crash scenarios A1–A7, concurrency scenarios D1–D4, 10-process flock mutual exclusion, and SIGKILL recovery stress tests passed without failures or corrupted states. |

---

## 1. Observation

### File & Line Verification
1. **Locking Implementation (`internal/engine/lock.go`)**:
   - Line 14: Opens permanent lock sidecar via `os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)` without truncation or deletion.
   - Line 20: Invokes authentic non-blocking exclusive advisory flock: `err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)`.
   - Line 23–25: Contentious collisions correctly detect `syscall.EWOULDBLOCK` or `syscall.EAGAIN` and map to `ErrLocked`.
   - Line 39–44: `releaseLock` calls `syscall.Flock(int(f.Fd()), syscall.LOCK_UN)` and closes descriptor. Does not delete or rename `<db>.lock`, preserving the inode across process lifetimes.
2. **KeyDir In-Memory Index (`internal/engine/keydir.go`)**:
   - Lines 8–13: `KeyDirEntry` stores absolute file offset `ValueOffset uint64`, `ValueLen uint32`, and monotonic `SeqNo uint64`.
   - Lines 30–50: Metadata accessors `KeyCount()`, `ValidEOF()`, `NextSeqNo()` are guarded by `e.mu.RLock()`.
3. **Synchronous Write & Durability Protocol (`internal/engine/write.go`)**:
   - Lines 11–55 (`Put`): Validates key/value boundaries (`1..1024` byte keys, `<= 1MB` values); acquires exclusive `e.mu.Lock()`; verifies engine not closed/poisoned; invokes `appendFrameLocked(ops)`.
   - Lines 102–131 (`appendFrameLocked`): Encodes frame via `format.EncodeFrame`; enforces overflow-safe quota check (`e.validEOF + frameSize > e.maxBytes -> ErrDiskFull`); executes position-independent `writeAtFull` loop at `e.validEOF`; executes physical barrier `e.db.Sync()`. On write or sync error, calls `e.rollbackAppendLocked(err)` and aborts.
   - Lines 46–54: Only AFTER successful `e.db.Sync()`, records entry in `e.keyDir`, increments `e.validEOF += len(frameBytes)`, advances `e.nextSeqNo++`, and releases lock before returning `nil`.
   - Lines 58–98 (`Delete`): Encodes tombstone operation (`format.OpDel`, nil value); writes and calls `e.db.Sync()`; only on sync success removes entry from `e.keyDir` via `delete(e.keyDir, string(key))`. Missing keys append a durable tombstone and succeed idempotently per specification.
4. **Zero-Seek Read Semantics (`internal/engine/read.go`)**:
   - Lines 10–41 (`Get`): Under `e.mu.RLock()`, absent key returns `(nil, false, nil)` without disk I/O; zero-byte value returns `([]byte{}, true, nil)` without disk I/O; positive-length value reads exactly `ValueLen` bytes from `ValueOffset` via `f.ReadAt` without mutating shared file position. Returns defensive copy.
5. **Replay & Crash Recovery (`internal/engine/engine.go`)**:
   - Lines 52–246 (`Open`): Validates path, acquires exclusive flock. If size 0, writes 32-byte `KVDB2026` header and syncs file and directory. If size >= 32, verifies Castagnoli CRC32C of header. Replays frames up to EOF. Idempotently truncates torn tail frames at EOF and syncs. If corruption occurs in the middle of the file with valid frames ahead, fails closed (`format.ErrCorruptFrameHeader`, `format.ErrTruncatedFrame`, `format.ErrCorruptPayload`).
6. **Binary Encoding & Checksums (`internal/format/`)**:
   - `internal/format/header.go`: `crcTableCast = crc32.MakeTable(crc32.Castagnoli)` (polynomial `0x82F63B78`). Verifies 32-byte DB header with CRC32C over bytes 0..28.
   - `internal/format/uabe.go`: Verifies 28-byte frame header with CRC32C over bytes 8..28. Verifies 4-byte frame trailer with CRC32C over payload bytes.

---

## 2. Logic Chain

1. **Non-Trivial Implementation**:
   - *Premise*: If an engine uses hardcoded responses or dummy facades, static inspection will reveal static constant returns or maps keyed on test keys.
   - *Observation*: Static scan of all return statements across `internal/engine/*.go` reveals zero constant return stubs. The only map in the entire engine is `e.keyDir map[string]KeyDirEntry`.
   - *Inference*: The implementation executes genuine business logic.
2. **Offline & Zero-Banned Library Compliance**:
   - *Premise*: Any import of third-party persistence/WAL libraries (sqlite, rocksdb, badger, pebble, bbolt) violates hackathon rule `02_banned_libraries.md` and integrity mode constraints.
   - *Observation*: `cat go.mod` contains no dependencies. `go list -f '{{.ImportPath}}: {{.Imports}}' ./...` outputs only Go standard library packages (`bufio`, `bytes`, `encoding/binary`, `errors`, `fmt`, `hash/crc32`, `io`, `math`, `os`, `path/filepath`, `sort`, `strconv`, `strings`, `sync`, `syscall`, `testing`, `time`) and local project packages.
   - *Inference*: The codebase is 100% compliant with standard-library-only constraints.
3. **Physical Durability & Syscall Authenticity**:
   - *Premise*: Under the Core Invariant, a write cannot be acknowledged before data is physically durable on disk via `fsync`.
   - *Observation*: `internal/engine/write.go:125` explicitly executes `e.db.Sync()`. Index mutation (`e.keyDir[string(key)] = ...`) occurs at line 47, strictly after line 125. If `Sync()` fails, `rollbackAppendLocked` is called and error is returned.
   - *Inference*: Durability guarantees are mathematically and physically enforced prior to ACK.
4. **Flock Mutual Exclusion & SIGKILL Safety**:
   - *Premise*: Process-level exclusive locking must prevent dual-instance corruption and must auto-release upon abnormal process termination (`kill -9`).
   - *Observation*: `internal/engine/lock.go:20` issues `syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)` on `<db>.lock`. Adversarial test `TestAdversarial_Lock_SIGKILL_InstantRelease_StressLoop` and `TestChallenger_Lock_SIGKILL_SurvivingProcessIntegrity` execute real subprocesses, kill them with `SIGKILL`, and verify instantaneous lock acquisition by a successor process.
   - *Inference*: Advisory file locking is authentically backed by the Linux kernel and robust against crash scenarios.

---

## 3. Adversarial Challenge & Stress Review

### Challenge Summary
**Overall Risk Assessment**: LOW

### Challenges Investigated
1. **Challenge 1: Advisory Lock Inode Invalidation Risk**
   - *Assumption*: Unlinking or renaming `<db>.lock` during close or cleanup could cause concurrent openers to lock different file inodes.
   - *Empirical Check*: Reviewed `internal/engine/lock.go` and `internal/engine/engine.go`. The sidecar file is opened with `O_CREATE|O_RDWR` and never unlinked or truncated (`releaseLock` calls `Flock(LOCK_UN)` and `Close()`). Inode stability verified.
2. **Challenge 2: False Durability ACK on Sync Failure**
   - *Assumption*: If `Sync()` returns an error (e.g. EIO), a flawed implementation might still update in-memory `keyDir` or return `nil`.
   - *Empirical Check*: `internal/engine/write.go:125-128` checks `err := e.db.Sync()`; on non-nil error, `rollbackAppendLocked` is called and `return nil, err` executes immediately. The caller receives the error, and `keyDir` remains unchanged.
3. **Challenge 3: Ambiguity Between Tail Torn Frame vs Middle File Corruption**
   - *Assumption*: A naive recovery might truncate middle-of-file corruption, discarding valid durable data.
   - *Empirical Check*: `internal/engine/engine.go:166-168` and `hasValidFrameAhead` scan forward. If corruption occurs before EOF and valid frames exist ahead, `Open` rejects the file with `format.ErrCorruptFrameHeader` or `format.ErrTruncatedFrame` (failing closed). Verified by `TestChallenger_Harness4_FailClosed_MiddleCorruption`.

### Stress Test Results
- `TestChallenger_Lock_MultiProcessMutualExclusion_10Processes`: 10 competing processes, 1 winner, 9 cleanly rejected with `ErrLocked` (PASS).
- `TestChallenger_Concurrency_D1_20WritersDisjointKeys`: 20 concurrent goroutines, zero race warnings, monotonic sequence numbers (PASS).
- `TestChallenger_Concurrency_D2_20WritersSameKey`: 20 concurrent writers targeting identical key, zero torn writes, latest write wins (PASS).
- `TestChallenger_Concurrency_D3_ContinuousReadersDuringHeavyWrites`: 2,727 writes, 152,504 reads, 0 read errors, 0 torn reads (PASS).
- `TestChallenger_Concurrency_D4_LargeValues_10KB_to_1MB_NoTornReads`: 45 large writes, 1,255 large reads up to 1MB, 0 torn reads (PASS).
- `TestChallenger_GroupA_Crash_Scenarios_A1_to_A7`: Scenarios A1 (0 bytes), A2 (torn header), A3/A4 (full frame durable), A6 (torn delete), A7 (1MB torn write) properly repaired or recovered (PASS).

---

## 4. Caveats

- Milestone M2 covers Phases E2 (Lifecycle & Locking) and E3 (Durable Append, PUT, GET, DELETE). While `Batch` and `CheckFile` are implemented, full batch stress qualification belongs to Milestone M3 (Phase E4), and online background compaction belongs to Milestone M4 (Phases E6–E8).
- The `Compact()` method in `internal/engine/engine.go` is currently a no-op method returning `nil` reserved for Milestone M4 Phase E8. This is expected given the phase progression in `docs/engine/PHASES.md`.

---

## 5. Conclusion

The work product for Milestone M2 has passed all forensic checks:
- **Verdict**: **CLEAN**
- **Zero integrity violations detected**.
- **Zero banned libraries**.
- **100% genuine implementation**.
- **Durability and locking invariants empirically proven**.

Milestone M2 is fully approved.

---

## 6. Verification Method & Evidence

The findings in this report were produced using the following commands:

```bash
# 1. Dependency and Banned Library Audit
go list -f '{{.ImportPath}}: {{.Imports}}' ./...

# 2. Workspace Result Artifact Audit
find . -not -path '*/.*' -type f \( -name '*.log' -o -name '*result*' -o -name '*output*' \)

# 3. Binary Build
make build

# 4. Engine Race Test Suite
go test -v -race ./internal/engine/...

# 5. Full Repository Race Test Suite
go test -v -race ./...
```

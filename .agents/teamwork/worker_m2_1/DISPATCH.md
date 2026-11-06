## 2026-11-06T13:10:17Z
You are worker_m2_1, a Storage Engine Developer working on Milestone M2 of the Go storage engine.
Your working directory is: /home/laik/workspace/srinathonn/.agents/teamwork/worker_m2_1/

Read ORIGINAL_REQUEST.md at:
/home/laik/workspace/srinathonn/.agents/teamwork/ORIGINAL_REQUEST.md

Read the authoritative project specifications and blueprints before writing code:
- Implementation Blueprint for E2: /home/laik/workspace/srinathonn/.agents/teamwork/explorer_m2_1/handoff.md
- Implementation Blueprint for E3: /home/laik/workspace/srinathonn/.agents/teamwork/explorer_m2_2/handoff.md
- Test Suite Specification for E2 & E3: /home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_m2_3/handoff.md
- Global Architecture & Milestones: /home/laik/workspace/srinathonn/.agents/teamwork/PROJECT.md
- Engine Specifications: docs/engine/PHASES.md (§ E2 & E3), docs/engine/11_CONCURRENCY_AND_LOCKING.md, docs/engine/05_WRITE_AND_COMMIT_PROTOCOL.md, docs/engine/06_PUT_GET_DELETE.md, docs/engine/13_IN_MEMORY_INDEX.md
- Invariant Rules: .agents/rules/01_invariants.md, .agents/rules/02_banned_libraries.md, .agents/rules/03_scenarios_matrix.md

MANDATORY INTEGRITY WARNING:
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

Scope and Write Ownership:
You own exclusively:
- internal/engine/engine.go
- internal/engine/lock.go
- internal/engine/write.go
- internal/engine/read.go
- internal/engine/keydir.go
- internal/engine/engine_test.go

Key Requirements:
1. Standard Library Only: Zero third-party dependencies, zero external persistence engines or WALs. 100% offline.
2. Cross-Process Locking (lock.go):
   - Use permanent sidecar file `<path>.lock`.
   - Acquire exclusive non-blocking advisory flock: `syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)`.
   - Return typed `ErrLocked` on contention (`EWOULDBLOCK` or `EAGAIN`).
   - Inode preservation invariant: NEVER unlink, delete, or rename `<path>.lock`. On Close, unlock `syscall.LOCK_UN` and close fd, keeping the file on disk.
3. Engine Lifecycle & Header (engine.go):
   - Path canonicalization via `filepath.Abs` and `filepath.Clean`. Ensure parent directory exists (`os.MkdirAll`).
   - In `Open(path, opts)`: acquire flock first. If data file is size 0, encode and write 32-byte DB header with `format.NewHeader(0)` and Castagnoli CRC32C, sync file, sync parent directory (`syncDir`), set validEOF=32, nextSeqNo=1. If size >= 32, read and decode header; fail closed immediately if corrupted; NEVER overwrite an invalid header.
   - Clean `Close()`: acquire `e.mu.Lock()`, flush, `db.Sync()`, close db, release flock, close lockFile, mark closed. Idempotent. All methods return `ErrClosed` if called after Close.
4. In-Memory Index (keydir.go):
   - `KeyDirEntry` with `ValueOffset uint64`, `FileOffset uint64`, `ValueLen uint32`, `SeqNo uint64`.
   - Map `map[string]KeyDirEntry` for fast in-memory key lookup.
5. Strict 10-Step Write Protocol (write.go):
   - `Put(key, val)` and `Delete(key)`:
     1. Validate key length (1..1024) and value length (0..1MB for Put, 0 for Delete).
     2. Acquire `e.mu.Lock()`. Reject if closed or poisoned.
     3. Build operation slice and encode UABE frame with `e.nextSeqNo`.
     4. Quota check with overflow safety against `e.maxBytes` (return `ErrDiskFull`).
     5. Append at `e.validEOF` using loop (`writeAtFull`).
     6. Physical durability barrier: `e.db.Sync()`.
     7. Update KeyDir (`Put`: set entry with offset pointing to value; `Delete`: delete from map).
     8. Advance `e.validEOF += frameLen` and `e.nextSeqNo++`.
     9. Release lock.
     10. Return nil.
   - Rollback handling: On write or sync failure, attempt `db.Truncate(int64(e.validEOF))` and `db.Sync()`. If rollback fails, set `e.poisoned = true`.
   - Deleting non-existent key: succeeds, appends tombstone frame, advances seqNo, deletes from map.
6. Zero-Seek Reads (read.go):
   - `Get(key)`: acquire `e.mu.RLock()`. Check closed. Look up in KeyDir.
   - If missing, return `nil, false, nil`.
   - If `entry.ValueLen == 0`, return `[]byte{}, true, nil` without disk I/O.
   - If `entry.ValueLen > 0`, allocate buffer, read via `readAtFull(e.db, buf, int64(entry.ValueOffset))`, return copy `buf, true, nil`.
7. Unit & Concurrency Test Suite (engine_test.go):
   - Implement the complete test suite outlined in `spec_miner_m2_3/handoff.md`:
     - Subprocess flock mutual exclusion, clean close handoff, SIGKILL kernel lock release, sidecar persistence.
     - Single PUT, GET, reopen cold restart durability.
     - Same-key overwrite persistence and latest resolution.
     - Empty value (0B) boundary and distinction from not-found.
     - 1MB value boundary.
     - DEL existing key and DEL non-existent key.
     - Binary keys and values (all 256 byte values, NUL, newlines).
     - Quota guard (`maxBytes`) and closed engine error rejection.
     - Concurrency scenarios D1 (disjoint writers), D2 (same key writers), D3 (readers during writes), and D4 (no torn reads with alternating patterns).
8. Verification:
   - Run `go test -v -race ./internal/engine/...`
   - Run `go test -v ./...`
   - Ensure all unit tests pass with zero failures and zero race warnings.

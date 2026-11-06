# Engine Architecture

> Status: **Specification / implementation target**  
> Owner: engine team  
> Canonical filename: `02_ENGINE_ARCHITECTURE.md`

## 1. Purpose and scope

Detailed engine modules, state ownership, locks, and syscall boundaries.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Suggested Go package layout: `internal/format` (header/frame codec and CRC), `internal/engine` (state, API, recovery, write path, compaction), `cmd/kvdb` (CLI/serve), and `internal/testutil`. Avoid circular dependencies. Keep serialization pure and independently testable.

## 4. Design and procedure

`Engine` owns `mu sync.RWMutex`, `db *os.File`, `lock *os.File`, canonical path, parent directory path/descriptor, `maxBytes`, `validEOF`, `nextSeqNo`, compaction generation, and `keyDir map[string]KeyDirEntry`. A writer holds `mu.Lock()` across quota check, append, sync, index application, and EOF/sequence advancement. A reader holds `mu.RLock()` while resolving and reading. A single lock is acceptable and simplifies batch visibility.

## 5. Edge cases and failure behaviour

Cross-process exclusion: open `<path>.lock` with create/read-write; acquire exclusive `flock`; retain descriptor for engine lifetime. Never unlink/rename it. Process death closes descriptors and releases the OS lock. Acquire this lock before recovery or compaction cleanup so two processes cannot race while interpreting or replacing data.db.

## 6. Verification and acceptance tests

Use `ReadAt`/`WriteAt` or explicit `pread`/`pwrite` wrappers; do not share a seek cursor. Implement full write loops because writes may be short. Treat a short read before expected EOF as incomplete/corrupt tail, not as a valid frame. Use `os.File.Sync()` for data sync and a separately opened parent directory descriptor with sync after file creation or rename where supported by the target OS. Check and propagate every close/sync/rename error.

Additional implementation notes:

Lock order must be fixed: process flock first, then in-process mutex. Never acquire these in reverse order. Compaction runs under the writer lock so no writes can be lost between snapshot and swap. Readers are blocked during swap unless the implementation has a proven descriptor-generation scheme; do not optimize this in the hackathon.

Code review checklist: no unsynchronized Engine fields; no shared file cursor; no map mutation under read lock; no index update before sync; no ACK under uncertain I/O; no lock-file replacement; and no unhandled syscall error.

## 7. Implementation notes

- Keep this document synchronized with the actual code and tests.
- Do not claim a behaviour is implemented or verified until evidence exists.
- For persistence changes, update the shared format, architecture diagram, scenario matrix, and torture parser in the same change.
- Record the exact command, seed, commit hash, and result for every crash test.

## 8. Related documents

- `../common/02_OFFICIAL_REQUIREMENTS.md`
- `../common/04_SHARED_DATA_FORMAT.md`
- `../common/05_CLI_AND_PROTOCOL_SPEC.md`
- `../common/10_OFFICIAL_32_SCENARIOS.md`

## 2026-11-06T12:46:57Z
MANDATORY FIRST STEP: Read `/home/laik/workspace/srinathonn/.agents/teamwork/ORIGINAL_REQUEST.md` before starting work.

Context: Milestone M2 (Lifecycle, Locking & Single Operations: Phases E2 and E3).
Scope Document: `/home/laik/workspace/srinathonn/.agents/teamwork/PROJECT.md`
Prior Specifications:
- `docs/engine/PHASES.md § E2 & E3`
- `docs/engine/11_CONCURRENCY_AND_LOCKING.md`
- `docs/engine/02_ENGINE_ARCHITECTURE.md`
- `internal/format/` (codec files created in M1)

Working directory: `/home/laik/workspace/srinathonn/.agents/teamwork/explorer_m2_1/`
Identity: `explorer_m2_1`

Task:
Analyze and formulate the concrete implementation strategy for Phase E2 (Engine Lifecycle & Locks):
1. Canonical path resolution: `filepath.Abs` and directory extraction.
2. Sidecar lock file: `<path>.lock` opened with `os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)`.
3. Process mutual exclusion via `syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)`. Return typed `ErrLocked` on contention. The lock file is NEVER unlinked, deleted, or renamed. Automatically freed by OS kernel on SIGKILL.
4. Data file initialization: If `<path>` does not exist or has 0 bytes, encode 32-byte DB Header (Gen 0) via `format.EncodeHeader`, write at offset 0, call `file.Sync()`, and call `parentDir.Sync()`. Set `validEOF = 32`.
5. In-process synchronization: Fixed lock order: OS flock first (at open), then `sync.RWMutex` for mutations/queries.
6. Clean `Close()`: flushes buffers, calls `file.Sync()`, closes descriptors, and unlocks flock.
7. Recommend clean struct definitions, methods, and file decomposition (`internal/engine/lock.go`, `internal/engine/engine.go`).

Deliverable:
Write report to `/home/laik/workspace/srinathonn/.agents/teamwork/explorer_m2_1/handoff.md`.
Do NOT implement code directly. Send a message to your caller upon completion with your report path.

# Phase T3 Implementation Plan: Subprocess and Process Lifetime Manager

> Canonical Phase: **T3**  
> Component: Child Process Spawner, POSIX Signal Control, Non-Blocking Pipe Management  
> Status: **Planned**  
> Related Docs: `docs/torture_tool/06_SUBPROCESS_MANAGEMENT.md`, `docs/torture_tool/07_RANDOM_CRASH_INJECTION.md`

---

## 1. Objective and Scope

Implement the process control subsystem responsible for launching, piping, monitoring, ungracefully terminating (`SIGKILL`), and restarting the Go `./kvdb serve <db_path>` child process. The manager must guarantee reliable unbuffered communication, avoid pipe buffer deadlocks by continuously draining `stderr`, detect premature child exit, and ensure zero leaked processes or lingering file locks.

---

## 2. Dependencies and Pre-Conditions

- **Executable:** `./kvdb` compiled binary (or mock harness during self-testing).
- **Runtime Environment:** Python standard library `subprocess`, `os`, `signal`, `threading`, `time`.
- **OS Target:** Linux POSIX environment.

---

## 3. Detailed Technical Specifications

### 3.1 Subprocess Spawning
- **Command:** `[kvdb_path, "serve", db_file_path]`
- **Pipes:**
  - `stdin`: `subprocess.PIPE` (managed by harness write mutex).
  - `stdout`: `subprocess.PIPE` (managed by harness line reader thread).
  - `stderr`: `subprocess.PIPE` (drained asynchronously by a dedicated thread into an in-memory ring buffer and file).
- **Process Group:** Spawn with `preexec_fn=os.setsid` (or `start_new_session=True`) to allow signaling the entire process group if auxiliary processes are ever spawned.

### 3.2 Non-Blocking Pipe & Deadlock Prevention
- OS pipe buffers on Linux are typically 64 KiB. If the engine writes extensive diagnostics to `stderr` and it is not consumed, the engine will block on `write(2)`, deadlocking the entire system.
- **Dedicated Stderr Thread:** A daemon thread continuously loops on `child.stderr.readline()` until EOF, appending lines to `run_stderr.log` and keeping the last 1,000 lines in memory for immediate error reporting.

### 3.3 Ungraceful Kill Semantics
- **Signal:** Must be strictly **POSIX SIGKILL (`signal.SIGKILL` / `kill -9`)**.
- **No Graceful Tear-down:** Do NOT send `SIGTERM`, `SIGINT`, or close `stdin` as crash injection. The objective is to simulate instant power loss or uncatchable process death at arbitrary microseconds.
- **Process Wait:** After `os.kill(pid, signal.SIGKILL)`, call `child.wait(timeout=5.0)` to ensure kernel process table cleanup (no zombies) and release of kernel file locks (`flock`).

---

## 4. Phase Breakdown & Subtasks

### Subtask T3.1: ChildProcessController Implementation
- Create `torture_tool/process/controller.py`:
  - `start() -> None`: Verify binary exists, configure pipes, spawn child, start stderr drainer thread, verify initial liveness.
  - `is_alive() -> bool`: Non-blocking check via `poll()`.
  - `kill() -> int`: Issue `os.kill(self.pid, signal.SIGKILL)`, await termination, return exit status.
  - `cleanup() -> None`: Defensive kill-and-wait to prevent leaked children on unexpected harness exceptions.

### Subtask T3.2: Asynchronous Stderr Drainer
- Implement `torture_tool/process/stderr_drainer.py`:
  - Thread-safe ring buffer capturing diagnostic output.
  - Real-time file sink writing to `<iteration_dir>/engine_stderr.log`.
  - Clean join and shutdown handling when the pipe closes on process death.

### Subtask T3.3: Health Check & Readiness Handshake
- Implement readiness detection:
  - If protocol includes `PING`, send `PING ready-check` and await `PONG` or first `ACK`.
  - Detect immediate child startup crashes (e.g., bad arguments, database lock contention, permission error) and raise actionable `ProcessStartupError`.

### Subtask T3.4: Lock Release & Process Death Verification
- Implement verification helper:
  - Confirm file lock is released by attempting an immediate `flock(LOCK_EX | LOCK_NB)` on the DB file or spawning a short-lived `kvdb check` command.

---

## 5. Failure Modes and Edge Cases

| Case | Scenario | Expected Handling |
|---|---|---|
| **P3.1** | Child exits prematurely before kill | `is_alive()` detects exit; flags unexpected early crash; captures stderr log. |
| **P3.2** | Stderr pipe buffer fills up | Stderr drainer consumes bytes continuously, avoiding write blockages. |
| **P3.3** | Broken pipe on stdin write | `BrokenPipeError` or `IOError` caught safely; marks in-flight requests as pending kill. |
| **P3.4** | Zombie processes | `child.wait()` is called following `SIGKILL`, reaping process state cleanly. |
| **P3.5** | Lingering child on harness crash | `atexit` handler and `try...finally` ensure `SIGKILL` is sent to any active PID. |

---

## 6. Verification and Completion Criteria

- [ ] Test spawner launches process and establishes working stdin/stdout/stderr pipes.
- [ ] Test stderr flooding ($>1$ MB of diagnostic output) does not block child execution.
- [ ] Test `kill()` reliably terminates child with `SIGKILL` (exit code `-9` or `137`).
- [ ] Test no orphan processes remain in `ps` after repeated cycles.


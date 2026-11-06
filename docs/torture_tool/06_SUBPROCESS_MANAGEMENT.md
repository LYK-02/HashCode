# Subprocess Management

> Status: **Specification / implementation target**  
> Owner: torture team  
> Canonical filename: `06_SUBPROCESS_MANAGEMENT.md`

## 1. Purpose and scope

Specifies child launch, pipes, kill semantics, restart, timeouts, and cleanup.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Launch the exact judge binary and command: `./kvdb serve <db_path>`. Use `subprocess.Popen` with stdin/stdout/stderr pipes, explicit text/binary mode, and line buffering appropriate to the protocol. Capture PID, start time, command, working directory, environment, and commit hash in the run manifest.

## 4. Design and procedure

One writer lock protects stdin. One dispatcher thread reads stdout and resolves request IDs. One stderr drainer continuously consumes stderr. Use explicit timeouts for startup, response wait, process exit, check, dump, and joins. Detect early child exit before sending requests. On normal EOF, close stdin and wait. For crash injection, use `os.kill(pid, signal.SIGKILL)` on POSIX and wait for process termination. Do not use SIGTERM as the tested crash.

## 5. Edge cases and failure behaviour

The kill delay must be genuinely random in milliseconds, selected from the seeded scheduler while writers are active. Record the selected delay and actual elapsed time. Avoid deterministic `kill after N operations` as the only kill method. A deterministic test hook can supplement random kill coverage but cannot replace it.

## 6. Verification and acceptance tests

After kill, collect exit code and remaining stdout/stderr. Mark pending requests unresolved. Restart with `kvdb open <path>` or `check`/`dump` commands as specified. Run check before dump. The harness must fail if check returns nonzero, dump cannot parse, or process times out. Never treat a crashed torture subprocess as a database pass.

Additional implementation notes:

Process cleanup: in `finally`, if child is still alive, kill it and wait. Close pipes. Join all threads with deadlines. Avoid killing the parent process group unless the engine spawns descendants. Keep temp DB directory and logs on failure; remove only on success if configured.

Tests: executable missing; permission denied; child exits immediately; stdout closes; stderr floods; child hangs; kill races with exit; SIGKILL delivery; restart lock release; check timeout; dump timeout; and harness interruption. Ensure no orphan child remains after any path.

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

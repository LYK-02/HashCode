# Torture Architecture

> Status: **Specification / implementation target**  
> Owner: torture team  
> Canonical filename: `02_TORTURE_ARCHITECTURE.md`

## 1. Purpose and scope

Defines controller, workers, protocol dispatcher, oracle, kill scheduler, and verifier.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

The harness is separate from the engine and should use only Python standard library where practical: `subprocess`, `threading`, `queue`, `random`, `time`, `signal`, `json`, `argparse`, `hashlib`, `pathlib`, and `tempfile`. Keep components small and independently testable.

## 4. Design and procedure

Components: CLI argument parser; deterministic random generator; workload generator; child process manager; stdin request writer; stdout response dispatcher; concurrent worker threads; append-only oracle journal; kill scheduler; restart/check/dump runner; dump parser; oracle state evaluator; report writer. Each component has a clear contract and does not reach into engine internals.

## 5. Edge cases and failure behaviour

The child manager launches `kvdb serve <path>` with stdin/stdout/stderr pipes. Use unbuffered or line-buffered protocol handling. One lock serializes writes to stdin. One stdout-reader thread parses response lines and resolves pending request IDs via a thread-safe map/queue. Workers wait on their own response futures. A separate stderr-drainer prevents pipe backpressure deadlock.

## 6. Verification and acceptance tests

Oracle journal is append-only and independent. It records START before sending a request, ACK only after receiving exact `ACK id`, FAIL when exact `ERR id` arrives, and process/iteration metadata. Each event is flushed to the oracle file so harness crash does not erase evidence. The expected state is derived from ACKed operations in their observed linearization order; in-flight operations are modeled as uncertain.

Additional implementation notes:

Kill scheduler chooses a random delay in milliseconds from the seeded PRNG while workers are active. It sends SIGKILL to the actual child PID (or process group only if child tree management requires it), waits for exit, then joins workers and drains response state. Do not send SIGTERM or request graceful shutdown as the primary crash injection.

Verifier runs `kvdb check`, then `kvdb dump`, parses the deterministic binary-safe format, and compares every expected key/value. It also checks batch membership and no unknown/torn values. It must distinguish `NOTFOUND` from empty value. On mismatch, emit first mismatch and full diff summary.

Avoid deadlocks: bound pipe reads/writes; always drain stderr; detect child exit; time out pending IDs; handle broken pipe; join threads with deadlines; and terminate leftover children. A harness timeout is a test failure, not a pass.

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

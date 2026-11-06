# Failure Reporting and Logging

> Status: **Specification / implementation target**  
> Owner: torture team  
> Canonical filename: `15_FAILURE_REPORTING_AND_LOGGING.md`

## 1. Purpose and scope

Defines event formats, summaries, artifact preservation, and actionable diagnosis.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Every run has a unique run directory and machine-readable manifest. Log events as JSON Lines so partial logs remain readable after harness interruption. Use UTC timestamps or monotonic nanoseconds for event ordering; record both wall and monotonic time if needed. Keep engine stdout protocol-only and capture stderr separately.

## 4. Design and procedure

Required event types: RUN_START, ITERATION_START, START, ACK, FAIL, KILL, CHILD_EXIT, RESTART, CHECK, DUMP, VERIFY, ITERATION_END, RUN_END. Include seed, mode, iteration, op ID, worker ID, request type, payload hashes/lengths, timestamp, PID, return code, and artifact paths as applicable. Do not store huge payloads repeatedly if hashes plus a separate payload fixture are sufficient for reproduction.

## 5. Edge cases and failure behaviour

One-line iteration output: iteration number, seed, mode, kill delay, active workers, operations started, ACKs, unresolved operations, check result, verification result, and elapsed time. Final summary includes total iterations, pass/fail, kills delivered, kills during in-flight writes/batches, ACK count, batch count, and total runtime.

## 6. Verification and acceptance tests

On first failure, stop new work; preserve database, oracle log, stdout/stderr, command manifest, generated operations, expected state, actual dump, and diff. Print exact mismatch: key hex, expected presence/value hash/length, actual presence/value hash/length, related op IDs, and last acknowledged sequence. Exit nonzero. Never continue after a failed invariant and later report aggregate PASS.

Additional implementation notes:

Classify failures: engine invariant violation; protocol mismatch; harness error; timeout/hang; check failure; dump parse failure; oracle inconsistency; environment issue. A harness error is not an engine pass. Preserve raw evidence so another team member can reproduce and inspect.

Tests: kill harness mid-log; disk full on artifact filesystem; malformed engine response; duplicate ACK; missing ACK; check nonzero; dump invalid; first-failure stop; artifact retention; and stable JSON parsing.

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

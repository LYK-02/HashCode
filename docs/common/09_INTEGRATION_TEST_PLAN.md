# Integration Test Plan

> Status: **Specification / implementation target**  
> Owner: common team  
> Canonical filename: `09_INTEGRATION_TEST_PLAN.md`

## 1. Purpose and scope

Defines end-to-end tests between the Go engine, CLI, Python harness, and filesystem.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Integration tests exercise the same binaries and stdin/stdout protocol the judge will use. Do not substitute direct engine-library calls for serve-mode tests. Use a fresh temporary directory per test, unique database paths, and a recorded seed. Ensure every test cleans up only after preserving failure evidence.

## 4. Design and procedure

Protocol smoke: start `kvdb serve`; send PUT/DEL/BATCH with unique IDs; verify matching ACK/ERR; close stdin; reopen; run check and dump. Verify no diagnostic text appears on stdout. Use two concurrent request producers through one dispatcher. Confirm ACK routing never crosses request IDs.

Durability: send a PUT, wait for ACK, `SIGKILL`, reopen, and verify value. Repeat for delete and batch. Crash-window tests should use random millisecond delays and optional deterministic test hooks in development builds; do not make production torture depend only on fixed operation counts.

## 5. Edge cases and failure behaviour

Atomicity: use batches with unique sentinel keys, duplicate keys, put-then-delete and delete-then-put ordering. Kill at random times; after restart verify every ACKed batch fully applied and every unacknowledged batch wholly present or absent. Verify readers during a batch observe only before/after states.

Recovery: create a valid DB, kill during recovery, restart repeatedly, and compare logical dump to oracle. Manually truncate one byte from EOF; verify only the last unacknowledged/incomplete frame is discarded. Test header truncation during initial creation separately from corruption of a database containing acknowledged operations.

## 6. Verification and acceptance tests

Compaction: populate live and deleted keys; record logical dump; run compaction; kill at temp creation, mid-write, after temp sync, after close, around rename, after directory sync, and during descriptor/index swap. Reopen and compare. Verify file shrinks after one million put/delete workload. Disk full: cap writes just below and above boundaries; ensure failed operation is not ACKed and previous data remains readable; test compaction under insufficient temporary space.

Additional implementation notes:

Concurrency: two writers on distinct keys, two writers on same key, long batch plus single puts, reader during batch, process-level second opener blocked by flock, and lock release after SIGKILL. Validate no mixed values and no lost acknowledged operations. Recovery performance: one million keys; run at least five measured opens after warmup and record median and max, with correctness check after each.

Every test report must include command, seed, database path, OS/filesystem, Go/Python versions, iteration count, elapsed time, expected result, actual result, and log artifact. Fail fast on first invariant violation; do not keep running and bury the first failure.

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

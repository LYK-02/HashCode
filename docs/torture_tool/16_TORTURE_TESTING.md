# Torture Testing

> Status: **Specification / implementation target**  
> Owner: torture team  
> Canonical filename: `16_TORTURE_TESTING.md`

## 1. Purpose and scope

Defines the harness's own test strategy and qualification criteria.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

The torture tool itself can be wrong. Test it independently before using it as proof. Unit-test the PRNG derivation, workload generation, protocol parser, request dispatcher, oracle replay, dump parser, comparison logic, timeout handling, and failure artifact writer.

## 4. Design and procedure

Use a fake child process to simulate ACK, ERR, delayed responses, out-of-order responses, duplicate IDs, malformed lines, stdout close, stderr flood, hangs, and early exit. Verify the dispatcher routes each response to the correct request and workers do not consume each other's ACKs. Verify stdin requests are written atomically under a lock.

## 5. Edge cases and failure behaviour

Oracle tests use known operation histories with overwrites, deletes, batches, same-key concurrency, empty values, and unresolved operations. Confirm acknowledged history is exact; unresolved single operation may be present/absent; unresolved batch may be all/none; and unknown or mixed values always fail. Ensure ACK order/sequence is sufficient to resolve same-key final state.

## 6. Verification and acceptance tests

Kill tests verify the actual signal is SIGKILL on POSIX, the process is killed while workers are active, the selected delay is random milliseconds from the seed, and no graceful close occurs before kill. A run with no pending operations at kill must be identified as weak coverage rather than silently treated as a strong crash test.

Additional implementation notes:

End-to-end qualification: run default 500 iterations; run large-values, large-batches, compaction, recovery-crash, and disk-full; run with a previously unseen seed; confirm all tests run offline; and verify first failure produces a reproducible artifact. Run the harness on the same laptop/filesystem used for judging.

Do not rig pass conditions, filter out failures, ignore mismatches, derive expected state from dump, or fake random delays. These behaviours can cause disqualification. A tool that finds a real bug is doing its job; record the bug and fix the engine.

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

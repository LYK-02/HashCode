# Torture Test Modes

> Status: **Specification / implementation target**  
> Owner: torture team  
> Canonical filename: `10_TORTURE_TEST_MODES.md`

## 1. Purpose and scope

Defines mandatory modes, flags, and the exact coverage each mode must produce.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Common flags: `--iterations N` (default 500), mandatory `--seed N`, `--mode NAME`, `--db-dir PATH`, `--max-bytes N`, `--keep-artifacts`, and optional `--verbose`. Every mode uses the same child manager, independent oracle, random kill scheduler, check/dump verifier, and failure reporting. A mode must never skip correctness verification.

## 4. Design and procedure

`default`: mixed PUT/DEL/BATCH with at least two writers, random keys, overwrites, and random millisecond kill. `large-values`: repeatedly write values up to exactly 1 MiB, including binary patterns, and kill while values are in flight. `large-batches`: send batches up to 10,000 operations and verify atomicity. `compaction`: create obsolete log data, run compaction, kill at random during maintenance, then verify logical equality and shrinkage. `recovery-crash`: kill a process while open/recovery is running, then repeat open/check until stable. `disk-full`: use small max_bytes, verify clean errors and previous data integrity.

## 5. Edge cases and failure behaviour

Optional `concurrency` mode intensifies same-key races and reader overlap. Every run prints a mode summary and per-iteration one-line result. A final summary includes passed, failed, kills, kills-with-inflight, ACK count, batches, and elapsed time. First failure stops the run and preserves artifacts.

## 6. Verification and acceptance tests

Mode validation must reject unknown names and incompatible flags. `--seed` is required. `--iterations` must be positive. `--max-bytes` must be large enough for a valid header or the test is specifically a header/quota test. Do not let a default mode silently skip batches or use only one writer.

Additional implementation notes:

The official judge commands include default 500 iterations and targeted large-values, compaction and recovery-crash invocations. Add large-batches and disk-full because the official minimum coverage explicitly includes them. Record the exact commands in README.

Tests: each mode produces the intended workload; kills happen during activity; check/dump always run; failures exit nonzero; summaries are parseable; and a new judge seed works without prior preparation.

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

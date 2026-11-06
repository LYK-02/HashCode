# Workload Generation

> Status: **Specification / implementation target**  
> Owner: torture team  
> Canonical filename: `04_WORKLOAD_GENERATION.md`

## 1. Purpose and scope

Defines deterministic operation distributions and adversarial key/value patterns.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Use `random.Random(seed)` or another explicitly documented deterministic PRNG. Do not use global random state or wall-clock timestamps as the sole source of operation IDs. Operation IDs should include iteration, worker, and monotonic counter so they are unique and reproducible. Persist the seed and generator version in the run manifest.

## 4. Design and procedure

Baseline distribution should mix PUT, DEL, and BATCH. Use a bounded key space to create overwrites and delete/reinsert sequences; also use unique-key phases to grow toward one million keys. Include same-key collisions across workers. Generate values across sizes from zero to normal small payloads; inject 1 MiB values in large-values mode. Include arbitrary bytes, NULs, newline, tabs, and invalid UTF-8.

## 5. Edge cases and failure behaviour

Batch generator creates 1–10,000 operations with a configurable size distribution. Include repeated keys, PUT then DEL, DEL then PUT, many operations on one key, all-distinct keys, empty value, and mixed large/small values. Preserve order exactly. Large-batches mode must deliberately hit 10,000 operations, not merely sample a smaller random batch.

## 6. Verification and acceptance tests

For reproducibility, every generated operation is recorded in START before it is sent. A seed must recreate operation content and timing decisions as closely as possible. Timing is inherently OS-scheduled; record actual delay and event timestamps so a failure can be replayed with the same workload and optionally deterministic fault hooks.

Additional implementation notes:

Use separate PRNG streams per worker derived deterministically from master seed + iteration + worker ID. This avoids thread scheduling changing each worker's random sequence. A central kill scheduler has its own stream. Avoid a shared `random.Random` instance across threads.

Modes should constrain distributions rather than bypass the common controller: `default`, `large-values`, `large-batches`, `compaction`, `recovery-crash`, `disk-full`, and optional `concurrency`. Each mode states the invariant it targets and minimum coverage. Do not make a mode silently skip verification.

Tests: same seed yields same operation list; different seeds differ; operation IDs unique; all edge distributions reached; large-values exactly 1 MiB; large-batches exactly 10,000 ops; arbitrary bytes survive encode/decode; and workload log replays without engine access.

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

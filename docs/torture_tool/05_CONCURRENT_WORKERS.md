# Concurrent Workers

> Status: **Specification / implementation target**  
> Owner: torture team  
> Canonical filename: `05_CONCURRENT_WORKERS.md`

## 1. Purpose and scope

Defines writer/reader coordination, operation IDs, and deterministic concurrency.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

At least two concurrent writer workers are mandatory. A reader worker is recommended and required for the D4 scenario. Workers share the child process through a synchronized request writer and response dispatcher. They must not independently write interleaved bytes to stdin or read arbitrary lines from stdout.

## 4. Design and procedure

Each worker owns a deterministic PRNG stream and operation counter. It builds an operation, appends START to the oracle, registers a pending response keyed by op ID, writes one complete request under stdin lock, flushes, and waits for the dispatcher to resolve ACK/ERR. If child dies, worker records an unresolved in-flight state and exits cleanly.

## 5. Edge cases and failure behaviour

For same-key stress, multiple workers target a small key set and use distinguishable fixed-pattern values (for example repeated byte patterns plus operation ID encoded in payload) so mixed values are easy to detect. For distinct-key stress, use disjoint key ranges. For batch concurrency, run one large batch writer alongside single PUT/DEL workers. The engine lock must serialize commits without losing any ACKed operation.

## 6. Verification and acceptance tests

Reader worker issues GET requests while batches execute. The result must be either pre-batch or post-batch state for the keys affected by the batch; never an intermediate combination. If serve protocol does not expose GET, D4 must be tested through a separate test client/API; do not claim D4 from post-crash dump alone.

Additional implementation notes:

Do not stop workers before kill injection in the default mode. The random kill must happen while operations are active. On kill, mark every pending request unresolved. A worker receiving EOF/BrokenPipe is not a successful operation. Use bounded joins and detect leaked threads.

Tests: two writers; three writers; same key; disjoint keys; long batch plus singles; reader during batch; server killed while requests pending; response order; pending map cleanup; and deterministic per-worker workload under different thread schedules.

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

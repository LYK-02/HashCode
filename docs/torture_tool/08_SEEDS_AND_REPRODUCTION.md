# Seeds and Reproduction

> Status: **Specification / implementation target**  
> Owner: torture team  
> Canonical filename: `08_SEEDS_AND_REPRODUCTION.md`

## 1. Purpose and scope

Defines CLI seed semantics, deterministic streams, manifests, and failure replay.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

`--seed` is mandatory for every mode. Parse it as an unsigned integer within a documented range. A run manifest records seed, mode, iteration count, PRNG algorithm/version, engine binary hash/commit, harness commit, database path, OS, filesystem, and start time. If no seed is supplied, exit with usage error rather than silently selecting one.

## 4. Design and procedure

Derive per-iteration and per-worker seeds deterministically from master seed, iteration index, worker ID, and stream purpose. Avoid Python's randomized built-in `hash()` for derivation. Use a stable mixing function or cryptographic hash of the textual tuple. Keep separate streams for workload and kill timing so adding an operation generator does not silently change kill delays.

## 5. Edge cases and failure behaviour

A reproduction record includes failing iteration, selected delay, actual delay, operation IDs, START/ACK/FAIL journal, child exit code, stdout/stderr, expected oracle state, actual dump, and database bytes. Print the exact command, e.g. `./torture --iterations 1 --seed 12345 --mode large-batches --replay failure.json` if replay support exists; otherwise provide the original seed and iteration selection.

## 6. Verification and acceptance tests

Seed replay should reproduce logical operations and kill-delay choices. Exact thread scheduling is not guaranteed; the report must say so. A deterministic replay mode may serialize operations or use barriers to reproduce a suspected race, but never replace the default concurrent random test.

Additional implementation notes:

Store failure artifacts in a unique directory such as `artifacts/failures/<timestamp>-<seed>-<iteration>/`. Never overwrite a prior failure. Keep successful summary logs small. Provide a `--keep-artifacts` option and default to keeping artifacts on failure.

Tests: seed required; same seed/generator version stable; worker streams stable; different seeds differ; failure manifest complete; replay parses; and report command is copy/paste runnable.

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

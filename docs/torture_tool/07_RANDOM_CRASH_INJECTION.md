# Random Crash Injection

> Status: **Specification / implementation target**  
> Owner: torture team  
> Canonical filename: `07_RANDOM_CRASH_INJECTION.md`

## 1. Purpose and scope

Defines unbiased kill timing and coverage evidence.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

The official torture tool must kill at a genuinely random moment using a random delay in milliseconds. The kill must occur while the database process is doing useful work, not only between completed operations. Use seeded randomness for reproducibility and record the actual delay.

## 4. Design and procedure

Recommended loop: start serve; wait for readiness; start at least two writers; optionally start reader; select delay from a documented range (for example 2–40 ms, with distribution adjusted by mode); sleep; send SIGKILL to child; wait; join workers; restart; check; dump; verify. Vary delay range across iterations to cover early and late write windows. Do not hard-code one delay or wait for a fixed operation count.

## 5. Edge cases and failure behaviour

To improve coverage, run many iterations with varied workload intensity and random delay. Include large values/batches to lengthen write windows. Track operations started, ACKed, failed, unresolved, and bytes submitted at kill time. If no operations were in flight at kill, mark that iteration as weak coverage and optionally rerun; do not falsely count it as meaningful crash-during-write coverage.

## 6. Verification and acceptance tests

SIGKILL is intentionally uncatchable; do not install graceful shutdown handlers or flush buffers before kill. On POSIX, send `signal.SIGKILL`. Windows equivalent is TerminateProcess; document if platform support is not implemented. The judging laptop's OS is the target.

Additional implementation notes:

Seed determinism covers PRNG decisions, not exact scheduler interleaving. Record event timestamps, worker IDs, selected delay, and operation journal. On failure, allow `--replay <artifact>` or print a copyable command using same seed/mode/iteration. Optional deterministic failpoints may reproduce exact instruction windows, but random process kills remain required.

Metrics: kills attempted/succeeded; kills with pending writes; kills with pending batches; ACKs observed; unresolved ops; average in-flight count; and scenario-specific outcomes. A 500-iteration count with zero in-flight kills is not acceptable evidence.

Tests: ensure kill uses SIGKILL; selected delay has millisecond granularity; active workers are running at kill; kill occurs before workers stop; seed replay reproduces generated workload; and the tool does not gracefully close the child before sending kill.

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

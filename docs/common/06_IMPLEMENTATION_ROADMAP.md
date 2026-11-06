# Implementation Roadmap

> Status: **Specification / implementation target**  
> Owner: common team  
> Canonical filename: `06_IMPLEMENTATION_ROADMAP.md`

## 1. Purpose and scope

A 24-hour execution plan with integration gates and risk-first ordering.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Hours 0–2 are joint design, not feature coding. Freeze the file header, frame bytes, subrecord grammar, CRC coverage, sequence rules, duplicate-key semantics, CLI commands, serve protocol, lock ownership, and ACK ordering. Create the repository and start the chronological prompt log. Commit the specification before parallel work begins.

## 4. Design and procedure

Hours 2–6: Engine pair implements header/frame codecs, CRC32C, append path, `put/get/del/batch`, minimal recovery and `serve`. Torture pair implements subprocess startup, request-ID protocol, independent oracle, START/ACK/FAIL log, random millisecond kill injection, restart/check/dump comparison. Gate: first real `SIGKILL` run by hour 6. Do not wait for compaction before testing crashes.

Hours 6–10: engine adds strict quota checks, permanent sidecar flock, in-process RwMutex, position-independent reads, error/rollback handling. Torture adds at least two writers, reader activity, deterministic PRNG seeds, large values and large batches. Gate: 100 consecutive single-file kill iterations pass.

Hours 10–15: engine adds compaction with temp-file sync, rename and parent-directory sync; orphan temp handling; recovery idempotence. Torture adds compaction and recovery-crash modes. Gate: compaction demonstrably shrinks the file and survives kills at each swap boundary.

## 5. Edge cases and failure behaviour

Hours 15–19: optimize recovery for one million keys; implement `dump` and `check`; benchmark with the actual laptop and real filesystem; complete all 32 scenario rows with verified seeds or honest `not verified`. Torture reaches 500 iterations. The source blueprint suggests a sub-0.4-second stretch target, but the official threshold is under 2 seconds. Do not sacrifice correctness for the stretch target.

Hours 19–22: freeze features; fix only critical correctness defects; align source comments with numbered architecture steps; complete README, ARCHITECTURE, BREAKING, PROMPTS; clean-clone offline build and test. Hours 22–24: run final soak, rehearse the 15-minute demo, and ensure every member can explain recovery and all crash points.

## 6. Verification and acceptance tests

Gate evidence must be recorded: commit hash, command, seed, iteration count, filesystem, runtime, result, and log path. A gate is not passed because code compiles. It is passed only when the stated behaviour is demonstrated. If a gate fails, name the owner and the specific invariant at risk; stop feature work and repair the highest-risk defect.

Additional implementation notes:

Suggested prioritization under time pressure: durable single writes; batch atomicity; tail recovery; independent oracle; concurrent writers; disk-full; compaction; recovery-crash; one-million-key benchmark; documentation and demo. Do not defer the torture harness until the end: without it, the engine has no evidence.

A final release candidate requires: no known invariant violations; all mandatory CLI commands; no hidden network dependency; 500 default iterations; all targeted modes attempted; all 32 proof rows accurate; build command verified on a clean clone; no fabricated test results; and Git history showing real incremental work.

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

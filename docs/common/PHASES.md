# Common Project Phases — End-to-End Execution Plan

> Owner: whole team  
> Applies to: engine, torture tool, integration, documentation and demo  
> Status vocabulary: `NOT STARTED`, `IN PROGRESS`, `BLOCKED`, `IMPLEMENTED`, `TESTED`, `PASS`, `FAIL`, `NOT VERIFIED`

## 1. Purpose

This document is the master phase plan. It coordinates the engine and torture teams so that parallel work uses one frozen binary format, one protocol, one definition of ACK, and one shared interpretation of the official scenarios.

A phase is complete only when its exit criteria are met and evidence is recorded. A compiling branch is not a completed phase. Do not skip a phase because the schedule is tight; reduce optional scope instead.

## 2. Non-negotiable execution rules

1. The official organiser specification is the source of truth.
2. The selected implementation stack is Go engine + Python torture tool. Do not mix in Rust-specific implementation instructions unless the team formally changes the decision and updates all documents.
3. Only one authoritative database file exists. The KeyDir is derived in-memory state.
4. The engine and torture team share the exact frame format and serve protocol.
5. ACK means the mutation has crossed the successful durability barrier. It must not be emitted before file sync and complete index publication.
6. Every batch is one atomic logical operation and must be represented by one validated frame.
7. Every crash test must be independently checked against the oracle.
8. Every failure is evidence. Preserve it, reproduce it, diagnose it, and record the fix.
9. A scenario is `PASS` only after the current commit has passed an identified test.
10. Never falsify prompt logs, test results, commit history, or coverage.

## 3. Phase map

| Phase | Name | Main outcome |
|---|---|---|
| 0 | Source and rules audit | Shared understanding of official requirements |
| 1 | Contract freeze | Frozen format, protocol, errors and decisions |
| 2 | Repository and build skeleton | Clean buildable project |
| 3 | Minimum viable engine | Durable PUT/GET/DEL/BATCH and basic recovery |
| 4 | Independent torture foundation | Oracle, subprocess control and first random kills |
| 5 | First integrated chaos gate | Real crashes during active operations |
| 6 | Correctness hardening | Concurrency, torn writes, quotas and error paths |
| 7 | Crash-safe compaction | Replacement protocol proven under kills |
| 8 | Targeted adversarial coverage | Recovery, compaction, disk-full and large workloads |
| 9 | Performance and scale | One-million-key recovery target |
| 10 | Proof and submission docs | Accurate diagram, scenario matrix, BREAKING and PROMPTS |
| 11 | Release candidate | Clean offline build and full validation |
| 12 | Demo rehearsal | Every member ready for the 15-minute audit |

## 4. Phase 0 — Source and rules audit

**Objective:** Understand what is mandatory before choosing implementation details.

### Work
- Read the official specification completely, including deliverables, scenarios, demo, scoring, FAQ and disqualification clauses.
- Extract all numeric limits and command requirements into `02_OFFICIAL_REQUIREMENTS.md`.
- Identify prohibited libraries and confirm the allowed dependency policy.
- Compare the supplied architectural blueprint with the Antigravity plan.
- Record conflicts as decisions, not silent assumptions.
- Assign one owner and one reviewer to every required document and code area.

### Exit criteria
- All team members can explain the five invariants.
- All members know the 32 official scenario IDs.
- Language choice and team split are recorded.
- No unresolved rule interpretation is being treated as fact.
- Prompt logging and Git history begin from the event start.

## 5. Phase 1 — Contract freeze

**Objective:** Prevent engine and torture implementations from diverging.

### Work
Freeze these items in a reviewed commit:
- 32-byte database header, magic, version, generation and header CRC.
- UABE frame header, payload length, sequence, operation count and payload CRC.
- PUT/DEL subrecord layout and byte order.
- Key/value/batch size limits and aggregate payload bound.
- Sequence-number policy and duplicate-key ordering.
- Batch atomicity boundary.
- Exact serve request/response grammar.
- Whether ACK includes durable sequence information.
- Oracle ordering rule for concurrent same-key writes.
- Error codes and stdout/stderr policy.
- `max_bytes` meaning, including header and compaction-temp policy.
- Empty-batch and delete-missing semantics.
- Supported OS/filesystem assumptions for flock and directory sync.

### Exit criteria
- Both teams approve the same spec commit.
- Go and Python golden fixtures agree byte-for-byte.
- No one is implementing against a draft protocol.
- Any later contract change requires a coordinated change across engine, torture parser, tests and docs.

## 6. Phase 2 — Repository and build skeleton

**Objective:** Ensure the project can be built and run offline from the start.

### Work
- Create repository directories and `.gitignore`.
- Add a minimal Go module and executable entry point.
- Add Python torture CLI with `--help`, mandatory `--seed`, and mode parsing.
- Add unit-test commands and one clean build command to README.
- Add CI or local scripts only if they work offline and do not introduce banned dependencies.
- Commit each working milestone separately.

### Exit criteria
- A clean clone builds the Go binary without network access.
- Python harness starts and validates arguments.
- `./kvdb --help` and `./torture --help` work.
- No database engine or prohibited storage dependency is present.
- The project can be run from repository root using documented commands.

## 7. Phase 3 — Minimum viable engine

**Objective:** Create a database that can be broken by the torture harness.

### Work
1. Implement header encode/decode and CRC32C.
2. Implement UABE frame encode/decode with strict bounds.
3. Implement full append loop using explicit offsets.
4. Implement PUT, GET and DEL.
5. Implement BATCH as one frame.
6. Implement KeyDir and ordered operation application.
7. Implement basic sequential recovery and tail truncation.
8. Implement sidecar process lock and in-process mutex.
9. Implement `serve` with exact request IDs and durable ACK.
10. Implement required one-shot CLI commands as soon as the core API is stable.

### Exit criteria
- Put/get/delete survives close and reopen.
- Empty values are distinct from missing keys.
- Batch is all-or-nothing.
- Invalid or incomplete final frame is not partially replayed.
- ACK is emitted only after successful sync and index publication.
- A manual SIGKILL test has been performed and recorded.

## 8. Phase 4 — Independent torture foundation

**Objective:** Build a trustworthy external verifier before adding sophisticated engine features.

### Work
- Implement deterministic operation generation.
- Implement append-only START/ACK/FAIL oracle journal.
- Implement subprocess startup and cleanup.
- Implement a single stdin writer lock.
- Implement one stdout dispatcher keyed by request ID.
- Implement continuous stderr draining.
- Implement genuine SIGKILL with seeded millisecond delay while writers are active.
- Implement restart, check, dump and independent comparison.
- Preserve first-failure artifacts.

### Exit criteria
- The harness can test a deliberately correct toy child and detect a deliberately broken one.
- It can route concurrent responses correctly.
- It distinguishes ACKed from unresolved operations.
- It does not use engine internals to compute expected state.
- At least two concurrent writer workers are active during kill tests.

## 9. Phase 5 — First integrated chaos gate

**Objective:** Put the engine under real process-kill pressure early.

### Work
- Run default mixed PUT/DEL/BATCH workloads.
- Use multiple writers and a reader where supported.
- Kill the engine while requests are pending.
- Restart and verify every iteration.
- Reproduce each failure using saved seed and artifacts.
- Fix the first invariant violation before expanding workload complexity.

### Exit criteria
- At least 100 consecutive iterations complete without an invariant violation.
- The run includes kills with operations genuinely in flight.
- Every unresolved operation is treated as uncertain, not assumed absent.
- Batch checks reject partial visibility.
- No harness deadlocks or orphan child processes remain.

## 10. Phase 6 — Correctness hardening

**Objective:** Cover less common but high-cost failure boundaries.

### Work
- Inject short writes, EINTR, EIO, sync failures and truncate failures in tests.
- Verify tail-only repair and fail-closed behaviour for middle corruption.
- Test sequence validation, malformed frames and allocation bounds.
- Test same-key concurrent writers and readers during batches.
- Test permanent sidecar flock across multiple processes.
- Test `max_bytes` boundary and real ENOSPC where practical.
- Ensure poisoned-engine behaviour after uncertain I/O.
- Confirm every error is returned without false ACK.

### Exit criteria
- No KeyDir update occurs before durability.
- Failed batches do not expose a subset.
- No process can concurrently own the same database path.
- Error paths do not append at uncertain offsets.
- Disk-full and I/O failures preserve all previously acknowledged state.

## 11. Phase 7 — Crash-safe compaction

**Objective:** Shrink the log without losing or resurrecting keys.

### Work
- Implement same-directory temp creation.
- Write valid replacement header and live records.
- Sync and close temp.
- Rename temp over the database.
- Sync parent directory.
- Swap descriptors and KeyDir under the writer lock.
- Define startup handling for orphan temp files and generation.
- Add deterministic test barriers around every swap step.

### Exit criteria
- Compaction preserves exact logical state.
- File shrinks after delete-heavy workload.
- Kill before rename leaves old file authoritative.
- Kill after rename leaves complete replacement authoritative.
- Repeated open after interrupted compaction succeeds.
- No stale locator or descriptor is used after swap.

## 12. Phase 8 — Targeted adversarial coverage

**Objective:** Prove behaviour beyond ordinary random workload.

Run and record separate seeded runs for:
- Maximum-size values.
- 10,000-operation batches.
- Concurrent writers and same-key contention.
- Reader visibility during batch.
- Recovery killed during replay.
- Recovery killed during tail truncation.
- Compaction killed at each barrier.
- Disk-full on PUT, BATCH and compaction.
- One million keys.
- One-byte final-tail truncation.

### Exit criteria
- Each mandatory mode has a documented command and result.
- All 32 official rows have an accurate status.
- Unverified rows are explicitly `NOT VERIFIED`.
- Failure artifacts can be replayed or inspected.
- No test mode bypasses the independent oracle.

## 13. Phase 9 — Performance and scale

**Objective:** Meet the official one-million-key recovery threshold without weakening correctness.

### Work
- Build a deterministic million-key database.
- Measure recovery on the actual judging laptop and filesystem.
- Record Go version, OS, filesystem, bytes scanned, frame count, key count, duration and memory.
- Profile allocations and map growth.
- Optimize only measured bottlenecks.
- Re-run all correctness tests after each optimization.

### Exit criteria
- Recovery is under two seconds for one million keys on the target machine.
- `check` and logical dump agree with expected state.
- Compaction can shrink the delete-heavy database.
- Benchmark report includes repeat count, median and maximum.
- No stretch target is presented as an official requirement.

## 14. Phase 10 — Proof and submission documentation

**Objective:** Make the implementation defensible under judge spot checks.

### Work
- Draw the actual architecture and on-disk layout.
- Number exact write/sync/ACK and compaction ordering.
- Complete all 32 scenario-to-mechanism rows.
- Name source file and function for every mechanism.
- Add real test mode and seed or mark `not verified`.
- Write the three durability, atomicity and recoverability arguments.
- Complete all five BREAKING.md sections within the page limit.
- Complete PROMPTS.md chronologically and honestly.
- Ensure README has one clean build command.

### Exit criteria
- Diagram matches the current source.
- Randomly selected proof rows can be traced in code.
- No row claims an unimplemented mechanism.
- Every team member can explain recovery.
- Prompt log and Git history are complete.

## 15. Phase 11 — Release candidate

**Objective:** Freeze a build that can be demonstrated offline.

### Work
- Stop feature work.
- Clone into a new directory.
- Disable Wi-Fi.
- Run the exact README build command.
- Run all eight CLI commands.
- Run default 500-iteration torture command.
- Run targeted modes with fresh seeds.
- Run `check`, `dump`, compaction and million-key benchmark.
- Verify final commit and archive contents.

### Exit criteria
- No missing executable or runtime dependency.
- No harness crash.
- No known invariant violation hidden from judges.
- All known failures are documented honestly.
- Final commit is pushed before deadline.

## 16. Phase 12 — Demo rehearsal

**Objective:** Prepare the team for the judge-controlled 15-minute session.

### Work
- Rehearse clean build in one minute.
- Rehearse 200-iteration torture run.
- Rehearse compaction and recovery-crash modes.
- Rehearse manual serve-mode kills.
- Ask every member three crash-here questions.
- Rehearse TTL extension without coding.
- Keep Wi-Fi off and AI/internet tools unavailable during the demo.

### Exit criteria
- Every member can explain exact disk state at three crash points.
- Every member knows ACK ordering and recovery behaviour.
- Commands work from a clean terminal.
- The diagram and source are aligned.
- The team can identify known limitations without guessing.

## 17. Change-control rules between phases

- A failed invariant blocks progression to the next correctness phase.
- A format change reopens Phase 1 and requires both teams to update.
- A protocol change requires oracle and integration tests.
- A compaction change requires all compaction crash tests again.
- A recovery change requires all recovery-crash and tail tests again.
- A performance optimization requires the full relevant correctness suite again.
- Documentation status changes only after evidence exists.

## 18. Final phase status table

Maintain this table in the project during the event.

| Phase | Owner | Status | Evidence / commit | Blocker |
|---|---|---|---|---|
| 0 Source audit | Whole team | NOT STARTED | — | — |
| 1 Contract freeze | Whole team | NOT STARTED | — | — |
| 2 Build skeleton | Integration | NOT STARTED | — | — |
| 3 MVP engine | Engine | NOT STARTED | — | — |
| 4 Torture foundation | Torture | NOT STARTED | — | — |
| 5 Chaos gate | Both | NOT STARTED | — | — |
| 6 Hardening | Both | NOT STARTED | — | — |
| 7 Compaction | Engine | NOT STARTED | — | — |
| 8 Targeted modes | Torture | NOT STARTED | — | — |
| 9 Performance | Engine | NOT STARTED | — | — |
| 10 Proof docs | Whole team | NOT STARTED | — | — |
| 11 Release candidate | Integration | NOT STARTED | — | — |
| 12 Demo rehearsal | Whole team | NOT STARTED | — | — |

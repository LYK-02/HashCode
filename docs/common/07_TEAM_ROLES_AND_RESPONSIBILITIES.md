# Team Roles and Responsibilities

> Status: **Specification / implementation target**  
> Owner: common team  
> Canonical filename: `07_TEAM_ROLES_AND_RESPONSIBILITIES.md`

## 1. Purpose and scope

Defines two parallel teams, individual ownership, shared contracts, and integration rules.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Team A owns the Go engine, library/API, binary format implementation, CLI, and durability. Team B owns the Python torture executable, independent oracle, process control, scenario execution, evidence capture, and proof documentation. The teams integrate only through the frozen CLI/serve protocol and committed format specification. The oracle must remain independent of engine internals.

## 4. Design and procedure

Engineer 1 — Framing and write path: owns header/frame encoder, CRC32C, checked serialization, quota precheck, full `WriteAt` loop, sync-before-index, ACK emission, and codec tests. Deliverables: stable format functions and a write-path syscall sequence that can be pointed to in source.

Engineer 2 — Recovery, compaction and concurrency: owns open/lock lifecycle, header validation, bounded scanner, tail truncation and sync, KeyDir reconstruction, `get`, compaction swap, temp cleanup, and concurrency tests. Deliverables: idempotent recovery and crash-safe compaction.

## 5. Edge cases and failure behaviour

Engineer 3 — Chaos controller and oracle: owns Python CLI parser, subprocess lifecycle, request dispatcher, independent event log, operation model, seed handling, random kill scheduler, restart/check/dump comparison, and reproducibility. Deliverables: trustworthy pass/fail evidence and actionable failure artifacts.

Engineer 4 — Adversarial modes and proof: owns large-value, large-batch, compaction, recovery-crash and disk-full modes; all scenario mapping; `ARCHITECTURE.md`, `BREAKING.md`, `PROMPTS.md` completeness audit; and demo rehearsal. Deliverables: evidence-backed documentation, not speculative claims.

## 6. Verification and acceptance tests

For a two-person team, combine Engineer 1+2 as Engine and Engineer 3+4 as Torture/Proof; keep the same interfaces. For three people, one engine owner, one torture/oracle owner, one integration/docs owner. Every file has one DRI and one reviewer. Cross-review must include at least one engineer from the other team.

Shared responsibilities: both teams jointly approve protocol/format changes; all members review invariant failures; all members understand recovery; no one merges an untested persistence change; and no one writes a test that trusts engine output as its own expected state.

Additional implementation notes:

Use small PRs or commits with a single concern. A commit message should state behaviour and test command. No last-minute bulk commit. Any protocol change requires a versioned spec update, engine and Python parser update, compatibility tests, and a joint sign-off. A failed integration test blocks merge.

Daily/continuous handoff format: changed files; interface change; tests run; known failures; exact reproduction command/seed; next action; commit hash. Never hand off only a verbal summary.

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

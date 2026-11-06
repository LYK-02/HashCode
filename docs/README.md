# Crash-Safe Key-Value Engine — Documentation Index

This folder contains 45 detailed Markdown specifications for the two-track implementation.

## Structure

- `common/` — project rules, architecture, shared format/protocol, team workflow, official scenarios, validation, demo, judge Q&A.
- `engine/` — Go storage engine implementation and verification specifications.
- `torture_tool/` — Python adversarial test harness, independent oracle, crash injection, modes, and reporting.

## Read first

1. `common/02_OFFICIAL_REQUIREMENTS.md`
2. `common/04_SHARED_DATA_FORMAT.md`
3. `common/05_CLI_AND_PROTOCOL_SPEC.md`
4. `common/03_SYSTEM_ARCHITECTURE.md`
5. `common/06_IMPLEMENTATION_ROADMAP.md`
6. `common/07_TEAM_ROLES_AND_RESPONSIBILITIES.md`

## Important implementation decision

The official rules allow any language. The supplied architectural blueprint recommends Rust for the engine, while the later Antigravity Final Plan specifies Go for the engine and Python for torture. This documentation chooses **Go + Python** to keep one coherent implementation plan aligned with the latest Antigravity plan. Do not mix Go and Rust implementation instructions.

## Critical caveats

- UABE is the selected one-frame atomic batch design: no separate commit marker is used.
- The header CRC protects frame lengths before allocation; the payload CRC protects the complete operation payload.
- ACK must follow successful file sync and complete in-memory index publication.
- Compaction requires temp-file sync, close, rename, and parent-directory sync.
- A permanent sidecar lock file must never be unlinked or replaced.
- An invalid header must not trigger blind destructive reinitialization if acknowledged data could exist.
- All 32 official scenarios must be marked with real evidence or honestly marked `NOT VERIFIED`.
- The official recovery threshold is **under 2 seconds for 1,000,000 keys**. Faster targets are stretch goals only.

## Status vocabulary

`NOT STARTED` → `IMPLEMENTED` → `TESTED` → `PASS`. Use `FAIL` for a reproduced defect and `NOT VERIFIED` when there is no adequate test evidence. Never use `PASS` based only on design reasoning.

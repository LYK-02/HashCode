# BRIEFING — 2026-11-06T11:47:00Z

## Mission
Mine and document all competition rules, invariants, crash scenarios (Groups A-F, 32 scenarios), deliverables, banned library constraints, and code quality mandates from `.agents/rules/` and project root.

## 🔒 My Identity
- Archetype: Specification Miner
- Roles: Rule & Specification Extraction, Verification Mapping
- Working directory: /home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_survey_2
- Original parent: 9fade4e7-7ff3-4d78-9ecd-6f9b6c00f5be
- Milestone: Hackathon Rules, Invariants & Scenarios Specification Mining

## 🔒 Key Constraints
- Read-only: Do NOT implement or modify any source code.
- Zero third-party storage engines/WALs (standard library only).
- The Core Invariant: Acknowledged writes 100% durable across kill -9, atomic batches, <= 2.0s recovery for 1,000,000 keys.
- Complete 32 crash scenarios matrix (Groups A through F).
- All deliverables and CLI commands must be mapped.
- Write only to own directory (.agents/teamwork/spec_miner_survey_2/).

## Current Parent
- Conversation ID: 9fade4e7-7ff3-4d78-9ecd-6f9b6c00f5be
- Updated: 2026-11-06T11:47:00Z

## Task Summary
- **What to build**: Comprehensive rules and invariants specification report in `handoff.md`.
- **Success criteria**: Exhaustive mapping of all 32 crash scenarios, invariants, deliverables, banned libraries, and code quality rules with tables for Features Discovered and Edge Cases.
- **Interface contracts**: PROJECT.md / rules files in `.agents/rules/`
- **Code layout**: Rules in `.agents/rules/`, project docs in root.

## Key Decisions Made
- Fully surveyed all 10 rule modules in `.agents/rules/`, `AGENTS.md`, `ORIGINAL_REQUEST.md`, `ARCHITECTURE.md`, `BREAKING.md`, `DEMO.md`, `CRASH_QUESTIONS.md`, `DESIGN_EXTENSION.md`, `README.md`, and `docs/common/`.
- Mapped all 32 crash scenarios across Groups A to F with triggers, physical disk states, detection mechanisms, and verification commands.
- Extracted write ordering protocol, UABE binary frame model, sidecar locking, zero-panic mandate, and 100-point scoring rubric.
- Completed comprehensive 5-component handoff report at `/home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_survey_2/handoff.md`.

## Artifact Index
- /home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_survey_2/DISPATCH.md — Dispatch assignment
- /home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_survey_2/BRIEFING.md — Situational awareness
- /home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_survey_2/progress.md — Progress heartbeat
- /home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_survey_2/handoff.md — Final specification report

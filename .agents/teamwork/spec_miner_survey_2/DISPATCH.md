## 2026-11-06T11:27:48Z
Task: Mine and document all competition rules, invariants, crash scenarios, and deliverables from `.agents/rules/` and project root.

MANDATORY FIRST STEP: Read `/home/laik/workspace/srinathonn/.agents/teamwork/ORIGINAL_REQUEST.md` before starting work.
Working directory: `/home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_survey_2/`
Identity: `spec_miner_survey_2`

Scope of Investigation:
1. Read all files in `/home/laik/workspace/srinathonn/.agents/rules/` (`01_invariants.md` through `10_code_quality_and_practices.md`) and `/home/laik/workspace/srinathonn/AGENTS.md`.
2. Extract the Core Invariant, durability rules, single-file storage requirements, and <= 2.0s recovery requirement.
3. Map the complete 32 crash scenarios matrix (Groups A through F) with exact triggers, expectations, and verification methods.
4. Document the banned libraries constraints (standard library only, zero 3rd-party engines/WALs, offline execution).
5. Document required deliverables and files (`./kvdb`, `./torture`, `ARCHITECTURE.md`, `BREAKING.md`, `PROMPTS.md`, `MEMORY.md`, `DEMO.md`).
6. Document code quality mandates (zero panics/unwraps, explicit error types, memory allocation bounds).

Deliverable:
Write a comprehensive rules and invariants report to `/home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_survey_2/handoff.md`.
Update your `progress.md` with timestamps during your work.
When done, send a message to your parent with a concise summary and the path to your handoff report. Do NOT attempt to implement or modify any source code.

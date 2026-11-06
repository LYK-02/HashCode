Continue implementation and verification of the Go storage engine and judge-facing CLI (`./kvdb`) from where it left off (after Milestone M2, starting at Milestone M3) through all remaining milestones (M3, M4, M5, and M_FINAL) for evaluation and benchmarking, strictly following `docs/` and `.agents/rules/`.

Working directory: c:\Users\LENOVO\HashCode-1
Integrity mode: demo
The team must NOT: copy code from existing open-source projects or database engines for core logic; use pre-built libraries, frameworks, or external persistence engines/WALs for core functionality (standard library only); run external scripts or delegate execution to other tools.

## Requirements

### R1. Milestone Implementation (M3 through M5 and Release Qualification)
Continue the Go storage engine and CLI implementation according to `docs/` and `.agents/teamwork/PROJECT.md`:
- Milestone M3: Atomic Batch Mutation & Crash Recovery (Phases E4 & E5: 1..10,000 ops atomicity in single UABE frame; sequential log replay from byte 32 to valid EOF; torn tail repair and truncation at EOF; fail-closed rejection of non-tail corruption with `CORRUPT_FRAME`).
- Milestone M4: Judge CLI, Protocol & Error Hardening (Phases E6 & E7: 8 subcommands `open`, `put`, `get`, `del`, `batch`, `dump`, `check`, `serve` with streaming protocol; logical quota checks; short-write rollback; zero panics).
- Milestone M5: Crash-Safe Compaction, Scale & Matrix Hardening (Phases E8, E9 & E10: crash-safe compaction via `<path>.compact.tmp` and directory sync; sub-2.0s recovery at 1,000,000 keys; 32 crash scenarios matrix compliance in Groups A through F).
- Milestone M_FINAL: End-to-end acceptance testing and release qualification.

### R2. Branch, Pull Request, and Local Merge Protocol
For each task or milestone completed:
- Create a dedicated local git feature branch.
- Verify that all unit, integration, and milestone exit gate tests pass.
- Merge the feature branch into `main` locally.
- Do NOT push commits to the remote origin repository without explicit user permission. When all tasks are completed, prompt the user for permission before running `git push`.

### R3. Project Memory and Prompt Log Maintenance
After each successful task completion, update `MEMORY.md` and `PROMPTS.md` in the repository root adhering to the project logging rules (`.agents/rules/05_prompt_logging.md` and `.agents/rules/07_memory_protocol.md`).

## Acceptance Criteria

### Milestone Delivery & Engine Invariants
- [ ] Milestone M3 Gate: Atomic batch mutation guarantees all-or-nothing execution; startup recovery replays valid frames, repairs torn tail frames at EOF, and fails closed on non-tail corruption.
- [ ] Milestone M4 Gate: All 8 CLI commands (`open`, `put`, `get`, `del`, `batch`, `dump`, `check`, `serve`) function per specification; quota errors trigger `DISK_FULL` without state corruption; zero unhandled panics.
- [ ] Milestone M5 & Release Gate: Crash-safe compaction reclaims space without data loss; 1,000,000 keys recover in $\le 2.0$ seconds; passes the 32 crash scenarios matrix using standard library only.

### Workflow & Logging Gates
- [ ] A dedicated local git feature branch is created and merged into `main` locally upon each task/milestone completion.
- [ ] `MEMORY.md` and `PROMPTS.md` are updated after each completed task following repo rules.
- [ ] No `git push` command is executed to the remote origin without explicit user permission.
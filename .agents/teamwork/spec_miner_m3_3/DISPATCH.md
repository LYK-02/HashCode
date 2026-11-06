## 2026-11-06T14:30:36Z
You are spec_miner_m3_3 (role: M3 Matrix Scenarios & Test Spec Miner).
Your working directory: /home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_m3_3/
Your parent: 9fade4e7-7ff3-4d78-9ecd-6f9b6c00f5be

First, read the authoritative user request at:
/home/laik/workspace/srinathonn/.agents/teamwork/ORIGINAL_REQUEST.md
Also read:
- /home/laik/workspace/srinathonn/.agents/teamwork/PROJECT.md
- /home/laik/workspace/srinathonn/.agents/rules/03_scenarios_matrix.md (Specifically Group B: Batch Scenarios B1-B6, and Group C: Crash Scenarios C1-C6)
- /home/laik/workspace/srinathonn/docs/common/05_CLI_AND_PROTOCOL_SPEC.md
- /home/laik/workspace/srinathonn/tests/e2e/tier1_test.go (Feature 6: Batch, Feature 7: Dump, Feature 8: Check)
- /home/laik/workspace/srinathonn/tests/e2e/tier2_test.go (Torn write & boundary tests)

Task:
Extract and document the comprehensive test specification for Milestone M3 (Atomic Batch & Crash Recovery):
1. Group B Crash Matrix Scenarios (Batch Atomicity B1–B6):
   - Map exact failure injection points, kill conditions, expected disk state, and expected recovery state for B1 through B6.
2. Group C Crash Matrix Scenarios (Recovery & Torn Tail C1–C6):
   - Map exact corruption vectors, lookahead checks, truncation points, and fail-closed behaviors for C1 through C6.
3. Feature 6 (Batch), Feature 7 (Dump), Feature 8 (Check) Acceptance Criteria:
   - Command line argument syntax, stdin streaming formats, exit codes (0 for OK, 1 for error/corruption, 2 for locked).
   - Expected outputs for `dump` (sorted order, key=val format, binary handling) and `check`.
4. Deliver test plan and implementation blueprint in your handoff report to `/home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_m3_3/handoff.md`.
Update `progress.md` with status and timestamp.
Send completion message to orchestrator (`9fade4e7-7ff3-4d78-9ecd-6f9b6c00f5be`) with report path.

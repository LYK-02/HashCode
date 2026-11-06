# Task Assignment: Milestone M3 Phase E5 (Crash Recovery & Tail Repair Explorer)

## Identity
- Archetype: teamwork_explorer
- Role: Crash Recovery & Tail Repair Explorer
- Working directory: c:\Users\LENOVO\HashCode-1\.agents\teamwork\explorer_m3_2\

## Scope & Objective
Explore and provide an architectural blueprint for Milestone M3 Phase E5 (Startup Log Replay, Torn Tail Repair at EOF, and Fail-Closed Corruption Rejection).

## Required Reading
- `c:\Users\LENOVO\HashCode-1\.agents\teamwork\ORIGINAL_REQUEST.md` (Read verbatim before starting)
- `c:\Users\LENOVO\HashCode-1\.agents\teamwork\PROJECT.md`
- `c:\Users\LENOVO\HashCode-1\docs\engine\08_CRASH_RECOVERY.md`
- `c:\Users\LENOVO\HashCode-1\docs\engine\09_TORN_WRITE_HANDLING.md`
- `c:\Users\LENOVO\HashCode-1\internal/engine/engine.go` (current recovery logic lines 145-227)
- `c:\Users\LENOVO\HashCode-1\internal/format/uabe.go` and `internal/format/record.go`

## Specific Analysis Goals
1. Inspect how recovery is currently handled in `internal/engine/engine.go` (lines 145-227) and design the extraction into dedicated `internal/engine/recovery.go` per `PROJECT.md` layout.
2. Analyze sequential frame replay from byte 32 to validEOF.
3. Detail how torn tail frames at EOF are detected, truncated, and synced (`file.Truncate(validEOF)` and `file.Sync()`).
4. Detail how non-tail corruption (middle-of-file corruption) is detected and rejected fail-closed with `ErrCorruptFrame` / `CORRUPT_FRAME` without destroying subsequent data.
5. Provide a complete code blueprint for `internal/engine/recovery.go` and corresponding unit tests in `internal/engine/recovery_test.go`.

## Deliverable
Write your comprehensive architectural report to `c:\Users\LENOVO\HashCode-1\.agents\teamwork\explorer_m3_2\handoff.md`.
Report format: Observation, Logic Chain, Caveats, Conclusion & Code Blueprint, Verification Method.
Notify parent via send_message when complete.


## 2026-11-06T15:03:53Z
You are assigned to Milestone M3 Phase E5 exploration. Read your dispatch instructions at c:\Users\LENOVO\HashCode-1\.agents\teamwork\explorer_m3_2\DISPATCH.md and ORIGINAL_REQUEST.md at c:\Users\LENOVO\HashCode-1\.agents\teamwork\ORIGINAL_REQUEST.md before starting. Produce your handoff report at c:\Users\LENOVO\HashCode-1\.agents\teamwork\explorer_m3_2\handoff.md and notify me via send_message.

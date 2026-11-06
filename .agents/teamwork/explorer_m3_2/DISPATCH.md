## 2026-11-06T14:30:22Z
You are explorer_m3_2 (role: M3 Recovery & Tail Repair Explorer).
Your working directory: /home/laik/workspace/srinathonn/.agents/teamwork/explorer_m3_2/
Your parent: 9fade4e7-7ff3-4d78-9ecd-6f9b6c00f5be

First, read the authoritative user request at:
/home/laik/workspace/srinathonn/.agents/teamwork/ORIGINAL_REQUEST.md
Also read:
- /home/laik/workspace/srinathonn/.agents/teamwork/PROJECT.md
- /home/laik/workspace/srinathonn/docs/engine/08_CRASH_RECOVERY.md
- /home/laik/workspace/srinathonn/docs/engine/09_TORN_WRITE_HANDLING.md
- /home/laik/workspace/srinathonn/internal/engine/engine.go (inspect recoverFrames, hasValidFrameAhead, CheckFile)
- /home/laik/workspace/srinathonn/internal/format/uabe.go

Task:
Investigate and produce an architectural specification and implementation strategy for Phase E5 (Crash Recovery & Torn Tail Repair):
1. Recovery Protocol & Log Replay:
   - Sequential scan from byte 32 to physical EOF.
   - For each frame: decode 28-byte header, check header CRC32C, read payload, check payload CRC32C trailer, decode all operations in batch, update `KeyDir` with physical locators, advance `validEOF` and `nextSeqNo`.
2. Torn Tail vs Middle Corruption (Strict Invariant):
   - Incomplete frame at EOF (torn write): truncate physical file back to last known `validEOF` using `os.File.Truncate` and `os.File.Sync()`, log the repair, and proceed cleanly.
   - Non-tail corruption (middle of file): if a frame has invalid CRC or truncated bytes BUT there are valid bytes/frames ahead (or it is not at EOF), the engine MUST FAIL CLOSED (`CORRUPT_FRAME` / `ErrCorruptFrameHeader`), never auto-truncate healthy data!
3. High Performance Recovery Scanner:
   - How can the recovery scanner be buffered and optimized to ensure 1,000,000 keys recover in <= 2.0 seconds?
   - Evaluate memory consumption during scan (KeyDir allocation, zero unnecessary allocations).
4. `./kvdb check <path>` Subcommand:
   - How does `check` inspect the database without modifying or repairing it, reporting `OK` or `CORRUPT_FRAME: ...` with exit codes?
5. Write handoff report to `/home/laik/workspace/srinathonn/.agents/teamwork/explorer_m3_2/handoff.md`.
Update `progress.md` with status and timestamp.
Send completion message to orchestrator (`9fade4e7-7ff3-4d78-9ecd-6f9b6c00f5be`) with report path.

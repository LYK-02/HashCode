# BRIEFING — 2026-11-06T15:10:00Z

## Mission
Investigate and design Milestone M3 Phase E5: Startup Log Replay, Torn Tail Repair at EOF, and Fail-Closed Middle Corruption Rejection for `internal/engine/recovery.go` and `internal/engine/engine.go`.

## 🔒 My Identity
- Archetype: explorer
- Roles: Crash Recovery & Tail Repair Explorer
- Working directory: /home/laik/workspace/srinathonn/.agents/teamwork/explorer_m3_2
- Original parent: 042e55c7-ac0b-4cdf-893f-70cfc8f0e343
- Milestone: M3 Phase E5 (Startup Log Replay & Recovery)
- Active Workspace: c:\Users\LENOVO\HashCode-1\.agents\teamwork\explorer_m3_2\
- Subagent Conversation ID: 41460909-4447-488d-9c59-cfe9aee22dc1

## 🔒 Key Constraints
- Read-only investigation — do NOT implement code changes
- Write reports and metadata ONLY to working directory `/home/laik/workspace/srinathonn/.agents/teamwork/explorer_m3_2/`
- Strict alignment with Project Invariants: The Core Invariant, zero banned libraries, single-file append-only log, <=2.0s recovery.
- Scenarios matrix Groups A & B compliance.
- No direct implementation in internal/engine/ until milestone plan execution.

## Current Parent
- Conversation ID: 493c36a2-c491-4951-8db4-de91a267fdf4
- Updated: 2026-11-06T15:10:00Z

## Investigation State
- **Explored paths**:
  - `internal/engine/engine.go`: analyzed inline recovery loop (lines 145-227), `hasValidFrameAhead` (lines 346-357), and `CheckFile`.
  - `internal/engine/lock.go`: POSIX `flock` sidecar lifecycle; confirmed Linux compilation with `GOOS=linux`.
  - `internal/format/{header,uabe,record,errors}.go`: UABE 28-byte frame header, payload records, 4-byte Castagnoli CRC trailer, format errors.
  - `tests/e2e/{tier1,tier2,tier3,tier4}_test.go`: verified existing test coverage for torn tail repair (Cases 36-40), fail-closed middle corruption (Cases 46-50), and 1M scale recovery benchmark.
  - `internal/engine/challenger_durability_test.go`: verified Harness 3 (Torn Tail at EOF) and Harness 4 (Fail-Closed Middle Corruption).
  - Specifications: `docs/engine/08_CRASH_RECOVERY.md`, `docs/engine/09_TORN_WRITE_HANDLING.md`, `docs/engine/07_BATCH_ATOMICITY.md`, `.agents/rules/03_scenarios_matrix.md`.
- **Key findings**:
  1. Modularity & Decoupling: Extract recovery logic from `engine.go` into `internal/engine/recovery.go` defining `RecoverLog(dataFile *os.File, fileSize uint64) (*RecoveryResult, error)` and `hasValidFrameAhead`.
  2. Torn Tail vs Middle Corruption Discrimination:
     - Tail condition 1: `offset + FrameHeaderSize > fileSize` -> strictly EOF torn tail -> truncate to `offset` and sync.
     - Tail condition 2: Corrupt frame header / invalid magic / CRC mismatch -> run `hasValidFrameAhead(dataFile, offset+1, fileSize)`. If true, fail closed (`ErrCorruptFrameHeader` / `ErrInvalidFrameMagic`). If false, truncate to `offset` and sync.
     - Tail condition 3: Header claims `offset + totalFrameSize > fileSize` -> if `hasValidFrameAhead`, fail closed (`ErrTruncatedFrame`). If false, truncate to `offset` and sync.
     - Tail condition 4: Payload corruption / trailer CRC mismatch -> if `offset + totalFrameSize < fileSize` or `hasValidFrameAhead`, fail closed (`ErrCorruptPayload`). If at exact EOF (`offset + totalFrameSize == fileSize`) and no valid frames ahead, truncate to `offset` and sync.
  3. Chunked Vectorized Scanning for `hasValidFrameAhead`:
     - Replaces slow byte-by-byte `ReadAt` syscall loop with 64 KiB chunked buffered reader using `bytes.Index(..., frameMagicBytes)` to achieve microsecond scanning across megabytes of corrupted space.
  4. Allocation & Performance Scaling:
     - Pre-estimate KeyDir capacity based on file size.
     - Single contiguous allocation for frame header and trailer.
     - Reusable payload buffer to eliminate 1,000,000 heap allocations during 1M key recovery.
  5. Idempotent Truncation & Stderr Diagnostics:
     - `repairTornTail` pairs `dataFile.Truncate` with immediate `dataFile.Sync()`, surviving kills during recovery (Scenario C2).
     - Diagnostic recovery logs output strictly to `os.Stderr`, preserving clean stdout for judge protocol.
- **Unexplored areas**: None for M3 Phase E5.
- Ready to produce comprehensive architectural handoff report.

## Key Decisions Made
- Recovery scanner extraction to `internal/engine/recovery.go`.
- Struct `RecoveryResult` to transfer reconstructed state cleanly to `Open()`.
- Chunked SIMD-accelerated `hasValidFrameAhead` algorithm using `bytes.Index`.
- Complete unit test suite design in `internal/engine/recovery_test.go`.

## Artifact Index
- DISPATCH.md — incoming dispatch instructions and task scope
- BRIEFING.md — persistent working memory
- progress.md — liveness heartbeat
- handoff.md — final 5-component handoff report and code blueprint

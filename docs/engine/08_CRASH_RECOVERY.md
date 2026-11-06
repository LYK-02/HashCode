# Crash Recovery

> Status: **IMPLEMENTED & VERIFIED (Milestone M3 / Phase E5)**  
> Owner: engine team  
> Canonical filename: `08_CRASH_RECOVERY.md`  
> Implementation: `internal/engine/recovery.go`, `internal/engine/engine.go`  
> Verification Suite: `internal/engine/recovery_test.go`, `internal/engine/challenger_m3_2_adversarial_test.go` (100% PASS)

## 1. Purpose and scope

Defines startup scanning, validation, replay, tail healing, and idempotence.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Recovery reconstructs the in-memory KeyDir from the authoritative log. Acquire the permanent sidecar flock before opening/scanning or removing temp artifacts. Open the data file read/write. Validate the 32-byte header and version. Start at offset 32 and scan frames sequentially until EOF or the first incomplete/invalid tail. `validEOF` is the end offset of the last fully validated frame.

## 4. Design and procedure

For each frame: read fixed 28-byte header; validate magic and header CRC; parse bounded payload length, sequence and op_count; verify total frame fits within file; read payload and 4-byte trailer; verify payload CRC; parse exactly op_count subrecords with bounds checks; validate monotonic sequence and all operation constraints; then apply operations to KeyDir in order. Never apply partial payload entries. Use buffered sequential reads or ReadAt with a large buffer; benchmark before micro-optimizing.

## 5. Edge cases and failure behaviour

At the first invalid/incomplete trailing frame, truncate file to the frame's start offset and sync the file. A one-byte truncation at EOF therefore discards only the incomplete last frame. If recovery itself is killed during truncation, the next open repeats the scan and truncates again; recovery must be idempotent. Do not advance `validEOF` until repair sync succeeds.

## 6. Verification and acceptance tests

A valid frame that was synced but whose ACK was not observed may be replayed. This is permitted by the official in-flight rule. A frame that was never fully synced may or may not appear after process death; if present and valid, it may be replayed. The oracle must model this ambiguity for in-flight operations.

Additional implementation notes:

Do not silently recreate a file with an invalid header if it might contain acknowledged data. For initial file creation, sync the 32-byte header and parent directory before accepting any mutation. A crash before that point cannot have a successful mutation ACK. For later unexpected header corruption, preserve the bytes and fail closed unless a proven self-repair format exists; do not destroy the only authoritative copy.

Performance target is under two seconds for one million keys on judging hardware. Preallocate the map based on expected key count where possible; use bounded buffers; avoid per-byte allocations; avoid repeated string conversions; and measure real recovery. Record key count, bytes scanned, frame count, wall time, CPU time, and memory. Correctness is mandatory; the blueprint's <0.4s is a stretch target, not the official threshold.

Tests: empty file creation kill; partial header; valid header; each frame truncation boundary; header CRC; payload CRC; bad length; bad op count; invalid sequence; recovery kill during replay; kill during truncation; repeated recovery; one-byte truncation; and one-million-key timing.

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

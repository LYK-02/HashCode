# Disk Full and Error Handling

> Status: **Specification / implementation target**  
> Owner: engine team  
> Canonical filename: `12_DISK_FULL_AND_ERROR_HANDLING.md`

## 1. Purpose and scope

Specifies quota enforcement, real ENOSPC, I/O failures, and poisoned-engine policy.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

The engine accepts `max_bytes` at open. It is a hard logical cap on the authoritative data file unless a different interpretation is explicitly documented. Count the 32-byte header and every frame. Before append, compute frame size and check overflow-safe `validEOF + frameSize <= maxBytes`. Reject before any disk mutation with a stable `DISK_FULL` error.

## 4. Design and procedure

A logical quota check does not prevent physical ENOSPC, quota exhaustion, EIO, permission errors, or filesystem failure. Every write, sync, truncate, rename, directory sync, open, and close error must be checked. A partial batch must never become visible. The engine must not update KeyDir or ACK when any durability step fails.

## 5. Edge cases and failure behaviour

If an append partially writes then fails, truncate to last synced EOF and sync. If sync fails, treat file contents as uncertain; do not blindly retry the sync and assume success. Re-establish known state with rollback and sync, or poison the engine and require close/reopen/recovery. If rollback itself fails, do not continue writes. Return a stable error code and preserve diagnostic details on stderr.

## 6. Verification and acceptance tests

For a batch, reserve/check the full encoded frame before writing. Do not stream subrecords to disk one by one under a quota because that can create partial batch state. For compaction, old database remains authoritative while temp is being built; temp ENOSPC must abort compaction without touching old data. After successful compaction, the authoritative file may be smaller and subsequent writes can fit again.

Additional implementation notes:

The torture tool should test max_bytes just below a frame, exactly at boundary, and just above; actual disk-full can be simulated with a tiny filesystem only if available, but the official configurable cap is mandatory. E1–E5 must be verified. Include a process kill after quota rejection and verify all previously ACKed data.

Error taxonomy: `INVALID_ARGUMENT`, `KEY_TOO_LARGE`, `VALUE_TOO_LARGE`, `BATCH_TOO_LARGE`, `DISK_FULL`, `IO_WRITE`, `IO_SYNC`, `IO_TRUNCATE`, `IO_RENAME`, `IO_DIR_SYNC`, `CORRUPT_HEADER`, `CORRUPT_FRAME`, `LOCKED`, `CLOSED`, `ENGINE_POISONED`. Do not expose stack traces or multiline diagnostics on serve stdout.

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

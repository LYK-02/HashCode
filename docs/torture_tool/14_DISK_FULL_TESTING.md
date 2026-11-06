# Disk Full Testing

> Status: **Specification / implementation target**  
> Owner: torture team  
> Canonical filename: `14_DISK_FULL_TESTING.md`

## 1. Purpose and scope

Defines quota-based exhaustion, real I/O errors, and compaction under constrained space.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Disk-full mode uses the engine's `--max-bytes` cap to simulate a full logical database. Start with a small cap, write acknowledged baseline data, then generate mutations until the next frame would exceed the cap. The engine must return `ERR id DISK_FULL` without modifying KeyDir or the authoritative file.

## 4. Design and procedure

Test boundary values: max_bytes smaller than header; exactly header size; one byte below required frame; exact final frame fit; one byte above; and a batch whose aggregate frame exceeds cap. For mandatory operational tests, ensure the DB can initialize and baseline writes fit before triggering exhaustion. Do not use invalid cap values as a substitute for E1–E5.

## 5. Edge cases and failure behaviour

E1: cap during single PUT returns error and prior data readable. E2: batch that cannot fit is rejected wholly; none of its operations visible. E3: kill after cap reached and restart; all prior ACKed data intact. E4: compaction temp write may fail or complete; old or new full data remains valid. E5: deletes create obsolete log records; successful compaction shrinks file and subsequent writes succeed again.

## 6. Verification and acceptance tests

Also test actual ENOSPC/EIO if an isolated small filesystem is available, but do not make the required test depend on privileged setup. Validate that write/sync/truncate errors do not cause a false ACK or stale KeyDir. If rollback cannot be synced, the engine must poison itself and refuse further writes.

Additional implementation notes:

Report configured cap, header bytes, frame bytes, file size before/after, operation IDs, error codes, check/dump result, and whether compaction restored capacity. Verify no hidden retry turns a failed operation into an ACK.

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

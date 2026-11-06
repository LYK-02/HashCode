# Torn Write Handling

> Status: **Specification / implementation target**  
> Owner: engine team  
> Canonical filename: `09_TORN_WRITE_HANDLING.md`

## 1. Purpose and scope

Covers short writes, partial frames, malformed tails, sync errors, and safe rollback.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Never assume one `write`/`pwrite` call writes all bytes or that a syscall boundary is an atomic storage boundary. Implement a loop that advances by the number of bytes actually written; handle EINTR; treat zero-byte progress as an error. Use explicit offset arithmetic and never append at an offset that is not known durable.

## 4. Design and procedure

A frame has two independent validation layers: header CRC protects length/sequence/count before allocation; payload CRC protects the payload. Scanner first validates the fixed header, checks payload length against a strict upper bound and file size, then reads payload and trailer. A short header, short payload, missing trailer, CRC mismatch, invalid op, or inconsistent payload layout invalidates the entire frame.

## 5. Edge cases and failure behaviour

Tail policy: only a malformed suffix at EOF is treated as a torn write and truncated. If invalid data occurs before later bytes, do not blindly truncate all later data and claim acknowledged data is safe. Report middle-of-file corruption as an integrity error unless the format has a proven resynchronization scheme. The hackathon does not require random bit rot, but the engine must not mislabel arbitrary middle corruption as a harmless tail.

## 6. Verification and acceptance tests

On append error, rollback to `lastSyncedEOF` with truncate and sync. If the truncate or its sync fails, the engine state is uncertain; mark it poisoned and refuse further writes. The `fsyncgate` note in the supplied blueprint warns that a failed fsync may cause the kernel to forget dirty pages while leaving offsets advanced. A second fsync returning success is not proof that the original bytes survived. The only safe continuation is to re-establish the file length and sync the rollback, or close/reopen and fully recover before accepting writes.

Additional implementation notes:

Bounds must be checked before allocations and slices. Never allocate from unchecked `payload_len`; cap operation count; use checked `uint64` addition; check conversion to `int`; ensure offsets fit OS file APIs; and ensure key/value lengths fit the remaining payload. Reject overflows rather than wrapping.

Tests: force short writes with a test writer; truncate frame at every byte boundary for small fixtures; corrupt each header field; corrupt payload and trailer; simulate EINTR/EIO/ENOSPC; fail sync and truncate; verify no ACK, no KeyDir update, and safe subsequent open.

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

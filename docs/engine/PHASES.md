# Engine Implementation Phases — Go Storage Engine

> Owner: Engine Team  
> Language: Go  
> Primary output: `kvdb` executable and reusable engine package  
> Rule: do not mark a phase complete until its tests pass on the current commit.

## 1. Engine phase sequence

| Phase | Focus | Required result |
|---|---|---|
| E0 | Contract and package setup | Format/protocol frozen |
| E1 | Codec and file header | Byte-compatible encoder/decoder |
| E2 | Engine lifecycle and locks | Safe open/close ownership |
| E3 | Append and single operations | Durable PUT/GET/DEL |
| E4 | Atomic batch | One frame, all-or-none |
| E5 | Recovery | Validated replay and torn-tail repair |
| E6 | Serve and CLI | Judge-facing interface |
| E7 | Error and disk-full hardening | No false ACK or corrupt rollback |
| E8 | Compaction | Crash-safe replacement |
| E9 | Scale and performance | 1M-key open under 2 seconds |
| E10 | Release verification | All engine requirements demonstrated |

## 2. E0 — Contract and package setup

### Implement
- Read `common/04_SHARED_DATA_FORMAT.md` and `common/05_CLI_AND_PROTOCOL_SPEC.md`.
- Create Go module and package boundaries.
- Agree exact integer types, byte order, CRC32C coverage and error codes.
- Agree sequence-number start and increment policy.
- Agree whether ACK carries sequence number.
- Agree `max_bytes` semantics and target OS/filesystem.
- Add compile/test commands to README.

### Do not
- Invent a second frame format.
- Add a persistent index.
- Add a storage/database dependency.
- Begin compaction before append/recovery semantics are stable.

### Exit gate
- Go and Python golden-format fixtures match.
- Package builds offline.
- Design decisions are committed and reviewed.

## 3. E1 — Header and frame codec

### Implement in order
1. Encode 32-byte header.
2. Decode and validate magic/version/reserved fields.
3. Calculate and verify header CRC32C.
4. Encode 28-byte UABE header.
5. Encode PUT/DEL subrecords.
6. Calculate payload CRC32C and append trailer.
7. Decode with strict length and count limits.
8. Verify exact payload consumption.
9. Return a fully validated operation list before any application.

### Required defensive checks
- Checked integer addition and conversion.
- Payload length bounded before allocation.
- Key length 1–1,024 bytes.
- Value length 0–1 MiB.
- Operation count 1–10,000.
- DEL has zero value length.
- Unknown operation types rejected.
- Sequence order validated.
- No trailing payload bytes.

### Exit gate
- Golden hex fixtures match Python parser.
- Boundary and malformed-input tests pass.
- Fuzz tests do not panic or allocate unbounded memory.
- Codec has no filesystem side effects.

## 4. E2 — Engine lifecycle, lock ownership and file creation

### Implement
- Canonicalize database path.
- Open permanent `<db>.lock` sidecar.
- Acquire exclusive flock before recovery or temp cleanup.
- Open/create database file.
- On first creation, write header completely.
- Sync header and parent directory before accepting mutations.
- Initialize mutex, KeyDir, valid EOF, next sequence and generation.
- Define clean Close behaviour.
- Never unlink/replace the lock sidecar.

### Lifecycle rules
- No API call after Close.
- Lock order: process flock, then engine mutex.
- A second process must block or fail cleanly.
- SIGKILL must release OS lock automatically.
- Do not destructively reinitialize a potentially populated invalid-header file.

### Exit gate
- Two processes cannot own same database concurrently.
- Kill lock holder; a new opener acquires lock.
- Initial header survives close/reopen.
- Empty DB opens with correct state.

## 5. E3 — Durable append and PUT/GET/DEL

### Implement mutation path
1. Validate API input.
2. Build complete frame in memory.
3. Acquire exclusive mutex.
4. Check quota with overflow-safe arithmetic.
5. Append at `validEOF` using a full-write loop.
6. Handle short writes and EINTR.
7. Sync file.
8. Apply operation to KeyDir.
9. Advance EOF and sequence.
10. Release lock.
11. Let serve layer emit ACK.

### Implement operations
- PUT overwrites by appending a new frame.
- GET reads via ReadAt and returns a copy.
- GET distinguishes empty value from NOTFOUND.
- DEL appends tombstone and removes live index entry after sync.
- Delete missing key succeeds as required by official rules.

### Exit gate
- PUT/GET/DEL survive process restart.
- ACK only follows Sync and index update.
- Empty and binary values work.
- Same-key overwrite resolves to latest operation.
- A killed process never returns a mixed value.

## 6. E4 — Atomic batch

### Implement
- Validate every operation before disk mutation.
- Encode all subrecords into one UABE frame.
- Preserve operation order and duplicate-key semantics.
- Enforce 10,000-operation maximum.
- Check aggregate frame size before append.
- Append and sync one frame.
- Apply all operations while holding exclusive mutex.
- Do not expose intermediate KeyDir state.

### Required cases
- PUT then DEL same key.
- DEL then PUT same key.
- Duplicate key many times.
- Invalid first, middle or last operation.
- 10,000 operations.
- Batch too large for configured cap.
- Reader during batch.
- Kill during append and sync.

### Exit gate
- Recovery applies full batch or none.
- Reader sees pre-batch or post-batch state.
- No batch subset can appear.
- Every batch ACK corresponds to one durable frame.

## 7. E5 — Recovery and tail repair

### Implement
1. Acquire process lock.
2. Validate file header.
3. Set scan offset to byte 32.
4. Read fixed frame header.
5. Verify magic and header CRC.
6. Validate bounded payload length and operation count.
7. Check frame fits within file.
8. Read payload and trailer.
9. Verify payload CRC.
10. Parse all subrecords exactly.
11. Verify sequence.
12. Apply the entire frame to KeyDir.
13. Advance valid EOF.
14. At incomplete/invalid final frame, truncate to frame start.
15. Sync truncation before exposing engine.

### Important distinction
Only an invalid suffix at EOF is treated as a torn tail. Invalid bytes in the middle of the file must not cause silent loss of later data. Preserve the file and report integrity failure unless a proven resynchronization scheme exists.

### Exit gate
- Truncation at every byte boundary of a small fixture is handled.
- One-byte EOF truncation drops only final incomplete frame.
- Recovery killed during replay can restart.
- Recovery killed during truncation can restart.
- Repeated open produces identical logical state.

## 8. E6 — CLI and serve mode

### Implement
- `open`, `put`, `get`, `del`, `batch`, `dump`, `check`, `serve`.
- Exact usage and exit codes.
- Deterministic sorted dump.
- Binary-safe serve protocol.
- Request IDs and stable error codes.
- stdout protocol-only; diagnostics on stderr.
- Flush each ACK/ERR.
- Keep one engine instance open for serve lifetime.

### Exit gate
- Every command matches README examples.
- Judge can manually send a batch.
- No startup banner/debug line pollutes stdout.
- ACK response is not buffered indefinitely.
- All commands work offline from clean clone.

## 9. E7 — Error paths, quota and disk-full

### Implement
- Logical `max_bytes` precheck.
- Actual ENOSPC/EIO propagation.
- Rollback partial append to last known durable EOF.
- Sync rollback.
- Poison engine if rollback cannot be proven.
- Stable error codes.
- No KeyDir mutation on failure.
- No ACK on failed write/sync.
- Full-frame preflight for batch.

### Exit gate
- E1–E5 disk-full scenarios pass.
- Failed PUT/BATCH leaves previous state readable.
- Kill after quota rejection preserves all ACKed data.
- No subsequent write proceeds on uncertain file state.

## 10. E8 — Crash-safe compaction

### Implement
- Acquire writer lock.
- Create same-directory temp file.
- Write new header with generation increment.
- Iterate live KeyDir entries deterministically.
- Encode replacement frames.
- Sync temp.
- Close temp and check error.
- Rename temp over database.
- Sync parent directory.
- Swap database descriptor, KeyDir, EOF and sequence under lock.
- Handle orphan temp on next open.

### Required crash boundaries
- Before temp creation.
- During header and record writes.
- Before and during temp sync.
- After sync, before rename.
- During/after rename.
- Before/after directory sync.
- Before descriptor/index swap.
- During cleanup.

### Exit gate
- Old or new complete logical database survives every tested crash.
- No stale locator is returned.
- Compaction shrinks delete-heavy database.
- Subsequent writes work after successful compaction.
- Temp ENOSPC leaves old DB intact.

## 11. E9 — Performance and scale

### Implement and measure
- Preallocate KeyDir where practical.
- Use bounded reusable buffers.
- Avoid repeated allocations and copies in recovery.
- Scan sequentially.
- Avoid per-record logging in hot path.
- Profile before changing map or hash strategy.
- Measure one million keys on target laptop/filesystem.

### Exit gate
- `open` under two seconds at one million keys.
- Record median and maximum over repeated runs.
- `check` and dump hash prove correctness.
- Compaction and all crash tests still pass after optimization.

## 12. E10 — Engine release gate

- [ ] All API limits enforced.
- [ ] All eight CLI commands implemented.
- [ ] Header/frame golden tests pass.
- [ ] CRC32C is Castagnoli, not IEEE CRC32.
- [ ] Full write loops and checked arithmetic.
- [ ] ACK after Sync and KeyDir publication.
- [ ] Batch atomicity proven.
- [ ] Recovery is idempotent.
- [ ] Sidecar lock permanent.
- [ ] Disk-full safe.
- [ ] Compaction crash-safe.
- [ ] One-million-key recovery under two seconds.
- [ ] No banned dependency.
- [ ] Offline clean build succeeds.
- [ ] Engine documentation and diagram match source.

## 13. Engine phase status

| Phase | Status | Commit | Test evidence | Known issue |
|---|---|---|---|---|
| E0 Contract | NOT STARTED | — | — | — |
| E1 Codec | NOT STARTED | — | — | — |
| E2 Lifecycle | NOT STARTED | — | — | — |
| E3 Single ops | NOT STARTED | — | — | — |
| E4 Batch | NOT STARTED | — | — | — |
| E5 Recovery | NOT STARTED | — | — | — |
| E6 CLI/serve | NOT STARTED | — | — | — |
| E7 Errors | NOT STARTED | — | — | — |
| E8 Compaction | NOT STARTED | — | — | — |
| E9 Performance | NOT STARTED | — | — | — |
| E10 Release | NOT STARTED | — | — | — |

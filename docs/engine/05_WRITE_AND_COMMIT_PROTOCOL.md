# Write and Commit Protocol

> Status: **Specification / implementation target**  
> Owner: engine team  
> Canonical filename: `05_WRITE_AND_COMMIT_PROTOCOL.md`

## 1. Purpose and scope

Defines the durable mutation path and failure semantics.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

All mutations use the same serialized UABE frame. A standalone PUT encodes one PUT record; DEL encodes one DEL; BATCH encodes all ordered operations in one frame. The frame is the only atomic unit. The write lock is held from quota validation through index publication so readers cannot see partial batches and writers cannot interleave bytes.

## 4. Design and procedure

Required sequence: (1) validate request and build full frame; (2) acquire exclusive engine lock; (3) verify frame size and overflow-safe `validEOF + frameSize <= maxBytes`; (4) write all bytes at exactly `validEOF` using a loop that handles short writes; (5) call file sync and require success; (6) apply validated operations to KeyDir in order; (7) advance `validEOF` and `nextSeqNo`; (8) release lock; (9) emit `ACK <op-id>` and flush stdout. If output is lost after durability, the operation remains committed; that is allowed.

## 5. Edge cases and failure behaviour

A write call returning nil error does not mean durable. A successful fsync is the barrier. Never update KeyDir or return success before it. For a batch, apply all entries while still holding the write lock; readers acquire the read lock and therefore see either the old map or the fully applied new map.

## 6. Verification and acceptance tests

Error path: if quota fails, do not write anything and return stable `DISK_FULL`. If write is partial or fails, truncate back to last known durable EOF and sync the truncation. If sync fails, the kernel may have discarded dirty pages while leaving offsets advanced; a blind retry is not proof. Roll back to `lastSyncedEOF`, sync successfully, and mark engine poisoned if rollback cannot be proven. Do not reuse an uncertain append offset or ACK. Do not mutate KeyDir on any failure path.

Additional implementation notes:

The file-size cap is an engine logical cap, not a guarantee that the physical disk cannot fill. Actual ENOSPC/EIO can still occur. Handle both quota precheck and real syscall errors. Quota should include header and all frame bytes. Define whether compaction temp space is included in `max_bytes`; document the selected interpretation and test it. Never silently raise the cap.

Crash windows: before write = old state; partial write = invalid tail discarded; full write before sync = old or new may be recovered, but no corruption; after sync before ACK = new state must recover; after ACK = new state must recover. These are the answers used in BREAKING.md and judge defense.

Tests inject short writes, ENOSPC, EIO, sync failure, process kill before/during/after sync, and stdout disconnect after durable commit. Verify KeyDir, EOF, disk bytes, ACK and restart state independently.

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

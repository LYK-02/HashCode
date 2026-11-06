# Compaction

> Status: **Specification / implementation target**  
> Owner: engine team  
> Canonical filename: `10_COMPACTION.md`

## 1. Purpose and scope

Defines live-set rewrite, crash-safe replacement, generation handling, and space constraints.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Compaction rewrites only current live key/value state into a fresh file, reducing append-log growth caused by overwrites and deletes. It is a maintenance operation; its correctness requirement is that before and after compaction the logical database is identical. It must survive process kill at every step. Run under the exclusive engine lock so no concurrent mutation can be omitted from the snapshot.

## 4. Design and procedure

Algorithm: (1) create `<db>.compact.tmp` in the same directory; (2) write a valid header with incremented compaction generation; (3) iterate live keys in deterministic key order; (4) encode current values into valid UABE frames (single records or bounded batches); (5) write all bytes; (6) sync temp file; (7) close temp descriptor and check close result; (8) atomically rename temp over data path; (9) sync parent directory; (10) open/retain the new database descriptor and rebuild/swap KeyDir/EOF/sequence state; (11) release lock. Exact descriptor handoff must be designed so no method uses the old inode after swap.

## 5. Edge cases and failure behaviour

The old data file remains authoritative until rename. If killed before rename, old file is intact; temp may be partial and is discarded on next open. If killed after rename, new file was synced before replacement and is the path's authoritative inode; directory sync makes the namespace update durable. If killed after rename but before cleanup, leftover temp or stale descriptors are handled on next open. Never unlink the only valid file before the replacement is synced.

## 6. Verification and acceptance tests

Directory durability: file sync persists temp content/inode; parent-directory sync persists the rename/create namespace update. Open the parent directory and sync it where supported. Do not assume file fsync also persists the directory entry. On platforms where directory sync semantics differ, document and test the supported target OS; the judging machine is expected to run the submitted implementation.

Additional implementation notes:

Disk-space policy: compaction may temporarily require old file + new temp file simultaneously. A `max_bytes` cap is logically defined for the authoritative database; decide whether it also limits temp size and document. Real ENOSPC during temp write must leave old database untouched and remove or ignore incomplete temp. If sync/rename/dir-sync fails, do not claim compaction success. Keep a recovery policy for the ambiguous state after rename but before directory sync.

Tests kill at temp create, header write, each live frame, temp sync, close, rename, directory sync, descriptor swap, and cleanup. Compare pre/post dumps; verify generation increases exactly once; verify file shrinks; test empty DB and all-deleted DB; test 1M put/delete workload; test disk full during temp creation and writing.

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

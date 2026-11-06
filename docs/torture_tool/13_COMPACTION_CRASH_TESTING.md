# Compaction Crash Testing

> Status: **Specification / implementation target**  
> Owner: torture team  
> Canonical filename: `13_COMPACTION_CRASH_TESTING.md`

## 1. Purpose and scope

Defines how to target every compaction barrier and verify old/new file authority.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Compaction mode first creates a workload with many obsolete records: PUT a large key set, overwrite values, delete a substantial fraction, and preserve a known live-set oracle. Record database size before compaction. Trigger compaction while the engine is active and kill the process at a random millisecond delay during the operation.

## 4. Design and procedure

Required crash windows: before temp create; after temp create; during header write; during live frame writes; after all temp writes before sync; during temp sync; after sync before close; after close before rename; during rename; after rename before directory sync; after directory sync before in-memory swap; and during cleanup. Random testing should be supplemented by test-only barrier hooks for deterministic coverage.

## 5. Edge cases and failure behaviour

After restart, run check/dump and compare exact logical live set. Either old or new complete database may be authoritative according to crash point; never accept missing acknowledged keys or partial values. If temp remains and old database is authoritative, cleanup may remove temp. If rename completed, the replacement must be complete because temp sync preceded rename. Verify no stale descriptor or index points into the old inode.

## 6. Verification and acceptance tests

Measure file size before/after successful compaction. After one million PUTs followed by one million DELs, the file must be able to shrink; test the required scale if hardware permits and record exact counts. Also test empty database, all keys deleted, many overwrites, and compaction with disk-full temp failure.

Additional implementation notes:

Tests must record barrier reached, seed, kill delay, file inode/generation, temp presence, data path presence, pre/post size, check output, dump hash, and oracle comparison. Do not claim every barrier was tested if only random timing was used and no evidence shows the window was reached.

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

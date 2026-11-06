# System Architecture

> Status: **Specification / implementation target**  
> Owner: common team  
> Canonical filename: `03_SYSTEM_ARCHITECTURE.md`

## 1. Purpose and scope

Defines the components, ownership boundaries, call flow, and persistence boundaries.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

The system has four logical layers: (1) CLI and serve protocol; (2) engine API and concurrency state; (3) binary log codec and recovery; (4) filesystem persistence. The Python torture tool is a separate executable that communicates only through the CLI/serve protocol. It must not import engine internals or read the engine's in-memory index.

## 4. Design and procedure

Engine state contains the database file descriptor, permanent lock-file descriptor, canonical database path, parent-directory descriptor or helper, `valid_eof`, `next_seq_no`, configured `max_bytes`, and an in-memory `KeyDir`. In Go, protect this state with `sync.RWMutex`; one exclusive writer lock is sufficient and easiest to reason about. Keep file access position-independent (`ReadAt`/`WriteAt` or equivalent) so readers do not race on a shared seek offset. Use a permanent `<db>.lock` inode and acquire an exclusive advisory lock before recovery; never unlink or replace this lock file.

The `KeyDir` maps raw key bytes (represented safely as Go strings or copied byte slices) to value locators: absolute value offset, value length, and sequence/generation metadata. It is derived state only. Recovery reconstructs it from valid frames. A `get` holds a read lock while resolving and reading the value; a write holds the write lock from quota check through durable append and complete index publication.

## 5. Edge cases and failure behaviour

Write flow: validate input; serialize a complete UABE frame in memory; acquire exclusive lock; check `valid_eof + frame_size <= max_bytes` with overflow-safe arithmetic; append using a full-write loop at `valid_eof`; sync the data file; only after successful sync apply all frame operations to KeyDir in payload order; advance `valid_eof` and sequence; emit ACK. If write or sync fails, do not update KeyDir or ACK. Treat the engine as poisoned after uncertain I/O until it has rolled back to a known durable EOF and successfully synced that rollback.

Recovery flow: acquire process lock; open/create file; validate header and version; scan frames sequentially with bounded lengths; validate header CRC, payload CRC, operation structure, sequence and exact payload consumption; apply only complete valid frames; truncate invalid/incomplete tail to last valid frame boundary; sync truncation; remove orphan compaction temp only after authoritative file selection is safe; expose engine only after recovery is complete.

## 6. Verification and acceptance tests

Compaction writes a complete replacement to a same-directory temp file. It must preserve the same logical state, write a valid header with incremented generation, encode live values, sync the temp file, close it, rename it over the database, sync the parent directory, then replace in-memory descriptors/index. The old file remains authoritative until rename. If killed before rename, discard temp on next open. If killed after rename, the complete synced replacement is authoritative. The exact order and descriptor handling must be implemented and tested; do not assume rename alone persists directory metadata.

Additional implementation notes:

Diagram requirements: show CLI, serve loop, API, RwLock, KeyDir, quota guard, UABE encoder, `pwrite`/`WriteAt`, `fsync`/`Sync`, ACK, recovery scanner, CRC checks, truncation, compaction temp, rename, directory sync, and sidecar flock. Label write and compaction steps numerically. Show the on-disk header/frame bytes. The diagram must match source code line-for-line at demo time.

Review `ARCHITECTURE.md` against this component map. Every component must have one owner, every shared state variable must have a lock rule, and every durable transition must name the syscall barrier that makes it safe.

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

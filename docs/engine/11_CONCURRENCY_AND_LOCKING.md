# Concurrency and Locking

> Status: **Specification / implementation target**  
> Owner: engine team  
> Canonical filename: `11_CONCURRENCY_AND_LOCKING.md`

## 1. Purpose and scope

Defines in-process synchronization, process ownership, reader semantics, and lock lifecycle.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

The minimum correct implementation uses one `sync.RWMutex` for all engine state. Writers acquire the exclusive lock for the complete durable mutation, including quota check, append, sync, KeyDir update, EOF and sequence advancement. Readers acquire a read lock for KeyDir lookup and value read. This intentionally blocks readers during a batch and is acceptable under the official rules.

## 4. Design and procedure

A read must not return a mixed value. `Get` holds the read lock while reading the value bytes from the file. Use `ReadAt` so it does not mutate a shared file cursor. Return a copy. If the implementation releases the lock before reading, it must prove that compaction cannot invalidate the locator or close the descriptor; avoid this optimization unless necessary.

## 5. Edge cases and failure behaviour

Cross-process lock: permanent `<db>.lock` sidecar, exclusive flock from open through close. Lock the sidecar before recovery, compaction, or temp cleanup. Do not unlink or rename the lock file: flock attaches to inode, and replacing the path can split ownership across two inodes. A second process must block or fail cleanly, not open concurrently as an independent writer.

## 6. Verification and acceptance tests

Lock order is process flock then engine mutex. Avoid callbacks or stdout writes while holding the mutex if they can re-enter the engine. Serve mode should serialize mutations in the engine; ACK responses can be dispatched by request ID. If stdout is blocked, durability has already happened before ACK; it must not compromise the database state.

Additional implementation notes:

On SIGKILL, OS closes file descriptors and releases advisory lock automatically. The sidecar file remains. On normal Close, sync any required state, close database descriptor, release/close lock descriptor, and return the first meaningful error. `Close` is not special in the crash model; a kill during close is just another kill.

Tests: two goroutines different keys; same key; reader during batch; multiple processes same path; second process blocked; kill lock holder and verify new opener acquires; lock sidecar never replaced; concurrent compact and put serialized; race detector; and no data race under repeated reads/writes.

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

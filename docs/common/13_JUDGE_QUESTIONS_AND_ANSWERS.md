# Judge Questions and Answers

> Status: **Specification / implementation target**  
> Owner: common team  
> Canonical filename: `13_JUDGE_QUESTIONS_AND_ANSWERS.md`

## 1. Purpose and scope

A practice bank for precise, source-grounded answers during the adversarial audit.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

**Why is ACK after fsync?** A successful mutation return is a durability promise. `Write` only places bytes in the OS page cache. The engine must call the file sync operation and observe success before publishing the new KeyDir state and emitting ACK. A process kill after ACK must leave the frame recoverable.

**Why one log instead of a persistent index?** A separate persistent index creates two authoritative structures whose updates can be interrupted in between. The index is derived from the log and rebuilt during open, so the log is the only durable source of truth.

**How is a batch atomic?** All operations are encoded in one UABE frame. Recovery validates the full header, bounded payload, CRC, and every operation before applying any of them. An incomplete or invalid tail frame is discarded as a whole. A synced frame is replayed in order.

## 4. Design and procedure

**What if the process dies after fsync but before ACK?** The operation may be present even though the caller did not receive ACK. The official rule permits an unacknowledged operation to be fully present or fully absent. The oracle therefore treats it as uncertain, not as guaranteed absent.

**What if write succeeds but fsync fails?** Do not update KeyDir or ACK. Attempt rollback to the last known durable EOF and sync that truncation. If rollback cannot be proven, mark the engine unusable and return an error; do not continue appending at an uncertain offset.

**Why CRC twice?** Header CRC protects the length/sequence/count fields before the scanner trusts them or allocates payload memory. Payload CRC detects incomplete/torn payload bytes. CRC detects accidental corruption; it is not a cryptographic authenticity mechanism.

**Why fsync the directory after rename?** Syncing the temp file persists its contents and inode. The directory sync persists the namespace change that makes the replacement name durable. Rename atomicity while running does not by itself prove crash durability.

## 5. Edge cases and failure behaviour

**What happens during compaction crash?** Before rename, old data.db remains authoritative and the temp file can be discarded. After a successful rename, the replacement was fully synced first; directory sync persists the name update. On restart, open the path and recover whichever complete file the namespace exposes. Never delete the old authoritative file before the new file is durable.

**Why a permanent lock file?** `flock` locks an inode, not a pathname. Unlinking/replacing an active lock path can let two processes lock different inodes. Keep the sidecar permanently and never rename/unlink it.

**How do you test without trusting the database?** The Python harness independently records START and ACK events and computes expected state from ACKed operations. It starts the engine as a child, injects SIGKILL at random millisecond delays, restarts, runs check/dump, and compares output to the independent model. The oracle never reads KeyDir or derives expected values from engine dump.

## 6. Verification and acceptance tests

**What is the hardest known weakness?** Be honest and cite a measured or unverified limitation, such as recovery scan cost for a very long log, filesystem portability of directory sync, temp-space requirements during compaction, or a mode not yet exhaustively tested. Do not invent a weakness-free claim.

**How do you add TTL?** Version the format and add a persisted expiry timestamp. `get` returns NOTFOUND after expiry; replay treats expired latest writes as tombstones to prevent older values from resurfacing; compaction omits expired keys; batches preserve ordering; clock rollback needs a defined persisted logical-time strategy. This is a design answer only unless the core is complete.

Additional implementation notes:

For every answer, point to the current function and test. Never recite an abstract guarantee that is contradicted by the implementation. If a scenario is unverified, say so and explain the intended mechanism separately.

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

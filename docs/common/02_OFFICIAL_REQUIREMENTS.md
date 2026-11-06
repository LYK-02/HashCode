# Official Requirements and Rules

> Status: **Specification / implementation target**  
> Owner: common team  
> Canonical filename: `02_OFFICIAL_REQUIREMENTS.md`

## 1. Purpose and scope

A traceable interpretation of the organiser's problem statement. This file separates mandatory requirements from implementation choices.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

This document is a compliance checklist, not a substitute for the supplied 19-page `Kill It and Prove It` PDF. If wording differs, the organiser's PDF wins. The system must be a single-process embedded key-value database, with one authoritative data file and a command-line wrapper. Required operations are `put`, `get`, `delete`, `batch`, `open`, and `close`. Required CLI commands are `open`, `put`, `get`, `del`, `batch`, `dump`, `check`, and `serve`.

## 4. Design and procedure

Data limits are byte-oriented: keys 1–1,024 bytes; values 0–1 MiB; at least one million keys; batch at most 10,000 operations. Values may be empty and may contain arbitrary bytes. A delete of a missing key is not an error. Batch operations are ordered and atomic. At least two writers must safely mutate concurrently; reads may run concurrently with writes and must not observe a half-applied batch. A single global lock is explicitly acceptable.

Storage must have exactly one authoritative data file. A sidecar lock file and a temporary compaction file are permitted. The file must shrink after one million puts followed by one million deletes. Compaction must itself be crash-safe. `max_bytes` is configurable; exceeding it must produce a clear error while leaving the database readable and recoverable.

## 5. Edge cases and failure behaviour

The five invariant statements are mandatory and must be copied into the test plan: acknowledged operations survive; in-flight operations are wholly present or absent; batches are atomic; values are never torn/mixed/garbage; and `open` can recover after any process kill, including a kill during recovery. The crash model is abrupt process death (`SIGKILL` on Linux/macOS or equivalent), not a graceful close. A kill may happen during write, sync, rename, or recovery. The specification assumes the OS/disk honestly preserve bytes after successful `fsync`; it does not require surviving OS crash or power loss, although stronger claims must be justified.

## 6. Verification and acceptance tests

Allowed: any language, standard libraries, test frameworks, AI tools with prompt logging, and reading documentation. Not allowed: databases or storage engines (including SQLite, RocksDB, LevelDB, LMDB, Redis, BoltDB, Badger, etc.), ORM/KV wrappers, libraries implementing WAL/journaling/B-trees/LSM for the team, copied database source, prewritten code from before event start, code sharing between teams, and internet/AI during the demo. Serialization libraries are a grey area; ask a judge. Everything must work offline. Maintain meaningful Git commits throughout.

Additional implementation notes:

Deliverables: engine and CLI; torture tool; `BREAKING.md` (maximum two pages, required five sections); `PROMPTS.md` (all prompts chronologically, unedited, tool and approximate time); `ARCHITECTURE.md` (diagram, all 32 scenario rows, three guarantee arguments); and a working README. Disqualification risks include banned libraries, copied/pre-event code, shared code, AI/internet during demo, rigged torture tests, falsified prompt log, or falsified history.

Scoring: 30 kill-run correctness; 15 torture quality; 10 architecture/proof; 20 crash-here answers; 15 BREAKING; 10 design extension. Penalties include recovery over two seconds (-5), no shrink after compaction (-5), disk-full corruption/unopenable database (-10), missing PROMPTS (-10), and -10 per invariant violation in kill runs (minimum zero in that category).

Before coding, mark each requirement as `MUST`, `SHOULD`, or `OPTIONAL`; do not downgrade a MUST. Record any judge clarification in the repository. Recheck this document against the official PDF before submission.

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

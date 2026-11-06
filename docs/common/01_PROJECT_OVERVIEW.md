# Project Overview

> Status: **Specification / implementation target**  
> Owner: common team  
> Canonical filename: `01_PROJECT_OVERVIEW.md`

## 1. Purpose and scope

Defines the product boundary, success criteria, system vocabulary, and the engineering principles shared by both teams.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Build a single-process, embedded key-value database from scratch. The database stores authoritative state in one data file, exposes a small library/API and the mandatory `kvdb` CLI, and must preserve acknowledged writes across arbitrary process termination. This is a storage-correctness challenge, not an application/UI challenge.

The recommended implementation split for this repository is **Go for the engine and CLI, Python 3 for the independent torture harness**. This follows the supplied Antigravity Final Plan. The separate architectural blueprint recommends Rust; that is a competing engineering recommendation, not an official rule. The official specification permits any language. If the team changes language, update every build, API, ownership, locking, and benchmark reference consistently before implementation.

## 4. Design and procedure

The authoritative durable state is `data.db`, an append-only sequence of self-validating Unified Atomic Batch Envelope (UABE) frames after a 32-byte file header. `data.db.lock` is a permanent sidecar used for exclusive process ownership. `data.db.compact.tmp` is temporary and may exist only during compaction. The key index is rebuilt in memory; do not introduce a second authoritative persistent index.

All successful mutations are durable before success is reported. A `put`, `delete`, or `batch` is encoded as one frame, appended completely, synchronized with `fsync`/the language's equivalent, then applied to the in-memory index, and only then acknowledged. Readers and writers are synchronized so a reader sees the state before or after a batch, never a partially applied batch.

## 5. Edge cases and failure behaviour

The five invariants are: (1) every operation acknowledged before a crash is present after recovery; (2) an unacknowledged operation may be fully present or fully absent, but never malformed; (3) a batch is all-or-nothing; (4) no key returns bytes that were never written as that key's value; and (5) recovery is repeatable after a crash during recovery. `open` must recover a database produced by this engine and complete in under two seconds with one million keys.

Hard limits: key 1–1,024 bytes; value 0–1 MiB; at least 1,000,000 keys; batch up to 10,000 operations. At least two concurrent writers are required; reads may overlap writes. The database must compact and shrink after one million puts followed by one million deletes. A configurable `max_bytes` cap must fail writes cleanly without damaging prior data.

## 6. Verification and acceptance tests

The design deliberately avoids a separate persistent index, multi-file transaction protocol, network service, sharding, MVCC, and an LSM/B-tree implementation. Complexity does not earn points by itself. Do not add TTL, replication, encryption, or a UI during the core build; TTL is only a verbal design-extension topic for the judge.

Treat the official PDF as authoritative when it conflicts with a proposal or example. The supplied plans contain illustrative code and assumptions that must be reviewed, not copied blindly. In particular, every syscall result must be checked, and every claimed scenario must be backed by current code and a recorded test.

Additional implementation notes:

Primary acceptance: clean offline build; all eight CLI commands; all five invariants; all 32 official scenarios; 500 default torture iterations with reproducible seeds; targeted large-value, large-batch, recovery-crash, compaction, and disk-full modes; accurate architecture proof; honest BREAKING document; complete AI prompt log; and a rehearsed 15-minute demo.

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

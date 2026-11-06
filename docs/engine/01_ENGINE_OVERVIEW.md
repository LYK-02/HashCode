# Engine Overview

> Status: **IMPLEMENTED & VERIFIED (Milestones M1–M3 Complete; M4 In Progress)**  
> Owner: engine team  
> Canonical filename: `01_ENGINE_OVERVIEW.md`  
> Latest Merged Commit: `09c6863` (Milestone M3: Batch Atomicity and Crash Recovery)

## 1. Purpose and scope

Defines the engine's API contract, state model, and correctness boundary.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

The engine is a Go package/library that implements byte-string key-value operations over one authoritative append-only file. The CLI is a thin wrapper; the Python torture program is an external client. The engine owns file descriptors, locks, the binary codec, recovery, compaction, and the in-memory index.

## 4. Design and procedure

Public API shape (idiomatic names may vary): `Open(path, maxBytes) (*Engine, error)`, `Put(key, value []byte) error`, `Get(key []byte) ([]byte, bool, error)`, `Delete(key []byte) error`, `Batch(ops []Operation) error`, `Compact() error`, `Check() error`, `Dump()`, and `Close() error`. Validate key/value sizes at the API boundary. Copy caller-owned buffers or guarantee immutable ownership until serialization completes.

## 5. Edge cases and failure behaviour

A successful mutation returns only after the frame is durable and KeyDir reflects it. A failed mutation must not claim success. `Get` returns a copy of bytes, not a slice into a mutable shared buffer. A missing key is represented separately from an empty value. `Delete` of a missing key succeeds as a durable delete/tombstone operation unless the team documents a safe no-op policy; choose one consistently.

## 6. Verification and acceptance tests

Engine invariants: validEOF is the first byte after the last fully validated and durable frame; nextSeqNo is strictly increasing; KeyDir contains only live keys and valid offsets; every locator points inside a validated frame; the lock descriptor outlives the engine; and no public method runs after Close. On any uncertain I/O failure, prevent subsequent writes until state is re-established.

Additional implementation notes:

Do not expose internal map pointers, file descriptors, or mutable buffers. Do not silently reset an invalid database header that could contain acknowledged records. Initialization recovery and corruption handling must be distinct. A partial initial header can be repaired only because no successful operation could have been acknowledged before the header was durable; a later invalid header requires a conservative, explicit policy.

Tests: API boundary sizes; empty values; arbitrary binary bytes; overwrite; delete missing/existing; batch ordering; concurrent calls; close/reopen; durable ACK; error propagation; and invalid lifecycle calls.

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

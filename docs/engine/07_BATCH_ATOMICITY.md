# Batch Atomicity

> Status: **Specification / implementation target**  
> Owner: engine team  
> Canonical filename: `07_BATCH_ATOMICITY.md`

## 1. Purpose and scope

Defines the batch contract and why one frame makes it all-or-nothing.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

A batch contains up to 10,000 PUT/DEL operations and is one logical mutation. Encode all subrecords into one contiguous UABE payload and append one frame. No operation becomes visible until the entire frame is written, synced, and then applied to KeyDir under the write lock. A batch is either entirely present or entirely absent after a crash.

## 4. Design and procedure

Validation is performed before disk I/O: count, operation types, key/value bounds, aggregate payload size, and integer overflow. If any operation is invalid, reject the entire batch with no append. A batch may contain duplicate keys; process operations in input order. The last operation for each key determines the final state. The API must not reorder, parallelize, or collapse operations in a way that changes semantics.

## 5. Edge cases and failure behaviour

Durability sequence is identical to a single mutation. Recovery validates the whole frame, including CRC and all operation boundaries, before applying the first subrecord. If the frame is truncated or CRC fails, truncate at frame start; none of its operations are applied. If the frame is fully valid, replay every operation in order. This eliminates a separate BEGIN/COMMIT marker in the selected design.

## 6. Verification and acceptance tests

Reader visibility: the writer holds the exclusive RwMutex lock from before append through full index update. Readers block until it is done. A reader can observe the old KeyDir before the batch or the new KeyDir after the batch, never an intermediate subset. If the team later optimizes with copy-on-write index snapshots, prove equivalent atomic publication before replacing the simple lock.

Additional implementation notes:

Disk-full: precompute full frame length and check quota before append. If actual write/sync fails mid-frame, truncate to last synced EOF and sync; no KeyDir changes. An E2-style cap halfway through the conceptual operations must not produce a partial batch because the frame is reserved/written as one unit. The harness must verify zero batch keys or all batch keys, not just sample one.

Tests: 1-op batch; 10-op; 10,000-op; duplicate keys; empty batch policy; invalid operation at first/middle/last position; partial append; sync failure; concurrent reader; concurrent writer; large aggregate value payload; and crash at random points. Record batch op IDs in oracle as a single event.

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

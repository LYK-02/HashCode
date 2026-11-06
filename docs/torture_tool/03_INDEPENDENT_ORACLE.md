# Independent Oracle

> Status: **Specification / implementation target**  
> Owner: torture team  
> Canonical filename: `03_INDEPENDENT_ORACLE.md`

## 1. Purpose and scope

Defines the reference model, event log, ACK semantics, and ambiguous in-flight operations.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

The oracle is the independent truth about what the client observed. It is owned by the torture process and persisted separately from `data.db`. The engine must never supply expected state. The oracle records request intent before send and durable ACK after response. It can also record explicit ERR responses.

## 4. Design and procedure

Event schema (JSON Lines): `START {iteration, op_id, worker_id, kind, operations, seed, timestamp_ns}`; `ACK {iteration, op_id, ack_order, timestamp_ns}`; `FAIL {iteration, op_id, code, timestamp_ns}`; `KILL {pid, delay_ms, timestamp_ns}`; `RESTART {exit_code, timestamp_ns}`; `VERIFY {result, expected_hash, actual_hash}`. Flush each event. Store byte values as hex or base64, not lossy UTF-8.

## 5. Edge cases and failure behaviour

For a sequential acknowledged history, apply operations in ACK order. The serve implementation must serialize durable commits and emit ACK in that same order; if it does not guarantee response order, the protocol must return a durable sequence number so the oracle can reconstruct commit order. Freeze this behaviour in the protocol spec. For concurrent same-key requests, do not invent an order based on thread scheduling; use the server's commit sequence or response ordering guaranteed by implementation.

## 6. Verification and acceptance tests

At kill time, every START without ACK/FAIL is in-flight. The official rules allow it to be wholly present or absent. For exact verification, compute a set/range of acceptable states or replay acknowledged operations and then account for uncertain in-flight operations. The simplest safe policy is to use unique keys for in-flight writes and verify ACKed keys exactly while allowing only known in-flight keys to vary; for same-key races, record all possible complete values and never accept mixed bytes. For batches, uncertain batch outcome must be all or none.

Additional implementation notes:

Do not remove START records after ACK; keep the full append-only event history. A crash between engine ACK and oracle ACK persistence is a harness-side ambiguity: the operation may have been acknowledged to the client but not durably recorded by oracle. Reduce this window by synchronously appending and flushing the oracle ACK immediately after reading response; document the residual harness limitation. A robust protocol may include an ACK sequence and harness fsync its journal before proceeding.

Verifier rules: every ACKed PUT's final value must match the last committed acknowledged write unless a later acknowledged DEL/PUT supersedes it; every ACKed DEL must be absent unless later acknowledged PUT restores it; all acknowledged batch operations apply as one ordered unit; unacknowledged operations may be present/absent but never partially; no value may be unknown or a byte mixture. Keep a separate operation history for same-key conflict resolution.

Oracle self-tests: replay known event fixture; duplicate ACK; missing START; ACK after FAIL; concurrent same-key sequence; in-flight batch; empty value; binary bytes; and process crash while writing oracle log. Reject inconsistent oracle journals instead of silently normalizing them.

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

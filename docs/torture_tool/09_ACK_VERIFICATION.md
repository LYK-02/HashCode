# ACK Verification

> Status: **Specification / implementation target**  
> Owner: torture team  
> Canonical filename: `09_ACK_VERIFICATION.md`

## 1. Purpose and scope

Defines how to prove every ACKed mutation survived and how to handle uncertain operations.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

ACK is the boundary between a guaranteed operation and an uncertain operation. The engine may emit `ACK <op-id>` only after the frame has been fully appended, synced, and applied to KeyDir. The harness records ACK only after it reads the exact matching response. It must not infer success from a write to stdin or from process exit code.

## 4. Design and procedure

The oracle records START before send and ACK/FAIL after response. ACKed operations are guaranteed present unless superseded by a later acknowledged operation on the same key. ERR operations are not guaranteed present and should not have changed state. START operations with no response at kill are in-flight and may be fully present or absent. They may never be partially visible.

## 5. Edge cases and failure behaviour

For concurrent same-key operations, use the server's commit order. The recommended protocol emits ACKs in serialized commit order, or includes a monotonic durable sequence in ACK. If ACK order is not guaranteed to equal commit order, the oracle cannot safely infer final value from receipt order; add a sequence field before implementation freezes.

## 6. Verification and acceptance tests

For a batch, the oracle stores the entire operation list as one event. If ACKed, apply all operations in order. If unresolved, permit either all operations or none. Never permit a subset. If a later acknowledged operation supersedes a key, apply that later operation after the earlier batch according to commit sequence.

Additional implementation notes:

After restart, run check and dump. Compare every expected key and value byte-for-byte. Verify ACKed deletes remain absent unless a later acknowledged put restores them. Verify no unknown keys/values appear except those attributable to unresolved in-flight operations. Use exact binary-safe parsing and distinguish empty value from missing key.

A harness-side failure window exists if the engine ACK reaches the harness but the harness dies before persisting ACK to oracle. Minimize it by writing and flushing ACK events immediately. For the judged test, process death is targeted at engine, not harness; still document the limitation. Never claim the oracle is infallible if it is not.

Tests: ACK immediately before kill; sync before ACK; ACK response lost; ERR; ACKed overwrite/delete sequence; same-key writers; ACKed batch; unresolved batch; empty values; and a deliberately corrupted dump to verify mismatch detection.

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

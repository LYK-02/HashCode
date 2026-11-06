# Large Values and Batches

> Status: **Specification / implementation target**  
> Owner: torture team  
> Canonical filename: `11_LARGE_VALUES_AND_BATCHES.md`

## 1. Purpose and scope

Targets maximum payloads, memory pressure, framing, and batch atomicity.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Large-value mode writes values of exactly 1,048,576 bytes and values near boundary sizes. Use repeating patterns, deterministic pseudorandom bytes, and embedded zero/newline bytes. Keys remain within 1,024 bytes. Verify exact value length and hash after restart; never decode binary as UTF-8 for comparison.

## 4. Design and procedure

Large-batch mode constructs exactly 10,000 valid operations, including duplicate keys, deletes, empty values, and large aggregate payloads within the implementation's supported memory bound. Kill while the batch is being sent, written, or synced. The expected state is all-or-none for the batch. If the protocol line size is too large, use a documented framing method or send a batch file through the CLI; do not truncate a line silently.

## 5. Edge cases and failure behaviour

Monitor peak memory and pipe pressure. The harness must avoid holding unnecessary copies of a 10,000-op batch in multiple queues. The engine must calculate payload length with checked arithmetic and reject unsupported aggregate size before disk I/O. A supported 10,000-operation batch cannot be rejected merely because it is inconvenient; define a reasonable maximum frame size consistent with official key/value/count limits.

## 6. Verification and acceptance tests

Large values exercise multi-block writes and partial writes. Verify header CRC catches bad lengths before allocation, payload CRC catches torn data, and recovery truncates the entire frame. A value spanning multiple blocks must never be returned as a prefix or mixed with another key's bytes.

Additional implementation notes:

Tests: 0, 1, typical, 1 MiB-1, 1 MiB values; 10,000 operations; maximum key; repeated key; large batch with final delete; kill at random delays; partial pipe write; engine OOM safeguards; and restart hash comparison.

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

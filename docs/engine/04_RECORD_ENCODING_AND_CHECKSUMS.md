# Record Encoding and Checksums

> Status: **Specification / implementation target**  
> Owner: engine team  
> Canonical filename: `04_RECORD_ENCODING_AND_CHECKSUMS.md`

## 1. Purpose and scope

Specifies operation subrecords, offsets, validation, and CRC32C behaviour.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Payload is a concatenation of exactly `op_count` operation subrecords. Each starts with `op_type:u8`, `key_len:u16`, `value_len:u32`, then `key[key_len]`, then `value[value_len]`. PUT type is 0x01; DEL type is 0x02. Key length is 1–1,024. Value length is 0–1,048,576. DEL requires value length zero. All keys and values are opaque byte strings; no UTF-8 assumption is made by the engine.

## 4. Design and procedure

Encoder steps: validate operation count; validate each type and size; compute payload length using checked addition; allocate one bounded buffer; write fields in little-endian; copy key/value bytes; compute header CRC over frame bytes 8–27; compute payload CRC over payload only; append trailer. Do not mutate the caller's operation list after encoding. Keep a single canonical encoder for PUT, DEL and BATCH.

## 5. Edge cases and failure behaviour

Decoder steps: verify frame header first; reject unknown magic/version; validate header CRC before trusting lengths; verify payload fits in file and configured max; read payload and trailer; verify payload CRC; parse exactly op_count subrecords; check every boundary before slicing; validate DEL rules; require final cursor equals payload length. Only then return a validated operation list to recovery or index application.

## 6. Verification and acceptance tests

CRC32C uses the Castagnoli polynomial. Do not accidentally use IEEE CRC32. In Go, `hash/crc32` with `crc32.Castagnoli` is the intended standard-library primitive. CRC is for accidental torn-write detection, not malicious tampering. If checksum mismatch occurs in the final frame, truncate to its starting offset. If invalid bytes appear in the middle followed by more data, distinguish unsupported corruption from a torn tail; do not silently discard later acknowledged frames.

Additional implementation notes:

Locator arithmetic: for operation cursor `p`, value offset is `frameStart + 28 + p + 7 + keyLen`; after applying the operation, advance `p += 7 + keyLen + valueLen`. Check overflow and bounds before each calculation. DEL has no value locator. A zero-length PUT is still a present key with a zero-length value.

Tests include every boundary size, all byte values, embedded NUL/newline, unknown op type, zero key, 1,025-byte key, 1 MiB+1 value, DEL with nonzero value, truncated operation header, truncated key/value, trailing junk, incorrect op_count, wrong CRC, and fuzzed parser inputs.

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

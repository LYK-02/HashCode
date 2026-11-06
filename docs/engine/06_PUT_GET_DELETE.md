# PUT GET DELETE

> Status: **Specification / implementation target**  
> Owner: engine team  
> Canonical filename: `06_PUT_GET_DELETE.md`

## 1. Purpose and scope

Operation semantics, index updates, binary values, and edge cases.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

PUT stores the exact byte sequence under a key, replacing any prior value. Validate key length 1–1,024 and value length 0–1 MiB. Empty value is valid. PUT always appends a new frame; do not overwrite an earlier value in place. The index locator points to the value bytes in the new frame.

## 4. Design and procedure

GET checks the in-memory KeyDir under a read lock. If absent, return NOTFOUND. If present, read exactly `value_len` bytes at `value_offset` using position-independent I/O. For a zero-length value, return a present empty slice and `found=true`; do not interpret it as missing. Return a copy. A read error must be surfaced; do not return partially filled bytes as a valid value.

## 5. Edge cases and failure behaviour

DELETE appends a tombstone operation in a UABE frame. On successful sync, remove the key from KeyDir. Deleting a missing key is not an error per the official specification; it may still append a tombstone to preserve a clear mutation ordering. Pick one policy and ensure ACK semantics and sequence numbers are consistent. A crash during delete may leave old value or absent value, but never a malformed value.

## 6. Verification and acceptance tests

Binary keys/values may include NUL, tabs, newlines, invalid UTF-8, and all 256 byte values. The engine must not use C-string semantics or split raw bytes. The CLI's text form must encode binary data, recommended hex in serve mode. Ensure key equality is byte equality. If Go string keys are used, convert from copied byte slices; Go strings can contain arbitrary bytes.

Additional implementation notes:

Overwrites: if key K is written V1 then V2, recovery applies frames in sequence and KeyDir points to V2. Delete then put resurrects the later value. Put then delete leaves key absent. Duplicate operations inside a batch obey exact payload order. A batch containing repeated K must not be deduplicated unless the semantics preserve final ordered result.

Tests: empty value vs missing; zero-length key rejected; max key/value accepted; max+1 rejected; overwrite; delete missing; delete then put; put then delete; binary bytes; same key concurrent writers; one-megabyte values; and read errors. Restart after each successful operation and compare exact bytes.

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

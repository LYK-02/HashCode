# Shared Data Format

> Status: **Specification / implementation target**  
> Owner: common team  
> Canonical filename: `04_SHARED_DATA_FORMAT.md`

## 1. Purpose and scope

Canonical byte-level contract shared by engine, CLI, torture tool, and documentation.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

All persisted multi-byte integers are little-endian. The authoritative file begins with a fixed 32-byte header. Header offsets: `0..8` magic bytes `KVDB2026`; `8..12` format version (`u32`, initially 1); `12..20` compaction generation (`u64`); `20..28` reserved flags (`u64`, zero); `28..32` header CRC32C (`u32`) over bytes `0..28`.

## 4. Design and procedure

Each UABE frame has a 28-byte fixed header, payload, and 4-byte trailer. Frame header offsets relative to frame: `0..4` magic `0x55414245` (`UABE`, u32); `4..8` header CRC32C over bytes `8..28`; `8..16` payload length (`u64`); `16..24` sequence number (`u64`); `24..28` operation count (`u32`). Payload starts at frame offset 28. Trailer is the four-byte CRC32C of the entire payload. Total frame size is `32 + payload_len`.

Each payload operation is `op_type:u8`, `key_len:u16`, `value_len:u32`, key bytes, then value bytes. `PUT=0x01`; `DEL=0x02`. Key length must be 1–1,024. Value length must be 0–1,048,576. DEL must have value length zero. A valid frame has 1–10,000 operations unless the team explicitly chooses and documents a different empty-batch policy. The recommended policy is reject an empty batch without writing a frame.

## 5. Edge cases and failure behaviour

Frame validation is all-or-nothing. Validate the fixed header before trusting payload length. Apply an explicit maximum payload bound derived from maximum operation count, maximum key/value sizes, and checked arithmetic before allocation. Reject integer overflow, unknown operation types, invalid key/value lengths, malformed deletes, payload underflow/overrun, extra trailing payload bytes, invalid sequence order, and CRC mismatch. Do not apply any operation until the entire frame and all subrecords have been validated. A frame containing duplicate keys is replayed in payload order; last operation wins.

## 6. Verification and acceptance tests

A `KeyDirEntry` locator points to the value bytes inside a valid frame. For a PUT with empty value, store a valid locator with length zero; do not confuse it with NOTFOUND. DEL removes the key from the live index. During recovery, compute absolute offsets from frame start + 28-byte frame header + operation-relative payload offset + 7-byte operation header + key length. Validate all arithmetic before slicing or seeking.

Additional implementation notes:

The CLI text protocol uses hex for binary keys and values. Hex is lowercase or case-insensitive on input; output formatting must be deterministic. The serve protocol uses one request per line, request IDs, and exactly one matching response per request. Never use tab/newline-delimited raw values. The torture tool must parse the protocol independently and treat ACK as a durability promise.

Any change to these bytes requires a versioning decision and updates to `03_SYSTEM_ARCHITECTURE.md`, `05_CLI_AND_PROTOCOL_SPEC.md`, engine codec tests, torture parser, README, and architecture diagram. Do not let Go and Python independently invent field layouts.

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

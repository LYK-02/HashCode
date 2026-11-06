# Binary File Format

> Status: **Specification / implementation target**  
> Owner: engine team  
> Canonical filename: `03_BINARY_FILE_FORMAT.md`

## 1. Purpose and scope

Normative description of the persisted file header and UABE frame envelope.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

The database file is a 32-byte fixed header followed by contiguous UABE frames. All integers are little-endian. Header: bytes 0–7 magic `KVDB2026`; 8–11 version `1`; 12–19 compaction generation; 20–27 reserved zero; 28–31 CRC32C over first 28 bytes. The scanner must reject unsupported versions rather than guessing field meaning.

## 4. Design and procedure

Frame: 28-byte header + payload + 4-byte trailer. Header fields: 4-byte magic `0x55414245`; 4-byte header CRC32C over the following 20 bytes; 8-byte payload length; 8-byte sequence; 4-byte op count. Payload length counts payload bytes only. Trailer is CRC32C over exactly payload bytes. Total frame length is 32 + payload length. The next frame starts immediately after the trailer.

## 5. Edge cases and failure behaviour

The UABE frame is the atomicity boundary. A standalone PUT or DEL has op_count=1. A batch is one frame containing N operations. The frame must be assembled completely in memory before append. There is no separate BEGIN/COMMIT chain in this design; frame completeness and CRC validation serve as the commit recognition mechanism. The successful sync of the entire frame is the durability barrier.

## 6. Verification and acceptance tests

Before allocation, validate frame magic, header CRC, payload length upper bound, file-offset arithmetic, sequence constraints, and op_count range. A malicious/corrupt payload length such as UINT64_MAX must never trigger allocation. Compute a maximum payload from 10,000 operations × (7-byte operation header + 1,024-byte key + 1 MiB value), using checked arithmetic; a practical configured frame-memory cap may be lower only if documented and still meets all mandatory inputs.

Additional implementation notes:

Header corruption policy: if file is shorter than 32 bytes during first initialization, no operation could have been acknowledged, so reinitialize safely after checking errors. If a full header is invalid on a database that may contain data, do not blindly truncate/recreate; preserve the file and report corruption or use a specifically proven recovery rule. The official invariant applies to files produced by the engine, so header creation ordering must ensure no ACK before header and directory entry are synced.

Maintain a golden hex fixture for header and frame, round-trip tests, checksum vectors for CRC32C/Castagnoli, malformed length tests, and cross-language Go/Python parser agreement.

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

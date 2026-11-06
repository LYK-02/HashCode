# Phase T0 Implementation Plan: Contract and Protocol Specification

> Canonical Phase: **T0**  
> Component: Wire Protocol, Request/Response Grammar, Inter-Process Contracts  
> Status: **Completed & Verified**  
> Related Docs: `docs/torture_tool/01_TORTURE_OVERVIEW.md`, `docs/common/05_CLI_AND_PROTOCOL_SPEC.md`, `docs/common/04_SHARED_DATA_FORMAT.md`

---

## 1. Objective and Scope

Define, freeze, and validate the exact wire protocol between the Python torture tool (`./torture`) and the Go database server process (`./kvdb serve <db_path>`). This phase establishes unambiguous message grammars, serialization standards for arbitrary binary data, commit acknowledgments, error codes, and golden test fixtures before any harness worker or engine server logic is implemented.

---

## 2. Dependencies and Pre-Conditions

- **Upstream Contracts:** `docs/common/05_CLI_AND_PROTOCOL_SPEC.md` must be treated as authoritative.
- **Runtime Environment:** Python 3 standard library only (`binascii`, `json`, `dataclasses`, `re`).
- **Engine Coordination:** Wire grammar agreed with Go storage engine team.

---

## 3. Detailed Technical Specifications

### 3.1 Wire Framing & Encoding
- **Transport:** Line-delimited ASCII over standard POSIX anonymous pipes (`stdin` / `stdout`).
- **Binary Safety:** All keys and values over the wire **must be hex-encoded** strings (`[0-9a-fA-F]*`) to permit arbitrary byte sequences including `\x00`, `\n`, `\r`, spaces, and non-UTF-8 payloads without delimiter ambiguity.
- **Line Ending:** Exactly `\n` (`0x0A`). Trailing `\r` stripped on parse.
- **Maximum Line Length:** Configurable limit supporting up to 1 MiB raw values (which hex-encode to 2 MiB characters) plus framing metadata (~2.1 MiB).

### 3.2 Request Grammar (Client $\to$ Engine via `stdin`)
```text
PUT <op_id> <hex_key> <hex_val>\n
DEL <op_id> <hex_key>\n
BATCH <op_id> <count> <hex_key_1>:<hex_val_1>,<hex_key_2>:,...\n
PING <op_id>\n
```
- `<op_id>`: String matching `[a-zA-Z0-9_\-\.]+` (e.g. `w1-000001`, `b-0042`).
- In `BATCH`:
  - Items delimited by comma `,`.
  - Key and value separated by colon `:`.
  - Deletion indicated by an empty value following colon (e.g., `<hex_key>:`).
  - `<count>` must match the number of comma-separated operations.

### 3.3 Response Grammar (Engine $\to$ Client via `stdout`)
```text
ACK <op_id> [seq_num]\n
ERR <op_id> <error_code> <message>\n
PONG <op_id>\n
```
- `ACK`: Emitted strictly after data and commit marker are durable (`fsync` returned successfully). Optional `seq_num` tracks total commit sequence.
- `ERR`: Indicates operation failure (e.g., quota exceeded, validation error).
- Stable error codes: `QUOTA_EXCEEDED`, `INVALID_REQUEST`, `PAYLOAD_TOO_LARGE`, `IO_ERROR`, `LOCKED`.

---

## 4. Phase Breakdown & Subtasks

### Subtask T0.1: Formal Grammar Definition & Specification Document
- Create internal protocol specification module in `torture_tool/protocol/spec.py` defining regex patterns, tokenizers, and constants.
- Define dataclasses: `PutRequest`, `DelRequest`, `BatchRequest`, `PingRequest`, `AckResponse`, `ErrResponse`.

### Subtask T0.2: Serializer & Deserializer Implementation
- Implement `encode_request(req) -> str`: Safe hex conversion, validation of sizes, output formatting with trailing `\n`.
- Implement `parse_response(line: str) -> Response`: Strict token splitting, validation of `<op_id>`, mapping error codes, rejecting extra fields or malformed characters.

### Subtask T0.3: Binary Safe Key/Value Utilities
- Implement helper routines for hex-encoding and decoding Python `bytes`.
- Explicitly test zero-length keys (rejected), zero-length values (valid empty byte strings), and boundary payload encodings.

### Subtask T0.4: Golden Protocol Fixtures
- Create bidirectional test fixtures in `torture_tool/tests/fixtures/protocol_golden.json`:
  - Valid PUT with text, null bytes, unicode emojis, and 1 MiB buffers.
  - Valid DEL.
  - Valid BATCH containing mixed PUTs and DELs, single items, duplicate keys.
  - ACK with and without sequence numbers.
  - ERR responses for each standardized error code.
  - Malformed lines (truncated hex, odd number of hex digits, negative count, missing op_id).

---

## 5. Failure Modes and Edge Cases

| Case | Scenario | Expected Handling |
|---|---|---|
| **P0.1** | Odd-length hex string | Parser fails with `ProtocolParseError`; does not panic or hang. |
| **P0.2** | Empty value in PUT | Allowed: encodes as empty string `""`, wire shows `<hex_key> \n`. |
| **P0.3** | Delimiter collision | Commas/colons inside binary keys/values are hex-encoded, preventing delimiter collisions in BATCH. |
| **P0.4** | Trailing garbage | Strict line parsing rejects any tokens beyond expected fields. |
| **P0.5** | High-entropy binary | Keys/values containing all byte values (`0x00`-`0xFF`) round-trip bit-identically. |

---

## 6. Verification and Completion Criteria

- [x] Golden fixtures validate 100% cleanly in Python test suite without warnings.
- [x] Round-trip encoding/decoding tests preserve arbitrary bytes bit-for-bit.
- [x] Go storage engine team signs off on wire format compatibility.
- [x] Shared fixtures checked into repository under `torture_tool/tests/fixtures/`.


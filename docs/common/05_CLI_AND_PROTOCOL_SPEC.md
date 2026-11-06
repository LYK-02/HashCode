# CLI and Protocol Specification

> Status: **Specification / implementation target**  
> Owner: common team  
> Canonical filename: `05_CLI_AND_PROTOCOL_SPEC.md`

## 1. Purpose and scope

Exact judge-facing commands and a safe multiplexed serve protocol.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

The judge-facing commands, invoked from repository root, are: `./kvdb open <path> [--max-bytes N]`; `./kvdb put <path> <key> <value>`; `./kvdb get <path> <key>`; `./kvdb del <path> <key>`; `./kvdb batch <path> <file>`; `./kvdb dump <path>`; `./kvdb check <path>`; and `./kvdb serve <path>`. `open` prints key count and recovery time. `get` prints the value or `NOTFOUND`. `dump` prints all key/value pairs sorted by key. `check` prints `OK` or a precise reason. `serve` remains alive and reads stdin until EOF or process termination.

## 4. Design and procedure

The batch file has one operation per line: `put <key> <value>` or `del <key>`. Because the official command example is illustrative and the engine supports arbitrary bytes, define whether CLI arguments are UTF-8 convenience input or hex. Recommended: one-shot CLI accepts human-readable UTF-8 strings for simple judging; `serve` uses hex for binary-safe operation. Document this distinction and test empty values. If one-shot commands also support hex, use an explicit prefix/flag so literal hex-looking text is unambiguous.

## 5. Edge cases and failure behaviour

Recommended serve requests: `PUT <op-id> <key-hex> <value-hex>`; `DEL <op-id> <key-hex>`; `BATCH <op-id> <count> <encoded-ops>`; optionally `PING`. Responses: `ACK <op-id>` only after successful durable commit; `ERR <op-id> <stable-error-code> <safe-message>` for validation, quota, or I/O failure. `NOTFOUND` is a get result, not a mutation ACK. Define exact batch encoding in a single grammar and publish examples. Reject malformed lines without desynchronizing later requests.

## 6. Verification and acceptance tests

Every request ID must be unique among in-flight requests. A single serve process may receive requests from multiple torture writers. The harness must serialize writes to stdin with a mutex and run exactly one stdout reader/dispatcher that routes responses by ID; worker threads must not independently call `readline()` because they can consume each other's ACKs. Writes and responses must be bounded; impose a maximum line length consistent with 10,000-operation batches and 1 MiB values, or use a length-prefixed binary protocol if line size becomes unsafe. The official requirement is stdin/stdout interoperability; any extension must remain easy for the judge to drive.

Additional implementation notes:

Stdout is protocol-only in serve mode. Send diagnostics to stderr. Flush after every ACK/ERR. Never print an ACK before `fsync` returns successfully and the index is published. A broken pipe means the client disappeared; stop emitting responses and allow process termination. Do not claim the caller received an ACK if it was only buffered in userspace.

Acceptance tests: all eight commands; invalid arguments; missing path; empty value; binary key/value in serve mode; duplicate request IDs; malformed hex; truncated batch input; unknown command; disk-full error; deterministic sorted dump; exact ACK matching; no debug text on stdout; and a clean offline run from repository root.

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

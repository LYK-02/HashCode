# Serve Mode

> Status: **Specification / implementation target**  
> Owner: engine team  
> Canonical filename: `15_SERVE_MODE.md`

## 1. Purpose and scope

Defines long-running stdin/stdout protocol, multiplexing, ACK durability, and shutdown.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Serve mode is the public interface used by the torture tool and judge to exercise the engine as a child process. Start the engine once, acquire the permanent database lock, recover, then read requests from stdin. Keep the process alive until EOF, fatal I/O error, or external kill. Do not require a network socket.

## 4. Design and procedure

Protocol request IDs correlate responses to operations. Recommended grammar: `PUT id key_hex value_hex`; `DEL id key_hex`; `BATCH id count <encoded-operation-list>`. Responses: `ACK id` only after successful sync and index publication; `ERR id CODE message` for failure. `GET` can be added for read tests, with a deterministic value response grammar. Freeze exact grammar in the common protocol document and use the same parser in all tests.

## 5. Edge cases and failure behaviour

Use one goroutine to read stdin and dispatch parsed requests, or a worker pool that calls engine methods safely. If multiple writers are used, the Python client must lock writes to stdin and use one dedicated stdout reader to dispatch responses by ID. A worker must never independently read the shared stdout pipe. Bound outstanding requests and output buffering. Flush each response line.

## 6. Verification and acceptance tests

ACK is a promise. The engine must call Sync and publish the entire operation before the serve loop emits ACK. If the process is killed after Sync but before ACK reaches the client, the operation may still appear after restart; oracle treats it as in-flight/uncertain. If ACK reaches client, oracle records it as committed. Do not ACK on write-only success.

Additional implementation notes:

On EOF, stop accepting new requests, wait for any already accepted requests according to a documented policy, close engine cleanly, and exit. The torture tool usually kills with SIGKILL, so no cleanup can be assumed. Handle broken stdout pipe without corrupting the database. Keep stdout protocol-only; log errors and diagnostics to stderr.

Tests: concurrent requests; request ID routing; delayed ACK; stdout flush; malformed request; duplicate ID; huge line; child exit; broken pipe; EOF; SIGKILL during operation; and ACK-vs-dump oracle comparison.

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

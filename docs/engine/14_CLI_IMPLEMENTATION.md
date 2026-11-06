# CLI Implementation

> Status: **IMPLEMENTED & VERIFIED (Milestone M4 / Phase E6)**  
> Owner: engine team  
> Canonical filename: `14_CLI_IMPLEMENTATION.md`  
> Implementation: `cmd/kvdb/main.go` (8 commands: `open`, `put`, `get`, `del`, `batch`, `dump`, `check`, `serve`)  
> Verification Suite: `tests/e2e/tier1_test.go`, `cmd/kvdb/main.go` (100% PASS)

## 1. Purpose and scope

Implementation rules for the eight judge-facing commands and output stability.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Keep CLI parsing separate from engine logic. Commands must match the official spellings and positional order. `open` opens/creates and recovers; `put` writes; `get` returns value or NOTFOUND; `del` deletes; `batch` reads a file with one operation per line; `dump` emits all live pairs sorted by key; `check` validates the file; `serve` processes a stream until EOF or kill.

## 4. Design and procedure

Exit codes: zero for success; nonzero for invalid arguments, I/O, lock, corruption, and disk-full errors. Print concise human diagnostics to stderr. Keep output deterministic for judge scripts. `open` reports key count and recovery duration. `check` prints exactly `OK` on success; on failure include stable code and offset. `dump` must be deterministic and binary-safe; use hex output or a documented escaping format rather than raw arbitrary bytes separated by tabs.

## 5. Edge cases and failure behaviour

Batch file grammar: line-oriented `put k v` and `del k`; document how spaces/empty values/binary bytes are represented. A simple whitespace grammar cannot represent arbitrary binary data, so this command may be a human-readable convenience interface while serve mode is binary-safe. Reject malformed lines with line number and do not commit any partial batch. Parse entire file into validated operations, then call one Batch API.

## 6. Verification and acceptance tests

Serve mode: stdout contains only ACK/ERR protocol lines. Diagnostics go to stderr. Flush each response. Request IDs must be echoed exactly. Parse request fully before calling engine. A malformed request returns ERR if an ID can be recovered; otherwise report to stderr and continue/terminate according to a documented framing policy. Do not print startup banners to stdout.

Additional implementation notes:

The CLI must not open and close the database for every operation in serve mode; open once and retain engine lock. One-shot commands may open, operate, close. Always close on normal completion and surface close errors. Do not depend on network, shell utilities, or package downloads at runtime.

Tests: every exact command; missing args; extra args; spaces; empty values; binary serve payload; large value; large batch file; sorted dump; invalid path; locked path; disk-full; stdout purity; and command exit codes.

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

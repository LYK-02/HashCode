# Engine Testing and Benchmarks

> Status: **Specification / implementation target**  
> Owner: engine team  
> Canonical filename: `16_ENGINE_TESTING_AND_BENCHMARKS.md`

## 1. Purpose and scope

Defines unit, property, fault-injection, performance, and release tests.

## 2. Source of truth

Derived from the supplied official problem statement and engineering plans. The organiser's `Kill It and Prove It` specification takes precedence over proposed implementation details. The selected implementation plan is Go for the engine and Python for the torture tool.

## 3. Requirements

Test the codec independently of filesystem code. Use table-driven tests and fuzzing for header/frame decoding. Use a temporary real filesystem directory for persistence tests; the official tool should not rely only on RAM disks because page-cache behaviour can hide persistence assumptions.

## 4. Design and procedure

Unit suites: header encode/decode; CRC32C vectors; frame encode/decode; malformed length rejection; operation bounds; batch order; locator offsets; empty value; binary values; sequence monotonicity; quota arithmetic; and error classification. Fuzz input must never panic, allocate unbounded memory, or apply partial operations.

## 5. Edge cases and failure behaviour

Persistence suites: put/get/reopen; overwrite/reopen; delete/reopen; batch/reopen; kill before/during/after append; sync-before-ACK; torn tail at every byte boundary; one-byte truncation; recovery crash; compaction crash at every barrier; orphan temp; lock release after process death; and disk-full. Use test hooks only for deterministic unit/integration coverage; final torture must still use genuine random millisecond delays.

## 6. Verification and acceptance tests

Concurrency suites: run Go race detector; multiple writers; same-key races; reader during batch; compact concurrent with writes; two processes opening same path; and kill while lock held. Assert only actual values appear, every ACK survives, and batches are all-or-nothing.

Additional implementation notes:

Benchmarks: recovery at 1M keys under two seconds; memory use; bytes scanned; map allocations; append latency; large batch encoding; large value write; compaction throughput; and file shrink ratio. Record laptop CPU, RAM, Go version, filesystem and cold/warm cache conditions. Official threshold is under two seconds; do not report a stretch target as an official requirement.

Release benchmark procedure: build optimized binary; create deterministic million-key dataset; close; kill or restart; measure open; run check; compare dump hash to expected; repeat at least five times; report median and max. If time exceeds two seconds, profile before applying micro-optimizations. Correctness and bounded parsing outrank speed.

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

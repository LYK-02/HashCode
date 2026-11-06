# E2E Test Infra: Go Storage Engine & CLI (`kvdb`)

## Test Philosophy
- **Opaque-box & Requirement-driven**: Tests interact solely via the judge-facing binary `./kvdb` and its documented subcommands (`open`, `put`, `get`, `del`, `batch`, `dump`, `check`, `serve`). Zero reliance on internal package internals.
- **Methodology**: 4-tier systematic approach combining Category-Partition, Boundary Value Analysis (BVA), Pairwise Combinatorial interaction testing, and Real-World Workload testing.

## Feature Inventory & Test Matrix
| # | Feature | Source | Tier 1 (Feature) | Tier 2 (Boundary) | Tier 3 (Pairwise) | Tier 4 (Real-World) |
|---|---------|--------|:----------------:|:-----------------:|:-----------------:|:-------------------:|
| 1 | Database Header & Lifecycle (`open`) | `03_BINARY_FILE_FORMAT.md` | 5 | 5 | ✓ | ✓ |
| 2 | Exclusive Lock (`data.db.lock`) | `11_CONCURRENCY_AND_LOCKING.md` | 5 | 5 | ✓ | ✓ |
| 3 | Single Write (`put`) | `06_PUT_GET_DELETE.md` | 5 | 5 | ✓ | ✓ |
| 4 | Single Read (`get`) | `06_PUT_GET_DELETE.md` | 5 | 5 | ✓ | ✓ |
| 5 | Single Delete (`del`) | `06_PUT_GET_DELETE.md` | 5 | 5 | ✓ | ✓ |
| 6 | Atomic Batch (`batch`) | `07_BATCH_ATOMICITY.md` | 5 | 5 | ✓ | ✓ |
| 7 | Full Dump (`dump`) | `05_CLI_AND_PROTOCOL_SPEC.md` | 5 | 5 | ✓ | ✓ |
| 8 | Integrity Check (`check`) | `05_CLI_AND_PROTOCOL_SPEC.md` | 5 | 5 | ✓ | ✓ |
| 9 | Streaming Protocol (`serve`) | `15_SERVE_MODE.md` | 5 | 5 | ✓ | ✓ |
| 10 | Crash-Safe Recovery | `08_CRASH_RECOVERY.md` | 5 | 5 | ✓ | ✓ |
| 11 | Logical Quota Guard (`--max-bytes`) | `12_DISK_FULL_AND_ERROR_HANDLING.md` | 5 | 5 | ✓ | ✓ |

## Test Architecture
- **Location**: `tests/e2e/`
- **Runner**: Shell / Go runner script invoked via `make test-e2e` or `go test ./tests/e2e/...`
- **Pass/Fail Semantics**: Exit code 0 on all assertions passing; nonzero with instant diagnostic diff on assertion failure.
- **Protocol Isolation**: stdout must contain only protocol tokens; stderr receives diagnostics.

## Real-World Application Scenarios (Tier 4)
| # | Scenario | Features Exercised | Complexity |
|---|----------|--------------------|------------|
| 1 | High-throughput Key-Value Ingestion with Random SIGKILLs | PUT, GET, Recovery, Sync Barrier | High |
| 2 | Financial Transaction Ledger Batch Commit & Rollback | BATCH, Atomic Replay, KeyDir Index | High |
| 3 | Cache Eviction & Compaction Cycle with Deleted Keys | PUT, DEL, Compaction, Dump | High |
| 4 | Multi-process Lock Contention & Failover on Kill | Open, Flock Sidecar, Process Kill | Medium |
| 5 | Massive 1M Keys Recovery & Checksum Verification | Open, Check, Recovery Bench ($\le 2.0$s) | Very High |

## Coverage Thresholds
- **Tier 1**: $\ge 55$ test cases ($\ge 5$ per feature across 11 features)
- **Tier 2**: $\ge 55$ test cases ($\ge 5$ boundary/corner cases per feature)
- **Tier 3**: $\ge 15$ cross-feature pairwise combination test cases
- **Tier 4**: $\ge 5$ comprehensive application scenarios
- **Total Minimum**: $\ge 130$ test cases

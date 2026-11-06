# Project: Go Storage Engine & Judge-Facing CLI (`./kvdb`)

## Architecture
- **Language**: Go 1.27+ (standard library only; zero banned persistence libraries).
- **Core Storage Model**: Single authoritative append-only data file (`data.db`) containing a 32-byte DB header followed by contiguous Universal Atomic Batch Entry (UABE) frames.
- **Process Concurrency**: Permanent sidecar lock file (`data.db.lock`) locked exclusively via `flock` from `open` through `close`.
- **In-Memory Lookup**: High-performance in-memory index (`KeyDir`) mapping keys to `(fileOffset, valueLen, seqNo)`.
- **Durability Boundary**: Strict 10-step write protocol where `file.Sync()` (`fsync`) physically completes before in-memory `KeyDir` publication and stdout `ACK <op-id>` emission.
- **Crash Recovery**: Sequential scan from byte 32 to EOF; automatic idempotent truncation and repair of torn tail frames at EOF; strict fail-closed rejection of non-tail corruption (`CORRUPT_FRAME`).
- **Compaction**: Atomic rewrite to `<path>.compact.tmp`, temp `Sync()`, atomic `os.Rename`, and parent directory `Sync()`.
- **CLI & Protocol**: Judge-facing executable `./kvdb` supporting `open`, `put`, `get`, `del`, `batch`, `dump`, `check`, and `serve` with stdout protocol isolation.

## Code Layout
```
/home/laik/workspace/srinathonn/
├── go.mod
├── Makefile
├── cmd/
│   └── kvdb/
│       └── main.go                  # CLI entrypoint for all 8 subcommands
├── internal/
│   ├── format/
│   │   ├── header.go                # 32-byte DB Header codec & Castagnoli CRC32C
│   │   ├── uabe.go                  # 28-byte UABE Frame Header codec & payload CRC32C trailer
│   │   ├── record.go                # PUT and DEL subrecords serialization
│   │   └── errors.go                # Format error definitions
│   ├── engine/
│   │   ├── engine.go                # Engine struct, Open(), Close(), RWMutex
│   │   ├── lock.go                  # flock lifecycle on data.db.lock
│   │   ├── write.go                 # 10-step write protocol, append loop, fsync barrier
│   │   ├── read.go                  # ReadAt locator lookup, zero-seek reads
│   │   ├── keydir.go                # High-speed in-memory KeyDir index
│   │   ├── batch.go                 # Atomic batch execution (1..10,000 ops)
│   │   ├── recovery.go              # Startup log replay, torn tail truncation at EOF
│   │   ├── compaction.go            # Crash-safe compaction via .compact.tmp & rename
│   │   └── quota.go                 # Logical max_bytes guard & disk-full rollback
│   ├── protocol/
│   │   └── serve.go                 # Stdin/stdout streaming serve mode protocol
│   └── testutil/
│       └── fixtures.go              # Golden binary test fixtures & generators
└── tests/
    └── e2e/                         # E2E opaque-box test suite (Tiers 1-4)
```

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| 1 | Database File Header | 32-byte fixed header at offset 0 (`KVDB2026`, version 1, CRC32C) | M1 | `docs/engine/03_BINARY_FILE_FORMAT.md` |
| 2 | UABE Frame Codec | 28B header, variable payload, 4B Castagnoli CRC32C trailer | M1 | `docs/common/04_SHARED_DATA_FORMAT.md` |
| 3 | Dual CRC32C Validation | Header CRC over bytes 8..27, Trailer CRC over payload bytes | M1 | `docs/engine/04_RECORD_ENCODING_AND_CHECKSUMS.md` |
| 4 | Go Module & Package Setup | Clean module initialization with standard library packages only | M1 | `docs/engine/PHASES.md § E0` |
| 5 | Process Exclusive Lock | Permanent `<db>.lock` sidecar locked with `flock` | M2 | `docs/engine/11_CONCURRENCY_AND_LOCKING.md` |
| 6 | Single File Storage | Single authoritative data file; zero external WAL or auxiliary logs | M2 | `.agents/rules/01_invariants.md` |
| 7 | Durability Sync Barrier | Physical `file.Sync()` before KeyDir update and ACK emission | M2 | `docs/engine/05_WRITE_AND_COMMIT_PROTOCOL.md` |
| 8 | In-Memory KeyDir Index | O(1) in-memory index mapping keys to file locators | M2 | `docs/engine/13_IN_MEMORY_INDEX.md` |
| 9 | PUT Operation | Appends UABE frame, updates KeyDir, handles empty & 1MB values | M2 | `docs/engine/06_PUT_GET_DELETE.md` |
| 10 | GET Operation | Zero-seek `ReadAt` lookup from KeyDir locator | M2 | `docs/engine/06_PUT_GET_DELETE.md` |
| 11 | DEL Operation | Appends tombstone frame (`value_len=0`), removes from KeyDir | M2 | `docs/engine/06_PUT_GET_DELETE.md` |
| 12 | Atomic Batch Mutation | 1..10,000 ops inside single contiguous frame (all-or-nothing) | M3 | `docs/engine/07_BATCH_ATOMICITY.md` |
| 13 | Startup Log Replay | Sequential frame scanner from byte 32 to validEOF | M3 | `docs/engine/08_CRASH_RECOVERY.md` |
| 14 | Torn Tail Repair at EOF | Truncates invalid final frame at EOF to validEOF and syncs | M3 | `docs/engine/09_TORN_WRITE_HANDLING.md` |
| 15 | Fail-Closed Corruption | Rejects corrupt middle-of-file frames with `CORRUPT_FRAME` | M3 | `docs/engine/09_TORN_WRITE_HANDLING.md` |
| 16 | Judge CLI (`./kvdb`) | 8 subcommands: `open`, `put`, `get`, `del`, `batch`, `dump`, `check`, `serve` | M4 | `docs/common/05_CLI_AND_PROTOCOL_SPEC.md` |
| 17 | Streaming Serve Protocol | Line-oriented stdin/stdout multiplexed engine with instant flush | M4 | `docs/engine/15_SERVE_MODE.md` |
| 18 | Logical Quota Guard | `max_bytes` checked before write; returns `DISK_FULL` without state change | M4 | `docs/engine/12_DISK_FULL_AND_ERROR_HANDLING.md` |
| 19 | Short Write Rollback | Rollback partial appends to `lastSyncedEOF` and sync | M4 | `docs/engine/12_DISK_FULL_AND_ERROR_HANDLING.md` |
| 20 | Crash-Safe Compaction | Atomic temp rewrite via `.compact.tmp`, rename, and directory sync | M5 | `docs/engine/10_COMPACTION.md` |
| 21 | Sub-2.0s Recovery (1M keys) | High-speed sequential scanner with buffered I/O ($\le 2.0$s) | M5 | `.agents/rules/01_invariants.md` |
| 22 | 32 Crash Scenarios Matrix | Verification against all 32 scenarios in Groups A through F | M5 | `.agents/rules/03_scenarios_matrix.md` |
| 23 | Banned Library Compliance | 100% offline standard-library-only execution | All | `.agents/rules/02_banned_libraries.md` |
| 24 | E2E Acceptance & Adversarial | 100% pass on Tiers 1-4 + Tier 5 Challenger adversarial hardening | M_FINAL | Project Pattern Dual Track |

## Milestones
| # | Name | Scope (Phases) | Dependencies | Status | Output Artifacts |
|---|------|----------------|-------------|--------|------------------|
| M1 | Core Foundation & Codecs | Phase E0 (Contract & Packages) + E1 (DB & UABE Codecs) | none | DONE | `go.mod`, `Makefile`, `internal/format/*`, codec tests |
| M2 | Lifecycle, Locking & Single Ops | Phase E2 (Lifecycle & Locks) + E3 (Append & Single Ops) | M1 | DONE | `internal/engine/{engine,lock,write,read,keydir}.go` |
| M3 | Batch Atomicity & Crash Recovery | Phase E4 (Atomic Batch) + E5 (Recovery & Tail Repair) | M2 | DONE | `internal/engine/{batch,recovery}.go`, recovery tests |
| M4 | CLI, Protocol & Error Hardening | Phase E6 (Serve & CLI) + E7 (Disk Full & Hardening) | M3 | PLANNED | `cmd/kvdb/*`, `internal/engine/quota.go`, `internal/protocol/*` |
| M5 | Compaction, Scale & Matrix Hardening | Phase E8 (Compaction) + E9 (1M Scale) + E10 (Release) | M4 | PLANNED | `internal/engine/compaction.go`, benchmark tests, matrix validation |
| M_FINAL | E2E Acceptance & Adversarial Hardening | Phase 1 (100% E2E Tiers 1-4) + Phase 2 (Tier 5 Adversarial) | M5 & E2E Track | PLANNED | Verified `./kvdb` release binary, test reports, `make demo-ready` |

## Interface Contracts

### internal/format ↔ internal/engine
- `EncodeHeader(h *Header) ([]byte, error)`: Serializes 32-byte header with Castagnoli CRC32C.
- `DecodeHeader(b []byte) (*Header, error)`: Validates magic `"KVDB2026"`, version `1`, and CRC32C.
- `EncodeFrame(seqNo uint64, ops []Operation) ([]byte, error)`: Encodes 28-byte UABE header, payload subrecords, and 4-byte Castagnoli trailer.
- `DecodeFrameHeader(b []byte) (*FrameHeader, error)`: Validates `0x55414245` magic and header CRC32C over bytes 8..27.
- `DecodePayloadAndTrailer(payload []byte, trailer []byte) ([]Operation, error)`: Validates trailer CRC32C over payload and parses operations.

### internal/engine ↔ cmd/kvdb
- `Open(path string, opts Options) (*Engine, error)`: Acquires flock on `<path>.lock`, recovers log, returns Engine.
- `(e *Engine) Close() error`: Flushes, syncs, closes data file, and releases flock.
- `(e *Engine) Put(key, val []byte) error`: Enforces write protocol; returns nil strictly after fsync.
- `(e *Engine) Get(key []byte) ([]byte, bool, error)`: Reads value via ReadAt; returns found flag.
- `(e *Engine) Delete(key []byte) error`: Appends tombstone frame; returns nil after fsync.
- `(e *Engine) Batch(ops []Operation) error`: Commits 1..10,000 ops atomically in single UABE frame.
- `(e *Engine) Dump() ([]Pair, error)`: Returns all live key/value pairs sorted lexicographically.
- `(e *Engine) Compact() error`: Performs crash-safe atomic compaction via `<path>.compact.tmp`.
- `(e *Engine) Check() error`: Scans header and all frames sequentially; reports any corruption.

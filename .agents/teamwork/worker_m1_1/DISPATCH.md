## 2026-11-06T12:04:07Z
MANDATORY FIRST STEP: Read `/home/laik/workspace/srinathonn/.agents/teamwork/ORIGINAL_REQUEST.md` before starting work.

Context: Milestone M1 (Core Foundation & Codecs: Phases E0 and E1).
Scope Document: `/home/laik/workspace/srinathonn/.agents/teamwork/PROJECT.md`
Explorer Findings & Specifications to Follow:
- `/home/laik/workspace/srinathonn/.agents/teamwork/explorer_m1_1/handoff.md` (Scaffolding, Makefile, layout)
- `/home/laik/workspace/srinathonn/.agents/teamwork/explorer_m1_2/handoff.md` (Byte-level codecs, CRC32C, errors)
- `/home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_m1_3/handoff.md` (Golden hex fixtures, unit tests, corruption vectors)

Working directory: `/home/laik/workspace/srinathonn/.agents/teamwork/worker_m1_1/`
Identity: `worker_m1_1`

MANDATORY INTEGRITY WARNING:
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

Write Ownership (You exclusively own and may create/modify these files):
- `go.mod`
- `Makefile`
- `.gitignore`
- `internal/format/header.go`
- `internal/format/uabe.go`
- `internal/format/record.go`
- `internal/format/errors.go`
- `internal/format/codec_test.go`
- `internal/testutil/fixtures.go`
- `internal/engine/engine.go` (initial skeleton / types)
- `cmd/kvdb/main.go` (initial skeleton / CLI router)

Implementation Requirements:
1. Initialize Go module `kvdb` in `go.mod` (Go 1.27, zero external dependencies).
2. Create `Makefile` with targets: `all`, `build`, `test`, `clean`, `demo-ready` using standard library compilation (`CGO_ENABLED=0`).
3. Create `.gitignore` ignoring build binaries (`kvdb`, `torture`), test database files (`*.db`, `*.lock`, `*.wal`, `*.tmp`), and test logs.
4. Implement `internal/format`:
   - `Header`: 32-byte header, Magic `"KVDB2026"`, Version `1`, CompactionGen, Reserved, Castagnoli CRC32C over bytes 0..27.
   - `FrameHeader`: 28-byte UABE header, Magic `0x55414245`, Header CRC32C over bytes 8..27, PayloadLength, SequenceNumber, OpCount (1..10,000).
   - `Operation`: PUT (`0x01`), DEL (`0x02`), Key (1..1024 bytes), Value (0..1MB, strictly 0 for DEL).
   - `Trailer`: 4-byte Castagnoli CRC32C trailer over payload bytes.
   - Granular typed errors in `internal/format/errors.go` (zero panics/unwraps).
5. Implement unit tests in `internal/format/codec_test.go` verifying:
   - Golden hex fixture byte-for-byte matching (from `spec_miner_m1_3/handoff.md`).
   - Boundary checks (keys, values, op count, DEL value len).
   - Bit-flip corruption detection (header magic, version, header CRC, payload CRC).
   - Anti-OOM memory bounds on untrusted payload lengths.
6. Verify offline build and tests:
   - Run `go test -v ./internal/format/...`
   - Run `go build -o kvdb ./cmd/kvdb`
   - Verify all tests pass with 0 failures.

Deliverable:
Write a comprehensive completion report to `/home/laik/workspace/srinathonn/.agents/teamwork/worker_m1_1/handoff.md`.
Document the exact build and test commands run and their full outputs.
When done, message your caller with a summary and the report path.

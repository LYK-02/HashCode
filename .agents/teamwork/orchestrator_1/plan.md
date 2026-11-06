# Orchestration Plan: Go Storage Engine & Judge-Facing CLI (`./kvdb`)

## Objective
Build, verify, and deliver the 100% durable, crash-proof Go storage engine and judge-facing CLI `./kvdb` compliant with all Hackathon invariants, 32 crash scenarios, UABE binary format, flock concurrency, <= 2.0s recovery, and standard-library-only offline requirements.

## Phases

### Phase 0: Survey & Scope Mapping
- Dispatch 3 parallel exploratory agents:
  1. `teamwork_spec_miner` (spec_miner_1): Mine `docs/` specifications (binary format, phases E0-E10, write protocol, CLI & protocol spec).
  2. `teamwork_spec_miner` (spec_miner_2): Mine `.agents/rules/` hackathon invariants (32 crash scenarios, banned libraries, deliverables, scoring).
  3. `teamwork_explorer` (explorer_3): Explore current workspace layout, Go setup, git pull status, existing skeletons/code, Makefile, and test fixtures.
- Merge survey reports into `PROJECT.md` (Feature Inventory, Architecture, Milestone Decomposition E0-E10, Interface Contracts, Code Layout).
- Create `TEST_INFRA.md` and spin up E2E Testing Track Orchestrator in parallel.

### Phase 1: Dual Track Execution
- **E2E Testing Track**: Autonomous test suite development (Tiers 1-4: Feature, Boundary, Pairwise, Real-world). Outputs `TEST_READY.md`.
- **Implementation Track**: Phase-by-phase implementation (E0 through E10 per `docs/engine/PHASES.md`):
  - Milestone E0: Contract & package setup, Go module initialization.
  - Milestone E1: DB Header & UABE Frame codec with Castagnoli CRC32C.
  - Milestone E2: Single file storage & flock-based exclusive lock (`data.db.lock`).
  - Milestone E3: In-memory index (KeyDir) & basic durability (write + Sync + KeyDir publish).
  - Milestone E4: Atomic batch execution & crash-recovery replay with torn tail truncation.
  - Milestone E5: Read path, iteration, streaming get/dump operations.
  - Milestone E6: Crash-safe compaction (atomic temporary file rename).
  - Milestone E7: Judge-facing CLI `./kvdb` (open, put, get, del, batch, dump, check, serve).
  - Milestone E8: UNIX domain socket / stdin server protocol streaming.
  - Milestone E9: 1M keys benchmark & <= 2.0s recovery optimization.
  - Milestone E10: Release verification against 32 crash scenarios matrix.
- Each milestone executes the strict gate cycle:
  `Explorer -> Worker -> Reviewers (2) -> Challengers (2) -> Forensic Auditor -> Gate Verdict`

### Phase 2: Final Acceptance & Adversarial Hardening
- Pass 100% E2E test suite (Tiers 1-4).
- Tier 5 Adversarial Coverage Hardening with Challengers.
- Complete live demo verification (`make demo-ready`, `DEMO.md`, `MEMORY.md`, `PROMPTS.md`).

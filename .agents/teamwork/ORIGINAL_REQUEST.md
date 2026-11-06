# Original User Request

## Approved Task Prompt — 2026-11-06T11:24:47Z

Implement the Go storage engine and judge-facing CLI (`./kvdb`) as a showcase for the "Kill It and Prove It" Hackathon judging, following all specifications in `docs/` and invariants in `.agents/rules/`.

Working directory: /home/laik/workspace/srinathonn
Integrity mode: demo
The team must NOT: copy code from existing open-source projects or database engines for core logic; use pre-built libraries, frameworks, or external persistence engines/WALs for core functionality (standard library only); run external scripts or delegate execution to other tools.

## Requirements

### R1. Phase-by-Phase Engine Implementation
Implement the Go storage engine phase by phase, one by one, adhering to the phase sequence in `docs/engine/PHASES.md` starting from E0 (Contract and package setup) through E10 (Release verification). Satisfy and verify each phase's exit gates before moving to the next phase.

### R2. Binary File Format and Storage Layout
Strictly adhere to `docs/common/04_SHARED_DATA_FORMAT.md` and `docs/engine/03_BINARY_FILE_FORMAT.md`:
- 32-byte DB Header (`KVDB2026`, version 1, Castagnoli CRC32C).
- 28-byte UABE Frame Header (`0x55414245`, header CRC32C, payload_len, seq_no, op_count).
- Operation subrecords (PUT/DEL) and 4-byte Castagnoli CRC32C trailer over the payload.
- Single authoritative `data.db` file with a permanent `data.db.lock` sidecar locked via `flock`.

### R3. Durability, Concurrency, and Error Hardening
Comply strictly with `.agents/rules/` and `docs/engine/05_WRITE_AND_COMMIT_PROTOCOL.md`:
- Guarantee 100% durability across SIGKILL: acknowledge writes strictly after successful file `Sync` and KeyDir publication.
- Guarantee atomic batches: all operations in a batch commit within a single frame or none at all.
- Replay valid frames on recovery, idempotently repair torn tails at EOF, and reject corrupted non-tail frames.
- Enforce clean error handling: handle short writes and disk full without false ACKs or state pollution; zero unwrap/panics.

### R4. CLI and Server Mode Interface
Deliver the judge-facing `./kvdb` executable implementing `open`, `put`, `get`, `del`, `batch`, `dump`, `check`, and `serve` commands conforming to `docs/common/05_CLI_AND_PROTOCOL_SPEC.md` with streaming support and clean stdout protocol responses.

## Acceptance Criteria

### Codec & Lifecycle Gates
- [ ] Binary encoding and decoding of DB header, UABE frames, and operations match golden fixtures byte-for-byte with Castagnoli CRC32C validation.
- [ ] Exclusive process-level locking via `data.db.lock` prevents concurrent engine instances and releases cleanly on SIGKILL.

### Operation & Durability Gates
- [ ] PUT, GET, and DEL operations persist across process restarts, and ACK is emitted only after `Sync` and in-memory index publication.
- [ ] Multi-operation batches are atomic: killed batches leave zero partial operations applied to disk or index.
- [ ] Recovery replays valid data, idempotently truncates torn tail frames at EOF, and detects corruption.
- [ ] Crash-safe compaction reclaims disk space using atomic temp-file rename without data loss.

### Performance & Quality Gates
- [ ] Single database recovery of 1,000,000 keys completes in $\le 2.0$ seconds.
- [ ] Clean build succeeds offline using only standard library packages (zero banned persistence/WAL dependencies).
- [ ] All 8 CLI commands (`open`, `put`, `get`, `del`, `batch`, `dump`, `check`, `serve`) function correctly according to specification.



## Follow-up — 2026-11-06T14:21:42Z

update memory and create branch and push and solve merge conflict if appear

# Comprehensive Rules, Invariants, Crash Scenarios & Deliverables Specification Report

**Document:** `handoff.md`  
**Miner / Investigator:** `spec_miner_survey_2`  
**Date:** 2026-11-06  
**Parent Conversation ID:** `9fade4e7-7ff3-4d78-9ecd-6f9b6c00f5be`  
**Working Directory:** `/home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_survey_2/`  
**Target Project:** Embedded Crash-Safe Key-Value Database (`./kvdb` & `./torture`)  
**Hackathon Event:** "Kill It and Prove It" 24-Hour Engineering Hackathon  

---

## 1. Observation

Direct extraction and verification from authoritative sources in `.agents/rules/`, `AGENTS.md`, `ORIGINAL_REQUEST.md`, `ARCHITECTURE.md`, `BREAKING.md`, `DEMO.md`, and `docs/common/`:

### 1.1 Authoritative Sources Inspected
- `AGENTS.md` (lines 1–33): Project participation in "Kill It and Prove It" 24-Hour Engineering Hackathon; lists authoritative rule modules `01_invariants.md` through `10_code_quality_and_practices.md`, deliverables, and persistent project state (`PROMPTS.md`, `MEMORY.md`, `DEMO.md`).
- `.agents/rules/01_invariants.md` (lines 1–37): The Core Invariant (5 points), Definition of "Returned Success", Definition of "Crash" (`kill -9` at any CPU instruction), and Mandatory Storage Constraints (Go + Python 3, single file `data.db` + sidecar `data.db.lock`, UABE frame model, write ordering, crash-safe compaction, `--max-bytes` quota guard, $\le 2.0$s recovery for 1,000,000 keys, key/val/batch limits).
- `.agents/rules/02_banned_libraries.md` (lines 1–22): Prohibited third-party engines/WALs/B-trees/LSMs, zero copied database code, zero pre-written code, 100% offline execution, prohibition on rigged torture tools.
- `.agents/rules/03_scenarios_matrix.md` (lines 1–49): Complete 32-scenario crash matrix (Groups A through F).
- `.agents/rules/04_deliverables_and_cli.md` (lines 1–45): Specifications for `./kvdb` (8 commands: `open`, `put`, `get`, `del`, `batch`, `dump`, `check`, `serve`), `./torture` (oracle, concurrent writers, random millisecond kill scheduler, targeted modes), `BREAKING.md` (5 required sections), `PROMPTS.md`, and `ARCHITECTURE.md` (3 parts).
- `.agents/rules/05_prompt_logging.md` (lines 1–17): Mandatory unedited chronological logging of all AI prompts with tool names and approximate timestamps; -10 point penalty for omission, disqualification for falsification.
- `.agents/rules/06_scoring_and_demo.md` (lines 1–43): 100-point scoring breakdown (Survives Kills: 30, Crash-Here: 20, Torture Quality: 15, BREAKING.md: 15, Architecture Diagram & Proof: 10, Design Extension: 10), deductions (-5s recovery, -5 compaction shrink, -10 disk-full corruption, -10 missing PROMPTS), disqualification rules, and 15-minute demo runbook.
- `.agents/rules/07_memory_protocol.md` (lines 1–16): Mandatory updating of `MEMORY.md` before tool execution tracking Date/Time, User, Implementer, What, Why, Fix/Actions, and Status.
- `.agents/rules/08_user_experience_and_tooling.md` (lines 1–40): Single-command build (`make demo-ready`), live terminal progress bar for `./torture`, instant failure diagnostics, human-readable `./kvdb check`, interactive dashboard.
- `.agents/rules/09_git_synchronization.md` (lines 1–11): Mandatory `git pull` before executing any task or code change; multi-member conflict prevention; offline demo isolation.
- `.agents/rules/10_code_quality_and_practices.md` (lines 1–73): Zero-panic/zero-unwrap mandate, granular error modeling, complete write loops, explicit fsync boundaries, memory bounds against OOM, Little-Endian binary layout, CRC32C checksums, modular package layout (`src/` in Go, `torture/` in Python).
- `.agents/teamwork/ORIGINAL_REQUEST.md` (lines 1–48): Task prompt defining Phase-by-phase implementation (E0–E10), binary file format, UABE frame structure, single authoritative file, standard library only.
- `ARCHITECTURE.md` (lines 1–352) & `BREAKING.md` (lines 1–77): Universal Atomic Batch Entry (UABE) format, dual CRC32C (Header CRC + Trailer CRC), lock file model, and the 12 critical crash points.

---

## 2. Core Invariants & Durability Guarantees

### 2.1 The Five Pillars of the Core Invariant
After a crash at any point in time (`kill -9` at any microsecond/instruction), and after `open()` completes recovery:
1. **Acknowledged writes are durable:** Every operation (`put`, `delete`, `batch`) that returned success (`ACK`) before the crash is present in the database.
2. **Unacknowledged writes are atomic:** Every operation that had not returned success at the moment of the crash is either fully present or fully absent.
3. **Batches are all-or-nothing:** Every batch is treated as one atomic operation. A batch is never partially present.
4. **Zero corruption:** No key ever holds garbage, torn values, or data from a different key.
5. **Guaranteed recovery:** `open()` never fails or crashes on a database file produced by this engine, regardless of how or when it was killed.

### 2.2 Definitions & Durability Boundary
- **Definition of "Returned Success":** Emitting an `ACK` token or returning `nil` from a mutation API is a binding physical promise. Durability is achieved **if and only if** `os.File.Sync()` (`fsync`) has successfully returned from the OS kernel. Returning success before physical `fsync` completes is a fatal invariant violation.
- **Definition of "Crash":** Immediate termination via `kill -9` (or `TerminateProcess`) with zero warning and no cleanup hooks. Kills can occur at any CPU instruction:
  - In the middle of `write()` / `pwrite()` (torn write);
  - In the middle of `fsync()`;
  - In the middle of renaming/swapping files during compaction;
  - In the middle of recovery replay or file truncation during the next `open()`.

### 2.3 Strict Write Ordering Protocol
Every mutation (`put`, `del`, or `batch`) must execute this non-reorderable 10-step protocol:
```
1. Input Validation: Validate key (1..1024B), value (0..1MB), batch ops (1..10,000)
2. Lock Acquisition: Acquire exclusive write lock (mu.Lock())
3. Frame Serialization: Encode atomic UABE frame into RAM buffer & compute Castagnoli CRC32Cs
4. Quota Guard: Verify valid_eof + frame_size <= max_bytes
5. Write Loop: Position-independent write (pwrite / WriteAt) at exact valid_eof until all bytes written
6. Durability Barrier: Call file.Sync() (syscall.Fsync) -> Hardware media flush
7. Index Publication: Apply operations to in-memory KeyDir in payload order
8. Metadata Advance: Advance valid_eof += frame_size and next_seq_no++
9. Lock Release: Release write lock (mu.Unlock())
10. ACK Token: Emit "ACK <op-id>" to stdout / return success
```

### 2.4 Single Authoritative File & Sidecar Locking
- **Single File Invariant:** All records, tombstones, and headers live in **exactly one** authoritative data file (`<path>`). No external WAL, separate transaction logs, auxiliary index files, or manifest files are permitted.
- **Sidecar Lock File (`<path>.lock`):** Process-level mutual exclusion is enforced using an OS-level exclusive advisory lock (`syscall.Flock(lockFd, syscall.LOCK_EX)`).
  - The sidecar lock file is **never unlinked, deleted, or renamed**.
  - Automatically released by the OS kernel upon `kill -9`.
  - Acquired **prior** to startup recovery to prevent multi-process recovery races.

### 2.5 Storage Limits & Sizing
- Keys: $1$ to $1,024$ bytes (arbitrary binary data).
- Values: $0$ to $1,048,576$ bytes ($1\text{ MB}$; arbitrary binary data, $0$ length allowed for PUT).
- Batches: $1$ to $10,000$ operations per batch frame.
- Database Scale: $\ge 1,000,000$ unique keys.
- Recovery Target: Recovery of $1,000,000$ keys must finish in $\le 2.0$ seconds.
- In-memory Index: KeyDir mapping `key -> (file_offset, val_len, seq_no)` consuming $\le 48\text{ MB}$ RAM for $1\text{M}$ keys.

---

## 3. The Complete 32 Crash Scenarios Matrix (Groups A through F)

Every single scenario must survive arbitrary `kill -9` without corruption, data loss of acknowledged writes, or unrecoverable states:

| ID | Group | Official Scenario Description | Physical Disk State at Moment of Kill | Engine Mechanism Handling It | Expected State After `open()` | Verification Method & Flags |
|---|---|---|---|---|---|---|
| **A1** | Single Write | Kill before any bytes of a `put` reach file | Previous committed log intact; 0 bytes appended to file | `open()` scans to previous `valid_eof`; uncommitted write absent | Key absent (or holds previous committed value); no error | `./torture --seed 101 --iterations 10` |
| **A2** | Single Write | Kill after some bytes of record written (torn write) | Partial frame bytes in OS buffer or disk; header/trailer CRC missing | Recovery scanner detects CRC32C failure or incomplete frame; truncates file to last `valid_eof` | Key absent or previous value; torn bytes discarded; zero corruption | `./torture --seed 102 --iterations 10` |
| **A3** | Single Write | Kill after record is fully written but before `fsync` | Frame bytes on disk or in OS page cache; no ACK sent to caller | Unacknowledged write: if bytes persisted, CRC validates; if lost, truncated to last `valid_eof` | Either old or new value is valid; zero corruption; file consistent | `./torture --seed 103 --iterations 10` |
| **A4** | Single Write | Kill after `fsync` returns but before `put` returns success | Frame fully durable on disk (CRC valid); caller never received ACK | Recovery verifies valid UABE frame & trailer CRC; applies to KeyDir | Key present with new value (durable even if caller never heard back) | `./torture --seed 104 --iterations 10` |
| **A5** | Single Write | Kill immediately after `put` returns success | Frame fully durable on physical disk; ACK was emitted | Recovery verifies valid frame and applies to KeyDir | Key present with new value; zero exceptions | `./torture --iterations 500 --seed 12345` |
| **A6** | Single Write | Kill during a `delete` | Tombstone frame either fully durable or incomplete | If durable, DEL tombstone applied; if incomplete, CRC fails and truncated | Key either absent or holds previous value; never corrupt | `./torture --seed 106 --iterations 10` |
| **A7** | Single Write | Kill during `put` of a 1 MB value | Multi-block partial write across filesystem boundaries | Trailer CRC32C covers all 1MB bytes; partial blocks fail CRC; truncated cleanly | Key absent or holds previous value; multi-block torn bytes ignored | `./torture --mode large-values --iterations 50 --seed 999` |
| **B1** | Batch | Kill after 3 of 10 operations in a batch written | Incomplete UABE frame on disk; Trailer CRC absent | Entire batch is inside ONE atomic frame; incomplete frame fails CRC | None of the 10 operations visible (0/10 applied) | `./torture --seed 201 --iterations 10` |
| **B2** | Batch | Kill after all 10 operations written but commit marker / trailer is not | 10 operation bytes written; 4-byte Trailer CRC absent | Frame validation fails missing trailer CRC; entire batch dropped | None of the 10 operations visible; previous state intact | `./torture --seed 202 --iterations 10` |
| **B3** | Batch | Kill after commit marker/trailer written but before `fsync` | Complete frame written; `fsync` unreturned | Either entire frame survives or drops on torn boundary; all-or-nothing | Either all 10 visible or none visible; never some (0/10 or 10/10) | `./torture --seed 203 --iterations 10` |
| **B4** | Batch | Kill after batch returns success | Full UABE frame durable on disk; ACK sent | Recovery scanner validates Frame & Trailer CRC; all 10 operations applied | All 10 operations visible | `./torture --iterations 500 --seed 12345` |
| **B5** | Batch | Batch puts key X and deletes key X in same batch | Frame contains both operations in payload body | Payload operations applied strictly in sequential subrecord order | Defined payload ordering honored; final DEL tombstone wins | Unit test: `TestBatchSameKey` & `./torture` |
| **B6** | Batch | Batch of 10,000 operations killed halfway | Partial multi-megabyte frame on disk | Trailer CRC check fails; batch dropped in toto; recovery completes in $\le 2.0$s | None of 10,000 visible; recovery completes in $< 2.0$s | `./torture --mode large-batches --iterations 50 --seed 206` |
| **C1** | Recovery | Kill during `open()` while replaying the log | Read-only scan in progress; file untouched | Replay is purely non-destructive in-memory scan; next `open()` re-scans | Next `open()` succeeds and reaches identical correct state | `./torture --mode recovery-crash --iterations 100 --seed 42` |
| **C2** | Recovery | Kill during `open()` while truncating or repairing torn tail | File truncated partially to new length | Next `open()` resumes scan; idempotent truncation to `valid_eof` succeeds | Next `open()` succeeds; torn tail completely truncated | `./torture --mode recovery-crash --iterations 100 --seed 42` |
| **C3** | Recovery | Kill during compaction before new file replaces old | Old file untouched; partial temp file `<path>.compact.tmp` on disk | Old file remains authoritative; orphan `.compact.tmp` cleaned on next `open()` | Old file authoritative, all data intact; orphan temp unlinked | `./torture --mode compaction --iterations 100 --seed 7` |
| **C4** | Recovery | Kill during compaction in middle of rename/swap | `os.Rename` either executed or pending | POSIX atomic rename: directory entry points to either old or new; both complete | Exactly one of old or new is authoritative; both complete; nothing lost | `./torture --mode compaction --iterations 100 --seed 7` |
| **C5** | Recovery | Kill during compaction after swap before cleanup of temp file | New file authoritative; orphan temp file remains | `open()` scans authoritative file, finds orphan temp, and calls `os.Remove()` | Data intact; temp file cleaned/ignored on next `open()` | `./torture --mode compaction --iterations 100 --seed 7` |
| **C6** | Recovery | Kill during `close()` | State identical to random kill during runtime | `close()` flushes and releases lock; kill before completion handled by standard recovery | Data intact; lock freed by OS; next `open()` succeeds | `./torture --seed 306 --iterations 10` |
| **D1** | Concurrency | Two writers putting different keys, killed at random | Frames interleaved; only fully fsynced frames valid | Each ACKed frame is independent and validated; all ACKed puts present | Every acknowledged put from either writer is present | `./torture --iterations 500 --seed 12345` ($\ge 2$ writers) |
| **D2** | Concurrency | Two writers putting same key, killed at random | Two frames for same key; last valid frame wins | KeyDir maps key to locator of highest valid sequence number | Key holds value that one writer actually wrote; never a mix of two | `./torture --seed 402 --iterations 100` |
| **D3** | Concurrency | One writer in long batch, another doing single puts, killed | Single put frames and batch frames serialized | `sync.RWMutex` serializes write calls; batch is atomic; ACKed puts present | Batch is all-or-nothing; all ACKed single puts are present | `./torture --seed 403 --iterations 100` |
| **D4** | Concurrency | Reader calling `get` during a batch on same keys (no crash) | In-memory KeyDir updated only after batch fsync under write lock | Reader holds `RLock`; KeyDir updated atomically under write lock | Reader sees state before batch or after batch, never in between | Unit test: `TestConcurrentRead` / concurrency torture |
| **E1** | Disk Full | File size cap reached during single `put` | `valid_eof + frame_size > max_bytes`; file untouched | Quota guard returns `ErrDiskFull`; write aborted; database readable | `put` returns clean error; database remains readable; previous data intact | `./torture --mode disk-full --max-bytes 65536 --seed 501` |
| **E2** | Disk Full | Cap reached halfway through a batch | Pre-write quota check fails before write | Quota check inspects entire batch size upfront; fails cleanly; 0 ops written | Batch returns clean error; none of its operations visible | `./torture --mode disk-full --max-bytes 65536 --seed 502` |
| **E3** | Disk Full | Cap reached, then process killed | File at or near cap; no uncommitted writes | Database opens cleanly; all acknowledged writes prior to cap present | Restart succeeds; all previously acknowledged data intact | `./torture --mode disk-full --max-bytes 65536 --seed 503` |
| **E4** | Disk Full | Cap reached during compaction | Temp file write exceeds quota | Compaction aborts; temp unlinked; original database file remains authoritative | Compaction fails or completes cleanly; no data lost; database opens | `./torture --mode disk-full --mode compaction --seed 504` |
| **E5** | Disk Full | After deletes free up space, compaction runs | Compaction purges tombstones; file shrinks | Database shrinks below cap; new puts succeed without manual intervention | Database usable again without manual intervention | Unit test: `TestCompactionShrink` & disk-full mode |
| **F1** | Repeated Abuse | 500 consecutive random kills and restarts | File repeatedly truncated to valid frames | Invariant verified after every single iteration; oracle match rate 100% | Invariant holds after every single iteration (zero corruption) | `./torture --iterations 500 --seed 12345` |
| **F2** | Repeated Abuse | Kill, restart, kill during recovery, restart, kill during recovery | Multiple aborted recovery passes | Recovery is idempotent; does not mutate durable log except truncating torn tail | Invariant holds; recovery is idempotent | `./torture --mode recovery-crash --iterations 100 --seed 42` |
| **F3** | Repeated Abuse | 1,000,000 keys written, process killed, `open()` called | 1,000,000 records in log file | Streaming 64KB buffered reader parses log and rebuilds index in $\le 2.0$s | Recovery completes in $\le 2.0$ seconds; all ACKed keys present | `go test -bench=BenchmarkRecovery1M` & `./kvdb open` |
| **F4** | Repeated Abuse | Database file truncated by 1 byte at end by hand | Last byte of trailer CRC chopped off | CRC check fails on last frame; scanner truncates to last valid frame | `open()` succeeds; at most last unacknowledged record lost | Unit test: `TestTruncatedByte` & `./kvdb check` |

---

## 4. Banned Libraries & Operational Rules

### 4.1 Strictly Prohibited (Immediate Disqualification / 0 Points, No Appeal)
- **Zero Third-Party Storage Engines / Persistence Libraries:**
  - SQLite, LevelDB, RocksDB, LMDB, BerkeleyDB, Redis, H2, Derby, BoltDB, Badger, DuckDB, or anything similar.
  - "If it stores keys and values for you, it is strictly banned."
- **Zero ORM or Key-Value Wrapper Libraries.**
- **Zero Third-Party WAL, Journaling, B-Tree, or LSM-Tree Libraries.**
- **Zero Source Code Copying:** Copying from existing database projects is strictly prohibited. Reading papers/specs is encouraged; pasting code causes disqualification.
- **Zero Pre-Written Code:** All code must be freshly authored during the hackathon.
- **Zero Network Access / Cloud Dependencies:** All code, tests, and demo runs must execute **100% offline without Wi-Fi**.
- **Zero Torture Rigging:** Torture tool must not kill only between operations or rely on the database's own dump as an oracle.
- **Zero Prompt / Git Falsification:** Deleting or doctoring `PROMPTS.md` or git history is grounds for immediate disqualification.

### 4.2 Permitted Technologies & Standard Library Capabilities
- **Engine & CLI:** Go ($\ge 1.22$) standard library only (`os`, `sync`, `syscall`, `hash/crc32`, `encoding/binary`, `bufio`, `io`, etc.).
- **Torture Harness:** Python 3 ($\ge 3.10$) standard library only (`subprocess`, `os`, `signal`, `time`, `random`, `json`, `hashlib`, `threading`, etc.).
- **Standard Toolchains:** Standard test harnesses (`go test`, `pytest`).
- **Standard Memory Mapping:** Standard library `mmap` is allowed if needed.
- **AI Tooling Permitted:** Claude, Gemini, ChatGPT, Copilot, Cursor are permitted provided all prompts are logged unedited in `PROMPTS.md`.

---

## 5. Deliverables & Judge-Facing Interfaces

### 5.1 Deliverable 1: Engine & CLI Wrapper (`./kvdb`)
Judge executes all 8 subcommands from repository root:
1. `./kvdb open <path> [--max-bytes N]`: Opens or creates database, executes crash recovery, outputs key count and recovery time.
2. `./kvdb put <path> <key> <value>`: Inserts/overwrites key-value pair.
3. `./kvdb get <path> <key>`: Prints value to stdout, or prints `NOTFOUND`.
4. `./kvdb del <path> <key>`: Deletes key (deleting nonexistent key is a no-op, not an error).
5. `./kvdb batch <path> <file>`: Reads file containing line-delimited `put <key> <value>` and `del <key>` operations; applies all-or-nothing atomically.
6. `./kvdb dump <path>`: Prints all keys and values, sorted lexicographically by key (`<key> <value>`).
7. `./kvdb check <path>`: Verifies complete file integrity and CRCs; prints `OK` (or human-readable summary) or precise failure reason.
8. `./kvdb serve <path>`: Long-running line-delimited IPC daemon over stdin/stdout.

#### Protocol in `serve` Mode:
- **Request Format:**
  - `PUT <op-id> <key-hex> <val-hex>`
  - `DEL <op-id> <key-hex>`
  - `BATCH <op-id> <count> <hex-encoded-ops>`
  - `GET <op-id> <key-hex>`
- **Response Format:**
  - `ACK <op-id>` (emitted strictly AFTER `file.Sync()` returns)
  - `VALUE <op-id> <val-hex>`
  - `NOTFOUND <op-id>`
  - `ERR <op-id> <error-code> <message>`
- **Stdout Rule:** In `serve` mode, stdout is protocol-only; debug and diagnostic messages must go to stderr.

### 5.2 Deliverable 2: Torture Tool (`./torture`)
Independent testing harness driving `./kvdb serve` as a child process:
- Standard Invocations:
  - `./torture --iterations 500 --seed 12345`
  - `./torture --iterations 100 --seed 999 --mode large-values`
  - `./torture --iterations 100 --seed 7 --mode compaction`
  - `./torture --iterations 100 --seed 42 --mode recovery-crash`
  - `./torture --iterations 100 --seed 501 --mode disk-full --max-bytes 65536`
- **Core Invariants of Torture Tool:**
  1. **Independent Oracle:** Writes its own verified ACK log to a separate file; never trusts `./kvdb dump` as ground truth.
  2. **Concurrent Writers:** $\ge 2$ concurrent worker threads streaming mutations.
  3. **Random Millisecond Kills:** Uses random millisecond delay before `kill -9` (never "after N operations").
  4. **Targeted Modes:** Exercises 1MB values, 10,000-op batches, compaction crashes, recovery crashes, and disk full.

### 5.3 Deliverable 3: `BREAKING.md` (Max 2 Pages)
Must contain all 5 required sections:
1. **Crash Points Table:** 8 to 15 rows with exact file, function line, disk state at crash, and what `open()` does on restart.
2. **The Ordering Argument:** Detailed proof explaining why `write`, `fsync`, and `rename` sequence guarantees the invariant, naming exact system calls (`pwrite`, `fsync`, `rename`, `flock`).
3. **Known Weaknesses:** At least three honest design weaknesses ($O(N)$ recovery scan without checkpointing, Go runtime GC overhead for 1M-key in-memory index, compaction requiring temporary disk headroom).
4. **What We Tried That Failed:** Real discarded attempts (Attempt 1: separate commit marker opcodes replaced by single-frame UABE; Attempt 2: database file flock replaced by permanent sidecar lock; Attempt 3: raw byte pipe streaming replaced by hex protocol).
5. **If We Had 24 More Hours:** Three prioritized improvements (SIMD/SSE4.2 hardware CRC32C, concurrent non-blocking compaction, embedded snapshot index checkpoints).

### 5.4 Deliverable 4: `PROMPTS.md`
- Chronological, unedited log of all AI prompts sent during the event.
- Includes AI tool name (e.g., `Antigravity (Gemini)`) and approximate timestamp.
- Verified before every turn completion. -10 point penalty if missing/empty.

### 5.5 Deliverable 5: `ARCHITECTURE.md`
- **Part 1 (Diagram):** High-level topology, component ownership, on-disk byte layout, numbered write ordering sequence, recovery flow, lock design.
- **Part 2 (Proof Table):** Complete 32-scenario matrix (Groups A through F) with disk state, handling mechanism, code location, and verified-by flag.
- **Part 3 (Three Guarantees Argued):** Durability, Atomicity, and Recoverability argued in plain English naming system calls.

---

## 6. Scoring Rubric, Deductions & 15-Minute Live Demo Protocol

### 6.1 100-Point Scoring Breakdown
| Category | Points | Evaluation Standard |
|---|:---:|---|
| **Survives Judge's Kill Runs** | **30** | Zero invariant violations across all torture runs and manual kills. **-10 per violation found**. Build failure = 0. |
| **Crash-Here Questions** | **20** | 3 spot-checked lines (7, 7, 6 pts). Immediate answer: exact physical disk state and exact `open()` action without running code. |
| **Torture Tool Quality** | **15** | Independent oracle (4), kills during operations not between them (4), concurrent writers & batches (4), targeted modes & seed reproducibility (3). |
| **BREAKING.md** | **15** | Crash points table accuracy (7), ordering argument (4), honest weaknesses and failed attempts (4). -2 per incorrect crash point. |
| **Architecture Diagram & Proof** | **10** | Diagram matches code (4), proof table complete on 3 spot-checked rows (4), 3 guarantees naming system calls (2). |
| **Design Extension Question** | **10** | Live architectural extension (e.g., Key TTL Expiry): format/recovery changes (5), subtle edge cases unprompted (4), clarity (1). |

### 6.2 Deductions & Disqualifications
- Recovery slower than $2.0$s at 1,000,000 keys: **-5 points**
- File does not shrink after deletes + compaction: **-5 points**
- Disk-full behavior corrupts data or breaks `open()`: **-10 points**
- Missing or empty `PROMPTS.md`: **-10 points**
- Any banned library / copied code / pre-written code / rigged torture: **0 points (Immediate Disqualification)**

### 6.3 15-Minute Live Judging Minute-by-Minute Schedule
- **0:00 – 1:00 (Build):** Judge runs single build command (`make build`) on clean clone. Must compile offline in $< 20$s.
- **1:00 – 4:00 (Standard Torture Run):** Judge runs `./torture --iterations 200 --seed <judge_seed>`. Must show live dashboard and zero failures.
- **4:00 – 6:00 (Targeted Modes):** Judge tests `./torture` in `compaction`, `recovery-crash`, and `large-values` modes.
- **6:00 – 8:00 (Manual Kills):** Judge starts `./kvdb serve`, sends batches, kills with `kill -9` 5–10 times manually, verifies with `./kvdb check` and `./kvdb dump`.
- **8:00 – 12:00 (Crash-Here Defense):** Judge points at 3 code lines in write/recovery paths. Team answers immediately using the 3-part template (Physical state $\to$ Detection $\to$ Recovery action).
- **12:00 – 15:00 (Design Extension Defense):** Judge asks hypothetical feature (TTL expiry). Team explains binary format changes, read/write/compaction changes, and subtle edge cases (clock skew, batch overrides).

---

## 7. Code Quality Mandates & Systems Engineering Standards

Per `.agents/rules/10_code_quality_and_practices.md`:
1. **Zero-Panic / Zero-Unwrap Invariant:**
   - No raw `.unwrap()`, `.expect()`, or unhandled `panic` in write, read, recovery, or compaction paths.
   - All fallible operations return explicit Go `error` / typed error values.
2. **Granular Error Taxonomy:**
   - Differentiate: `INVALID_ARGUMENT`, `KEY_TOO_LARGE`, `VALUE_TOO_LARGE`, `BATCH_TOO_LARGE`, `DISK_FULL`, `IO_WRITE`, `IO_SYNC`, `IO_TRUNCATE`, `IO_RENAME`, `IO_DIR_SYNC`, `CORRUPT_HEADER`, `CORRUPT_FRAME`, `LOCKED`, `CLOSED`, `ENGINE_POISONED`.
3. **Deterministic File I/O & Syscall Discipline:**
   - Loop on all writes until all slice bytes are transferred (`pwrite` loop).
   - Check every syscall return value (`read`, `write`, `fsync`, `truncate`, `rename`, `flock`).
   - Explicit `fsync` barriers: append $\to$ `fsync` $\to$ in-memory publish $\to$ `ACK`.
4. **Memory Safety & Guarded Allocations (Anti-OOM):**
   - Never allocate memory based on untrusted frame headers. Validate `PayloadLength <= 64MB` and remaining file size before buffer allocation.
   - KeyDir entries store only 64-bit file offset, 32-bit value length, and 64-bit sequence number—never duplicate values in RAM.
   - Use streaming readers with 64KB+ buffers to achieve $< 2.0$s recovery for 1,000,000 keys.
5. **Binary Layout Standards:**
   - Strictly Little-Endian for all multi-byte integers.
   - Dual Castagnoli CRC32C: Header CRC32C covers bytes 8..28; Trailer CRC32C covers payload body.
6. **Code Layout:**
   - Engine and CLI in Go under `src/`:
     - `src/main.go` (CLI subcommands & serve dispatcher)
     - `src/engine/engine.go` (High-level KVDB engine API, RWMutex)
     - `src/storage/log.go` (Append log, Pwrite, WriteAt, File.Sync)
     - `src/storage/frame.go` (UABE binary codec, dual CRC32C)
     - `src/index/keydir.go` (In-memory KeyDir hash index)
     - `src/recovery/recover.go` (Replay, tail truncation, temp pruning)
     - `src/compaction/compact.go` (Live state rewrite, atomic rename, directory fsync)
   - Independent torture test suite in Python under `torture/`:
     - `torture/torture.py` (CLI harness, progress dashboard)
     - `torture/oracle.py` (Independent journal & ACK verifier)
     - `torture/workload.py` (Concurrent mutation workers)
     - `torture/injector.py` (Random millisecond SIGKILL scheduler)
     - `torture/verifier.py` (Dump vs oracle verification engine)

---

## 8. Features Discovered

| # | Category | Feature | Description | Inputs | Outputs | Error Behavior | Discovered Via |
|---|----------|---------|-------------|--------|---------|----------------|----------------|
| 1 | Engine Storage | 32-Byte File Header | Fixed database header with magic bytes, version, generation, reserved flags, and Castagnoli CRC32C | First 32 bytes of `data.db` | Verified Header struct | Rejects corrupted header on `open()`; fails closed | `01_invariants.md`, `04_SHARED_DATA_FORMAT.md` |
| 2 | Engine Storage | Universal Atomic Batch Entry (UABE) | Self-contained atomic frame with 28B header, payload of 1..10,000 ops, and 4B trailer CRC32C | Frame header + ops payload | Serialized binary frame | Incomplete/torn frame fails CRC and gets truncated | `01_invariants.md`, `03_BINARY_FILE_FORMAT.md` |
| 3 | Engine Storage | Dual-CRC32C Protection | Separate CRCs for frame header (protecting bounds) and payload trailer (protecting operations) | Header bytes 8..28; Payload body | 2x uint32 Castagnoli CRC32C | Header CRC mismatch aborts scan; payload CRC mismatch marks torn write | `ARCHITECTURE.md`, `04_RECORD_ENCODING_AND_CHECKSUMS.md` |
| 4 | Engine Concurrency | Sidecar File Lock | Permanent `<path>.lock` file locked via exclusive advisory `flock` | File descriptor of `<path>.lock` | OS lock ownership | Returns error if locked; auto-freed by OS on `kill -9` | `01_invariants.md`, `11_CONCURRENCY_AND_LOCKING.md` |
| 5 | Engine Write Protocol | Hardware Durability Barrier | Position-independent write (`WriteAt`/`Pwrite`) followed by blocking `file.Sync()` prior to index publication or ACK | Frame bytes at `valid_eof` | Physical disk commit | Truncates to last synced EOF on write failure; marks poisoned on sync failure | `01_invariants.md`, `05_WRITE_AND_COMMIT_PROTOCOL.md` |
| 6 | Engine Quota | `--max-bytes` Quota Guard | Logical cap on authoritative database file size checked prior to I/O | `valid_eof + frame_size` vs `max_bytes` | Normal write or rejection | Rejects with `ErrDiskFull`; leaves database readable & intact | `01_invariants.md`, `12_DISK_FULL_AND_ERROR_HANDLING.md` |
| 7 | Engine Compaction | Copy-on-Write Compaction | Sweeps live keys to `<path>.compact.tmp`, fsyncs temp, atomically renames over target, fsyncs parent directory | Live KeyDir snapshot | Compacted database with incremented generation | Aborts cleanly on full disk; orphan temp unlinked on recovery | `01_invariants.md`, `10_COMPACTION.md` |
| 8 | Engine Recovery | Streaming Replay & Tail Truncation | 64KB buffered sequential scan from offset 32 to EOF; detects torn tails via CRC mismatch and truncates cleanly | Authoritative database file | Fully restored in-memory KeyDir ($\le 2.0$s for 1M keys) | Idempotent; truncates torn tail and unlinks orphan temp files | `01_invariants.md`, `08_CRASH_RECOVERY.md` |
| 9 | CLI Subcommand | `open` | Opens or creates database, runs recovery replay, outputs key count and recovery time in seconds | `<path> [--max-bytes N]` | Key count and elapsed recovery time | Prints explicit error and exits non-zero on lock collision or corruption | `04_deliverables_and_cli.md`, `05_CLI_AND_PROTOCOL_SPEC.md` |
| 10 | CLI Subcommand | `put` | Inserts or overwrites single key-value pair | `<path> <key> <value>` | Success status or error | Rejects invalid key/val sizes or disk full | `04_deliverables_and_cli.md`, `05_CLI_AND_PROTOCOL_SPEC.md` |
| 11 | CLI Subcommand | `get` | Retrieves value associated with key | `<path> <key>` | Raw value to stdout, or `NOTFOUND` | Returns non-zero or `NOTFOUND` if missing | `04_deliverables_and_cli.md`, `05_CLI_AND_PROTOCOL_SPEC.md` |
| 12 | CLI Subcommand | `del` | Deletes key by appending tombstone frame | `<path> <key>` | Success status | Missing key deletion is a no-op (not an error) | `04_deliverables_and_cli.md`, `05_CLI_AND_PROTOCOL_SPEC.md` |
| 13 | CLI Subcommand | `batch` | Applies atomic batch of puts/deletes from file | `<path> <file>` | Success status | All-or-nothing; rejects malformed lines or cap violation | `04_deliverables_and_cli.md`, `05_CLI_AND_PROTOCOL_SPEC.md` |
| 14 | CLI Subcommand | `dump` | Dumps all active keys and values sorted lexicographically by key | `<path>` | Sorted lines `<key> <value>` | Returns empty if DB empty | `04_deliverables_and_cli.md`, `05_CLI_AND_PROTOCOL_SPEC.md` |
| 15 | CLI Subcommand | `check` | Verifies file structure, header, and all UABE frame CRCs | `<path>` | `OK` summary or error reason | Reports exact byte offset and failure cause if corrupted | `04_deliverables_and_cli.md`, `05_CLI_AND_PROTOCOL_SPEC.md` |
| 16 | CLI Subcommand | `serve` | Long-running stdin/stdout IPC daemon for high-speed concurrent mutation | `<path>` | `ACK <op-id>`, `VALUE <op-id> <val>`, `NOTFOUND <op-id>` | Emits `ERR <op-id> <code> <msg>`; diagnostics to stderr | `04_deliverables_and_cli.md`, `05_CLI_AND_PROTOCOL_SPEC.md` |
| 17 | Torture Harness | Independent Oracle | Standalone testing harness maintaining its own append-only journal of ACKed mutations | Engine stdout stream | Pass/Fail diff report | Discrepancy between `./kvdb dump` and oracle halts test | `04_deliverables_and_cli.md`, `03_INDEPENDENT_ORACLE.md` |
| 18 | Torture Harness | Random Millisecond Kill Scheduler | Injects `kill -9` after genuinely random millisecond sleep intervals during active I/O | Seed, target PID | SIGKILL delivery | Preserves reproduction seed and artifacts on failure | `04_deliverables_and_cli.md`, `07_RANDOM_CRASH_INJECTION.md` |
| 19 | Torture Harness | Targeted Stress Modes | Specialized test modes: `large-values` (1MB), `large-batches` (10K ops), `compaction`, `recovery-crash`, `disk-full` | `--mode <name> --seed <N>` | Mode verification results | Fails immediately on any invariant break | `04_deliverables_and_cli.md`, `10_TORTURE_TEST_MODES.md` |
| 20 | Engineering Workflow | AI Prompt Logger | Hook-based unedited chronological logging of all user prompts to `PROMPTS.md` | Invocation events | Appended prompt entries in `PROMPTS.md` | -10 points if missing; disqualification if falsified | `05_prompt_logging.md`, `AGENTS.md` |
| 21 | Engineering Workflow | Memory Execution Journal | Persistent context synchronization in `MEMORY.md` | Prompt context & code modifications | Standardized structured journal entry | Avoids amnesia across agent invocations | `07_memory_protocol.md`, `AGENTS.md` |
| 22 | Engineering Workflow | Mandatory Git Synchronization | Mandatory `git pull` prior to executing any code or refactoring task | Local repo state | Synchronized workspace with `origin/main` | Prevents merge conflicts in multi-member team | `09_git_synchronization.md`, `AGENTS.md` |

---

## 9. Edge Cases Matrix

| # | Feature | Input / Condition | Observed / Required Behavior |
|---|---------|-------------------|-----------------------------|
| 1 | Single Put | Zero-length value (`value = ""`) | Valid PUT operation; stored as length 0 locator in KeyDir; `get` returns empty string (never `NOTFOUND`). |
| 2 | Single Put | 1 MB value spanning multiple disk blocks (Scenario A7) | Frame trailer CRC32C covers all 1MB bytes; partial block write fails CRC on restart; truncated cleanly. |
| 3 | Single Delete | Delete nonexistent key (Scenario A6) | No-op; returns success; appends tombstone or leaves state untouched without error. |
| 4 | Batch | Key PUT then DEL in same batch (Scenario B5) | Both operations serialized in payload; applied in payload order; DEL tombstone wins; key absent after recovery. |
| 5 | Batch | Empty batch file (0 operations) | Rejected before disk write with validation error; no empty frame written. |
| 6 | Batch | 10,000 operations killed at operation 5,000 (Scenario B6) | All operations inside single frame; missing trailer CRC drops entire frame; 0 of 10,000 operations visible. |
| 7 | Concurrency | Two writers simultaneously writing different values to same key (Scenario D2) | Exclusive write lock serializes frames; KeyDir maps key to higher sequence number; no mixed/interleaved bytes. |
| 8 | Concurrency | Reader calls `get` while 10,000-op batch is writing (Scenario D4) | Reader acquires `RLock()`; writer holds `Lock()`; reader sees complete state before batch or after batch, never partial. |
| 9 | Recovery | Hand-truncated database file chopped by 1 byte at tail (Scenario F4) | Scanner detects missing/incomplete trailer CRC at EOF; truncates to last valid frame; previous data intact. |
| 10 | Recovery | Process killed during recovery tail truncation (Scenario C2, F2) | Recovery is idempotent; second restart re-scans from byte 32, detects truncated tail, and completes truncation. |
| 11 | Compaction | Kill during `os.Rename` swapping temp over database (Scenario C4) | POSIX atomic rename guarantees directory points either to old or new file; both are fully synced; zero data lost. |
| 12 | Compaction | Kill after swap before `.compact.tmp` is deleted (Scenario C5) | New file is authoritative; `open()` detects orphan `.compact.tmp` and unlinks it cleanly during startup scan. |
| 13 | Disk Full | Single put exceeds `--max-bytes` cap (Scenario E1) | Quota guard fails pre-check before any bytes written; returns `ErrDiskFull`; database remains readable and recoverable. |
| 14 | Disk Full | Compaction runs when disk is at 99% capacity (Scenario E4) | Temp file write fails with `ErrDiskFull`; compaction aborts; temp unlinked; original database untouched and intact. |
| 15 | Process Lock | Second process attempts to run `./kvdb open` on active DB | Exclusive `flock` on `<path>.lock` detects lock collision; fails immediately with explicit `LOCKED` error. |
| 16 | Malformed Input | Corrupted frame header claims 2 GB payload length | Recovery scanner validates `PayloadLength <= 64MB` and file bounds before allocating buffer; avoids OOM crash. |

---

## 10. Logic Chain

1. **Premise:** The hackathon's Core Invariant requires that all acknowledged writes survive arbitrary process termination (`kill -9`) at any instruction, batches are atomic, recovery completes in $\le 2.0$s for 1,000,000 keys, and zero third-party persistence engines are permitted.
2. **Observation from `.agents/rules/01_invariants.md` and `04_SHARED_DATA_FORMAT.md`:** A single authoritative data file with a 32-byte header (`KVDB2026`) and repeating Universal Atomic Batch Entry (UABE) frames (`0x55414245`) with dual Castagnoli CRC32C checksums provides atomic self-contained boundaries.
3. **Inference on Atomicity:** By enclosing all operations of a batch inside a single UABE frame ending with a `PayloadCRC32C` trailer, atomicity is guaranteed at the frame level. A crash at any instruction before the trailer is fsynced invalidates the frame's CRC, causing the startup recovery scanner to reject the batch in toto without requiring complex multi-file rollback.
4. **Observation from `docs/engine/05_WRITE_AND_COMMIT_PROTOCOL.md`:** Data is durable if and only if `os.File.Sync()` returns 0. Therefore, the engine must never emit `ACK` or update the in-memory `KeyDir` before `file.Sync()` completes.
5. **Observation from `02_banned_libraries.md`:** Third-party storage libraries (SQLite, RocksDB, BoltDB, etc.) or external WAL packages result in immediate disqualification (0 points). Therefore, the Go implementation must use strictly Go standard library (`os`, `sync`, `syscall`, `hash/crc32`, `encoding/binary`).
6. **Observation from `03_scenarios_matrix.md` and `ARCHITECTURE.md`:** All 32 scenarios (Groups A through F) are comprehensively accounted for: Group A (single writes), Group B (batches), Group C (recovery & maintenance), Group D (concurrency), Group E (disk full), and Group F (repeated abuse).
7. **Observation from `04_deliverables_and_cli.md` and `06_scoring_and_demo.md`:** The judge drives the laptop offline (Wi-Fi OFF) during the 15-minute live demo. The project deliverables (`./kvdb`, `./torture`, `ARCHITECTURE.md`, `BREAKING.md`, `PROMPTS.md`, `MEMORY.md`, `DEMO.md`) are directly tied to the 100-point rubric.

---

## 11. Caveats

1. **Hardware / File System Assumptions:** The specification assumes the underlying OS kernel and storage controller honestly flush data to physical media when `fsync` returns. It does not model raw hardware controller power-cut bugs or silent hardware sector bit-rot, although Castagnoli CRC32C protects against transmission corruption.
2. **Directory Sync Platform Differences:** Calling `fsync` on the parent directory inode after `os.Rename` during compaction is POSIX compliant (Linux ext4, XFS, macOS), but Windows handles directory syncing differently. Since the judging environment is Linux (`linux`), Linux syscall semantics are authoritative.
3. **Memory Sizing for 1,000,000 Keys:** The in-memory `KeyDir` requires compact 24-byte struct entries (`int64 ValueOffset`, `uint32 ValueLength`, `uint64 SequenceNo`). Storing values or full key strings repeatedly in RAM would cause GC latency spikes; key locators must point to file offsets.

---

## 12. Conclusion

All 10 authoritative rule modules in `.agents/rules/`, project root guides, and engineering design specifications have been surveyed, cross-verified, and synthesized.
- The Core Invariant, durability rules, single-file UABE storage constraints, and $\le 2.0$s recovery targets are rigorously defined.
- The 32 crash scenarios matrix (Groups A through F) is completely mapped with exact triggers, physical disk states, detection mechanisms, and verification commands.
- All banned library restrictions, offline demo protocols, deliverables contracts (`./kvdb`, `./torture`, `BREAKING.md`, `ARCHITECTURE.md`, `PROMPTS.md`, `MEMORY.md`, `DEMO.md`), and code quality mandates (zero unwraps/panics, memory bounding) are fully established as unambiguous constraints for the implementation team.

---

## 13. Verification Method

To independently verify this specification report:
1. **Rule File Inspection:**
   - Run `head -n 40 .agents/rules/01_invariants.md` to confirm the 5 core invariant rules and write ordering sequence.
   - Run `cat .agents/rules/02_banned_libraries.md` to confirm prohibited storage engines and offline requirements.
   - Run `cat .agents/rules/03_scenarios_matrix.md` to verify the 32 scenario IDs (A1–A7, B1–B6, C1–C6, D1–D4, E1–E5, F1–F4).
   - Run `cat .agents/rules/04_deliverables_and_cli.md` to confirm the 8 `./kvdb` subcommands and `./torture` modes.
   - Run `cat .agents/rules/06_scoring_and_demo.md` to verify the 100-point scoring rubric and penalty schedule.
   - Run `cat .agents/rules/10_code_quality_and_practices.md` to verify zero-unwrap, memory bounds, and directory structure.
2. **Cross-Reference Architectural Proofs:**
   - Inspect `ARCHITECTURE.md` Section 3 to confirm alignment with all 32 scenarios.
   - Inspect `BREAKING.md` Section 1 to confirm the 12 critical crash points.
   - Inspect `docs/common/04_SHARED_DATA_FORMAT.md` and `docs/common/05_CLI_AND_PROTOCOL_SPEC.md` to confirm byte layouts and CLI contracts.

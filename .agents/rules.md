# Hackathon Rules & Invariants: Kill It and Prove It

> **Context:** 24-Hour Engineering Hackathon: "Kill It and Prove It"  
> **Goal:** Build an embedded, single-process, crash-proof key-value database from scratch that survives arbitrary `kill -9` termination at any CPU instruction, and prove it to a judge with a rigorous torture testing tool.

---

## 1. The Core Invariant (The Non-Negotiable Rule)

After a crash at any point in time, and after `open()` has completed recovery:
1. **Acknowledged writes are durable:** Every operation (`put`, `delete`, `batch`) that returned success (`ACK`) before the crash is present in the database.
2. **Unacknowledged writes are atomic:** Every operation that had not returned success at the moment of the crash is either fully present or fully absent.
3. **Batches are all-or-nothing:** Every batch is treated as one atomic operation. A batch is never partially present.
4. **Zero corruption:** No key ever holds garbage, torn values, or data from a different key.
5. **Guaranteed recovery:** `open()` never fails or crashes on a database file produced by this engine, regardless of how or when it was killed.

### Definition of "Returned Success"
When `put`, `delete`, or `batch` returns to the caller or emits an `ACK`, it is a contractual promise that bytes are durable on physical disk (`fsync` completed). Returning success before physical durability is a fatal invariant violation.

### Definition of "Crash"
Immediate termination via `kill -9` (or `TerminateProcess`) with zero warning and no chance to run cleanup code. Crash can happen:
- During a `write()` system call (torn writes).
- During an `fsync()`.
- During file renaming/swapping in compaction.
- During recovery replay or repair in the next `open()`.

---

## 2. Architecture & Design Constraints

### Storage Layer
- **Exactly ONE authoritative data file on disk.** A separate lock file or temporary file during compaction is allowed, but all authoritative key-value state lives in one file.
- **On-Disk Format:** Append-only log with CRC32 checksums, length prefixes, commit markers, and an in-memory hash index (or memory-efficient sparse index).
- **Compaction:** Mandatory and crash-safe. Must shrink the file after puts followed by deletes.
- **Disk Full Protection:** Must accept configurable `--max-bytes`. When reached, operations fail with a clean error, existing data remains uncorrupted, and database remains readable and recoverable.
- **Recovery Speed:** `open()` must finish recovery in under 2.0 seconds with 1,000,000 keys.

### Strict Write Ordering
To guarantee durability and atomicity, the write path must adhere to:
```
1. Write record body (CRC32, key len, val len, key, val)
2. fsync(file)
3. Write commit marker (COMMIT opcode, CRC32)
4. fsync(file)
5. Emit ACK / Return Success to caller
```
*Note: A commit marker on disk strictly implies the record payload is fully durable on disk.*

### Data Limits
- **Keys:** 1 to 1,024 bytes.
- **Values:** 0 to 1 MB (must handle multi-block torn writes).
- **Capacity:** At least 1,000,000 keys in a single database.
- **Batch Size:** Up to 10,000 operations per batch.
- **Concurrency:** At least two concurrent writers without file corruption; readers (`get`) run concurrently with writers and never see half-applied batches. A single global write lock is completely acceptable.

---

## 3. Strict Rules & Prohibitions

### Strictly Forbidden (Automatic Disqualification / 0 Points)
- **NO third-party database or storage engines:** SQLite, RocksDB, LevelDB, LMDB, BerkeleyDB, Redis, BoltDB, Badger, DuckDB, or anything that stores keys and values for you.
- **NO external WAL, journaling, or B-tree/LSM libraries.**
- **NO copy-pasting code from existing database projects.**
- **NO network calls or cloud dependencies:** Everything must run 100% offline.
- **NO pre-written code from before the event.**
- **NO falsifying or omitting prompt logs or commit history.**

### Allowed
- Standard library of the chosen language in full (I/O, threading, hashing, serialisation, `fsync` / `os.fsync` / `FileChannel.force`).
- Test frameworks (`pytest`, `cargo test`, `go test`, etc.).
- AI tools, provided that Deliverable 4 (`PROMPTS.md`) is maintained.

---

## 4. Mandatory Deliverables & Directory Structure

The repository must strictly adhere to the required layout:

```text
├── README.md           # Single build command, CLI docs, format diagram, recovery algorithm
├── ARCHITECTURE.md     # Architecture diagram, 32-scenario proof table, 3 guarantees
├── BREAKING.md         # Crash points table (8-15 rows), ordering argument, 3 weaknesses
├── PROMPTS.md          # Chronological unedited AI prompt log with tool names & timestamps
├── diagrams/           # Architecture diagrams (Mermaid, Excalidraw, SVG/PNG)
├── src/                # Database engine core implementation
├── torture/            # Independent torture test tool & oracle
├── tests/              # Unit, integration, and regression tests
└── .agents/            # Agent rules, prompt-logger hooks, and automation
```

---

## 5. Mandatory CLI Interface (`./kvdb`)

The engine must provide a command-line wrapper at `./kvdb`:
```bash
./kvdb open <path> [--max-bytes N]  # Opens/creates, runs recovery, prints key count & recovery time
./kvdb put <path> <key> <value>     # Writes key-value
./kvdb get <path> <key>             # Prints value, or NOTFOUND
./kvdb del <path> <key>             # Deletes key
./kvdb batch <path> <file>          # Atomic batch ("put k v" or "del k" per line)
./kvdb dump <path>                  # Prints every key and value, sorted by key
./kvdb check <path>                 # Verifies file integrity, prints OK or reason
./kvdb serve <path>                 # Long-running mode: reads ops from stdin, prints ACK <op-id>
```

---

## 6. Mandatory Torture Tool Interface (`./torture`)

The torture tool must drive `serve` mode as a child process and prove correctness:
```bash
./torture --iterations 500 --seed 12345
./torture --iterations 100 --seed 999 --mode large-values
./torture --iterations 100 --seed 7 --mode compaction
./torture --iterations 100 --seed 42 --mode recovery-crash
```

### Torture Tool Requirements
1. **Independent Oracle:** Maintains its own verified log of received `ACK`s. Never trusts the database's own dump as ground truth.
2. **Concurrent Writers:** At least 2 writers streaming random ops (`put`, `del`, `batch`).
3. **Random Millisecond Kills:** Sends `kill -9` after random millisecond delays (never between operations).
4. **Post-Crash Verification:** Re-opens DB, runs `./kvdb check`, runs `./kvdb dump`, and verifies 100% equivalence against the oracle.
5. **Idempotence & Recovery Crash:** Simulates kills during recovery itself.

---

## 7. The 32 Scenarios Matrix (Must Pass & Document in `ARCHITECTURE.md`)

| Group | IDs | Description |
| :--- | :--- | :--- |
| **A: Single Write** | A1–A7 | Kill before write, torn write, before fsync, after fsync before return, after return, during delete, 1MB value. |
| **B: Batch** | B1–B6 | Kill partway through batch, before commit marker, before commit fsync, after ACK, same-key ops, 10k batch killed. |
| **C: Recovery & Maint** | C1–C6 | Kill during replay, during tail repair, before compaction swap, during rename, after swap before cleanup, during close. |
| **D: Concurrency** | D1–D4 | Two writers different keys, same key, long batch vs single put, reader during batch. |
| **E: Disk Full** | E1–E5 | Single write hit cap, batch hit cap, kill after cap, cap during compaction, writes resume after space freed. |
| **F: Repeated Abuse** | F1–F4 | 500 random kills/restarts, killed during recovery repeatedly, 1M key recovery < 2s, 1 byte truncated at tail. |

---

## 8. AI Prompt Logging Policy (`PROMPTS.md`)

- **Automatic Logging:** `.agents/hooks.json` automatically intercepts all AI interactions via `prompt_logger.py` and records each prompt in `PROMPTS.md` with timestamps and tool names.
- **Zero Falsification:** Every prompt must remain unedited. Missing prompt logs result in a -10 point penalty; falsifying results in disqualification.
- **Judge Crash-Here Preparation:** AI is used as a technical sparring partner. Every code commit and crash scenario must be thoroughly understood by team members for the 15-minute live oral defense.


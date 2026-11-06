# Deliverables & CLI Interface Specification

> **Authority:** "Kill It and Prove It" Hackathon Guide (Sections 6, 7, 8, 9, 9A, 10, 11)

## 1. Deliverable 1: Engine & CLI Wrapper (`./kvdb`)
The judge must be able to run these exact commands from the repository root:
- `./kvdb open <path> [--max-bytes N]` : Opens or creates, runs recovery, prints key count and recovery time.
- `./kvdb put <path> <key> <value>` : Stores key-value; overwrites if exists.
- `./kvdb get <path> <key>` : Prints value, or `NOTFOUND`.
- `./kvdb del <path> <key>` : Removes key. Deleting missing key is not error.
- `./kvdb batch <path> <file>` : File has one op per line (`put k v` or `del k`). All-or-nothing atomic.
- `./kvdb dump <path>` : Prints every key and value, sorted by key.
- `./kvdb check <path>` : Verifies file integrity, prints `OK` or a reason.
- `./kvdb serve <path>` : Long-running mode: reads ops from stdin, prints `ACK <op-id>` only after durable on disk.

## 2. Deliverable 2: Torture Tool (`./torture`)
The tool must drive `./kvdb serve` as a child process and attempt to break it:
- `./torture --iterations 500 --seed 12345`
- `./torture --iterations 100 --seed 999 --mode large-values`
- `./torture --iterations 100 --seed 7 --mode compaction`
- `./torture --iterations 100 --seed 42 --mode recovery-crash`
### Torture Tool Invariants:
1. **Independent Oracle:** Writes its own acknowledged ops log; never trusts database dump as ground truth.
2. **Concurrent Writers:** At least 2 concurrent writers streaming operations.
3. **Random Millisecond Kills:** Uses random millisecond delay before `kill -9` (not "after N ops").
4. **Targeted Modes:** Exercises 1MB values, 10,000-op batches, compaction kills, recovery-time kills, and disk-full errors.

## 3. Deliverable 3: `BREAKING.md` (Max 2 Pages)
Must contain all 5 required sections:
1. **Crash Points Table:** 8 to 15 rows. Exact file and function line, disk state at crash, and what `open()` does.
2. **The Ordering Argument:** Explains why `write`, `fsync`, and `rename` sequence guarantees the invariant, naming specific system calls.
3. **Known Weaknesses:** At least three honest weaknesses of the design.
4. **What We Tried That Failed:** Real bugs and designs abandoned based on torture tool findings.
5. **If We Had 24 More Hours:** Three concrete prioritized improvements.

## 4. Deliverable 4: `PROMPTS.md`
- Chronological, unedited log of all prompts sent to any AI tool during the event.
- Includes tool name and approximate timestamp.
- Maintained automatically via `.agents/hooks.json` and agent rules.

## 5. Deliverable 5: `ARCHITECTURE.md`
- **Part 1 (Diagram):** Components, on-disk byte layout, numbered write ordering sequence, recovery flow, lock design.
- **Part 2 (Proof Table):** All 32 scenarios from Groups A–F with disk state, handling mechanism, code location, verification status.
- **Part 3 (Three Guarantees Argued):** Durability, Atomicity, Recoverability in plain English naming system calls.


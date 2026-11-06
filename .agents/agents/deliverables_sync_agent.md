# Deliverables Sync Agent (`deliverables-sync-agent`)

> **Location:** `.agents/agents/deliverables_sync_agent.md`  
> **Role:** Continuous Documentation Auditor & Deliverables Synchronizer  
> **Managed Documents:** `README.md`, `ARCHITECTURE.md`, `BREAKING.md`

---

## 1. Mission & Scope
The **Deliverables Sync Agent** is an autonomous documentation auditor and updater. Its primary responsibility is to prevent **documentation drift** between the Rust/Go database engine (`src/`), the torture tool (`torture/`), and the hackathon's graded submission documents:

1. **[`README.md`](file://README.md) (Deliverable 1):**
   - Single clean build command (`make build` or `cargo build --release`).
   - CLI command reference for all 8 `./kvdb` subcommands.
   - Authoritative on-disk format specification and byte diagram.
   - Plain-English recovery algorithm.

2. **[`ARCHITECTURE.md`](file://ARCHITECTURE.md) (Deliverable 5 — 10 Points):**
   - Architecture component diagram (client API, log writer, in-memory index, commit logic, compaction, locks).
   - Scenario-to-mechanism proof table covering **all 32 rows** across Groups A–F.
   - The three guarantees argued (Durability, Atomicity, Recoverability) naming exact system calls.

3. **[`BREAKING.md`](file://BREAKING.md) (Deliverable 3 — 15 Points):**
   - Crash points table (8 to 15 rows with exact file/function lines and disk state).
   - Ordering argument naming system calls (`write`, `fsync`, `rename`).
   - At least three honest architectural weaknesses.
   - Real bugs and designs abandoned based on torture test findings.
   - 24-more-hours prioritized roadmap.

---

## 2. Invocation Triggers
The agent is activated:
- Whenever code changes in `src/` (e.g. format adjustments, opcode changes, write order updates).
- Whenever a new crash scenario is proven or bug found by `./torture`.
- Upon direct user invocation or via `.agents/scripts/deliverables_updater.py`.
- Before committing deliverables to git.

---

## 3. Tool Permissions & Execution Rules
- **Read & Research:** Full access to read source code, tests, and configuration files.
- **Write Tools:** Authorized to update `README.md`, `ARCHITECTURE.md`, `BREAKING.md`, and log activities in `MEMORY.md`.
- **Invariants:**
  - Never fabricate test verification status (use `"not verified"` until tested by `./torture`).
  - Maintain exact formatting required by the hackathon rubric.
  - Keep `BREAKING.md` strictly within the 2-page limit.


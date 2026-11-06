# Workspace Agent Guidelines

This project participates in the **"Kill It and Prove It" 24-Hour Engineering Hackathon**.
All operations, code design, file structures, and verification suites must strictly comply with the competition invariants and deliverables.

### Authoritative Rule Modules in `.agents/rules/`
- 👉 [01_invariants.md](file://.agents/rules/01_invariants.md) — The Core Invariant, durability promises, write ordering, single-file storage, and $\le 2.0$s recovery.
- 👉 [02_banned_libraries.md](file://.agents/rules/02_banned_libraries.md) — Zero third-party storage engines/WALs, standard library only, offline execution.
- 👉 [03_scenarios_matrix.md](file://.agents/rules/03_scenarios_matrix.md) — The complete 32 crash scenarios matrix (Groups A through F).
- 👉 [04_deliverables_and_cli.md](file://.agents/rules/04_deliverables_and_cli.md) — CLI `./kvdb`, torture tool `./torture`, `BREAKING.md`, `ARCHITECTURE.md`.
- 👉 [05_prompt_logging.md](file://.agents/rules/05_prompt_logging.md) — Chronological unedited prompt logging policy (`PROMPTS.md`).
- 👉 [06_scoring_and_demo.md](file://.agents/rules/06_scoring_and_demo.md) — 100-point rubric, penalties, disqualification rules, and 15-minute live demo protocol.
- 👉 [07_memory_protocol.md](file://.agents/rules/07_memory_protocol.md) — Real-time memory updating protocol (`MEMORY.md`) tracking Date, Time, What, Why, Fix, and State.

### Persistent Project State
- 📜 [PROMPTS.md](file://PROMPTS.md) — Complete unedited AI prompt log.
- 🧠 [MEMORY.md](file://MEMORY.md) — Live execution journal and contextual memory.

### Key Directives
- **Zero Banned Libraries:** No SQLite, RocksDB, LevelDB, BoltDB, or external persistence engines. Standard library only.
- **The Core Invariant:** Acknowledged writes are 100% durable across `kill -9` at any instruction. Batches are strictly atomic. Recovery must be $\le 2.0$ seconds for 1,000,000 keys.
- **Deliverables Enforcement:** Maintain `README.md`, `ARCHITECTURE.md`, `BREAKING.md`, `PROMPTS.md`, `MEMORY.md`, `./kvdb`, and `./torture`.
- **Automatic Logging & Memory:** Every prompt is logged in `PROMPTS.md`, and `MEMORY.md` is updated before executing tasks.

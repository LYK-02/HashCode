# Banned Libraries & Operational Rules

> **Authority:** "Kill It and Prove It" Hackathon Guide (Sections 3, 13)

## 1. Strictly Prohibited (Immediate Disqualification / 0 Points)
- **NO third-party database, storage engine, or persistence libraries:**
  - SQLite, LevelDB, RocksDB, LMDB, BerkeleyDB, Redis, H2, Derby, BoltDB, Badger, DuckDB, or anything similar.
  - If it stores keys and values for you, it is strictly banned.
- **NO ORM or key-value wrapper libraries.**
- **NO third-party write-ahead logging (WAL), journaling, B-tree, or LSM-tree libraries.**
- **NO copying source code from existing database projects.** Reading external documentation or papers is encouraged; pasting code is strictly prohibited.
- **NO pre-written code from before the hackathon starts.** All code must be freshly authored in this repository.
- **NO network access or cloud dependencies:** All code, tests, and demos must run 100% offline on the laptop without Wi-Fi.
- **NO rigging the torture tool:** The torture tool must not kill only between operations or rely on the database's own dump as an oracle.

## 2. Permitted Technologies
- **Programming Language:** Any compiled or interpreted language (e.g., Rust, Go, C, C++, Python, Node.js, Java).
- **Standard Library in Full:** File I/O, threading, atomics, synchronization locks, hashing (`crc32`, `sha256`), serialization, and `fsync` / `FlushFileBuffers` equivalents.
- **Test Frameworks:** Standard test harnesses (e.g., `cargo test`, `go test`, `pytest`).
- **Memory-Mapped Files:** Standard library `mmap` is allowed if needed.
- **AI Tools:** Claude, ChatGPT, Gemini, Copilot, Cursor are fully permitted provided all prompts are logged unedited in `PROMPTS.md`.


# Demo Defense & Scoring Rubric

> **Authority:** "Kill It and Prove It" Hackathon Guide (Sections 11, 12, 16)

## 1. 100-Point Scoring Breakdown

| Area | Points | Criteria & How It Is Scored |
| :--- | :---: | :--- |
| **Survives Judge's Kill Runs** | **30** | 30 if zero violations across all torture runs and manual kills. **Minus 10 per violation found** (min 0). Build failure scores 0. |
| **Crash-Here Questions** | **20** | Up to 7 per question (two at 7, one at 6). Judge points at 3 lines in code: *"The process dies right here. What is on disk? What does the next open() do?"* Full marks require exact disk state and recovery action. |
| **Torture Tool Quality** | **15** | Independent oracle (4), kills during operations not between them (4), concurrent writers and batches (4), targeted modes & seed reproducibility (3). |
| **BREAKING.md** | **15** | Crash points table accuracy (7), ordering argument (4), honest weaknesses and failed attempts (4). Minus 2 per crash point described incorrectly. |
| **Architecture Diagram & Proof** | **10** | Diagram shows components, format, write ordering, and recovery flow matching code (4). Proof table complete and accurate on judge's 3 spot-checked rows (4). Three guarantees name calls and ordering (2). |
| **Design Extension Question** | **10** | Judge asks hypothetical feature (e.g. key TTL expiry). Correct format/recovery changes (5), identifying subtle edge cases unprompted (4), clarity (1). |

---

## 2. Automatic Deductions & Disqualifications

### Deductions
- **Recovery slower than 2.0s at 1,000,000 keys:** `-5 points`
- **File does not shrink after deletes + compaction:** `-5 points`
- **Disk-full behavior corrupts data or breaks open():** `-10 points`
- **Missing or empty `PROMPTS.md`:** `-10 points`

### Disqualification (0 Points, No Appeal)
- Any banned third-party storage library/engine (SQLite, RocksDB, BoltDB, etc.).
- Code copied from an existing database project.
- Pre-written code from before the hackathon.
- Rigged torture tool (kills only between operations, or oracle trusts database dump).
- Falsifying `PROMPTS.md` or git commit history.

---

## 3. The 15-Minute Live Demo Schedule

- **0:00 – 1:00 (Build):** Judge runs single build command from `README.md` on clean clone. Must work first time.
- **1:00 – 4:00 (Torture Run):** Judge runs `./torture --iterations 200 --seed <judge_seed>`. Must show zero failures.
- **4:00 – 6:00 (Targeted Modes):** Judge runs `./torture` in `compaction` and `recovery-crash` modes.
- **6:00 – 8:00 (Manual Kills):** Judge runs `./kvdb serve`, feeds batches, and kills with `kill -9` 5–10 times manually. Verifies with `./kvdb check` and `./kvdb dump`.
- **8:00 – 12:00 (Crash-Here Questions):** Judge points at 3 source lines in write/recovery paths. Team must answer immediately without running code.
- **12:00 – 15:00 (Design Extension):** Judge asks architectural extension question (e.g., adding TTL expiry).


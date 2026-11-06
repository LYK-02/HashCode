# Design Extension Defense: Key Expiry / TTL (`DESIGN_EXTENSION.md`)

> **Event:** "Kill It and Prove It" Hackathon — Oral Defense Protocol (Minutes 12 to 15)  
> **Value:** 10 Points.  
> **Prompt from Judge:** *"Add an expiry time to every key. Keys past expiry must be invisible and must be purged during compaction. Do not write code. Tell me what changes and what breaks."*

---

## 1. On-Disk Binary Format Changes
- **Record Header Addition:** Add an 8-byte field `expires_at: u64` (Unix epoch milliseconds or seconds) to the record header.
  - A value of `0` denotes "no expiry" (immortal key).
  - A positive timestamp marks the exact moment the key expires.
- **Wire Overhead:** Fixed +8 bytes per record. The CRC32 checksum covers this new field.

---

## 2. Engine Read & Write Path Changes
- **Write Path (`put` & `batch`):**
  - Accept an optional TTL/expiry timestamp: `put(key, value, [ttl])`.
  - Calculate `expires_at = current_time() + ttl` and serialize into the record header before `fsync`.
- **Read Path (`get`):**
  - Upon retrieving the record offset from the in-memory index, inspect `expires_at`.
  - If `expires_at > 0 && current_time() >= expires_at`, return `NOTFOUND`.
  - The key becomes instantly invisible to clients without touching disk.

---

## 3. Recovery Path Changes (`open`)
- When replaying the log to build the in-memory index:
  - If a record has an expired timestamp relative to system boot time, it can either be omitted from the index or marked as expired.
  - Omission reduces in-memory index memory consumption, speeding up recovery.

---

## 4. Compaction Purge
- During compaction (copy-on-write rewrite to a temp file):
  - For each active key in the database, compare `expires_at` against `compaction_start_time`.
  - If `expires_at <= compaction_start_time`, omit the record from the compacted log entirely.
  - This shrinks the database file and physically reclaims space occupied by dead keys.

---

## 5. Subtle Edge Cases & What Breaks (Critical for Scoring Full Marks)

Judges look for awareness of subtle systems-level problems. Address these unprompted:

### Subtle Problem 1: System Clock Jumps / Skew (NTP Adjustments)
- **The Issue:** Wall-clock time (`gettimeofday()` / `CLOCK_REALTIME`) can jump backward due to NTP sync or leap seconds. An expired key could suddenly become visible again ("resurrected"), violating causality.
- **The Solution:** Use monotonic timestamps (`CLOCK_MONOTONIC`) for relative in-memory TTL calculation during runtime, and store absolute UTC timestamps on disk. When reading, if the system clock has jumped backward behind the record's commit timestamp, enforce monotonic non-decreasing time (`max(wall_clock, record_commit_time)`).

### Subtle Problem 2: Expired Key Overwritten in an In-Flight Batch
- **The Issue:** A batch puts `key X` with TTL = 1s, and later in the same batch deletes `key X` or updates it with TTL = 100s.
- **The Solution:** Batches are applied sequentially and atomically. The in-memory index reflects the final operation in the batch only after the batch commit marker is durable. The last operation's TTL strictly overrides previous ones.

### Subtle Problem 3: Compaction Timing Races
- **The Issue:** A key expires *during* a long-running compaction run.
- **The Solution:** Compaction uses a snapshot timestamp taken at the moment compaction begins (`T_start`). A key expiring after `T_start` is copied to the new file and will be filtered out by `get()` at runtime, then purged on the *next* compaction cycle.


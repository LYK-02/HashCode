# 15-Minute Live Demo Runbook (`DEMO.md`)

> **Event:** "Kill It and Prove It" 24-Hour Engineering Hackathon  
> **Environment:** Local laptop, Wi-Fi OFF (Offline), 1–2 Judges driving terminal.  
> **Target:** 100 / 100 Points.  
> **Interactive Runner:** Run `./demo.sh` (or `make demo`) for an automated, interactive step-by-step walkthrough.

---

## Interactive Demo Launcher

To present the entire 15-minute live judging flow seamlessly to mentors and judges:
```bash
./demo.sh
# or: make demo
# or: python3 demo.py
```
*At each step, the script explains what is being proven, prompts `[Y/n]`, and allows entering custom judge seeds, iteration counts, and key-value inputs.*

---

## Demo Schedule & Script (00:00 to 15:00)

```
[0:00 - 1:00]  Build from clean clone          -> Build works first time (Single command)
[1:00 - 4:00]  Standard Torture Run (200 iter) -> Zero failures (Judge's seed)
[4:00 - 6:00]  Targeted Modes (Compaction/Rec) -> Zero failures (Targeted seeds)
[6:00 - 8:00]  Jury DB Playground & Live Kills -> Interactive Put/Get/Del/Seed & kill -9 proof
[8:00 - 12:00] Crash-Here Oral Defense         -> 3 code lines: Disk state & open() action
[12:00 - 15:00] Design Extension (Key Expiry)   -> Architectural changes & subtle edge cases
```

---

### Minute 0 to 1: Clean Build
The judge opens a fresh terminal inside the clean clone.
```bash
# Build command (from README.md):
make build
# or: go build -o kvdb ./cmd/kvdb && chmod +x kvdb torture
```
- **Expected:** Compiles with zero errors and zero external downloads in $< 20$ seconds. Produces `./kvdb` and `./torture`.

---

### Minute 1 to 4: Standard Torture Run (30 Points)
The judge runs the torture test with a random seed:
```bash
./torture --iterations 200 --seed 48291
```
- **What Judge Looks For:**
  - Zero failures across all 200 iterations.
  - Live progress showing iterations, kills, and oracle equivalence.
  - Independent oracle verification after each crash restart.

---

### Minute 4 to 6: Targeted Modes (Compaction & Recovery Crash)
The judge tests high-stress edge cases:
```bash
# Mode 1: Compaction kills
./torture --iterations 100 --seed 7 --mode compaction

# Mode 2: Crash during recovery replay
./torture --iterations 100 --seed 42 --mode recovery-crash

# Mode 3: Large values (1MB records spanning disk blocks)
./torture --iterations 50 --seed 999 --mode large-values
```
- **Expected:** All passes. Recovery completes in $< 2.0$s.

---

### Minute 6 to 8: Jury Interactive DB Playground & Live SIGKILL Proof
Mentors and judges can directly interact with the database using their own values, keys, deletes, and seeds.

#### Option A: Guided Interactive Playground (Recommended via `./demo.sh`)
Launch the interactive runner:
```bash
./demo.sh
# Select [5] from menu or proceed to Step 5
```
**Interactive Actions Available for Mentors:**
1. **PUT Key/Value:** Enter custom key and value. Writes immediately and calls `fsync()` to ensure physical media durability.
2. **GET Key:** Read back the durable value directly from the engine's index and storage file.
3. **DELETE Key:** Write an append-only tombstone frame and verify that `./kvdb get` returns `NOTFOUND`.
4. **DUMP Live Keys:** View all live keys and values rendered in a clean decoded ASCII table.
5. **CHECK Integrity:** Run `./kvdb check` to audit Castagnoli CRC32C checksums, headers, and zero corruption.
6. **SEEDED OPERATIONS (Mentor's Seed):**
   - Mentor enters any integer seed (e.g. `48291` or `98765`) and count (e.g. 20 ops).
   - Engine deterministically generates reproducible `PUT` and `DEL` mutations.
   - Verifies that all operations match the expected PRNG state.
7. **SEEDED CRASH TEST (Mid-Stream SIGKILL + Durability Audit):**
   - Streams mutations driven by the mentor's PRNG seed into a live `./kvdb serve` daemon.
   - At 50% progress, sends a torn uncommitted packet and terminates the server with raw `kill -9`.
   - Runs `./kvdb open` recovery and `./kvdb check`.
   - Verifies:
     - 100% of acknowledged writes are durably present in the database.
     - Uncommitted in-flight write is safely pruned at valid EOF.
     - Check outputs `OK` (zero corruption).
8. **COMPACT Database:** Triggers crash-safe copy-on-write compaction and atomic file replacement.

---

#### Option B: Raw CLI Manual Commands (Fallback for Judges)
If the mentor prefers typing raw bash commands directly into the terminal:
```bash
DB="/tmp/jury_manual.db"

# 1. Initialize Database
./kvdb open $DB

# 2. Put custom mentor key
./kvdb put $DB "jury_key" "jury_value_100"

# 3. Retrieve and verify
./kvdb get $DB "jury_key"
# Output: jury_value_100

# 4. Delete key and verify NOTFOUND
./kvdb del $DB "jury_key"
./kvdb get $DB "jury_key"
# Output: NOTFOUND

# 5. Live serve mode with SIGKILL mid-flight
./kvdb serve $DB &
PID=$!

# Send acknowledged write, then partial uncommitted bytes and kill
echo "PUT 1 $(echo -n 'durable_key' | xxd -p) $(echo -n 'durable_val' | xxd -p)"
kill -9 $PID

# 6. Recover and verify integrity
./kvdb open $DB
./kvdb check $DB
# Output: OK (zero corruption)

# 7. Verify acknowledged write survived kill -9
./kvdb get $DB "durable_key"
# Output: durable_val
```
- **Expected:** `check` outputs `OK`. No half-applied batches or torn values. All acknowledged writes durable.

---

### Minute 8 to 12: Crash-Here Questions (20 Points)
Refer to [`CRASH_QUESTIONS.md`](file://CRASH_QUESTIONS.md) for full flashcards.
- **Rule:** Answer immediately without running code.
- **Formula:** Name the line $\to$ Describe exact disk state (record header vs. body vs. commit marker) $\to$ Explain what `open()` does (CRC mismatch / EOF truncation).

---

### Minute 12 to 15: Design Extension — TTL Key Expiry (10 Points)
Refer to [`DESIGN_EXTENSION.md`](file://DESIGN_EXTENSION.md) for full architectural points.
- **Rule:** Do not write code. Explain the on-disk format, read/write/compaction changes, and edge cases (backward clock skew and batch overwrites).


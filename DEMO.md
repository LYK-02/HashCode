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
[6:00 - 8:00]  Manual Kills (serve mode)       -> kill -9 5-10x, check OK, dump intact
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

### Minute 6 to 8: Manual Kills in `serve` Mode
The judge tests manual interactive kills:
```bash
# 1. Start long-running server in background
./kvdb serve /tmp/demo_db.kv &
PID=$!

# 2. Write an acknowledged key and send in-flight bytes, then kill immediately
echo "PUT 1 $(echo -n 'mentor_key' | xxd -p) $(echo -n 'mentor_val' | xxd -p)"
kill -9 $PID

# 3. Recover and verify integrity
./kvdb open /tmp/demo_db.kv
./kvdb check /tmp/demo_db.kv
# Expected output: OK (zero corruption)

# 4. Dump database and verify acknowledged write
./kvdb dump /tmp/demo_db.kv
./kvdb get /tmp/demo_db.kv "mentor_key"
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


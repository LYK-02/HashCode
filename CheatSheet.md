Ran git status check
Ran git pull rebase · Exited with code 128
Ran git restore binary · Exited with code 128
Ran git restore binary
Ran git pull rebase · Exited with code 128
Ran git status
Analyzed MEMORY.md:L1830-1861
Edited MEMORY.md

### Command Cheat Sheet: How to Use the Engine (`./kvdb`) & Torture Tool (`./torture`)

---

### 1. Build Binaries
Clean and build from source (takes $\approx 1.4\text{s}$ offline):
```bash
make clean && make build
```
This generates:
- `./kvdb` — compiled Go storage engine.
- `./torture` — executable Python chaos torture harness.

---

### 2. How to Use the Engine (`./kvdb`)

All commands operate on a single database file (e.g. `mydb.db`) and automatically manage the `<db>.lock` sidecar.

#### 1. `open` — Initialize or Open Database
Opens the database, replays valid frames, repairs any torn tail, and prints key count:
```bash
./kvdb open mydb.db
# Output: Database opened: 0 keys
```

#### 2. `put` — Store a Single Key-Value Pair
Appends an atomic frame, fsyncs to physical disk, and updates the in-memory index:
```bash
./kvdb put mydb.db "user:101" "Alice Smith"
./kvdb put mydb.db "session:token" "abc123xyz"
```

#### 3. `get` — Retrieve a Value
Look up a key. Returns the exact value string, or `NOTFOUND` if missing/deleted:
```bash
./kvdb get mydb.db "user:101"
# Output: Alice Smith

./kvdb get mydb.db "missing_key"
# Output: NOTFOUND
```

#### 4. `del` — Delete a Key (Tombstone)
Appends an atomic tombstone record and purges the key from `KeyDir`:
```bash
./kvdb del mydb.db "user:101"

# Deleting a non-existent key succeeds cleanly (idempotent):
./kvdb del mydb.db "non_existent_key"
```

#### 5. `batch` — Execute Multi-Operation Atomic Batch
Applies 1 to 10,000 operations in a single atomic frame (all-or-nothing guarantee):
```bash
# 1. Create a batch file with operations (put/del):
cat << 'EOF' > /tmp/ops.txt
put user:201 Bob
put user:202 Charlie
del user:101
put user:203 Dave
EOF

# 2. Execute the batch:
./kvdb batch mydb.db /tmp/ops.txt
```

#### 6. `dump` — Print All Live Keys (Sorted)
Dumps all live keys in lexicographically sorted hex format (`<key_hex> <val_hex>`):
```bash
./kvdb dump mydb.db
# Output:
# 757365723a323031 426f62
# 757365723a323032 436861726c6965
# 757365723a323033 44617665
```

#### 7. `check` — Verify Integrity
Scans all UABE frames, verifies Castagnoli CRC32C checksums, and reports status:
```bash
./kvdb check mydb.db
# Output: OK
```

#### 8. `compact` — Reclaim Dead Space & Rebuild Authoritative DB
Removes tombstones and overwritten historical records, shrinking disk footprint:
```bash
./kvdb compact mydb.db
```

#### 9. `serve` — Streaming Line Protocol Server
Runs an interactive server over standard input/output for streaming concurrent operations:
```bash
# Start server:
./kvdb serve mydb.db

# In another terminal or via pipe:
echo -e "PUT op1 6b31 7631\nGET op2 6b31\nPING op3" | ./kvdb serve mydb.db
# Output:
# ACK op1 1
# VAL op2 7631
# PONG op3
```

---

### 3. How to Use the Chaos Torture Tool (`./torture`)

The torture tool runs concurrent writers, measures p99 fsync latency, fires raw `kill -9` signals mid-operation, and asserts 100% equivalence against an independent `oracle.wal`.

#### 1. Default Standard Mode (Live Demo & Judge's Seed)
```bash
# Standard 50-iteration run with seed 12345:
./torture --iterations 50 --seed 12345 --workers 2

# Judge's arbitrary seed (e.g., 98765):
./torture --iterations 200 --seed 98765 --workers 2
```
*Live progress output:*
```text
[▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓] 100% | Iter:   50/  50 | 66 ops/s | p99: 7.1ms | Kills:  50 | ACKs:   1170 | Inv: OK
RESULT: 100% PASSED (All Core Invariants Preserved)
```

#### 2. Compaction Crash Mode
Injects `kill -9` while compacting the database to prove atomic temp-file replacement:
```bash
./torture --iterations 20 --seed 7 --mode compaction
```

#### 3. Recovery-Crash Mode
Fires nested `kill -9` signals *while `open()` is replaying frames* to prove idempotent recovery:
```bash
./torture --iterations 20 --seed 42 --mode recovery-crash
```

#### 4. Large-Values Mode (Multi-Block 1MB Values)
Streams 1 MB payloads spanning dozens of 4KB disk blocks and kills mid-syscall:
```bash
./torture --iterations 10 --seed 999 --mode large-values
```

#### 5. Large-Batches Mode (Up to 10,000 Operations per Batch)
Injects kills during huge atomic transactions to prove zero partial batch exposure:
```bash
./torture --iterations 10 --seed 42 --mode large-batches
```

#### 6. Disk-Full Mode (Quota Guard)
Simulates storage exhaustion and tests graceful quota rejection under process termination:
```bash
./torture --iterations 10 --seed 601 --mode disk-full
```

#### 7. Full Flags Reference
| Flag | Description | Default | Example |
|---|---|:---:|---|
| `--seed <int>` | **Mandatory** random seed for deterministic PRNG | *None* | `--seed 12345` |
| `--iterations <n>` | Number of crash-and-recover cycles | `500` | `--iterations 50` |
| `--mode <name>` | `default`, `compaction`, `recovery-crash`, `large-values`, `large-batches`, `disk-full` | `default` | `--mode compaction` |
| `--workers <n>` | Concurrent writer client threads ($\ge 2$) | `2` | `--workers 4` |
| `--timeout <sec>` | Operation and subprocess timeout | `10.0` | `--timeout 5.0` |
| `--db <path>` | Custom database file path | `data.db` | `--db /tmp/test.db` |
| `--artifact-dir <path>`| Directory where failure logs and dumps are stored | `artifacts` | `--artifact-dir /tmp/logs` |

#### 8. Reproducing Any Failure Bundle
If an iteration ever detects an anomaly, run the auto-generated script inside the bundle:
```bash
# 1-Click exact reproduction:
bash artifacts/failure_iter_0003_seed_999/reproduce.sh
```

---

### 4. Benchmark & Test Commands

#### Run the 1,000,000 Keys Recovery Benchmark
Writes 1M physical records to disk, calls `fsync`, closes, and times `open()`:
```bash
go test -v -count=1 -run TestRecovery_1MillionKeys_Under2Seconds ./internal/engine
# Result: 329.45 ms (< 2.0s ceiling)
```

#### Run All Unit and Chaos Tests
```bash
# Go engine test suite:
go test ./...

# Python torture tool unit tests (78 tests):
python3 -m unittest discover -s torture_tool/tests
```
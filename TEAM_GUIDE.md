# Team Engineering Handbook: KVDB Architecture, Role Guides & Edge Cases (`TEAM_GUIDE.md`)

> **For the Entire Team:** Read this handbook before the live judging session. It is divided into:
> 1. **Common Section:** For all team members (architecture, invariants, demo protocol).
> 2. **Engine Member Section:** Deep dive for Go storage engine engineers.
> 3. **Torture Tool Member Section:** Deep dive for Python chaos harness engineers.
> 4. **The 10 Critical Edge Cases:** Complete oral defense cheat sheet for tricky judge questions.

---

# Part 1: Common Section (For All Team Members)

### 1. The Big Picture: What We Built
We built an append-only, crash-safe, key-value storage engine inspired by the **Bitcask** storage model:
* **Single Authoritative Data File (`data.db`):** All puts, deletes, and batches append to the end of one continuous log file. We never overwrite or mutate bytes in place.
* **In-Memory Hash Index (`KeyDir` in RAM):** To achieve microsecond read latency without index files on disk, we maintain an in-memory hash map pointing each key to its disk location:
  $$\text{Key} \longrightarrow (\text{File Offset},\; \text{Value Length},\; \text{Sequence Number})$$
* **Permanent Sidecar Lock (`<db>.lock`):** A zero-byte sidecar file held exclusively with OS-level `flock` to guarantee single-writer exclusivity without stale lock file bugs.
* **The Core Invariant:** Every write acknowledged with `ACK` is **100% durable** across sudden `kill -9` at any instruction. Batches are strictly atomic (all-or-nothing). Recovery scans 1,000,000 keys in $\le 2.0\text{ seconds}$ (our engine achieves **$\approx 329\text{ ms}$**).

---

### 2. The 15-Minute Live Judging Protocol

| Minute | Stage | What Happens | Command to Type | Expected Result | Points |
| :---: | :--- | :--- | :--- | :--- | :---: |
| **0:00 – 1:00** | **Clean Build** | Judge watches you compile offline | `make clean && make build` | Compiles in $< 2\text{s}$; creates `./kvdb` & `./torture` | **Gate** |
| **1:00 – 4:00** | **Standard Torture**| Judge provides random seed for 200 iterations | `./torture --iterations 200 --seed <judge_seed>` | 100% PASS; zero invariant violations | **30 pts** |
| **4:00 – 6:00** | **Targeted Modes** | Compaction & recovery chaos | `./torture --iterations 100 --seed 7 --mode compaction`<br>`./torture --iterations 100 --seed 42 --mode recovery-crash` | 100% PASS; all ACKs durable | **15 pts** |
| **6:00 – 8:00** | **Manual Kills** | Judge kills `./kvdb serve` manually | Pipe operations and `kill -9` | `./kvdb check` returns `OK` | *(In 30 pts)* |
| **8:00 – 12:00**| **Crash-Here Defense**| Judge points at 3 code lines in editor | Oral answer (See Part 4 cheat sheet) | Exact disk state & `open()` action | **20 pts** |
| **12:00 – 15:00**| **Design Extension**| Judge asks: "Add TTL/Key Expiry" | Oral answer (See Part 4 Edge Case 8) | Format changes + subtle clock skew bugs | **10 pts** |

---

### 3. Disqualification Traps to Remember
1. **Zero Banned Libraries:** No SQLite, RocksDB, LevelDB, BoltDB, DuckDB, or external WAL engines. Standard library only (Go stdlib and Python stdlib).
2. **Offline Execution:** Wi-Fi **OFF**. Zero network dependencies during compilation or testing.
3. **Single File Invariant:** Exactly 1 data file on disk (`data.db`). No secondary WAL files, index files, or separate metadata databases.
4. **No Destructive Blind Initialization:** If `open()` encounters an invalid header on an existing database, it must **never** wipe the file and start fresh.

---

# Part 2: Storage Engine Member Section (Go Implementation)

### 1. Repository Layout
```text
cmd/kvdb/main.go            -> CLI entrypoint implementing all 8 judge subcommands
internal/engine/
  ├── engine.go             -> Engine lifecycle, Open(), Close(), KeyCount()
  ├── write.go              -> WriteFrame(), WriteAt(), File.Sync(), KeyDir publication
  ├── read.go               -> Lock-free Get() via pread (ReadAt)
  ├── batch.go              -> Multi-op batch envelope validation and execution
  ├── recovery.go           -> Sub-0.5s RecoverLog(), applyPayloadToKeyDir(), repairTornTail()
  ├── compact.go            -> 11-step crash-safe compaction and parent directory sync
  ├── lock.go               -> OS kernel advisory flock management (<db>.lock)
  └── scale_1m_test.go      -> 1,000,000 keys benchmark test suite
internal/format/
  ├── header.go             -> 32-byte file header (Magic "KVDB2026", version, flags, CRC)
  ├── frame.go              -> 28-byte UABE frame header, Castagnoli CRC32C, payload encoding
  └── errors.go             -> Typed error classification (ErrCorruptPayload, ErrDiskFull, etc.)
```

---

### 2. Anatomy of a Frame: The UABE Envelope
Every operation (single PUT, single DEL, or 10,000-op atomic BATCH) is encapsulated in a **Universal Atomic Batch Entry (UABE)**:
```text
┌────────────────────────────────────────────────────────────┐
│ 28-Byte Frame Header (Protected by hdr_crc32c)             │
│ - Magic (4B) | Version (2B) | Flags (2B)                   │
│ - SeqNum (8B) | RecordCount (4B) | PayloadLength (4B)      │
│ - Header CRC32C (4B)                                       │
├────────────────────────────────────────────────────────────┤
│ Variable Payload (Length = PayloadLength)                  │
│ - Subrecord 1: OpType (1B) | KeyLen (4B) | Key | ValLen (4B) | Value │
│ - Subrecord 2: OpType (1B) | KeyLen (4B) | Key | ValLen (4B) | Value │
│ - ... up to RecordCount subrecords                         │
├────────────────────────────────────────────────────────────┤
│ 4-Byte Frame Trailer                                       │
│ - Castagnoli payload_crc32c (4B)                           │
└────────────────────────────────────────────────────────────┘
```

---

### 3. The 4 Fundamental Engine Paths

#### Write Path (`put` & `batch`)
1. **Quota Check:** Verify `valid_eof + frame_size <= max_bytes`. Reject before disk I/O if quota exceeded!
2. **Encode Frame:** Pack into UABE envelope with header CRC and trailer CRC.
3. **`f.WriteAt(buf, valid_eof)`:** Bytes copied into operating system page cache.
4. **`f.Sync()` Barrier:** Physical flush pushing cache blocks to SSD platter.
5. **Publish to `KeyDir`:** Update in-memory hash map with disk offset.
6. **Send `ACK`:** Client receives confirmation only after step 4 completes.

#### Read Path (`get`)
1. Look up key in `KeyDir` (RAM). If missing $\to$ immediately return `NOTFOUND` (zero disk seeks!).
2. If present $\to$ read value directly via `f.ReadAt(buf, val_offset)`.
3. *Why lock-free?* `ReadAt` uses the `pread(2)` syscall, taking an explicit file offset without mutating the file descriptor seek pointer. Dozens of threads can read concurrently with zero lock contention.

#### Delete Path (`del`)
1. Append an atomic tombstone record (`OpType = DEL`, value length = 0).
2. Call `f.Sync()`.
3. Remove key from `KeyDir`.
4. The historical record on disk is now dead garbage to be reclaimed during compaction.

#### Compaction Path (`compact`)
1. Open temporary file `<db>.compact.tmp`.
2. Write 32-byte `KVDB2026` header.
3. Iterate live entries in `KeyDir`, copying only active records into the temp file. (All deleted keys and historical overwrites are permanently omitted).
4. `f.Sync()` the temporary file to disk.
5. Close temporary file descriptor.
6. Call `os.Rename("<db>.compact.tmp", "<db>")` (atomic filesystem inode swap).
7. Call `fsync()` on parent directory descriptor to guarantee directory metadata survived.
8. Reopen database and rebuild `KeyDir`.

---

### 4. How We Achieved Sub-0.5s Recovery for 1M Keys (329 ms)
The 2.0s limit was conquered through 4 optimizations in `internal/engine/recovery.go`:
1. **Zero-Allocation In-Place Parsing (`applyPayloadToKeyDir`):**
   We verify the 4-byte Castagnoli CRC trailer first. Once valid, we parse subrecords in-place directly from the read buffer and insert offsets into `KeyDir`. This eliminated **3,000,000 intermediate heap allocations** (`DecodedRecord` structs and defensive value slices).
2. **Map Capacity Pre-Allocation:**
   At startup, we size the map capacity to `(fileSize - 32) / 16` (up to 1,250,000 keys). Go allocates the bucket array once, completely eliminating CPU-stalling map rehashing during recovery.
3. **Contiguous Frame Reading:**
   We read the payload and trailer in a single contiguous `ReadAt` call, reducing OS system call overhead by $50\%$.
4. **Native Hardware SIMD CRC32C:**
   Castagnoli polynomial ($0x82F63B78$) executes on native CPU hardware instructions (`SSE4.2` on x86, `ARMv8-A` on ARM), verifying checksums at $> 10\text{ GB/sec}$.

---

# Part 3: Torture Tool Member Section (Python Chaos Harness)

### 1. Harness Architecture (`torture_tool/`)
```text
torture                     -> Python 3 launcher script (runs torture_tool/main.py)
torture_tool/
  ├── cli.py                -> Argument parser (--seed, --iterations, --mode, --workers)
  ├── oracle/
  │   ├── journal.py        -> Append-only JSONL event journal (oracle.wal)
  │   └── evaluator.py      -> Replays journal events and computes expected state
  ├── scheduler/
  │   └── crash_scheduler.py-> Warmup fsync calibration and adaptive kill delays
  ├── concurrency/
  │   ├── worker.py         -> Concurrent client worker threads with latency tracking
  │   ├── pool.py           -> WorkerPool orchestrating lifecycle and aggregation
  │   └── competing_client.py-> Secondary process exercising flock collisions
  ├── reporting/
  │   ├── terminal_ui.py    -> Live terminal progress bar with IOPS and p99 latency
  │   └── packager.py       -> Preserves forensic artifacts and writes reproduce.sh
  ├── verifier/
  │   ├── dump_parser.py    -> Strict parser for sorted hex dumps from ./kvdb dump
  │   └── comparator.py     -> Compares expected oracle state vs actual database dump
  └── modes/                -> Targeted crash modes (compaction, recovery, large-workloads)
```

---

### 2. The Independent Oracle Protocol
The oracle **never trusts the database dump as ground truth**:
1. Before transmitting any request over the pipe, worker threads log a `START` event with operation ID, key, and value to `oracle.wal`.
2. When the engine responds with `ACK`, workers log an `ACK` event with sequence number.
3. If an error is returned, workers log a `FAIL` event.
4. **State Evaluation:**
   - Every key with an `ACK` **must exist** in the recovered dump with the exact value.
   - Any key with only a `START` (interrupted in-flight during crash) is allowed to be wholly present or wholly absent, but **never partially written or corrupted**.
   - If a batch was interrupted in-flight, either **all** subrecords exist or **none** exist.

---

### 3. The 4 Advanced Torture Enhancements

#### Enhancement 1: Instant Single-Command Failure Reproduction
When any discrepancy occurs, `FailurePackager` creates `artifacts/failure_iter_XXXX/reproduce.sh`:
```bash
#!/usr/bin/env bash
./torture --seed <exact_seed> --iterations 1 --mode <exact_mode> --workers 2 --db /tmp/reproduce.db
```
The judge can reproduce the failure with a single command.

#### Enhancement 2: Adaptive Millisecond Kill Delays (Fsync Calibration)
In warmup, `calibrate_fsync_latency()` measures physical round-trip write + fsync latency ($T_{\text{fsync}} \approx 0.73\text{–}1.8\text{ ms}$). The scheduler centers 70% of random delays around $[0.5 \cdot T_{\text{fsync}},\; 1.5 \cdot T_{\text{fsync}}]$, striking at the exact microsecond `pwrite` completes before `File.Sync()` returns.

#### Enhancement 3: Live Throughput & IOPS Meter in Terminal UI
Terminal renders real-time throughput and 99th-percentile operation latency:
```text
[▓▓▓▓▓▓▓▓▓▓▓▓░░░░░░░░░░░░]  50% | Iter:   25/  50 | 1,420 ops/s | p99: 1.8ms | Kills:  25 | ACKs:    496 | Inv: OK
```

#### Enhancement 4: Competing CLI Client (`CompetingCLIClient`)
Spawns an independent OS process attempting `./kvdb put` directly from the shell while `./kvdb serve` is running. Exercises OS-level `flock(LOCK_EX)` collisions and verifies kernel lock release upon `kill -9`.

---

# Part 4: The 10 Critical Edge Cases (Oral Defense Cheat Sheet)

When judges ask difficult systems questions, use these clear, structured answers:

---

### Edge Case 1: Torn Frame Header at EOF (< 28 Bytes)
* **What happened:** Process was killed while writing the first 15 bytes of the 28-byte UABE frame header.
* **On Disk:** The first $N$ frames are 100% intact. At EOF, 15 bytes of partial metadata exist.
* **Detection:** `open()` calls `readHeader()`. File offset has fewer than 28 bytes remaining before EOF.
* **Action:** `RecoverLog` halts scan, calls `repairTornTail(valid_eof)` (`ftruncate` to last valid frame + `File.Sync()`). Uncommitted bytes are cleanly wiped.

---

### Edge Case 2: Torn Frame Payload at EOF (Payload CRC Mismatch)
* **What happened:** 28-byte header was written, but process died while writing a 1MB value. 400KB reached disk.
* **On Disk:** Valid header exists, but payload is truncated and trailer is missing or corrupted.
* **Detection:** `readTrailer()` reads the last 4 bytes. Computed Castagnoli CRC32C over the payload does not match the trailer.
* **Action:** `repairTornTail` truncates the file back to `valid_eof` before the torn header. Log is restored to pristine committed state.

---

### Edge Case 3: Crash between `File.Sync()` and sending `ACK`
* **What happened:** The engine successfully called `f.Sync()`. The data is durable on physical disk, but `kill -9` fired before the server printed `ACK` over `stdout`.
* **On Disk:** The entire UABE frame (Header + Payload + Castagnoli Trailer CRC) is 100% intact on disk.
* **Detection:** `open()` scans the frame. Both header CRC and trailer CRC pass.
* **Action:** `KeyDir` is updated with the record. **Invariant 2 is preserved:** An acknowledged write must survive; an un-acknowledged write *may* survive, provided it is 100% complete and never torn.

---

### Edge Case 4: Atomic Multi-Op Batch Interrupted Mid-Execution
* **What happened:** A batch of 10,000 operations was being written when `kill -9` fired after operation #4,200.
* **On Disk:** Partial payload bytes on disk; trailer CRC missing.
* **Detection:** Single-frame UABE envelope means the entire batch shares one trailer CRC at the end. CRC validation fails.
* **Action:** The entire frame is rejected and truncated. Zero operations are applied to `KeyDir`. Atomicity is guaranteed: all 10,000 operations commit together, or none do.

---

### Edge Case 5: Compaction Crash before vs. during vs. after `os.Rename`
* **Crash before rename:** Temporary file `<db>.compact.tmp` is partially written. Authoritative `data.db` is untouched. On next `open()`, the engine detects orphan `.compact.tmp` and unlinks it. Zero data lost.
* **Crash during/after rename:** `rename(2)` is atomic in POSIX. Either the old file or the new file is linked. Both files were fully fsynced before rename. Next `open()` recovers whichever file is authoritative with zero corruption.

---

### Edge Case 6: Parent Directory Descriptors after File Rename
* **What happened:** `os.Rename` replaced the data file, but the machine suffered a power loss before directory metadata flushed.
* **Danger:** Filesystem journal may replay directory entries out of order, leading to file pointing to inode 0 (a 0-byte ghost file).
* **Our Defense:** After `os.Rename`, we open the parent directory descriptor (`os.Open(".")`) and explicitly call `dir.Sync()`. This forces the filesystem directory table to persist on disk before continuing.

---

### Edge Case 7: Corrupted Frame in the Middle vs. Torn Tail at EOF
* **Torn Tail at EOF:** Caused by process crash or power loss during active write. Handled by `repairTornTail` truncation.
* **Corrupted Frame in the Middle:** Caused by physical disk sector bit rot or malicious tampering.
* **Our Defense:** If a CRC error occurs before EOF (valid frames exist afterward), `open()` **refuses to truncate**. It halts with `ErrCorruptMiddleFrame` to prevent silently destroying committed historical data.

---

### Edge Case 8: Key Expiry / TTL Design Extension & Clock Skew
* **Judge Question:** *"Add an expiry time to every key. Keys past expiry must be invisible and purged during compaction. What breaks?"*
* **Format:** Add `ExpiresAt uint64` (Unix epoch ms, `0` = immortal) to each UABE record.
* **Read Path:** If `ExpiresAt > 0 && currentTimeMillis() >= ExpiresAt`, return `NOTFOUND`.
* **Compaction:** If `ExpiresAt <= T_compact`, omit record from compacted temp file.
* **Subtle Clock Skew Trap (Must say unprompted!):** If system wall clock jumps backward (NTP sync or leap second), an expired key could reappear as live ("zombie key").
* **Defense:** Use monotonic time (`CLOCK_MONOTONIC`) for relative in-memory expirations, and evaluate:
  $$\text{EffectiveTime} = \max(\text{wall\_clock},\; \text{record\_commit\_time})$$

---

### Edge Case 9: Nested Crash During Startup Recovery (`recovery-crash`)
* **What happened:** The process dies from `kill -9` *while `open()` is replaying frames or truncating a torn tail*.
* **On Disk:** File truncation is atomic; valid frames remain untouched.
* **Action:** Next `open()` restarts recovery from byte 0. Replay is purely mathematical and idempotent:
  $$\text{Replay}(\text{Replay}(\text{Log})) \equiv \text{Replay}(\text{Log})$$
* **Result:** Nested crashes during recovery cannot corrupt existing committed frames.

---

### Edge Case 10: Disk Full / Quota Rejection & The Linux "Fsyncgate"
* **Danger:** On Linux, if `write()` succeeds into page cache but `fsync()` fails with `ENOSPC`, the Linux kernel clears the dirty page bit and discards the page, but leaves the file offset advanced ("Dirty Page Amnesia").
* **Our Defense:** Userspace pre-I/O quota checking. Before calling `WriteAt`, the engine asserts:
  $$\text{valid\_eof} + \text{frame\_size} \le \text{max\_bytes}$$
* If quota is exceeded, `ErrDiskFull` is returned immediately. Zero bytes touch the kernel page cache, preserving clean state for existing data.

---

# Part 5: Summary Checklist for Demo Day

- [x] **Wi-Fi OFF:** Disconnect all network connections before the judge sits down.
- [x] **Clean Build:** Verify `make clean && make build` compiles cleanly.
- [x] **Run Torture:** Verify `./torture --iterations 50 --seed 12345 --workers 2` passes 100%.
- [x] **Run 1M Benchmark:** Verify `go test -run TestRecovery_1MillionKeys_Under2Seconds ./internal/engine` ($\approx 329\text{ ms}$).
- [x] **Open Deliverables:** Keep [`GUIDE.md`](file://GUIDE.md), [`README.md`](file://README.md), [`ARCHITECTURE.md`](file://ARCHITECTURE.md), and [`BREAKING.md`](file://BREAKING.md) open in editor tabs.

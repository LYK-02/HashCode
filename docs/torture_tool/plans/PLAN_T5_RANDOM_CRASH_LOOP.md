# Phase T5 Implementation Plan: Random Crash Injection Loop

> Canonical Phase: **T5**  
> Component: Crash Scheduler, Iteration Lifecycle Coordinator, Mid-Flight Kill Injection  
> Status: **Completed & Verified**  
> Related Docs: `docs/torture_tool/07_RANDOM_CRASH_INJECTION.md`, `docs/torture_tool/08_SEEDS_AND_REPRODUCTION.md`, `docs/torture_tool/10_TORTURE_TEST_MODES.md`

---

## 1. Objective and Scope

Construct the end-to-end iteration crash loop that repeatedly executes the full crash-recovery cycle. The coordinator must start the engine, launch concurrent workers, introduce seeded random millisecond delays, fire ungraceful `SIGKILL` while mutations are actively in-flight, reap the child process, invoke post-recovery checks (`kvdb check` and `kvdb dump`), and verify consistency against the independent oracle before proceeding to the next iteration.

---

## 2. Dependencies and Pre-Conditions

- **Phases T0 through T4:** Complete protocol, generator, oracle, process manager, and concurrent dispatcher.
- **Engine Binary:** Working `./kvdb` supporting `serve`, `check`, and `dump`.
- **Runtime Environment:** Python standard library `time`, `signal`, `threading`, `shutil`, `pathlib`.

---

## 3. Detailed Technical Specifications

### 3.1 Single Iteration Execution Sequence
```text
  [1. Init DB] ---> [2. Start kvdb serve] ---> [3. Launch Workers (>=2)]
                                                       |
  [6. kvdb dump] <-- [5. SIGKILL & Wait] <-- [4. Sleep Random Delay (ms)]
        |
        v
  [7. Compare to Oracle] ---> [8. Pass -> Next Iteration / Fail -> Halt & Preserve]
```

1. **Workspace Setup:** Create isolated database file `data.db` (or reuse existing database to simulate cumulative crash histories across iterations).
2. **Process Startup:** Launch `./kvdb serve <db_path>` via `ChildProcessController`.
3. **Worker Activation:** Start $N \ge 2$ concurrent worker threads generating PUT/DEL/BATCH operations.
4. **Kill Timing Selection:**
   - Sleep for a seeded random millisecond duration:
     $$T_{\text{delay}} \in [T_{\text{min}}, T_{\text{max}}] \quad (\text{e.g., } 5\text{ms} \dots 250\text{ms})$$
   - Include special edge timings:
     - Immediate kill ($0\text{ms}-2\text{ms}$, killing during startup or first batch send).
     - Mid-flight kill (killing while multiple threads have open in-flight requests).
5. **Crash Injection:** Send `SIGKILL` (`kill -9`) directly to the server process.
6. **Worker Reaping:** Wait for child process exit; shut down dispatcher; signal worker threads to terminate and record all in-flight unacknowledged operations.
7. **Post-Crash Validation:**
   - Run `./kvdb check <db_path>`: Verify zero corruption, valid file header, intact checksums.
   - Run `./kvdb dump <db_path>`: Extract recovered state.
8. **Oracle Verification:** Hand actual dump and oracle event log to `OracleEvaluator`.
9. **Decision:**
   - If match $\to$ log one-line status and continue.
   - If mismatch $\to$ **HALT IMMEDIATELY**, preserve all artifacts, exit non-zero.

---

## 4. Phase Breakdown & Subtasks

### Subtask T5.1: Crash Scheduler
- Implement `torture_tool/scheduler/crash_scheduler.py`:
  - `CrashScheduler` using isolated seeded PRNG stream.
  - Algorithms for choosing delay distributions (uniform random, exponential, immediate, and write-synchronized triggers).
  - Measurement of active in-flight count at the exact moment `SIGKILL` is issued.

### Subtask T5.2: In-Flight Verification Gate
- To ensure the test is genuinely adversarial, track whether kills hit active requests:
  - Record metric: `in_flight_at_kill_count`.
  - Flag warnings if 10 consecutive kills happen when zero requests were in flight (indicates workers were starved or delay was too long).

### Subtask T5.3: Iteration Coordinator
- Implement `torture_tool/runner/iteration.py`:
  - `IterationRunner.run_iteration(iter_num, config) -> IterationResult`.
  - Full lifecycle orchestration: setup, spawn, run workers, schedule kill, wait, execute check, execute dump, compare.

### Subtask T5.4: Multi-Iteration Loop Controller
- Implement `torture_tool/runner/suite.py`:
  - `TestSuiteRunner`: Loops from iteration `1` to `config.iterations`.
  - Console live output showing iteration progress, seed, active in-flight count, verification latency, and overall pass/fail status.
  - Interruption handler (`SIGINT` / `Ctrl+C`): Clean shutdown of children without leaving orphans.

---

## 5. Failure Modes and Edge Cases

| Case | Scenario | Expected Handling |
|---|---|---|
| **K5.1** | Engine deadlocks before kill delay | Harness timeout fires; flags engine hang; captures stderr; aborts run. |
| **K5.2** | Engine exits cleanly before kill | Flagged as early exit defect; serve mode must remain running until killed or EOF. |
| **K5.3** | Lock contention on restart | Ensure previous process was reaped with `wait()` before invoking `check` or `dump`. |
| **K5.4** | False pass via quiet run | Invariant check: Verify at least some iterations had active writes during `SIGKILL`. |
| **K5.5** | Mismatch detected | Loop breaks immediately; never runs iteration $k+1$ over a corrupted state. |

---

## 6. Verification and Completion Criteria

- [x] Complete 100 consecutive automated crash iterations without harness crashes or deadlocks.
- [x] Log proves kills occurred while operations were actively in-flight (`in_flight > 0`).
- [x] Post-recovery `kvdb check` and `kvdb dump` executed on every iteration.
- [x] Ctrl+C cleanly terminates the loop and all background threads.


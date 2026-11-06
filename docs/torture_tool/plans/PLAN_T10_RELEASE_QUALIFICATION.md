# Phase T10 Implementation Plan: Release Qualification and Live Demo Protocol

> Canonical Phase: **T10**  
> Component: Qualification Verification, Root CLI Linkage, Live Demo Runbook  
> Status: **Planned**  
> Related Docs: `docs/torture_tool/16_TORTURE_TESTING.md`, `DEMO.md`, `CRASH_QUESTIONS.md`, `docs/common/06_SCORING_AND_RUBRIC.md`

---

## 1. Objective and Scope

Complete final integration, qualification, and presentation readiness for the Python torture tool. Ensure that the primary deliverable executable `./torture` at the repository root is fully functional, supports all required judging flags, runs 100% offline without dependencies, passes the complete 500-iteration default run and 100-iteration targeted runs under unseen judge seeds, and provides an impressive live progress UI for the 15-minute hackathon evaluation session.

---

## 2. Dependencies and Pre-Conditions

- **All Phases T0–T9 Complete:** Codebase verified and self-tested.
- **Engine Binary:** Fully built `./kvdb` Go executable at root.
- **Environment:** Clean Linux host without network access (offline test run).

---

## 3. Detailed Technical Specifications

### 3.1 Deliverable Executable `./torture`
- Root entry point: `./torture` executable script (with `#!/usr/bin/env python3` and executable permissions `chmod +x torture`).
- Forwards arguments to `torture_tool/main.py`.
- Displays clear, formatted help:
  ```bash
  $ ./torture --help
  usage: torture --seed SEED [--iterations N] [--mode MODE] [--db PATH] [--workers W] ...
  ```

### 3.2 Required Qualification Test Matrix
Execute and record test logs for all six mandatory qualification invocations:
```bash
# 1. Default qualification run (500 crash/recovery iterations)
./torture --iterations 500 --seed 12345

# 2. Targeted: Large values stress (1 MiB boundary payloads)
./torture --iterations 100 --seed 999 --mode large-values

# 3. Targeted: Large batches atomicity stress (10,000 operations)
./torture --iterations 100 --seed 888 --mode large-batches

# 4. Targeted: Compaction crash stress
./torture --iterations 100 --seed 777 --mode compaction

# 5. Targeted: Recovery-crash stress
./torture --iterations 100 --seed 666 --mode recovery-crash

# 6. Targeted: Disk-full quota stress
./torture --iterations 100 --seed 555 --mode disk-full
```

### 3.3 Live Judging Experience (15-Minute Protocol)
- **Live Terminal Dashboard:**
  - Dynamic progress bar updating on terminal line:
    `[▓▓▓▓▓▓▓▓▓▓▓▓░░░░░░░░] 60% | Iteration: 300/500 | Kills in-flight: 284 | Live ACKs: 42,100 | Invariant: OK`
  - Real-time display of active writers, kill delays, and recovery verification times ($\le 2.0\text{s}$).
- **Judge Unseen Seed Execution:**
  - Run with an arbitrary seed chosen on the spot by the judges (e.g. `./torture --seed 849203 --iterations 50`).
  - Demonstrate deterministic reproduction: run with same seed twice, showing identical operation counts and kill schedules.

---

## 4. Phase Breakdown & Subtasks

### Subtask T10.1: Root `./torture` Packaging & Environment Setup
- Create root `./torture` script.
- Ensure strict executable permissions (`chmod +x torture`).
- Implement offline check verifying no network requests are attempted.

### Subtask T10.2: Terminal UI & Progress Reporting
- Implement `torture_tool/reporting/terminal_ui.py`:
  - Rich ANSI live progress display.
  - Graceful fallback for non-TTY / CI pipe execution (plain line output).

### Subtask T10.3: Full Qualification Test Runs
- Execute all 6 qualification suites.
- Collect aggregate runtime metrics, operation throughput, and crash statistics.
- Verify zero memory leaks or file descriptor exhaustion over 500 consecutive cycles.

### Subtask T10.4: Demo Script and Judging Rehearsal
- Validate compatibility with instructions in `DEMO.md`.
- Prepare scripted answers for adversarial judge questions in `CRASH_QUESTIONS.md`.

---

## 5. Failure Modes and Edge Cases

| Case | Scenario | Expected Handling |
|---|---|---|
| **Q10.1** | Non-TTY output (redirected to file or CI) | Terminal UI detects `sys.stdout.isatty() == False` and falls back to plain newline logging. |
| **Q10.2** | Judge provides negative or non-integer seed | Input validation displays usage message and exits with status 2. |
| **Q10.3** | Process runs out of file descriptors | Verify file descriptors (`open()`, pipes) are closed immediately when iteration concludes. |
| **Q10.4** | System clock drift | Monotonic timers (`time.monotonic_ns()`) used for all duration and latency calculations. |

---

## 6. Verification and Completion Criteria

- [ ] `./torture` executable at root executes seamlessly without setup.
- [ ] 500-iteration default run completes with 100% PASS and zero invariant violations.
- [ ] All 5 targeted mode runs (100 iterations each) complete with 100% PASS.
- [ ] Unseen seed run demonstrated and verified live.
- [ ] Output complies with all hackathon judging criteria and rules.


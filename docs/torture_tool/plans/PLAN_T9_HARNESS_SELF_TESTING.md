# Phase T9 Implementation Plan: Harness Self-Testing Suite

> Canonical Phase: **T9**  
> Component: Harness Verification, Mock Child Simulation, Chaos Injections  
> Status: **Completed & Verified**  
> Related Docs: `docs/torture_tool/16_TORTURE_TEST_TESTING.md`, `docs/torture_tool/01_TORTURE_OVERVIEW.md`

---

## 1. Objective and Scope

"Test the tester." The adversarial harness must itself be subjected to rigorous self-testing before its results can be trusted as authoritative proof of the database engine's correctness. This phase implements a comprehensive mock test suite that simulates broken, buggy, hostile, or slow database engine behaviors to guarantee that the harness's protocol parser, response dispatcher, oracle, and report packager detect all failure classes and **never report a false positive (never reporting PASS on an engine flaw or harness timeout)**.

---

## 2. Dependencies and Pre-Conditions

- **Complete Harness Implementation:** Phases T0 through T8 complete.
- **Isolated Testing Framework:** Python standard library `unittest` or `pytest`.
- **Zero Third-Party Test Dependencies:** Tests run offline using standard Python standard library.

---

## 3. Detailed Technical Specifications

### 3.1 Mock Child Engine Simulation (`MockEngine`)
A lightweight Python script `torture_tool/tests/mocks/fake_kvdb.py` that mimics `./kvdb serve` with programmatic fault injection via environment variables or control flags:
1. `DELAY_MODE`: Artificially delays responses by $X$ seconds to test timeout handling.
2. `OUT_OF_ORDER_MODE`: Returns ACKs in reverse or scrambled order.
3. `MALFORMED_MODE`: Emits corrupted protocol strings (`"ACK\n"`, `"ACK foo bar baz\n"`, invalid hex).
4. `DUPLICATE_ID_MODE`: Returns two ACKs for the same operation ID.
5. `STDERR_FLOOD_MODE`: Dumps $10\text{ MB}$ of garbage text to `stderr` to test pipe deadlock prevention.
6. `EARLY_EXIT_MODE`: Crashes immediately with code `1` or `139` before serving any requests.
7. `SILENT_HANG_MODE`: Reads stdin but produces zero output.

### 3.2 Oracle Boundary Invariant Tests
Direct unit tests feeding fabricated `START`/`ACK`/`FAIL` histories and corrupted dump outputs to `OracleEvaluator`:
- Injecting a missing ACKed key $\to$ must return `ACKED_WRITE_LOST`.
- Injecting a 1-bit flip in a 1 MiB value $\to$ must return `VALUE_CORRUPTED`.
- Injecting 3 out of 4 keys from an unacknowledged batch $\to$ must return `BATCH_PARTIAL_APPLICATION`.
- Injecting a key that was never submitted $\to$ must return `UNKNOWN_KEY_DETECTED`.

---

## 4. Phase Breakdown & Subtasks

### Subtask T9.1: Mock Engine Simulator Script
- Implement `torture_tool/tests/mocks/fake_kvdb.py` handling CLI subcommands `serve`, `check`, `dump`.
- Support simulated failure injection triggers.

### Subtask T9.2: Dispatcher & Pipe Stress Tests
- Write `torture_tool/tests/test_dispatcher.py`:
  - Run concurrent worker pool against `fake_kvdb.py` with scrambled ACK ordering.
  - Verify every worker receives its exact expected response.
  - Verify stderr flooding does not block the harness.

### Subtask T9.3: Timeout & Early Exit Guards
- Write `torture_tool/tests/test_process_manager.py`:
  - Verify that when the child hangs silently, the harness raises a clean timeout error and marks the iteration as FAILED (not PASS).
  - Verify that when the child crashes during startup, actionable diagnostics are preserved.

### Subtask T9.4: Oracle Negative Verification Suite
- Write `torture_tool/tests/test_oracle_negative.py`:
  - Test all edge cases where an engine invariant is violated.
  - Assert that `OracleEvaluator.validate()` returns failure for every single violation scenario.
  - Assert that failure packager creates `manifest.json`, snapshot, diff, and exits with code 1.

---

## 5. Failure Modes and Edge Cases

| Case | Scenario | Expected Handling |
|---|---|---|
| **S9.1** | Harness misidentifies timeout as crash pass | Test verifies that timeout explicitly fails the test with `ENGINE_HANG_TIMEOUT`. |
| **S9.2** | Fake engine closes stdout prematurely | Dispatcher catches EOF; cancels active futures with `ProcessDiedError`. |
| **S9.3** | High thread contention in dispatcher | Thread-safety tests run with 16 threads issuing 5,000 operations without race conditions. |
| **S9.4** | Oracle accepts partial batch | Test suite fails if any partial batch configuration returns PASS. |

---

## 6. Verification and Completion Criteria

- [x] All self-tests in `torture_tool/tests/` pass 100% cleanly.
- [x] Stderr pipe flooding test completes without deadlock.
- [x] Out-of-order response routing verified across concurrent threads.
- [x] Every negative test correctly triggers a non-zero exit code and failure artifact bundle.


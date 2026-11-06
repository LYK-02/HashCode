# Phase T6 Implementation Plan: State Verification and Failure Reporting

> Canonical Phase: **T6**  
> Component: Dump Parser, State Comparator, Diagnostic Bundle Packager, Non-Zero Exit Guard  
> Status: **Planned**  
> Related Docs: `docs/torture_tool/15_FAILURE_REPORTING_AND_LOGGING.md`, `docs/torture_tool/09_ACK_VERIFICATION.md`

---

## 1. Objective and Scope

Implement the post-recovery verification and failure diagnostic reporting subsystem. When `./kvdb dump` is invoked after a crash, this component deterministically parses the output, compares it against the independent oracle reference model, and performs strict validation (presence of all ACKed mutations, absence of partial batches, absence of unknown keys or torn bytes). On the first failure, it immediately generates a complete, self-contained diagnostic artifact bundle and exits with a non-zero code.

---

## 2. Dependencies and Pre-Conditions

- **Phase T2 & T5:** Oracle evaluator and crash runner operational.
- **Engine Command:** `./kvdb dump <db_path>` prints all live key/value pairs in deterministic lexicographical order.
- **Runtime Environment:** Python standard library `shutil`, `json`, `hashlib`, `pathlib`.

---

## 3. Detailed Technical Specifications

### 3.1 Binary-Safe Dump Parsing
- The engine's `./kvdb dump` prints sorted key-value pairs.
- Format specification (hex-encoded to handle null bytes and newlines safely):
  ```text
  <hex_key> <hex_value>\n
  ```
  - An empty value is represented as `<hex_key> \n` (space followed by newline, or explicit marker).
- Dump parser reads the output line by line, decodes hex to `bytes`, and constructs `actual_state: dict[bytes, bytes]`.
- Enforces strict parsing: reject duplicate keys in dump (engine defect), malformed hex, or unsorted keys if sorted order is specified.

### 3.2 State Verification Logic
The verifier calls `OracleEvaluator.validate(actual_state)` and checks:
1. **Durable Acknowledged Keys:** Every `(key, value)` with an observed `ACK` prior to kill must match bit-for-bit.
2. **Atomic In-Flight Batches:** For every unacknowledged batch, verify that either **all** mutations from the batch exist, or **none** exist.
3. **Absence of Torn Payloads:** If an unacknowledged single PUT is present, its value must match the exact generated payload (length and SHA-256 hash).
4. **No Phantom Keys:** No keys may exist in `actual_state` that were not part of a valid `START` operation in the journal.

### 3.3 Artifact Bundle Structure on Failure
On the first failure, work stops immediately, and the following files are preserved in `artifacts/failure_iter_<iter>_<timestamp>/`:
- `manifest.json`: Run metadata, seed, mode, git commit hash of engine & harness, exact CLI arguments.
- `reproduce.sh`: Executable bash script containing the exact single-iteration command to reproduce the failure.
- `corrupted.db`: A snapshot copy of the database file in its failed state.
- `engine_stdout.log` & `engine_stderr.log`: Complete process output.
- `oracle_events.jsonl`: The append-only event log leading up to the crash.
- `expected_state.json`: Summary of guaranteed vs ambiguous oracle keys.
- `actual_dump.txt`: Raw dump output.
- `mismatch_diff.txt`: Human-readable side-by-side diff highlighting the first mismatching key, expected value hash/len, and actual value hash/len.

---

## 4. Phase Breakdown & Subtasks

### Subtask T6.1: Dump Parser Implementation
- Implement `torture_tool/verifier/dump_parser.py`:
  - `parse_dump_output(dump_text: str) -> dict[bytes, bytes]`.
  - Validate hex formatting, line integrity, and empty-value edge cases.

### Subtask T6.2: Deep State Verifier & Diff Engine
- Implement `torture_tool/verifier/comparator.py`:
  - `StateComparator.compare(expected_oracle, actual_dict) -> DiffReport`.
  - Pinpoint exact discrepancies:
    - Missing ACKed keys
    - Value mismatches (length and byte diff)
    - Partial batch violations
    - Phantom / undeclared keys.

### Subtask T6.3: Failure Bundle Packager
- Implement `torture_tool/reporting/packager.py`:
  - `FailurePackager.preserve_failure(iteration, db_path, run_context, diff_report) -> Path`.
  - Copy database file, write `reproduce.sh`, dump JSON diagnostics.

### Subtask T6.4: Console Summary Formatter
- Implement `torture_tool/reporting/console.py`:
  - Single-line iteration results:
    `[ITER 042/500] SEED=12345 MODE=default DELAY=18ms IN_FLIGHT=4 OPS=180 ACKS=176 STATUS=PASS (124ms)`
  - Final summary table displaying:
    - Total iterations completed
    - Total kills delivered
    - Total kills during active in-flight writes
    - Total operations verified
    - Pass/Fail verdict.

---

## 5. Failure Modes and Edge Cases

| Case | Scenario | Expected Handling |
|---|---|---|
| **V6.1** | Engine dump fails with error code | Verifier flags `DUMP_COMMAND_FAILED`, captures stderr, halts run. |
| **V6.2** | Dump contains duplicate keys | Verifier flags `DUPLICATE_KEYS_IN_DUMP` as a critical index corruption bug. |
| **V6.3** | Value has 1-byte corruption | Hash mismatch detects corruption instantly; diff reports expected vs actual byte values. |
| **V6.4** | Artifact filesystem full | Fallback to writing minimal error summary to stderr; exit non-zero. |
| **V6.5** | Continuing after failure | **Strictly prohibited:** First failure immediately halts the test to prevent overwriting evidence. |

---

## 6. Verification and Completion Criteria

- [ ] Deliberately inject a corrupted key into dump output; verify the comparator immediately catches it and fails.
- [ ] Deliberately omit 1 key from an unacknowledged 10-key batch; verify comparator flags `BATCH_PARTIAL_APPLICATION`.
- [ ] Failure generates all required artifacts in the artifact directory, including `reproduce.sh`.
- [ ] Process exits with non-zero exit code (`1`) on failure, and `0` on clean completion.


# Phase T2 Implementation Plan: Independent Reference Oracle

> Canonical Phase: **T2**  
> Component: Ground-Truth Model, Event Journal, Uncertainty & Atomicity Evaluator  
> Status: **Completed & Verified**  
> Related Docs: `docs/torture_tool/03_INDEPENDENT_ORACLE.md`, `docs/torture_tool/09_ACK_VERIFICATION.md`, `docs/common/02_OFFICIAL_REQUIREMENTS.md`

---

## 1. Objective and Scope

Implement the independent reference oracle that maintains the authoritative ground truth of what the client submitted and observed. The oracle must **never** read from Go engine internals, KeyDir structures, or construct expected states from `kvdb dump`. It logs an append-only JSONL event journal (`START`, `ACK`, `FAIL`) and models the state transitions, precisely resolving acknowledged writes, explicitly failed writes, and ambiguous in-flight operations across process crashes.

---

## 2. Dependencies and Pre-Conditions

- **Phase T0 & T1:** Protocol models and deterministic operations defined.
- **Strict Independence Invariant:** Zero dependency on engine source code or files.
- **Runtime Environment:** Python standard library `json`, `dataclasses`, `threading`, `typing`.

---

## 3. Detailed Technical Specifications

### 3.1 Event Journal Schema (Append-Only JSONL)
Events are written to `<run_dir>/oracle_events.jsonl` and flushed immediately upon creation:
1. `START`: Emitted immediately before sending request bytes across the pipe.
   ```json
   {"type": "START", "ts_ns": 1730900000100, "iter": 1, "op_id": "w1-1-001", "kind": "PUT", "key_hex": "666f6f", "val_hex": "626172"}
   ```
2. `ACK`: Emitted immediately upon receiving and parsing a valid `ACK <op_id>` from stdout.
   ```json
   {"type": "ACK", "ts_ns": 1730900000500, "iter": 1, "op_id": "w1-1-001", "ack_order": 1, "seq_num": 42}
   ```
3. `FAIL`: Emitted immediately upon receiving an `ERR <op_id>` response.
   ```json
   {"type": "FAIL", "ts_ns": 1730900000600, "iter": 1, "op_id": "w1-1-002", "code": "QUOTA_EXCEEDED"}
   ```

### 3.2 State Evaluation Logic & Post-Crash Uncertainty Rules
When verifying post-recovery state:
1. **Acknowledged Mutations (`ACK` received prior to SIGKILL):**
   - **MUST be present in recovered state** in observed commit order.
   - If an ACKed write is missing $\to$ **CRITICAL INVARIANT VIOLATION**.
2. **Explicitly Failed Mutations (`FAIL` received):**
   - **MUST NOT have modified state** unless overwritten by a later ACKed write.
3. **In-Flight Single Operation (`START` logged, but no `ACK` or `FAIL` before SIGKILL):**
   - The operation may be **either wholly applied OR wholly absent**. Both outcomes are valid.
   - Torn or corrupted values are strictly invalid.
4. **In-Flight Batch Operation (`START` logged for `BATCH`, no `ACK` received):**
   - **Atomic All-or-None:** Either **all** operations in the batch took effect, OR **none** took effect.
   - **Partial batch application is a critical disqualification failure.**
5. **Concurrent Same-Key Mutations:**
   - Evaluated using acknowledged order / sequence numbers. If two workers write to key `K` concurrently and both ACK before crash, the state must reflect the final ACKed order or valid linearization.

---

## 4. Phase Breakdown & Subtasks

### Subtask T2.1: Event Journal Logger
- Implement `torture_tool/oracle/journal.py`:
  - `OracleJournal` thread-safe manager writing to file with `flush()` and `os.fsync()` on critical boundaries to prevent journal data loss if the host crashes.
  - Serialization of `START`, `ACK`, `FAIL`, and iteration markers.

### Subtask T2.2: Reference State Machine
- Implement `torture_tool/oracle/state.py`:
  - `ReferenceDatabaseState`: In-memory dictionary tracking guaranteed key-values.
  - Linear execution of acknowledged mutations to produce deterministic base state.

### Subtask T2.3: In-Flight & Ambiguity Resolver
- Implement `torture_tool/oracle/evaluator.py`:
  - `OracleEvaluator.validate(actual_state: dict[bytes, bytes]) -> EvaluationResult`:
  - Step 1: Base verification against guaranteed ACKed state.
  - Step 2: Set-containment verification for pending single operations (allowing present or absent).
  - Step 3: All-or-none verification for pending batches (branch state into all-applied vs none-applied; verify actual matches one valid branch).
  - Step 4: Strict check for unknown keys or unexpected byte values.

### Subtask T2.4: Oracle Unit Fixture Matrix
- Write standalone unit tests in `torture_tool/tests/test_oracle.py`:
  - Simple overwrite and delete sequence.
  - Interleaved same-key concurrent writes.
  - Unacknowledged single put (test valid present and valid absent; test invalid torn payload).
  - Unacknowledged batch (test valid 100% applied, valid 0% applied, reject 50% partial batch).
  - Explicit error handling.

---

## 5. Failure Modes and Edge Cases

| Case | Scenario | Expected Handling |
|---|---|---|
| **O2.1** | Partial batch in actual dump | Evaluator flags `BATCH_PARTIAL_APPLICATION` error with mismatched keys. |
| **O2.2** | Missing ACKed key | Evaluator flags `ACKED_WRITE_LOST` with op ID and timestamp. |
| **O2.3** | Phantom key appears | Actual dump has a key never referenced in any `START` record $\to$ flags `UNKNOWN_KEY_DETECTED`. |
| **O2.4** | Same-key write race | Evaluator checks linearization: if Op1 ACKed before Op2 ACKed, final state must reflect Op2. |
| **O2.5** | Journal corruption | Journal lines are independent JSON; parse errors flag harness internal fault. |

---

## 6. Verification and Completion Criteria

- [x] Unit tests pass covering all permutations of acknowledged, failed, and ambiguous in-flight mutations.
- [x] Evaluator correctly accepts both all-applied and none-applied states for in-flight batches.
- [x] Evaluator strictly rejects partial batches (e.g. 9 out of 10 applied).
- [x] Evaluator strictly rejects torn values (e.g. value truncated or having bit flips).
- [x] Zero dependencies on any engine code or data file format.


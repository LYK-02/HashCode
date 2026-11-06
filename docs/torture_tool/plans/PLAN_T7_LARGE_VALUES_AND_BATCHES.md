# Phase T7 Implementation Plan: Targeted Workloads: Large Values and Batches

> Canonical Phase: **T7**  
> Component: 1 MiB Value Stress, 10,000-Op Batch Atomicity, Wire Buffer Boundaries  
> Status: **Planned**  
> Related Docs: `docs/torture_tool/11_LARGE_VALUES_AND_BATCHES.md`, `docs/torture_tool/10_TORTURE_TEST_MODES.md`

---

## 1. Objective and Scope

Extend the torture harness with dedicated adversarial modes for extreme payload sizes and massive transactions:
1. `--mode large-values`: Generates exact boundary values up to 1 MiB, containing binary nulls, embedded delimiters, and non-UTF-8 sequences, killing the engine while large payloads are actively streaming or writing to disk.
2. `--mode large-batches`: Generates atomic batches of up to 10,000 operations containing mixed PUTs, DELs, and duplicate keys, testing strict all-or-none persistence when killed mid-write.

---

## 2. Dependencies and Pre-Conditions

- **Phase T0–T6:** End-to-end crash loop, oracle, and verification pipeline active.
- **Engine Support:** Capacity to handle 1 MiB values and 10,000-op batches via serve/batch interfaces without truncating buffers.
- **Runtime Environment:** Python standard library `hashlib`, `os`.

---

## 3. Detailed Technical Specifications

### 3.1 Large-Values Mode (`--mode large-values`)
- **Payload Specifications:**
  - Sizes: Boundary values including $64\text{ KiB}$, $256\text{ KiB}$, $512\text{ KiB}$, and exact $1\text{ MiB}$ ($1,048,576$ bytes).
  - Content Types:
    - Pure null bytes (`b"\x00" * 1048576`).
    - Random cryptographic noise (`os.urandom(1048576)` or seeded PRNG bytes).
    - Repeating delimiter patterns (`b",:\n\r" * 200000`).
  - SHA-256 Checksum: The oracle records the payload length and `hashlib.sha256(val).hexdigest()`. To conserve memory, full multi-megabyte payloads in the oracle can be regenerated deterministically from seed or stored once as golden references.
- **Kill Strategy:**
  - Fire `SIGKILL` while a 1 MiB value is being written across the pipe or while `fsync` is executing.
  - Verify post-recovery: If ACKed $\to$ must match exact length and SHA-256. If in-flight $\to$ either old value or complete new value; never a truncated/partial write.

### 3.2 Large-Batches Mode (`--mode large-batches`)
- **Batch Specifications:**
  - Total Operations: Scaled up to 10,000 operations in a single `BATCH` command.
  - Composition:
    - 60% PUTs (small to medium values).
    - 20% DELs.
    - 20% Overwrites targeting earlier keys within the same batch (duplicate key intra-batch resolution).
  - Size on Wire: A 10,000-op batch can generate a wire request line of several megabytes. The harness write buffer and engine read buffer must support large frames without buffer overflows.
- **Atomicity Rule:**
  - If killed during processing: Upon restart, either **all 10,000 operations** are applied, or **zero** are applied.
  - Any intermediate state ($1 \le N < 10000$) is an immediate fail.

---

## 4. Phase Breakdown & Subtasks

### Subtask T7.1: Boundary Value Synthesizer
- Implement `torture_tool/generator/large_values.py`:
  - `generate_large_payload(size_bytes, pattern_type, prng) -> bytes`.
  - Content verification utilities calculating fast SHA-256 digests.

### Subtask T7.2: Giant Batch Synthesizer
- Implement `torture_tool/generator/large_batches.py`:
  - `generate_giant_batch(count=10000, key_pool, prng) -> BatchOperation`.
  - Intra-batch duplicate key resolver ensuring the oracle tracks the final state intended by the batch.

### Subtask T7.3: High-Capacity Pipe Buffering
- Update `torture_tool/concurrency/writer.py` and `dispatcher.py` to stream large buffers efficiently without blocking or memory thrashing.
- Validate that pipe chunking (`write()` emitting 64 KiB chunks) does not interleave with other threads.

### Subtask T7.4: Mode Runners for T7
- Implement specialized sub-runners in `torture_tool/modes/`:
  - `LargeValuesModeRunner`: Drives 100 iterations of 1 MiB write stress.
  - `LargeBatchesModeRunner`: Drives 100 iterations of 10,000-op batch atomicity stress.

---

## 5. Failure Modes and Edge Cases

| Case | Scenario | Expected Handling |
|---|---|---|
| **L7.1** | Value truncated at 64 KiB pipe buffer boundary | Detected immediately: recovered value length $\ne 1048576$; flags torn write defect. |
| **L7.2** | 9,999 operations of 10,000 applied | Comparator flags `BATCH_PARTIAL_APPLICATION`: 1 key missing from atomic batch. |
| **L7.3** | Engine runs out of memory on batch parsing | Engine must return `ERR <op_id> OUT_OF_MEMORY` or exit cleanly; no data file corruption. |
| **L7.4** | Wire line length exceeds buffer | Test verifies engine and harness agree on maximum line/batch constraints. |

---

## 6. Verification and Completion Criteria

- [ ] Execute 100 iterations of `--mode large-values` with verified kills during in-flight writes.
- [ ] Recovered 1 MiB values match bitwise SHA-256 hashes.
- [ ] Execute 100 iterations of `--mode large-batches` with 10,000-op batches.
- [ ] Zero partial batch applications observed across all iterations.


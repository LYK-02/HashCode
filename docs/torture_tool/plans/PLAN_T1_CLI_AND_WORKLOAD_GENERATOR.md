# Phase T1 Implementation Plan: CLI and Deterministic Workload Generator

> Canonical Phase: **T1**  
> Component: CLI Parser, Hierarchical PRNG, Deterministic Workload Synthesizer  
> Status: **Planned**  
> Related Docs: `docs/torture_tool/04_WORKLOAD_GENERATION.md`, `docs/torture_tool/08_SEEDS_AND_REPRODUCTION.md`, `docs/torture_tool/10_TORTURE_TEST_MODES.md`

---

## 1. Objective and Scope

Develop the command-line interface and the deterministic pseudo-random workload generator for `./torture`. The generator must produce reproducible sequences of operations across runs using an explicit `--seed`, supporting varying key distributions, operation ratios, empty/binary keys and values, and targeted scale limits without relying on shared, unseeded global state.

---

## 2. Dependencies and Pre-Conditions

- **Phase T0:** Protocol data structures and byte-encoding utilities available.
- **Runtime Environment:** Standard library `argparse`, `random`, `hashlib`, `dataclasses`, `sys`, `pathlib`.

---

## 3. Detailed Technical Specifications

### 3.1 CLI Arguments and Defaults
```bash
./torture --seed <int> [--iterations 500] [--mode default|large-values|large-batches|compaction|recovery-crash|disk-full] [--db <path>] [--workers 2] [--timeout 30] [--artifact-dir <path>]
```
- `--seed`: **Mandatory**. Integer seed (e.g. `12345`). Lack of seed must immediately abort with exit code `2`.
- `--iterations`: Number of crash/recovery cycles to execute. Default: `500`.
- `--mode`: Workload mode selector. Default: `default`.
- `--workers`: Number of concurrent worker threads. Minimum: `2`. Default: `2`.
- `--db`: Base database directory or path prefix.
- `--artifact-dir`: Location to preserve run logs and failure artifacts. Default: `artifacts/<timestamp>_<seed>/`.

### 3.2 Hierarchical Deterministic PRNG Derivation
To prevent race conditions and cross-thread PRNG contamination:
- The root seed is expanded using SHA-256 to create independent seed streams:
  $$\text{Seed}_{\text{worker}, i} = \text{int}(\text{SHA256}(\text{Seed}_{\text{root}} \parallel \text{"worker"} \parallel i)[0:8])$$
  $$\text{Seed}_{\text{scheduler}} = \text{int}(\text{SHA256}(\text{Seed}_{\text{root}} \parallel \text{"scheduler"})[0:8])$$
  $$\text{Seed}_{\text{iteration}, k} = \text{int}(\text{SHA256}(\text{Seed}_{\text{root}} \parallel \text{"iteration"} \parallel k)[0:8])$$
- Each worker thread instantiates its own isolated `random.Random(seed)` instance. No calls to global `random.*` are permitted.

### 3.3 Workload Characteristics
- **Key Space:** Bounded key pool (e.g., $100$ to $1,000$ keys) to encourage overwrites, deletes, and same-key concurrency collisions.
- **Operation Distribution:**
  - `PUT`: 60%
  - `DEL`: 20%
  - `BATCH`: 20% (containing 2 to 50 operations per batch in default mode)
- **Payload Types:**
  - ASCII text keys/values
  - Pure null bytes (`\x00` $\times N$)
  - Multi-byte UTF-8 and invalid UTF-8 sequences
  - Empty values (`b""`)
  - Maximum single values up to 1 MiB

---

## 4. Phase Breakdown & Subtasks

### Subtask T1.1: CLI Parser & Config Model
- Implement `torture_tool/cli.py` using `argparse`.
- Enforce validation rules: positive integer iterations, valid modes, minimum 2 workers, mandatory seed.
- Generate structured `RunConfig` dataclass.

### Subtask T1.2: Deterministic Stream Manager
- Implement `torture_tool/generator/prng.py` providing `SeedManager`.
- Implement stream splitting functions providing non-overlapping sub-generators for workers, scheduler, and iteration coordinators.

### Subtask T1.3: Workload Generators
- Implement `torture_tool/generator/workload.py`:
  - `generate_key(prng, key_pool_size) -> bytes`
  - `generate_value(prng, min_size, max_size) -> bytes`
  - `generate_operation(prng, worker_id, op_counter) -> Operation`
  - `generate_batch(prng, worker_id, op_counter, batch_size) -> BatchOperation`
- Assign globally unique operation IDs: `<worker_id>-<iteration>-<seq_num>`.

### Subtask T1.4: Mode-Specific Parameter Presets
- Configure generator distributions based on `--mode`:
  - `large-values`: Force values between 64 KiB and 1 MiB.
  - `large-batches`: Force batch sizes between 500 and 10,000 operations.
  - `compaction`: High overwrite/delete ratio (70% DEL / overwrite) on a small key pool (50 keys).
  - `disk-full`: High-frequency large puts under tight quota.

---

## 5. Failure Modes and Edge Cases

| Case | Scenario | Expected Handling |
|---|---|---|
| **W1.1** | Missing `--seed` | CLI prints clear error and exits with code 2. |
| **W1.2** | Worker thread preemption | Workers do not share RNG objects, ensuring sequence generation remains deterministic for each thread. |
| **W1.3** | Seed overflow | Any integer (arbitrary bit width) is hashed safely into 64-bit integer seeds. |
| **W1.4** | Empty key generation | Disallowed: key generation enforces minimum length $\ge 1$ byte. |
| **W1.5** | Reproducibility verification | Running with `--seed 42` twice produces identical lists of operations for every worker. |

---

## 6. Verification and Completion Criteria

- [ ] Unit test confirms identical seeds produce bitwise-identical operation streams.
- [ ] Unit test confirms different seeds produce distinct operation streams.
- [ ] Generator produces valid payloads with null bytes, empty values, and arbitrary binary bytes.
- [ ] No global `random.*` functions used anywhere in the codebase.


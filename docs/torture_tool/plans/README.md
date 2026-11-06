# Torture Tool Implementation Plans Index

> Location: `docs/torture_tool/plans/`  
> Component: Python Adversarial Torture Harness (`./torture`)  
> Sequence: Phases **T0** through **T10**  
> Status: **Implementation Plans Ready for Review and Execution**

---

## Phase Matrix

| Plan File | Phase | Core Focus | Key Deliverables & Criteria |
|---|---|---|---|
| [PLAN_T0_CONTRACT_AND_PROTOCOL.md](./PLAN_T0_CONTRACT_AND_PROTOCOL.md) | **T0** | Contract and Protocol | Binary-safe hex framing, `PUT`/`DEL`/`BATCH`/`PING`, golden test fixtures (COMPLETED) |
| [PLAN_T1_CLI_AND_WORKLOAD_GENERATOR.md](./PLAN_T1_CLI_AND_WORKLOAD_GENERATOR.md) | **T1** | CLI & Deterministic Workload | Mandatory `--seed`, SHA-256 PRNG stream isolation, reproducible workloads |
| [PLAN_T2_INDEPENDENT_ORACLE.md](./PLAN_T2_INDEPENDENT_ORACLE.md) | **T2** | Independent Oracle | Append-only JSONL journal, in-flight ambiguity resolution, all-or-none batch atomicity |
| [PLAN_T3_SUBPROCESS_MANAGER.md](./PLAN_T3_SUBPROCESS_MANAGER.md) | **T3** | Subprocess & Pipe Control | Non-blocking stderr drainer, deadlock prevention, ungraceful `SIGKILL` |
| [PLAN_T4_DISPATCHER_AND_WORKERS.md](./PLAN_T4_DISPATCHER_AND_WORKERS.md) | **T4** | Dispatcher & Concurrent Workers | Stdin mutex, single stdout reader thread demuxing via `Future` map, $\ge 2$ writers |
| [PLAN_T5_RANDOM_CRASH_LOOP.md](./PLAN_T5_RANDOM_CRASH_LOOP.md) | **T5** | Random Crash Loop | Seeded millisecond crash delays, active in-flight kills, 100-cycle iteration loop |
| [PLAN_T6_VERIFICATION_AND_REPORTING.md](./PLAN_T6_VERIFICATION_AND_REPORTING.md) | **T6** | Verification & Failure Bundles | Binary-safe dump parser, bitwise diff engine, halt on 1st fail, `reproduce.sh` |
| [PLAN_T7_LARGE_VALUES_AND_BATCHES.md](./PLAN_T7_LARGE_VALUES_AND_BATCHES.md) | **T7** | Large Values & Batches | Exact 1 MiB payloads with nulls, 10,000-op atomic batch stress across pipe boundaries |
| [PLAN_T8_TARGETED_MODES.md](./PLAN_T8_TARGETED_MODES.md) | **T8** | Targeted Maintenance Modes | Recovery crash (`open` replay kill), compaction crash (temp file/swap kill), disk-full quota |
| [PLAN_T9_HARNESS_SELF_TESTING.md](./PLAN_T9_HARNESS_SELF_TESTING.md) | **T9** | Harness Self-Testing | Mock child process, delayed/scrambled ACKs, stderr flooding, negative oracle validation |
| [PLAN_T10_RELEASE_QUALIFICATION.md](./PLAN_T10_RELEASE_QUALIFICATION.md) | **T10** | Release Qualification | 500 default iterations, unseen seed support, root `./torture` CLI, live demo dashboard |

---

## Implementation Policy & Invariants

1. **Strict Reference Independence:** The torture tool never imports Go packages, inspects KeyDir structures, or uses `kvdb dump` to create expectations.
2. **Crash Fidelity:** All kills are genuine `SIGKILL` (`kill -9`) delivered while requests are actively in flight.
3. **No Continuation on Failure:** The test stops immediately upon the first invariant discrepancy and preserves all diagnostics.
4. **Standard Library Only:** Built purely on Python standard library modules (`subprocess`, `threading`, `signal`, `json`, `hashlib`, `argparse`).


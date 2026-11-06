# Torture Tool Implementation Phases — Python Adversarial Harness

> Owner: Torture Team  
> Language: Python 3  
> Primary output: executable `./torture`  
> Independence rule: expected state must never be read from engine internals or inferred from database output.

## 1. Torture phase sequence

| Phase | Focus | Required result |
|---|---|---|
| T0 | Contract and protocol | Exact shared request/response grammar |
| T1 | CLI and deterministic generator | Reproducible workloads |
| T2 | Independent oracle | START/ACK/FAIL reference model |
| T3 | Child process manager | Reliable pipes, kill and restart |
| T4 | Response dispatcher and workers | Safe concurrent client |
| T5 | First random kill loop | Kill while operations are active |
| T6 | Verification and reporting | Independent post-recovery comparison |
| T7 | Targeted workload modes | Large values and batches |
| T8 | Recovery/compaction/disk-full modes | Maintenance and failure coverage |
| T9 | Harness self-testing | Prove the tester is not the bug |
| T10 | Release qualification | 500 iterations and judge-ready commands |

## 2. T0 — Contract and protocol

### Work
- Read common shared format and protocol docs.
- Freeze exact request grammar and response grammar.
- Decide if ACK contains durable sequence number.
- Decide ordering rule for concurrent same-key writes.
- Decide how BATCH is represented without truncation.
- Decide binary-safe dump format.
- Agree stable error codes.
- Create golden request/response fixtures.

### Exit gate
- Python parser and Go server agree on fixtures.
- ACK order is sufficient to resolve same-key state.
- Every request has unique operation ID.
- No team has an independent undocumented protocol variant.

## 3. T1 — CLI and deterministic workload generator

### Implement
- `argparse` CLI.
- Mandatory `--seed`.
- `--iterations` default 500.
- `--mode` validation.
- Database directory and artifact options.
- Stable seed derivation.
- Separate PRNG stream for each worker and kill scheduler.
- Unique operation IDs.
- PUT/DEL/BATCH generation.
- Empty, binary, maximum-size and duplicate-key cases.

### Exit gate
- Same seed and generator version produce same logical operations.
- Different seeds produce different workloads.
- Operation IDs are unique.
- Exactly 1 MiB values and 10,000-operation batches can be generated.
- No global random state is shared unsafely between workers.

## 4. T2 — Independent oracle

### Implement
- Append-only JSONL event journal.
- START before sending each request.
- ACK only after exact response received.
- FAIL for explicit ERR.
- In-flight tracking.
- Byte-safe operation serialization.
- Replay of acknowledged operations.
- Uncertain-state handling for operations pending at kill.
- Batch event as one indivisible logical unit.

### Oracle rules
- ACKed operations are guaranteed.
- Explicitly failed operations must not have changed state.
- Unresolved single mutation may be present or absent.
- Unresolved batch may be fully present or fully absent.
- No partial batch is acceptable.
- No unknown or mixed value is acceptable.
- Same-key concurrency must use server commit sequence or a guaranteed ACK order.

### Exit gate
- Oracle unit fixtures cover overwrite, delete, batch, same-key and unresolved operations.
- Oracle does not import engine modules.
- Oracle never uses `dump` as expected state.
- Journal is flushed after each event.
- Inconsistent event histories fail closed.

## 5. T3 — Child process manager

### Implement
- Start `./kvdb serve <db_path>`.
- Capture PID and command.
- Pipe stdin, stdout and stderr.
- One synchronized stdin writer.
- One stdout reader/dispatcher.
- One stderr drainer.
- Startup and request timeouts.
- Detect child early exit.
- SIGKILL and wait.
- Restart/check/dump commands.
- Cleanup all children and threads.

### Exit gate
- No pipe deadlocks.
- stderr is drained continuously.
- Broken pipe and early exit are detected.
- Child is actually terminated with SIGKILL.
- No orphan process remains after success or failure.
- A second process can reopen after the killed process releases flock.

## 6. T4 — Response dispatcher and concurrent workers

### Implement
- Pending response map keyed by operation ID.
- Thread-safe registration before send.
- Atomic line write under stdin lock.
- One dispatcher consumes stdout.
- Dispatcher resolves exact ACK/ERR future.
- At least two writer workers.
- Optional reader worker.
- Per-worker deterministic random stream.
- Bounded outstanding request count.

### Exit gate
- Concurrent requests do not consume each other's responses.
- Duplicate IDs are rejected.
- Unknown response IDs are reported.
- Workers terminate cleanly after child death.
- Same-key and distinct-key stress workloads are available.

## 7. T5 — First random crash loop

### Implement
For each iteration:
1. Create isolated database path.
2. Start engine serve process.
3. Wait for readiness.
4. Start concurrent workers.
5. Generate and send mixed operations.
6. Persist START and observed ACK/FAIL events.
7. Choose a seeded random millisecond delay.
8. SIGKILL the engine while requests are active.
9. Wait for child exit and mark pending requests unresolved.
10. Restart and run check.
11. Run dump and parse actual state.
12. Compare actual state to oracle.
13. Save result and continue only if no failure.

### Exit gate
- At least 100 consecutive integrated iterations pass.
- Some kills demonstrably happen with pending mutations/batches.
- SIGKILL is used, not graceful shutdown.
- Kill delay and actual timing are logged.
- First mismatch stops the run and preserves evidence.

## 8. T6 — Verification and failure reporting

### Implement
- Deterministic dump parser.
- Binary-safe values and keys.
- Empty-value vs missing-key distinction.
- Exact key/value comparison.
- Batch all-or-none validation.
- Expected/actual hashes and lengths.
- First mismatch details.
- Run manifest and JSONL events.
- Artifact retention.
- Nonzero exit on any failure.

### Exit gate
- A deliberately corrupted dump is detected.
- Missing ACKed key is detected.
- Wrong value bytes are detected.
- Partial batch is detected.
- Unknown key is detected.
- Harness error is never reported as engine PASS.

## 9. T7 — Large values and large batches

### Large values
- Generate exact 1 MiB payloads.
- Include zero bytes, newline and invalid UTF-8.
- Use deterministic content hashes.
- Kill while large payloads are in flight.
- Verify exact length and bytes after restart.

### Large batches
- Generate exactly 10,000 operations.
- Include duplicate keys and mixed PUT/DEL.
- Include empty and large values within aggregate limit.
- Kill while batch is sent, written or synced.
- Verify all-or-none semantics.

### Exit gate
- Maximum cases are actually reached and logged.
- Protocol does not silently truncate oversized requests.
- Oracle stores complete operation semantics.
- Every restart is checked.

## 10. T8 — Recovery, compaction and disk-full modes

### Recovery-crash mode
- Create a DB with enough frames for measurable recovery.
- Start recovery process.
- Kill during replay and during tail truncation.
- Restart repeatedly.
- Compare stable state to oracle.
- Record evidence that kill overlapped recovery.

### Compaction mode
- Create obsolete log history using overwrites/deletes.
- Record logical live set and file size.
- Trigger compaction.
- Kill at random delays.
- Supplement random tests with deterministic barriers for each critical swap boundary.
- Restart and compare live state.
- Verify shrinkage.

### Disk-full mode
- Set small `max_bytes`.
- Write baseline ACKed data.
- Trigger quota failure on PUT and BATCH.
- Kill after rejection.
- Test compaction temp failure.
- Compact successfully after deletes and verify capacity is restored.

### Exit gate
- Every mode uses the same independent oracle.
- No targeted mode bypasses restart/check/dump.
- Failed mode exits nonzero.
- Mode-specific coverage counts are printed.

## 11. T9 — Harness self-testing

### Test the tester
- Fake child emits normal ACK and ERR.
- Fake child delays responses.
- Fake child returns responses out of order.
- Fake child duplicates or corrupts response IDs.
- Fake child floods stderr.
- Fake child closes stdout.
- Fake child hangs.
- Fake child exits before readiness.
- Oracle fixture has ambiguous in-flight batch.
- Dump parser receives malformed and truncated output.
- Artifact writer encounters filesystem errors.

### Exit gate
- Dispatcher is proven correct.
- Oracle rejects impossible event histories.
- Timeouts never turn into success.
- Child cleanup works after exceptions.
- Seed replay and artifact parsing work.

## 12. T10 — Release qualification

### Required commands
- `./torture --iterations 500 --seed <seed>`
- `./torture --iterations 100 --seed <seed> --mode large-values`
- `./torture --iterations 100 --seed <seed> --mode large-batches`
- `./torture --iterations 100 --seed <seed> --mode compaction`
- `./torture --iterations 100 --seed <seed> --mode recovery-crash`
- `./torture --iterations 100 --seed <seed> --mode disk-full`

Adjust iteration counts only where runtime requires; record actual counts and do not claim an unrun command passed.

### Release gate
- 500 default iterations complete.
- Judge-selected unseen seed works.
- Kills during active writes are demonstrated.
- At least two writers are used.
- Oracle is independent.
- First failure creates a complete artifact bundle.
- All modes run offline.
- No harness crash, deadlock or orphan process.
- README commands match actual CLI.

## 13. Torture phase status

| Phase | Status | Commit | Test evidence | Known issue |
|---|---|---|---|---|
| T0 Contract | COMPLETED | — | 21 passing unit tests in `torture_tool/tests/test_protocol.py` | None |
| T1 Generator | COMPLETED | — | 15 passing unit tests in `torture_tool/tests/test_generator.py` | None |
| T2 Oracle | COMPLETED | — | 6 passing unit tests in `torture_tool/tests/test_oracle.py` | None |
| T3 Process manager | COMPLETED | — | 4 passing unit tests in `torture_tool/tests/test_process.py` | None |
| T4 Workers | COMPLETED | — | 4 passing unit tests in `torture_tool/tests/test_concurrency.py` | None |
| T5 Random kill | COMPLETED | — | 3 passing unit tests in `torture_tool/tests/test_runner.py` | None |
| T6 Verification | COMPLETED | — | 6 passing unit tests in `torture_tool/tests/test_verifier.py` | None |
| T7 Large workloads | NOT STARTED | — | — | — |
| T8 Targeted modes | NOT STARTED | — | — | — |
| T9 Self-tests | NOT STARTED | — | — | — |
| T10 Release | NOT STARTED | — | — | — |

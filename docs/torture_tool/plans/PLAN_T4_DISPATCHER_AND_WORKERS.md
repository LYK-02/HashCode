# Phase T4 Implementation Plan: Response Dispatcher and Concurrent Workers

> Canonical Phase: **T4**  
> Component: Multiplexed Pipe I/O, Response Dispatcher, Concurrent Worker Threads  
> Status: **Completed & Verified**  
> Related Docs: `docs/torture_tool/05_CONCURRENT_WORKERS.md`, `docs/torture_tool/02_TORTURE_ARCHITECTURE.md`

---

## 1. Objective and Scope

Implement the concurrent request-response client architecture. A single `./kvdb serve` child process communicates over standard pipes. To simulate realistic concurrent adversarial stress, multiple worker threads must issue mutations concurrently without cross-thread pipe contention or response race conditions. This is solved by using a centralized, thread-safe response dispatcher that routes asynchronous incoming responses from a single stdout reader thread to the exact waiting worker thread by operation ID.

---

## 2. Dependencies and Pre-Conditions

- **Phase T0 & T3:** Protocol serializers and process controllers in place.
- **Concurrency Invariant:** At least 2 active concurrent writer threads running in parallel.
- **Runtime Environment:** Python standard library `threading`, `concurrent.futures`, `queue`, `time`.

---

## 3. Detailed Technical Specifications

### 3.1 Multiplexed Pipe Architecture
```text
 +---------------------------------------------------------------+
 |                        Torture Harness                        |
 |                                                               |
 |  [Worker 1] ----\                                             |
 |                  +--> [ Stdin Mutex ] ===> stdin ===> [kvdb]  |
 |  [Worker 2] ----/                                       |     |
 |     |                                                   v     |
 |     | (waits on Future)                              stdout   |
 |     |                                                   |     |
 |     +<--- [ Pending Map ] <--- [ Dispatcher Thread ] <--+     |
 +---------------------------------------------------------------+
```

1. **Stdin Serializer:**
   - A single `threading.Lock()` protects writing to `child.stdin`.
   - Each request line is written atomically and followed by `flush()`.
2. **Stdout Dispatcher Thread:**
   - Exactly ONE background thread reads lines sequentially from `child.stdout`.
   - Parses `ACK <op_id>` or `ERR <op_id>` lines.
   - Looks up `<op_id>` in a synchronized `dict[str, concurrent.futures.Future]`.
   - Sets the result on the matching future and removes the entry from the registry.
3. **Concurrent Worker Threads:**
   - Multiple threads ($N \ge 2$) execute independent operation generation loops.
   - For each operation:
     - Register `Future` in pending registry under `<op_id>`.
     - Log `START` event to oracle journal.
     - Acquire stdin lock, write line, flush.
     - Wait on `Future.result(timeout=...)` or until process is killed.
     - Upon `Future` resolution, log `ACK` or `FAIL` event to oracle journal.

---

## 4. Phase Breakdown & Subtasks

### Subtask T4.1: Dispatcher and Pending Registry
- Implement `torture_tool/concurrency/dispatcher.py`:
  - `ResponseDispatcher` class managing `dict[str, Future]` with internal `threading.Lock`.
  - Registration method `register(op_id) -> Future`.
  - Background reader loop parsing `child.stdout.readline()`.
  - Cancellation / teardown method: When process dies, complete all remaining pending futures with `ProcessDiedError`.

### Subtask T4.2: Thread-Safe Stdin Writer
- Implement `torture_tool/concurrency/writer.py`:
  - `StdinWriter` class wrapping `child.stdin` with write lock, metrics tracking (bytes written, requests sent), and broken-pipe error isolation.

### Subtask T4.3: Concurrent Worker Thread Implementation
- Implement `torture_tool/concurrency/worker.py`:
  - `WorkerThread(threading.Thread)` accepting `worker_id`, `workload_generator`, `dispatcher`, `writer`, `journal`.
  - Execution loop: loop until stop event signaled.
  - Track per-worker statistics: operations generated, ACKs observed, errors encountered, in-flight count at stop.

### Subtask T4.4: Worker Pool Coordinator
- Implement `torture_tool/concurrency/pool.py`:
  - `WorkerPool` managing start, join, and clean abort of $N$ workers.
  - Verification that workers run concurrently across real OS threads.

---

## 5. Failure Modes and Edge Cases

| Case | Scenario | Expected Handling |
|---|---|---|
| **C4.1** | Duplicate operation ID | Registration rejects duplicate with internal exception; IDs must be strictly unique. |
| **C4.2** | Unknown response ID from engine | Dispatcher logs unknown response to stderr/artifacts; flags protocol violation. |
| **C4.3** | Child killed while worker waits | Dispatcher notices EOF on stdout, marks all outstanding futures with `ProcessDiedError`; workers unblock cleanly. |
| **C4.4** | Stdin write blocks | Requests are bounded; timeouts prevent indefinite thread lockups. |
| **C4.5** | Thread cross-talk | Since only the dispatcher thread reads stdout, workers can never accidentally consume each other's responses. |

---

## 6. Verification and Completion Criteria

- [x] Unit test with mock process verifies 4 concurrent workers issuing 1,000 requests without dropped or mismatched ACKs.
- [x] Dispatcher cleanly routes ACKs out of order (e.g. Worker 2 response arrives before Worker 1).
- [x] Upon abrupt process termination, all active worker threads unblock within 100ms.
- [x] Stdin writes remain strictly atomic (no line interleaving).


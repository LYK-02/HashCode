# Deliverable 4: AI Prompt Log (PROMPTS.md)

> **Hackathon:** Kill It and Prove It - 24-Hour Engineering Hackathon  
> **Requirement:** Contains every prompt sent to any AI tool during the hackathon in chronological order, unedited, with tool name and approximate timestamp.

---

### Prompt (2026-11-06 12:09:31)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
bring my setting from vs code
```

---

### Prompt (2026-11-06 12:33:36)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
read all make rules.md auto logger for  prompt .md etc for that in .AGENTS folder
```

---

### Prompt (2026-11-06 13:53:30)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Continue
```

---

### Prompt (2026-11-06 13:54:11)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update rules @[/home/laik/workspace/srinathonn/.agents/rules]
```

---

### Prompt (2026-11-06 14:00:15)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Continue
```

---

### Prompt (2026-11-06 14:02:23)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
recheck all
```

---

### Prompt (2026-11-06 14:08:15)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Continue
```

---

### Prompt (2026-11-06 14:09:05)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
make memory updater after every prompt it update context in it and before excuting anything memory.md contain everythiin like date time ehat , why , fix etc
```

---

### Prompt (2026-11-06 14:21:14)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
also add username who sent prompt and worked who implement thr in memory all
```

---

### Prompt (2026-11-06 14:32:40)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
add this to .agents folder
```

---

### Prompt (2026-11-06 14:34:13)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Side Question: can we improve user experience?

Yes. We can significantly improve the user experience across three critical dimensions:

---

### 1. The Judge’s 15-Minute Demo Experience (Scoring & Clarity)
During the live demo, the judge drives your laptop with Wi-Fi off. Friction here loses points.
* **Single-Command Build & Verification (`make demo-ready` or `./demo.sh`):**
  * One script that cleans, builds from scratch, runs a 50-iteration sanity torture test, and verifies all 8 `./kvdb` commands in 20 seconds.
* **Interactive Demo Runbook (`DEMO.md`):**
  * A minute-by-minute cheat sheet mapped to the 0–15 min rubric so you can execute judge requests instantly without hesitation.
* **Crash-Here & Extension Flashcards:**
  * Quick-reference summaries for the 3 crash-here questions and the TTL expiry design extension (worth 30 points combined).

---

### 2. The CLI & Engine Experience (`./torture` & `./kvdb`)
The judge watches `./torture` run for 3–5 minutes. A wall of unreadable terminal text looks fragile; a clean dashboard looks rock solid.
* **Live Terminal Progress Bar for `./torture`:**
  * Show a clean, colorized 1-line or pinned status block:
    ```text
    [Seed: 12345] Iteration: 214/500 [42%] | Kills: 214 | ACKed: 12,840 | Oracle: 100% OK | Mode: default
    ```
* **Instant Failure Diagnostics:**
  * If an invariant ever breaks, print a clean diff: exact `op-id`, expected value from oracle vs. actual value found in dump, and the exact byte offset in the log.
* **Human-Readable `./kvdb check` Output:**
  * Instead of a silent exit or raw error, output:
    ```text
    Integrity Check: OK (1,000,000 keys verified, 0 torn records, recovery time: 1.14s)
    ```

---

### 3. Your Pair-Programming Experience in Antigravity
* **Interactive 32-Scenario Progress Dashboard (Generative UI):**
  * Render an interactive matrix in chat with checkboxes for all 32 scenarios (Groups A through F) that updates as we prove and verify each one.
* **Automated Context Priming:**
  * Maintain a pinned `CURRENT_FOCUS.md` or status widget in `MEMORY.md` so whenever you send a prompt like `Continue`, the agent immediately knows the exact file, struct, and function being worked on.
* **Git Commit Helper:**
  * A helper command (`make commit msg="..."`) that automatically verifies `PROMPTS.md` and `MEMORY.md` are synchronized before creating a clean, judge-pleasing git commit.

---

Which area would you like to prioritize first: **the live torture tool UI**, the **interactive 32-scenario dashboard**, or the **15-minute demo runbook**?  add this in .agents to fix it
```

---
### Prompt (2026-11-06 14:43:23)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
before excuting anything pull from git before doing add this in .agents
```

---

### Prompt (2026-11-06 14:50:54)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
in agent add a good coding practice and code quality in .agents
```

---

### Prompt (2026-11-06 15:10:18)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
add new skill that store imp thing for demo  in md file
```



### Prompt (2026-11-06 15:23:08)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
BREAKING.md,ARCHITECTURE.md and README.md add auto updater / logger for that md file i will share plan later make agent in .agent
```

---

### Prompt (2026-11-06 15:59:23)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
@[/home/laik/workspace/srinathonn/docs] read all and fill the missing gaps and make a BREAKING.md and ARCHITECTURE.md and readme>md and update there agentstell me  @[/home/laik/workspace/srinathonn/.agents]  update .agents if needed  we are using go + python
```

---

### Prompt (2026-11-06 16:11:24)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
all good and can we add artitecture diagram?
```

---

### Prompt (2026-11-06 16:22:16)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
made both high level and low level artitecture in Architecture.md
```

---

### Prompt (2026-11-06 16:27:49)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
just add simple diagram not html and all
```

---

### Prompt (2026-11-06 16:28:28)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
made both high level and low level artitecture in Architecture.md and just add simple diagram not html and all

### Prompt (2026-11-07 00:54:19)

- **User:** LENOVO (AkashNSBhat)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
m3 done
```

---

```

---

### Prompt (2026-11-06 17:04:17)

- **User:** ashwin (Ashwin Nethan)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Thoroughly analyze the entire `docs/torture_tool/` folder and all its Markdown files. Understand every requirement, specification, edge case, and dependency in depth. Based on this context, create a comprehensive, production-grade, phase-by-phase implementation plan for building a powerful and reliable Python torture-testing tool. Break every phase into clear subtasks, implementation steps, dependencies, and completion criteria. Create well-structured `PLAN.md` files for each phase. Ensure nothing is overlooked, avoid assumptions, and strictly follow the official problem statement and existing project decisions. Plan first, do not start implementation.
```

---

### Prompt (2026-11-06 17:10:37)

- **User:** ashwin (Ashwin Nethan)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Proceed with creating these files in docs/torture_tool/plans.
```

---

### Prompt (2026-11-06 17:28:44)

- **User:** ashwin (Ashwin Nethan)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Implement Phase T0 strictly following `docs/torture_tool/plans/PLAN_T0_CONTRACT_AND_PROTOCOL.md`. Complete all subtasks, write production-quality code, add fixtures and tests, verify protocol compatibility with the Go engine, fix all failures, and update the plan checklist. Do not skip requirements or start T1.
```

---
### Prompt (2026-11-06 17:43:54)

- **User:** ashwin (Ashwin Nethan)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Implement Phase T1 strictly following `docs/torture_tool/plans/PLAN_T1_CLI_AND_WORKLOAD_GENERATOR.md`. Complete all subtasks, write production-quality code, add comprehensive tests, verify deterministic workload generation and CLI functionality, fix all failures, and update the plan checklist. Do not skip requirements or start T2.
```

---

### Prompt (2026-11-06 17:51:02)

- **User:** ashwin (Ashwin Nethan)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Move on to Phase T2. Follow `docs/torture_tool/plans/PLAN_T2_INDEPENDENT_ORACLE.md` closely and implement the independent reference oracle, event journal, state tracking, and uncertainty/atomicity evaluation. Complete all subtasks, add thorough tests for crash and ambiguous-write scenarios, verify everything, and update the checklist. Keep the oracle completely independent of Go engine internals. Do not begin T3.
```

---

### Prompt (2026-11-06 17:58:30)

- **User:** ashwin (Ashwin Nethan)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Implement Phase T3 using `docs/torture_tool/plans/PLAN_T3_SUBPROCESS_MANAGER.md` as the guide. Build the subprocess manager with reliable process spawning, non-blocking pipe handling, stderr draining, SIGKILL support, and clean restart/termination. Cover process failures and pipe deadlocks with tests, verify everything works, and update the phase checklist. Stop before T4.
```

---

### Prompt (2026-11-06 18:04:18)

- **User:** ashwin (Ashwin Nethan)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Implement Phase T4 as outlined in `docs/torture_tool/plans/PLAN_T4_DISPATCHER_AND_WORKERS.md`. Build the thread-safe response dispatcher and concurrent worker system, ensuring correct request routing, synchronization, and race-free operation handling. Add tests for parallel requests, response ordering, and concurrency edge cases. Run the full relevant test suite, fix issues, and update the checklist. Don't move to T5.
```

---

### Prompt (2026-11-06 18:07:49)

- **User:** ashwin (Ashwin Nethan)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Implement Phase T5 using `docs/torture_tool/plans/PLAN_T5_RANDOM_CRASH_LOOP.md`. Build the complete crash-injection loop with seeded kill timing, concurrent in-flight operations, process recovery, and post-crash consistency checks against the independent oracle. Test repeated crash scenarios and edge cases, fix all failures, and update the checklist. Keep the implementation within T5; don't start T6.
```

---

### Prompt (2026-11-06 18:15:40)

- **User:** ashwin (Ashwin Nethan)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Proceed with Phase T6 using `docs/torture_tool/plans/PLAN_T6_VERIFICATION_AND_REPORTING.md`. Implement binary-safe dump parsing, strict state comparison, first-failure detection, and complete diagnostic artifact generation with reproduction scripts. Add tests for corrupted dumps and invariant violations, verify all failure paths, and update the checklist. Do not start T7.
```

---

### Prompt (2026-11-06 18:20:46)

- **User:** ashwin (Ashwin Nethan)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Implement Phase T7 following `docs/torture_tool/plans/PLAN_T7_LARGE_VALUES_AND_BATCHES.md`. Add the large-values and large-batches testing modes, covering 1 MiB binary payloads, buffer boundaries, and 10,000-operation batch atomicity under crash conditions. Include edge-case tests, verify performance and correctness, and update the checklist. Keep changes limited to T7; don't proceed to T8.
```

---

### Prompt (2026-11-06 18:30:22)

- **User:** ashwin (Ashwin Nethan)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Implement Phase T8 according to `docs/torture_tool/plans/PLAN_T8_TARGETED_MODES.md`. Add recovery-crash, compaction-crash, and disk-full testing modes, including maintenance-time crash injection, quota enforcement, and post-recovery consistency checks. Test all scenarios and edge cases, fix failures, and update the phase checklist. Do not proceed to T9.
```

---

### Prompt (2026-11-06 18:36:56)

- **User:** ashwin (Ashwin Nethan)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Work on Phase T9 using `docs/torture_tool/plans/PLAN_T9_HARNESS_SELF_TESTING.md`. Build the harness self-testing suite with mock engine simulations, protocol failure injection, delayed responses, broken pipes, and timeout scenarios. Ensure the harness never reports false positives. Run comprehensive tests, fix any issues, and update the checklist. Leave T10 untouched.
```

---

### Prompt (2026-11-06 18:56:24)

- **User:** ashwin (Ashwin Nethan)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Complete Phase T10 following `docs/torture_tool/plans/PLAN_T10_RELEASE_QUALIFICATION.md`. Finalize the root `./torture` CLI, verify all required modes, run the 500-iteration default and 100-iteration targeted tests with unseen seeds, and prepare the live demo runbook and progress output. Fix all issues, validate release readiness, and update the checklist with actual results.
```

---

### Prompt (2026-11-06 19:51:53)

- **User:** ashwin (Ashwin Nethan)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Clean up `.gitignore`, exclude generated artifacts and temporary files, then stage all legitimate project source, tests, and documentation. Review `git status` and staged changes carefully, ensure no important files are missed or unwanted files included, then commit and push to `origin/main`. Do not force-push or delete project data. Show me the final commit and push results.
```

---

### Prompt (2026-11-06 16:39:59)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
@[/home/laik/workspace/srinathonn/docs] read all and we are doing engine
```

---

### Prompt (2026-11-06 16:42:34)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
/teamwork  @[/home/laik/workspace/srinathonn/docs] read all and we are doing engine
```

---

### Prompt (2026-11-06 16:54:25)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
approve i trust u ,do  better
```

---

### Prompt (2026-11-06 18:31:19)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update memory what we diid till now
```

---

### Prompt (2026-11-06 19:52:42)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update memory and create branch and push and solve merge conflict if appear
```

---

### Prompt (2026-11-06 19:54:25)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update memory what we diid till now
```

---

### Prompt (2026-11-06 19:54:57)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update memory and create branch and push and solve merge conflict if appear
```

---

### Prompt (2026-11-06 19:55:30)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update memory what we diid till now
```

---

### Prompt (2026-11-06 19:55:37)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update memory and create branch and push and solve merge conflict if appear
```

---

### Prompt (2026-11-06 19:55:46)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update memory what we diid till now
```

---

### Prompt (2026-11-06 19:56:06)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update memory and create branch and push and solve merge conflict if appear
```

---

### Prompt (2026-11-06 20:01:04)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update memory and create branch and push and solve merge conflict if appear
```

---

### Prompt (2026-11-06 20:03:35)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update memory what we diid till now
```

---

### Prompt (2026-11-06 20:03:35)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update memory and create branch and push and solve merge conflict if appear
```

---

### Prompt (2026-11-06 20:03:39)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update memory what we diid till now
```

---

### Prompt (2026-11-06 20:03:46)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update memory and create branch and push and solve merge conflict if appear
```

---

### Prompt (2026-11-06 20:03:49)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update memory what we diid till now
```

---

### Prompt (2026-11-06 20:03:57)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update memory and create branch and push and solve merge conflict if appear
```

---

### Prompt (2026-11-06 20:03:59)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
update memory what we diid till now
```

---

### Prompt (2026-11-06 22:36:52)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
how to open cli
```

---

### Prompt (2026-11-06 22:39:20)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
bash: /kvdb: No such file or directory
```

---

### Prompt (2026-11-06 22:42:22)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
can i add HashCode image like agy cli ?
```

---

### Prompt (2026-11-06 22:44:50)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
_  ____      ______  ____  
 | |/ /\ \    / /  _ \|  _ \ 
 | ' /  \ \  / /| | | | |_) |
 |  <    \ \/ / | |_| |  _ < 
 |_|\_\   \__/  |____/|____/  Kill It and Prove It — Srinathon 3.0
```

---

### Prompt (2026-11-06 22:47:59)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
how to use cli
```

---

### Prompt (2026-11-06 22:51:13)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
./kvdb open quota.db --max-bytes 500
Database opened: 1 keys
```

---

### Prompt (2026-11-06 22:53:27)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
u test
```

---

### Prompt (2026-11-06 23:08:11)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
# KVDB System Architecture Diagrams (Deliverable 5)

> **Hackathon:** Kill It and Prove It - 24-Hour Engineering Hackathon  
> **Target:** Go Engine (`./kvdb`) & Python Torture Harness (`./torture`)  
> **Source of Truth:** Section 9A / `docs/common/03_SYSTEM_ARCHITECTURE.md`

---

## 1. Component Map, Concurrency & Ownership Boundaries

```mermaid
flowchart TD
    subgraph ClientAndHarness ["Client & Test Harness Isolation"]
        Harness["Python Torture Harness (./torture)"]
        CLI["CLI Driver (./kvdb put/get/del/batch/dump/check)"]
        Serve["Serve Daemon (JSON-Lines IPC Protocol)"]
    end

    subgraph ProcessIsolation ["OS Process & Locking Isolation"]
        SidecarLock["Sidecar Lock Inode (<path>.lock)<br/>Exclusive flock(LOCK_EX)"]
        ProcMutex["In-Process Concurrency Control<br/>sync.RWMutex (One Exclusive Writer)"]
    end

    subgraph MemoryState ["Volatile In-Memory Engine State (Derived Only)"]
        KeyDir["KeyDir Hash Index<br/>map[string]KeyDirEntry<br/>(Offset, ValueLen, SeqNo, Generation)"]
        QuotaGuard["Quota Guard<br/>(valid_eof + frame_size <= max_bytes)"]
        SeqCounter["Global Monotonic Sequence Counter<br/>(next_seq_no)"]
    end

    subgraph CoreEnginePipeline ["Core Engine Pipeline"]
        Encoder["UABE Frame Codec<br/>(Magic 0x55414245 + HdrCRC + Ops + PayloadCRC)"]
        LogWriter["Position-Independent Log Writer<br/>(os.File.WriteAt / pwrite)"]
        SyncBarrier["Hardware Durability Barrier<br/>(os.File.Sync / fsync)"]
        Scanner["Streaming Recovery Scanner<br/>(64KB BufReader + Dual CRC32C Validator)"]
        Compactor["Copy-on-Write Compactor<br/>(Temp File -> Sync -> Rename -> DirSync)"]
    end

    subgraph PersistentStorage ["Physical Storage Media"]
        ActiveDB[("Authoritative Log File<br/>(<path>)<br/>32B Header + Repeating UABE Frames")]
        TempDB[("Compaction Temp File<br/>(<path>.compact.tmp)")]
        ParentDir[("Parent Directory Metadata<br/>(Directory Descriptor + fsync)")]
    end

    %% Client Interactions
    Harness -->|Subprocess CLI / Stdin-Stdout IPC| Serve
    CLI --> ProcMutex
    Serve --> ProcMutex

    %% Concurrency & Locking
    ProcMutex --> SidecarLock
    ProcMutex --> QuotaGuard
    ProcMutex --> KeyDir
    ProcMutex --> SeqCounter

    %% Write Pipeline
    QuotaGuard -->|Quota OK| Encoder
    Encoder --> LogWriter
    LogWriter -->|1. WriteAt(frame, valid_eof)| ActiveDB
    LogWriter -->|2. File.Sync()| SyncBarrier
    SyncBarrier -->|3. Durable Barrier Return| KeyDir
    KeyDir -->|4. Publish Mutation & Advance EOF| Serve
    Serve -->|5. Emit ACK <op-id> to Client| Harness

    %% Recovery Pipeline
    SidecarLock -->|Acquired Before Recovery| Scanner
    Scanner -->|Stream & Validate CRC32C| ActiveDB
    Scanner -->|Reconstruct Valid State| KeyDir
    Scanner -->|Clean Orphan Temp| TempDB

    %% Compaction Pipeline
    ProcMutex -->|Exclusive Writer Lock Held| Compactor
    KeyDir -->|Iterate Live Keys| Compactor
    Compactor -->|1. Write Live Frames| TempDB
    Compactor -->|2. Temp.Sync()| SyncBarrier
    Compactor -->|3. os.Rename(temp, target)| ActiveDB
    Compactor -->|4. Dir.Sync()| ParentDir
    Compactor -->|5. Swap Descriptors & Rebuild KeyDir| KeyDir
```

---

## 2. Numbered Write & Commit Sequence (Zero Phantom ACK Guarantee)

```mermaid
sequenceDiagram
    autonumber
    actor Client as Client / Torture Tool
    participant IPC as Serve Protocol / CLI
    participant Engine as Engine State (Go)
    participant Disk as Physical Storage File
    participant KeyDir as In-Memory Index

    Client->>IPC: {"op": "put", "key": "k1", "value": "v1"}
    IPC->>Engine: Put("k1", "v1")
    Note over Engine: Step 1: Input Validation (Key <= 1KB, Val <= 1MB)
    Engine->>Engine: Acquire sync.RWMutex.Lock() [Exclusive]
    Note over Engine: Step 2: Encode UABE Frame in RAM (Header + Payload + CRC32C Trailer)
    Note over Engine: Step 3: Quota Guard Check (valid_eof + frame_len <= max_bytes)
    
    Engine->>Disk: Step 4: WriteAt(frame_bytes, valid_eof) [Full write loop]
    Engine->>Disk: Step 5: File.Sync() [fsync durability barrier]
    Disk-->>Engine: fsync returned successfully (Bytes durable on physical medium)
    
    Engine->>KeyDir: Step 6: Apply operation to KeyDir (Set k1 -> offset, len, seq)
    Engine->>Engine: Step 7: Advance valid_eof += frame_len and next_seq_no++
    Engine->>Engine: Release sync.RWMutex.Unlock()
    
    Engine-->>IPC: Operation Committed (op_id, seq)
    IPC-->>Client: ACK <op-id> (Acknowledged to caller)

    Note over Client,Disk: INVARIANT: If kill -9 strikes anywhere in Steps 1-5, NO ACK was emitted.<br/>If kill -9 strikes in Steps 6-7, recovery scanner finds valid CRC and recovers the key.
```

---

## 3. Compaction Lifecycle & Atomic Swap

```mermaid
sequenceDiagram
    autonumber
    participant Worker as Compaction Worker
    participant Engine as Engine State (Write Lock)
    participant Temp as Temp File (<path>.compact.tmp)
    participant Disk as Authoritative File (<path>)
    participant Dir as Parent Directory

    Note over Worker: Trigger: dead_bytes > 50% && total_size > 10MB
    Worker->>Engine: Acquire Exclusive mu.Lock() (Block all concurrent writes)
    Worker->>Temp: Create temp file with Header (Gen = old_gen + 1)
    
    loop For each live entry in KeyDir
        Worker->>Disk: Read live value at recorded offset
        Worker->>Temp: Append live UABE frame
    end
    
    Worker->>Temp: File.Sync() [Ensure all compacted frames reach physical storage]
    Worker->>Temp: Close temp file descriptor
    
    Worker->>Disk: os.Rename(temp_path, authoritative_path) [POSIX Atomic Rename]
    Worker->>Dir: Open parent directory & call dir.Sync() [Persist rename metadata]
    
    Worker->>Engine: Re-open authoritative file descriptor & rebuild KeyDir
    Worker->>Engine: Release mu.Unlock()
    Note over Worker,Disk: If killed before Rename: old file intact, orphan temp pruned on open.<br/>If killed after Rename: new file complete, header gen verified.
```

---

## 4. Crash Recovery Pipeline Flowchart

```mermaid
flowchart TD
    Start(["Engine Open Request (path, max_bytes)"]) --> LockSidecar["1. Open & acquire exclusive flock on <path>.lock"]
    
    LockSidecar --> CheckExists{"File exists & size >= 32?"}
    CheckExists -- No --> InitHeader["Create new file & write 32B Header (KVDB2026, Gen 0) & fsync"]
    InitHeader --> PruneTemp["Prune orphan .compact.tmp if present"]
    PruneTemp --> Ready(["Engine Online (Empty KeyDir, EOF=32)"])
    
    CheckExists -- Yes --> ReadHeader["2. Read 32-byte header & verify Magic + CRC32C"]
    ReadHeader --> HeaderValid{"Header Magic == 'KVDB2026' & Header CRC valid?"}
    HeaderValid -- No --> FatalPanic["Fatal Error: Unrecoverable header corruption"]
    
    HeaderValid -- Yes --> ScanLoop["3. Initialize Streaming BufReader at offset 32<br/>Set scan_offset = 32, valid_eof = 32"]
    ScanLoop --> ReadFrameHdr["Read 28-byte UABE Frame Header"]
    
    ReadFrameHdr --> FrameHdrValid{"Bytes read == 28 &&<br/>Magic == 0x55414245 &&<br/>HdrCRC32C valid?"}
    FrameHdrValid -- No (EOF or Torn) --> HaltScan["Stop scanning: torn/partial frame or clean EOF encountered"]
    
    FrameHdrValid -- Yes --> CheckPayloadBounds{"PayloadLen <= 64MB &&<br/>OpCount <= 10,000?"}
    CheckPayloadBounds -- No --> HaltScan
    
    CheckPayloadBounds -- Yes --> ReadPayload["Read PayloadLen bytes + 4-byte Trailer CRC"]
    ReadPayload --> TrailerValid{"Trailer CRC32C == CRC(Payload)?"}
    TrailerValid -- No (Torn Payload) --> HaltScan
    
    TrailerValid -- Yes --> ApplyOps["Apply all operations in frame to KeyDir in payload order"]
    ApplyOps --> AdvanceOffset["valid_eof = scan_offset + 28 + PayloadLen + 4<br/>scan_offset = valid_eof<br/>next_seq_no = max(next_seq_no, frame.seq + 1)"]
    AdvanceOffset --> ScanLoop
    
    HaltScan --> TruncateCheck{"valid_eof < actual_file_size?"}
    TruncateCheck -- Yes --> TruncateTail["4. Truncate file to valid_eof & call File.Sync()"]
    TruncateCheck -- No --> PruneTemp
    TruncateTail --> PruneTemp
```

---

## 5. On-Disk Binary Specification (Exact Byte Offsets)

```mermaid
classDiagram
    class FileHeader32B {
        +00..08: Magic Bytes [8B] = 'KVDB2026' (0x4B56444232303236)
        +08..12: Version [4B] = 1 (u32 LE)
        +12..20: Compaction Generation [8B] (u64 LE)
        +20..28: Reserved Flags [8B] = 0 (u64 LE)
        +28..32: Header CRC32C [4B] (Checksum over bytes 0..28)
    }

    class UABEFrameHeader28B {
        +00..04: Frame Magic [4B] = 0x55414245 ('UABE')
        +04..08: Header CRC32C [4B] (Checksum over bytes 8..28)
        +08..16: Payload Length [8B] (u64 LE, total payload bytes)
        +16..24: Mutation Sequence [8B] (u64 LE, monotonic global)
        +24..28: Operation Count [4B] (u32 LE, 1 to 10,000 ops)
    }

    class OperationRecord {
        +00..01: OpType [1B] (0x01 = PUT, 0x02 = DEL)
        +01..03: Key Length [2B] (u16 LE, 1 to 1,024 bytes)
        +03..07: Value Length [4B] (u32 LE, 0 to 1,048,576 bytes; 0 for DEL)
        +07..KeyLen: Raw Key Bytes
        +KeyLen..ValLen: Raw Value Bytes
    }

    class UABEFrameTrailer4B {
        +00..04: Payload CRC32C [4B] (Castagnoli checksum over entire payload body)
    }

    FileHeader32B "1" --> "0..N" UABEFrameHeader28B : followed by
    UABEFrameHeader28B "1" --> "1..N" OperationRecord : contains payload of
    OperationRecord "1..N" --> "1" UABEFrameTrailer4B : sealed by
```

fix error
```

---

### Prompt (2026-11-06 23:08:25)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Theme: Default 
Appearance: Match VS Code 
Export 
Copy 
Reload
Unable to render this diagram
No diagram type detected matching given configuration for text: # KVDB System Architecture Diagrams (Deliverable 5)

> **Hackathon:** Kill It and Prove It - 24-Hour Engineering Hackathon  
> **Target:** Go Engine (`./kvdb`) & Python Torture Harness (`./torture`)  
> **Source of Truth:** Section 9A / `docs/common/03_SYSTEM_ARCHITECTURE.md`

---

## 1. Component Map, Concurrency & Ownership Boundaries

```mermaid
flowchart TD
    subgraph ClientAndHarness ["Client & Test Harness Isolation"]
        Harness["Python Torture Harness (./torture)"]
        CLI["CLI Driver (./kvdb put/get/del/batch/dump/check)"]
        Serve["Serve Daemon (JSON-Lines IPC Protocol)"]
    end

    subgraph ProcessIsolation ["OS Process & Locking Isolation"]
        SidecarLock["Sidecar Lock Inode (<path>.lock)<br/>Exclusive flock(LOCK_EX)"]
        ProcMutex["In-Process Concurrency Control<br/>sync.RWMutex (One Exclusive Writer)"]
    end

    subgraph MemoryState ["Volatile In-Memory Engine State (Derived Only)"]
        KeyDir["KeyDir Hash Index<br/>map[string]KeyDirEntry<br/>(Offset, ValueLen, SeqNo, Generation)"]
        QuotaGuard["Quota Guard<br/>(valid_eof + frame_size <= max_bytes)"]
        SeqCounter["Global Monotonic Sequence Counter<br/>(next_seq_no)"]
    end

    subgraph CoreEnginePipeline ["Core Engine Pipeline"]
        Encoder["UABE Frame Codec<br/>(Magic 0x55414245 + HdrCRC + Ops + PayloadCRC)"]
        LogWriter["Position-Independent Log Writer<br/>(os.File.WriteAt / pwrite)"]
        SyncBarrier["Hardware Durability Barrier<br/>(os.File.Sync / fsync)"]
        Scanner["Streaming Recovery Scanner<br/>(64KB BufReader + Dual CRC32C Validator)"]
        Compactor["Copy-on-Write Compactor<br/>(Temp File -> Sync -> Rename -> DirSync)"]
    end

    subgraph PersistentStorage ["Physical Storage Media"]
        ActiveDB[("Authoritative Log File<br/>(<path>)<br/>32B Header + Repeating UABE Frames")]
        TempDB[("Compaction Temp File<br/>(<path>.compact.tmp)")]
        ParentDir[("Parent Directory Metadata<br/>(Directory Descriptor + fsync)")]
    end
    Harness -->|Subprocess CLI / Stdin-Stdout IPC| Serve
    CLI --> ProcMutex
    Serve --> ProcMutex
    ProcMutex --> SidecarLock
    ProcMutex --> QuotaGuard
    ProcMutex --> KeyDir
    ProcMutex --> SeqCounter
    QuotaGuard -->|Quota OK| Encoder
    Encoder --> LogWriter
    LogWriter -->|1. WriteAt(frame, valid_eof)| ActiveDB
    LogWriter -->|2. File.Sync()| SyncBarrier
    SyncBarrier -->|3. Durable Barrier Return| KeyDir
    KeyDir -->|4. Publish Mutation & Advance EOF| Serve
    Serve -->|5. Emit ACK <op-id> to Client| Harness
    SidecarLock -->|Acquired Before Recovery| Scanner
    Scanner -->|Stream & Validate CRC32C| ActiveDB
    Scanner -->|Reconstruct Valid State| KeyDir
    Scanner -->|Clean Orphan Temp| TempDB
    ProcMutex -->|Exclusive Writer Lock Held| Compactor
    KeyDir -->|Iterate Live Keys| Compactor
    Compactor -->|1. Write Live Frames| TempDB
    Compactor -->|2. Temp.Sync()| SyncBarrier
    Compactor -->|3. os.Rename(temp, target)| ActiveDB
    Compactor -->|4. Dir.Sync()| ParentDir
    Compactor -->|5. Swap Descriptors & Rebuild KeyDir| KeyDir
```

---

## 2. Numbered Write & Commit Sequence (Zero Phantom ACK Guarantee)

```mermaid
sequenceDiagram
    autonumber
    actor Client as Client / Torture Tool
    participant IPC as Serve Protocol / CLI
    participant Engine as Engine State (Go)
    participant Disk as Physical Storage File
    participant KeyDir as In-Memory Index

    Client->>IPC: {"op": "put", "key": "k1", "value": "v1"}
    IPC->>Engine: Put("k1", "v1")
    Note over Engine: Step 1: Input Validation (Key <= 1KB, Val <= 1MB)
    Engine->>Engine: Acquire sync.RWMutex.Lock() [Exclusive]
    Note over Engine: Step 2: Encode UABE Frame in RAM (Header + Payload + CRC32C Trailer)
    Note over Engine: Step 3: Quota Guard Check (valid_eof + frame_len <= max_bytes)
    
    Engine->>Disk: Step 4: WriteAt(frame_bytes, valid_eof) [Full write loop]
    Engine->>Disk: Step 5: File.Sync() [fsync durability barrier]
    Disk-->>Engine: fsync returned successfully (Bytes durable on physical medium)
    
    Engine->>KeyDir: Step 6: Apply operation to KeyDir (Set k1 -> offset, len, seq)
    Engine->>Engine: Step 7: Advance valid_eof += frame_len and next_seq_no++
    Engine->>Engine: Release sync.RWMutex.Unlock()
    
    Engine-->>IPC: Operation Committed (op_id, seq)
    IPC-->>Client: ACK <op-id> (Acknowledged to caller)

    Note over Client,Disk: INVARIANT: If kill -9 strikes anywhere in Steps 1-5, NO ACK was emitted.<br/>If kill -9 strikes in Steps 6-7, recovery scanner finds valid CRC and recovers the key.
```

---

## 3. Compaction Lifecycle & Atomic Swap

```mermaid
sequenceDiagram
    autonumber
    participant Worker as Compaction Worker
    participant Engine as Engine State (Write Lock)
    participant Temp as Temp File (<path>.compact.tmp)
    participant Disk as Authoritative File (<path>)
    participant Dir as Parent Directory

    Note over Worker: Trigger: dead_bytes > 50% && total_size > 10MB
    Worker->>Engine: Acquire Exclusive mu.Lock() (Block all concurrent writes)
    Worker->>Temp: Create temp file with Header (Gen = old_gen + 1)
    
    loop For each live entry in KeyDir
        Worker->>Disk: Read live value at recorded offset
        Worker->>Temp: Append live UABE frame
    end
    
    Worker->>Temp: File.Sync() [Ensure all compacted frames reach physical storage]
    Worker->>Temp: Close temp file descriptor
    
    Worker->>Disk: os.Rename(temp_path, authoritative_path) [POSIX Atomic Rename]
    Worker->>Dir: Open parent directory & call dir.Sync() [Persist rename metadata]
    
    Worker->>Engine: Re-open authoritative file descriptor & rebuild KeyDir
    Worker->>Engine: Release mu.Unlock()
    Note over Worker,Disk: If killed before Rename: old file intact, orphan temp pruned on open.<br/>If killed after Rename: new file complete, header gen verified.
```

---

## 4. Crash Recovery Pipeline Flowchart

```mermaid
flowchart TD
    Start(["Engine Open Request (path, max_bytes)"]) --> LockSidecar["1. Open & acquire exclusive flock on <path>.lock"]
    
    LockSidecar --> CheckExists{"File exists & size >= 32?"}
    CheckExists -- No --> InitHeader["Create new file & write 32B Header (KVDB2026, Gen 0) & fsync"]
    InitHeader --> PruneTemp["Prune orphan .compact.tmp if present"]
    PruneTemp --> Ready(["Engine Online (Empty KeyDir, EOF=32)"])
    
    CheckExists -- Yes --> ReadHeader["2. Read 32-byte header & verify Magic + CRC32C"]
    ReadHeader --> HeaderValid{"Header Magic == 'KVDB2026' & Header CRC valid?"}
    HeaderValid -- No --> FatalPanic["Fatal Error: Unrecoverable header corruption"]
    
    HeaderValid -- Yes --> ScanLoop["3. Initialize Streaming BufReader at offset 32<br/>Set scan_offset = 32, valid_eof = 32"]
    ScanLoop --> ReadFrameHdr["Read 28-byte UABE Frame Header"]
    
    ReadFrameHdr --> FrameHdrValid{"Bytes read == 28 &&<br/>Magic == 0x55414245 &&<br/>HdrCRC32C valid?"}
    FrameHdrValid -- No (EOF or Torn) --> HaltScan["Stop scanning: torn/partial frame or clean EOF encountered"]
    
    FrameHdrValid -- Yes --> CheckPayloadBounds{"PayloadLen <= 64MB &&<br/>OpCount <= 10,000?"}
    CheckPayloadBounds -- No --> HaltScan
    
    CheckPayloadBounds -- Yes --> ReadPayload["Read PayloadLen bytes + 4-byte Trailer CRC"]
    ReadPayload --> TrailerValid{"Trailer CRC32C == CRC(Payload)?"}
    TrailerValid -- No (Torn Payload) --> HaltScan
    
    TrailerValid -- Yes --> ApplyOps["Apply all operations in frame to KeyDir in payload order"]
    ApplyOps --> AdvanceOffset["valid_eof = scan_offset + 28 + PayloadLen + 4<br/>scan_offset = valid_eof<br/>next_seq_no = max(next_seq_no, frame.seq + 1)"]
    AdvanceOffset --> ScanLoop
    
    HaltScan --> TruncateCheck{"valid_eof < actual_file_size?"}
    TruncateCheck -- Yes --> TruncateTail["4. Truncate file to valid_eof & call File.Sync()"]
    TruncateCheck -- No --> PruneTemp
    TruncateTail --> PruneTemp
```

---

## 5. On-Disk Binary Specification (Exact Byte Offsets)

```mermaid
classDiagram
    class FileHeader32B {
        +00..08: Magic Bytes [8B] = 'KVDB2026' (0x4B56444232303236)
        +08..12: Version [4B] = 1 (u32 LE)
        +12..20: Compaction Generation [8B] (u64 LE)
        +20..28: Reserved Flags [8B] = 0 (u64 LE)
        +28..32: Header CRC32C [4B] (Checksum over bytes 0..28)
    }

    class UABEFrameHeader28B {
        +00..04: Frame Magic [4B] = 0x55414245 ('UABE')
        +04..08: Header CRC32C [4B] (Checksum over bytes 8..28)
        +08..16: Payload Length [8B] (u64 LE, total payload bytes)
        +16..24: Mutation Sequence [8B] (u64 LE, monotonic global)
        +24..28: Operation Count [4B] (u32 LE, 1 to 10,000 ops)
    }

    class OperationRecord {
        +00..01: OpType [1B] (0x01 = PUT, 0x02 = DEL)
        +01..03: Key Length [2B] (u16 LE, 1 to 1,024 bytes)
        +03..07: Value Length [4B] (u32 LE, 0 to 1,048,576 bytes; 0 for DEL)
        +07..KeyLen: Raw Key Bytes
        +KeyLen..ValLen: Raw Value Bytes
    }

    class UABEFrameTrailer4B {
        +00..04: Payload CRC32C [4B] (Castagnoli checksum over entire payload body)
    }

    FileHeader32B "1" --> "0..N" UABEFrameHeader28B : followed by
    UABEFrameHeader28B "1" --> "1..N" OperationRecord : contains payload of
    OperationRecord "1..N" --> "1" UABEFrameTrailer4B : sealed by
```
```

---

### Prompt (2026-11-06 23:14:19)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Unable to render this diagram
Parse error on line 41:
...riter -->|1. WriteAt(frame, valid_eof)| 
-----------------------^
Expecting 'SQE', 'DOUBLECIRCLEEND', 'PE', '-)', 'STADIUMEND', 'SUBROUTINEEND', 'PIPE', 'CYLINDEREND', 'DIAMOND_STOP', 'TAGEND', 'TRAPEND', 'INVTRAPEND', 'UNICODE_TEXT', 'TEXT', 'TAGSTART', got 'PS'
```

---

### Prompt (2026-11-06 23:39:12)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
# Engine Implementation Phases — Go Storage Engine

> Owner: Engine Team  
> Language: Go  
> Primary output: `kvdb` executable and reusable engine package  
> Rule: do not mark a phase complete until its tests pass on the current commit.

## 1. Engine phase sequence

| Phase | Focus | Required result |
|---|---|---|
| E0 | Contract and package setup | Format/protocol frozen |
| E1 | Codec and file header | Byte-compatible encoder/decoder |
| E2 | Engine lifecycle and locks | Safe open/close ownership |
| E3 | Append and single operations | Durable PUT/GET/DEL |
| E4 | Atomic batch | One frame, all-or-none |
| E5 | Recovery | Validated replay and torn-tail repair |
| E6 | Serve and CLI | Judge-facing interface |
| E7 | Error and disk-full hardening | No false ACK or corrupt rollback |
| E8 | Compaction | Crash-safe replacement |
| E9 | Scale and performance | 1M-key open under 2 seconds |
| E10 | Release verification | All engine requirements demonstrated |

## 2. E0 — Contract and package setup

### Implement
- Read `common/04_SHARED_DATA_FORMAT.md` and `common/05_CLI_AND_PROTOCOL_SPEC.md`.
- Create Go module and package boundaries.
- Agree exact integer types, byte order, CRC32C coverage and error codes.
- Agree sequence-number start and increment policy.
- Agree whether ACK carries sequence number.
- Agree `max_bytes` semantics and target OS/filesystem.
- Add compile/test commands to README.

### Do not
- Invent a second frame format.
- Add a persistent index.
- Add a storage/database dependency.
- Begin compaction before append/recovery semantics are stable.

### Exit gate
- Go and Python golden-format fixtures match.
- Package builds offline.
- Design decisions are committed and reviewed.

## 3. E1 — Header and frame codec

### Implement in order
1. Encode 32-byte header.
2. Decode and validate magic/version/reserved fields.
3. Calculate and verify header CRC32C.
4. Encode 28-byte UABE header.
5. Encode PUT/DEL subrecords.
6. Calculate payload CRC32C and append trailer.
7. Decode with strict length and count limits.
8. Verify exact payload consumption.
9. Return a fully validated operation list before any application.

### Required defensive checks
- Checked integer addition and conversion.
- Payload length bounded before allocation.
- Key length 1–1,024 bytes.
- Value length 0–1 MiB.
- Operation count 1–10,000.
- DEL has zero value length.
- Unknown operation types rejected.
- Sequence order validated.
- No trailing payload bytes.

### Exit gate
- Golden hex fixtures match Python parser.
- Boundary and malformed-input tests pass.
- Fuzz tests do not panic or allocate unbounded memory.
- Codec has no filesystem side effects.

## 4. E2 — Engine lifecycle, lock ownership and file creation

### Implement
- Canonicalize database path.
- Open permanent `<db>.lock` sidecar.
- Acquire exclusive flock before recovery or temp cleanup.
- Open/create database file.
- On first creation, write header completely.
- Sync header and parent directory before accepting mutations.
- Initialize mutex, KeyDir, valid EOF, next sequence and generation.
- Define clean Close behaviour.
- Never unlink/replace the lock sidecar.

### Lifecycle rules
- No API call after Close.
- Lock order: process flock, then engine mutex.
- A second process must block or fail cleanly.
- SIGKILL must release OS lock automatically.
- Do not destructively reinitialize a potentially populated invalid-header file.

### Exit gate
- Two processes cannot own same database concurrently.
- Kill lock holder; a new opener acquires lock.
- Initial header survives close/reopen.
- Empty DB opens with correct state.

## 5. E3 — Durable append and PUT/GET/DEL

### Implement mutation path
1. Validate API input.
2. Build complete frame in memory.
3. Acquire exclusive mutex.
4. Check quota with overflow-safe arithmetic.
5. Append at `validEOF` using a full-write loop.
6. Handle short writes and EINTR.
7. Sync file.
8. Apply operation to KeyDir.
9. Advance EOF and sequence.
10. Release lock.
11. Let serve layer emit ACK.

### Implement operations
- PUT overwrites by appending a new frame.
- GET reads via ReadAt and returns a copy.
- GET distinguishes empty value from NOTFOUND.
- DEL appends tombstone and removes live index entry after sync.
- Delete missing key succeeds as required by official rules.

### Exit gate
- PUT/GET/DEL survive process restart.
- ACK only follows Sync and index update.
- Empty and binary values work.
- Same-key overwrite resolves to latest operation.
- A killed process never returns a mixed value.

## 6. E4 — Atomic batch

### Implement
- Validate every operation before disk mutation.
- Encode all subrecords into one UABE frame.
- Preserve operation order and duplicate-key semantics.
- Enforce 10,000-operation maximum.
- Check aggregate frame size before append.
- Append and sync one frame.
- Apply all operations while holding exclusive mutex.
- Do not expose intermediate KeyDir state.

### Required cases
- PUT then DEL same key.
- DEL then PUT same key.
- Duplicate key many times.
- Invalid first, middle or last operation.
- 10,000 operations.
- Batch too large for configured cap.
- Reader during batch.
- Kill during append and sync.

### Exit gate
- Recovery applies full batch or none.
- Reader sees pre-batch or post-batch state.
- No batch subset can appear.
- Every batch ACK corresponds to one durable frame.

## 7. E5 — Recovery and tail repair

### Implement
1. Acquire process lock.
2. Validate file header.
3. Set scan offset to byte 32.
4. Read fixed frame header.
5. Verify magic and header CRC.
6. Validate bounded payload length and operation count.
7. Check frame fits within file.
8. Read payload and trailer.
9. Verify payload CRC.
10. Parse all subrecords exactly.
11. Verify sequence.
12. Apply the entire frame to KeyDir.
13. Advance valid EOF.
14. At incomplete/invalid final frame, truncate to frame start.
15. Sync truncation before exposing engine.

### Important distinction
Only an invalid suffix at EOF is treated as a torn tail. Invalid bytes in the middle of the file must not cause silent loss of later data. Preserve the file and report integrity failure unless a proven resynchronization scheme exists.

### Exit gate
- Truncation at every byte boundary of a small fixture is handled.
- One-byte EOF truncation drops only final incomplete frame.
- Recovery killed during replay can restart.
- Recovery killed during truncation can restart.
- Repeated open produces identical logical state.

## 8. E6 — CLI and serve mode

### Implement
- `open`, `put`, `get`, `del`, `batch`, `dump`, `check`, `serve`.
- Exact usage and exit codes.
- Deterministic sorted dump.
- Binary-safe serve protocol.
- Request IDs and stable error codes.
- stdout protocol-only; diagnostics on stderr.
- Flush each ACK/ERR.
- Keep one engine instance open for serve lifetime.

### Exit gate
- Every command matches README examples.
- Judge can manually send a batch.
- No startup banner/debug line pollutes stdout.
- ACK response is not buffered indefinitely.
- All commands work offline from clean clone.

## 9. E7 — Error paths, quota and disk-full

### Implement
- Logical `max_bytes` precheck.
- Actual ENOSPC/EIO propagation.
- Rollback partial append to last known durable EOF.
- Sync rollback.
- Poison engine if rollback cannot be proven.
- Stable error codes.
- No KeyDir mutation on failure.
- No ACK on failed write/sync.
- Full-frame preflight for batch.

### Exit gate
- E1–E5 disk-full scenarios pass.
- Failed PUT/BATCH leaves previous state readable.
- Kill after quota rejection preserves all ACKed data.
- No subsequent write proceeds on uncertain file state.

## 10. E8 — Crash-safe compaction

### Implement
- Acquire writer lock.
- Create same-directory temp file.
- Write new header with generation increment.
- Iterate live KeyDir entries deterministically.
- Encode replacement frames.
- Sync temp.
- Close temp and check error.
- Rename temp over database.
- Sync parent directory.
- Swap database descriptor, KeyDir, EOF and sequence under lock.
- Handle orphan temp on next open.

### Required crash boundaries
- Before temp creation.
- During header and record writes.
- Before and during temp sync.
- After sync, before rename.
- During/after rename.
- Before/after directory sync.
- Before descriptor/index swap.
- During cleanup.

### Exit gate
- Old or new complete logical database survives every tested crash.
- No stale locator is returned.
- Compaction shrinks delete-heavy database.
- Subsequent writes work after successful compaction.
- Temp ENOSPC leaves old DB intact.

## 11. E9 — Performance and scale

### Implement and measure
- Preallocate KeyDir where practical.
- Use bounded reusable buffers.
- Avoid repeated allocations and copies in recovery.
- Scan sequentially.
- Avoid per-record logging in hot path.
- Profile before changing map or hash strategy.
- Measure one million keys on target laptop/filesystem.

### Exit gate
- `open` under two seconds at one million keys.
- Record median and maximum over repeated runs.
- `check` and dump hash prove correctness.
- Compaction and all crash tests still pass after optimization.

## 12. E10 — Engine release gate

- [ ] All API limits enforced.
- [ ] All eight CLI commands implemented.
- [ ] Header/frame golden tests pass.
- [ ] CRC32C is Castagnoli, not IEEE CRC32.
- [ ] Full write loops and checked arithmetic.
- [ ] ACK after Sync and KeyDir publication.
- [ ] Batch atomicity proven.
- [ ] Recovery is idempotent.
- [ ] Sidecar lock permanent.
- [ ] Disk-full safe.
- [ ] Compaction crash-safe.
- [ ] One-million-key recovery under two seconds.
- [ ] No banned dependency.
- [ ] Offline clean build succeeds.
- [ ] Engine documentation and diagram match source.

## 13. Engine phase status

| Phase | Status | Commit | Test evidence | Known issue |
|---|---|---|---|---|
| E0 Contract | NOT STARTED | — | — | — |
| E1 Codec | NOT STARTED | — | — | — |
| E2 Lifecycle | NOT STARTED | — | — | — |
| E3 Single ops | NOT STARTED | — | — | — |
| E4 Batch | NOT STARTED | — | — | — |
| E5 Recovery | NOT STARTED | — | — | — |
| E6 CLI/serve | NOT STARTED | — | — | — |
| E7 Errors | NOT STARTED | — | — | — |
| E8 Compaction | NOT STARTED | — | — | — |
| E9 Performance | NOT STARTED | — | — | — |
| E10 Release | NOT STARTED | — | — | — |
 explain me all phase in simple word

### Prompt (2026-11-07 00:54:19)

---

### Prompt (2026-11-06 20:15:59)


- **User:** LENOVO (AkashNSBhat)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
<<<<<<< HEAD
m3 done
```

---

```

---

### Prompt (2026-11-07 00:54:19)

- **User:** LENOVO (AkashNSBhat)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
m3 done
=======
@[c:\Users\LENOVO\HashCode-1\.agents\teamwork] @[c:\Users\LENOVO\HashCode-1\docs] Okay..so now refer these files and continue from where we left of and i need you to create a pr and merge after each successfull task completion and also update the memory and prompt folder and ask my permission after all completed before pushing /teamwork
>>>>>>> feature/milestone-m3-batch-recovery
```

---

### Prompt (2026-11-07 00:56:36)

- **User:** LENOVO (AkashNSBhat)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
m3 done
```

---

### Prompt (2026-11-07 01:25:54)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
@[/home/laik/workspace/srinathonn/docs] @[/home/laik/workspace/srinathonn/.agents] @[/home/laik/workspace/srinathonn/internal] do  all we are team enginge from doc read rules@[/home/laik/workspace/srinathonn/MEMORY.md]
```

---

### Prompt (2026-11-07 02:04:06)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
@[/home/laik/workspace/srinathonn/.agents] @[/home/laik/workspace/srinathonn/docs] @[/home/laik/workspace/srinathonn/torture_tool] @[/home/laik/workspace/srinathonn/internal] final fix fix structure if needed remove junk and push read pdf and test all criteria ans 1million under 2 sec read all folder i give u
```

---

### Prompt (2026-11-07 02:18:07)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
ok what is 1 million key
```

---

### Prompt (2026-11-07 02:23:04)

- **User:** laik (LYK-02)
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
can i reduce timing to under 0.5
```

---


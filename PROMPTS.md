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

<<<<<<< Updated upstream
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
=======
### Prompt (2026-11-06 15:40:07)

- **User:** ashwin
- **Tool:** Antigravity (Gemini)
- **Prompt:**
```text
Reorganize our existing Kill It and Prove It repository into three clearly separated areas:

1. `engine/` – Go database CLI, source code, tests and engine-specific `docs/`.
2. `torture_tool/` – Python torture-testing tool, oracle, tests and torture-specific `docs/`.
3. Root `docs/` – shared requirements, official scenario matrix, integration plan, team responsibilities and demo guide.

Keep the official root deliverables: `README.md`, `ARCHITECTURE.md`, `BREAKING.md`, and `PROMPTS.md`. Preserve `AGENTS.md`, `MEMORY.md`, `.agents/` rules, scripts and hooks.

Create a clean, scalable folder structure and documentation outlines. Inspect existing files before moving anything, preserve useful content, update paths and references, and avoid duplicate documentation.

Do not implement database or torture-tool code yet. Do not change the agreed Go + Python stack. First show me the proposed folder tree and migration plan for approval.
>>>>>>> Stashed changes
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


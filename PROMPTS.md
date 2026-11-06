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


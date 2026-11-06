# User Experience, Demo Excellence & Tooling Standards

> **Authority:** Hackathon Submission Excellence & Live Judging Protocol

## 1. The Judge's 15-Minute Demo Experience (Scoring & Clarity)
During the live demo, the judge drives your laptop with Wi-Fi off. Friction costs points.
- **Single-Command Build & Verification (`make demo-ready` or `./demo.sh`):**
  - One clean script that builds from scratch, runs a 50-iteration sanity torture test, and verifies all 8 `./kvdb` subcommands in under 20 seconds.
- **Interactive Demo Runbook (`DEMO.md`):**
  - A minute-by-minute cheat sheet mapped directly to the 0–15 min rubric so team members execute judge requests instantly without hesitation.
- **Crash-Here & Extension Flashcards:**
  - Quick-reference summaries for the 3 crash-here questions and the TTL expiry design extension (worth 30 points combined).

---

## 2. CLI & Engine Experience (`./torture` & `./kvdb`)
The judge watches `./torture` run for 3–5 minutes. A wall of unreadable terminal text looks fragile; a clean dashboard looks rock solid.
- **Live Terminal Progress Bar for `./torture`:**
  - Display a clean, colorized, 1-line or pinned status block:
    ```text
    [Seed: 12345] Iteration: 214/500 [42%] | Kills: 214 | ACKed: 12,840 | Oracle: 100% OK | Mode: default
    ```
- **Instant Failure Diagnostics:**
  - If an invariant ever breaks, print a clean diff: exact `op-id`, expected value from oracle vs. actual value found in dump, and the exact byte offset in the log.
- **Human-Readable `./kvdb check` Output:**
  - Instead of a silent exit or raw error, output:
    ```text
    Integrity Check: OK (1,000,000 keys verified, 0 torn records, recovery time: 1.14s)
    ```

---

## 3. Pair-Programming Experience in Antigravity
- **Interactive 32-Scenario Progress Dashboard (Generative UI):**
  - Render an interactive matrix in chat with checkboxes for all 32 scenarios (Groups A through F) that updates as we prove and verify each one.
- **Automated Context Priming:**
  - Maintain a pinned `CURRENT_FOCUS.md` or status widget in `MEMORY.md` so whenever a prompt like `Continue` is sent, the agent immediately knows the exact file, struct, and function being worked on.
- **Git Commit Helper:**
  - A helper command or workflow that automatically verifies `PROMPTS.md` and `MEMORY.md` are synchronized before creating a clean, judge-pleasing git commit.


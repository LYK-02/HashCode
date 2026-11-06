---
name: hackathon-demo-vault
description: Automatically captures, structures, and updates critical demo preparation artifacts (DEMO.md, CRASH_QUESTIONS.md, DESIGN_EXTENSION.md) for the 15-minute live judging session in the 'Kill It and Prove It' Hackathon. Activate whenever preparing for judging, updating demo runbooks, rehearsing crash-here questions, verifying offline readiness, or documenting design extensions.
---

# Hackathon Demo Vault Skill (`hackathon-demo-vault`)

The **Hackathon Demo Vault** manages the mission-critical knowledge and runbooks required to score 100/100 points during the **15-minute live offline judging session**.

During the demo:
- **Wi-Fi is OFF** (100% offline).
- The **judge drives your laptop**; the team answers.
- Time is strictly kept (15 minutes sharp).

This skill continuously synchronizes code changes into three authoritative Markdown documents in the repository root:
1. [`DEMO.md`](file://DEMO.md) — The live minute-by-minute execution runbook.
2. [`CRASH_QUESTIONS.md`](file://CRASH_QUESTIONS.md) — Answer bank for the 20-point "Crash-Here" questions.
3. [`DESIGN_EXTENSION.md`](file://DESIGN_EXTENSION.md) — Blueprint for the 10-point Key Expiry (TTL) question.

---

## 1. Managed Markdown Artifacts

### A. [`DEMO.md`](file://DEMO.md) (Live Demo Runbook)
Maps directly to Section 11 of the hackathon guide:
- **Minutes 0–1 (Build from clean clone):** The exact one-line command to build the engine and torture tool.
- **Minutes 1–4 (Torture tool execution):** `./torture --iterations 200 --seed <judge_choice>`.
- **Minutes 4–6 (Targeted torture modes):** `./torture --iterations 100 --seed <seed> --mode compaction` and `--mode recovery-crash`.
- **Minutes 6–8 (Manual kills):** Running `./kvdb serve`, piping batches, killing 5–10 times with `kill -9`, followed by `./kvdb check` and `./kvdb dump`.
- **Triage Matrix:** Quick emergency remedies if a command is mistyped or terminal freezes.

### B. [`CRASH_QUESTIONS.md`](file://CRASH_QUESTIONS.md) (Crash-Here Answer Bank)
Covers the 20-point oral defense (Minutes 8–12). The judge points at 3 arbitrary lines in the write or recovery paths and asks:
> *"The process dies right here. What is on disk? What does the next open() do?"*

The skill formats each answer using the strict three-part formula:
1. **Exact Code Location:** Function name, system call, and line number.
2. **Physical Disk State:** Which bytes are on disk vs. OS cache (record header, body, torn write, un-fsynced commit marker).
3. **Recovery Action:** Exactly how `open()` detects and handles it (e.g., CRC32 mismatch causes recovery scanner to truncate to last valid commit marker).

### C. [`DESIGN_EXTENSION.md`](file://DESIGN_EXTENSION.md) (TTL Key Expiry Architecture)
Covers the 10-point design extension question (Minutes 12–15).
> *"Add an expiry time to every key. Keys past expiry must be invisible and must be purged during compaction. Do not write code. Tell me what changes and what breaks."*

Details required:
- **On-Disk Format Change:** Adding `expires_at: u64` (epoch timestamp) to record header (+8 bytes).
- **Read Path (`get`):** Compare current timestamp with `expires_at`. If expired, treat as `NOTFOUND`.
- **Recovery Path (`open`):** Replay builds in-memory index; expired keys omitted or marked expired.
- **Compaction Purge:** Expired records dropped during copy-on-write rewrite.
- **Subtle Edge Cases Addressed:**
  - *Clock skew / backward jumps:* System clock NTP corrections. Mitigated by using monotonic clocks for relative TTL or strictly validating against last commit timestamp.
  - *Batch overwrites:* Key updated with a shorter or longer TTL in a multi-op batch.

---

## 2. Workflows & Runbooks

### Workflow 1: Pre-Demo Readiness Audit
Run before the deadline to ensure complete submission compliance:
1. Verify `README.md` has a single build command.
2. Run single-command build: `make demo-ready` or `cargo build --release`.
3. Disconnect network / disable Wi-Fi to verify 100% offline functionality.
4. Run torture sanity run: `./torture --iterations 100 --seed 42`.
5. Verify `PROMPTS.md` and `MEMORY.md` are up to date.

### Workflow 2: Updating Crash-Here References
Whenever the engine write path or recovery scanner is refactored:
1. Inspect `src/log.rs`, `src/record.rs`, and `src/recovery.rs`.
2. Update line numbers and disk state explanations in `CRASH_QUESTIONS.md`.
3. Cross-reference with `BREAKING.md` Section 1 (Crash Points Table).

### Workflow 3: Rehearsing the Oral Defense
Use this skill to quiz the team with random crash points:
- Provide a random line from the write ordering sequence.
- Require answering without looking at code.


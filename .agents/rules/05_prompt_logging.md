# AI Prompt Logging Policy

> **Authority:** "Kill It and Prove It" Hackathon Guide (Section 9, Section 13)

## 1. Hackathon Requirement
Judges review `PROMPTS.md` to see the team's engineering journey and critical thinking:
- **Mandatory Content:** Every prompt sent to any AI tool during the event in chronological order, unedited.
- **Metadata:** Include the AI tool name (e.g. `Antigravity (Gemini)`) and approximate timestamp.
- **Penalty for Missing:** -10 points if `PROMPTS.md` is omitted or empty.
- **Penalty for Falsification:** Immediate disqualification.

## 2. Agent Responsibilities
1. **Automated Logging:** The lifecycle hook in `.agents/hooks.json` triggers `.agents/scripts/prompt_logger.py` on `PreInvocation` to record incoming user instructions.
2. **Double-Check Invariant:** Before completing any user turn, the agent verifies that the latest user prompt is logged in `PROMPTS.md`. If missed, the agent appends it immediately.
3. **No Falsification or Deletion:** Under no circumstances should existing prompt entries in `PROMPTS.md` be pruned, rewritten, or deleted.
4. **Transparent Engineering:** When prompts push back, diagnose bugs found by `./torture`, or clarify architectural trade-offs, they showcase authentic engineering depth that scores points during judging.


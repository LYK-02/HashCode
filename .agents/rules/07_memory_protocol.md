# Agent Memory & Execution Journal Protocol

> **Authority:** Workspace Rule (`MEMORY.md`)

## 1. The Memory Invariant
Before executing any tool call, code generation, refactoring, or architectural change:
1. **Context Update:** The agent must ensure `MEMORY.md` reflects the current user request, prompt context, and operational state.
2. **Standard Entry Format:** Every major action or prompt must record:
   - **Date & Time:** Exact ISO / local timestamp.
   - **User (Prompted by):** The username / author of the prompt (e.g. `laik (LYK-02)`).
   - **Implementer (Executed by):** The worker / AI agent who implemented it (e.g. `Antigravity (Gemini)`).
   - **What:** The specific action, feature, or bug being addressed.
   - **Why:** The underlying technical requirement, invariant guarantee, or hackathon constraint.
   - **Fix / Actions:** The precise files modified, system calls used, or code implemented.
   - **Status:** Current operational state (`ACTIVE`, `IN_PROGRESS`, `COMPLETED`, `VERIFIED`).
3. **No Amnesia:** If a session resets, the agent reads `MEMORY.md` first to reconstruct the exact mental model, current phase, and outstanding items.

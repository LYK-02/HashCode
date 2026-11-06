# Git Synchronization Protocol

> **Authority:** Multi-Member Teamwork & Continuous Repository Synchronization

## 1. Mandatory Pre-Execution Sync Invariant
Before executing any task, code implementation, refactoring, or architectural change:
1. **Always Pull First (`git pull`):** The agent must synchronize the local repository with the remote repository (`origin/main`) before performing any work.
2. **Multi-Member Protection:** In a hackathon team (2 to 4 members), teammates may commit or push changes concurrently. Pulling before any action prevents merge conflicts, eliminates race conditions, and guarantees the agent builds upon the latest team codebase.
3. **Execution Handling:** Since network operations and `.git` updates require write access, `git pull` must be executed with appropriate permissions (`BypassSandbox: true` when sandboxed) to reach the remote repository cleanly.
4. **Offline Hackathon Mode (Hour 24):** During the live 15-minute demo, Wi-Fi is disabled; the local repository becomes authoritative and offline rules apply per Section 11 of the competition guide.


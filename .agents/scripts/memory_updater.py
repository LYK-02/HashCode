#!/usr/bin/env python3
"""
Automated Memory & Execution Journal Updater.
Maintains MEMORY.md with Date, Time, User, Implementer, What, Why, Fix/Actions, and Status.
"""

import sys
import os
import json
import getpass
import subprocess
from datetime import datetime

def get_user_identity():
    try:
        system_user = getpass.getuser()
    except Exception:
        system_user = "laik"
    
    try:
        git_user = subprocess.check_output(
            ["git", "config", "user.name"], 
            stderr=subprocess.DEVNULL
        ).decode().strip()
    except Exception:
        git_user = ""
    
    if git_user and git_user != system_user:
        return f"{system_user} ({git_user})"
    return system_user

def main():
    payload = {}
    try:
        raw_input = sys.stdin.read()
        if raw_input.strip():
            payload = json.loads(raw_input)
    except Exception:
        payload = {}

    user_input = payload.get("lastUserInput", "").strip()
    workspace_paths = payload.get("workspacePaths", [])
    
    if workspace_paths and os.path.isdir(workspace_paths[0]):
        workspace_root = workspace_paths[0]
    else:
        workspace_root = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))

    memory_file = os.path.join(workspace_root, "MEMORY.md")
    now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    username = get_user_identity()
    implementer = "Antigravity (Gemini)"

    # If user_input is present, log prompt context entry
    if user_input:
        state_file = os.path.join(workspace_root, ".agents", ".memory_updater_state.json")
        last_prompt = None
        if os.path.exists(state_file):
            try:
                with open(state_file, "r") as sf:
                    st = json.load(sf)
                    last_prompt = st.get("last_prompt")
            except Exception:
                pass

        if user_input != last_prompt and os.path.exists(memory_file):
            entry = (
                f"\n### [{now}] Prompt Context Event\n"
                f"- **User (Prompted by):** {username}\n"
                f"- **Implementer (Executed by):** {implementer}\n"
                f"- **What:** User requested: `{user_input}`\n"
                f"- **Why:** Direct user instruction driving project evolution.\n"
                f"- **Fix / Implementation Plan:** In-progress (tracked by active agent turn).\n"
                f"- **Status:** ACTIVE\n"
            )
            with open(memory_file, "a", encoding="utf-8") as mf:
                mf.write(entry)

            try:
                with open(state_file, "w", encoding="utf-8") as sf:
                    json.dump({"last_prompt": user_input, "timestamp": now}, sf)
            except Exception:
                pass

    print("{}")

if __name__ == "__main__":
    main()

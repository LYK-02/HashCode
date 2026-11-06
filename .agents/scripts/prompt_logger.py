#!/usr/bin/env python3
"""
Automated AI Prompt Logger for 'Kill It and Prove It' Hackathon.
Deliverable 4 Requirement:
"A file PROMPTS.md (or a folder prompts/) containing every prompt you sent to
any AI tool during the hackathon, in chronological order, unedited. Include the
tool name and approximate time."
"""

import sys
import os
import json
import hashlib
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
    try:
        raw_input = sys.stdin.read()
        if not raw_input.strip():
            print("{}")
            return

        payload = json.loads(raw_input)
    except Exception:
        print("{}")
        return

    user_input = payload.get("lastUserInput")
    if not user_input or not user_input.strip():
        print("{}")
        return

    # Find workspace root
    workspace_paths = payload.get("workspacePaths", [])
    if workspace_paths and os.path.isdir(workspace_paths[0]):
        workspace_root = workspace_paths[0]
    else:
        workspace_root = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))

    prompts_file = os.path.join(workspace_root, "PROMPTS.md")
    state_file = os.path.join(workspace_root, ".agents", ".prompt_logger_state.json")

    # Deduplicate against last logged prompt
    prompt_hash = hashlib.sha256(user_input.encode("utf-8")).hexdigest()
    last_hash = None
    if os.path.exists(state_file):
        try:
            with open(state_file, "r") as sf:
                state = json.load(sf)
                last_hash = state.get("last_hash")
        except Exception:
            pass

    if prompt_hash == last_hash:
        print("{}")
        return

    # Prepare markdown header if file doesn't exist
    if not os.path.exists(prompts_file):
        header = (
            "# Deliverable 4: AI Prompt Log (PROMPTS.md)\n\n"
            "> **Hackathon:** Kill It and Prove It - 24-Hour Engineering Hackathon\n"
            "> **Requirement:** Contains every prompt sent to any AI tool during the hackathon in chronological order, unedited, with tool name and approximate timestamp.\n\n"
            "---\n\n"
        )
        with open(prompts_file, "w", encoding="utf-8") as pf:
            pf.write(header)

    now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    tool_name = "Antigravity (Gemini)"
    user_name = get_user_identity()

    entry = (
        f"### Prompt ({now})\n\n"
        f"- **User:** {user_name}\n"
        f"- **Tool:** {tool_name}\n"
        f"- **Prompt:**\n"
        f"```text\n{user_input.strip()}\n```\n\n"
        f"---\n\n"
    )

    with open(prompts_file, "a", encoding="utf-8") as pf:
        pf.write(entry)

    # Update state
    try:
        with open(state_file, "w", encoding="utf-8") as sf:
            json.dump({"last_hash": prompt_hash, "timestamp": now}, sf)
    except Exception:
        pass

    print("{}")

if __name__ == "__main__":
    main()

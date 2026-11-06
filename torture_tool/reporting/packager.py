"""Automated failure forensic artifact bundle packager and reproduction generator."""

from __future__ import annotations

import json
import os
import shutil
import stat
import subprocess
from dataclasses import asdict
from pathlib import Path
from typing import Any, Dict, Optional

from torture_tool.cli import RunConfig
from torture_tool.runner.iteration import IterationResult
from torture_tool.verifier.comparator import DiffReport


def get_git_commit(cwd: Optional[Path] = None) -> str:
    """Extract current git commit hash, or 'unknown' if outside git."""
    try:
        res = subprocess.run(
            ["git", "rev-parse", "HEAD"],
            capture_output=True,
            text=True,
            timeout=2.0,
            cwd=cwd,
        )
        if res.returncode == 0:
            return res.stdout.strip()
    except Exception:
        pass
    return "unknown"


class FailurePackager:
    """Preserves self-contained forensic evidence when a crash test fails."""

    @classmethod
    def preserve_failure(
        cls,
        iteration_result: IterationResult,
        config: RunConfig,
        db_path: Path,
        journal_path: Path,
        diff_report: Optional[DiffReport] = None,
        raw_dump_text: Optional[str] = None,
    ) -> Path:
        """Create a complete artifact directory containing database copy, logs, diff, and reproduce.sh."""
        timestamp = int(os.environ.get("SOURCE_DATE_EPOCH", 0)) or 1730900000
        bundle_name = (
            f"failure_iter_{iteration_result.iteration:04d}_seed_{iteration_result.seed}"
        )
        target_dir = config.artifact_dir / bundle_name
        target_dir.mkdir(parents=True, exist_ok=True)

        # 1. Snapshot database file
        if db_path.exists():
            shutil.copy2(db_path, target_dir / "corrupted.db")
        mock_state = db_path.with_name(db_path.name + ".state")
        if mock_state.exists():
            shutil.copy2(mock_state, target_dir / "corrupted.db.state")

        # 2. Copy oracle journal
        if journal_path.exists():
            shutil.copy2(journal_path, target_dir / "oracle_events.jsonl")

        # 3. Write raw dump text
        if raw_dump_text is not None:
            (target_dir / "actual_dump.txt").write_text(raw_dump_text, encoding="utf-8")

        # 4. Write human readable diff report
        if diff_report is not None:
            diff_text = diff_report.generate_human_diff()
            (target_dir / "mismatch_diff.txt").write_text(diff_text, encoding="utf-8")

        # 5. Write manifest.json
        manifest: Dict[str, Any] = {
            "iteration": iteration_result.iteration,
            "seed": iteration_result.seed,
            "mode": iteration_result.mode,
            "delay_ms": iteration_result.delay_ms,
            "in_flight_at_kill": iteration_result.in_flight_at_kill,
            "operations_started": iteration_result.operations_started,
            "acks_received": iteration_result.acks_received,
            "git_commit": get_git_commit(),
            "config": {
                "workers": config.workers,
                "timeout": config.timeout,
                "db_path": str(config.db_path),
                "kvdb_bin": str(config.kvdb_bin),
            },
            "violations": [
                {
                    "type": str(v.violation_type),
                    "description": v.description,
                    "op_id": v.op_id,
                }
                for v in (iteration_result.eval_result.violations if iteration_result.eval_result else [])
            ],
        }
        with open(target_dir / "manifest.json", "w", encoding="utf-8") as f:
            json.dump(manifest, f, indent=2)

        # 6. Generate executable reproduce.sh script
        reproduce_script = (
            "#!/usr/bin/env bash\n"
            "# Auto-generated crash reproduction script\n"
            "set -euo pipefail\n\n"
            f"# Command to reproduce iteration {iteration_result.iteration}:\n"
            f"./torture --seed {iteration_result.seed} --iterations 1 --mode {iteration_result.mode} --workers {config.workers} --db reproduce.db\n"
        )
        reproduce_path = target_dir / "reproduce.sh"
        reproduce_path.write_text(reproduce_script, encoding="utf-8")
        reproduce_path.chmod(reproduce_path.stat().st_mode | stat.S_IEXEC)

        return target_dir


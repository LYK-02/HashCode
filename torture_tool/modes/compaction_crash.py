"""Adversarial mode targeting compaction crashes and atomic rename swaps (Group C scenarios)."""

from __future__ import annotations

import os
import signal
import subprocess
import time
from pathlib import Path
from typing import Dict, List, Optional

from torture_tool.cli import RunConfig
from torture_tool.oracle.evaluator import EvaluationResult, OracleEvaluator
from torture_tool.oracle.journal import OracleJournal
from torture_tool.runner.iteration import IterationResult, parse_raw_dump


class CompactionCrashRunner:
    """Stress tests compaction by killing the process while writing temp files or swapping."""

    def __init__(self, config: RunConfig) -> None:
        self.config = config

    def run_compaction_crash(
        self,
        iteration: int,
        db_file: Path,
        kill_delay_ms: float = 10.0,
    ) -> IterationResult:
        start_time = time.monotonic()
        seed = self.config.seed + iteration

        initial_size = db_file.stat().st_size if db_file.exists() else 0

        # Trigger compaction command: ./kvdb compact <db_path>
        cmd = [str(self.config.kvdb_bin), "compact", str(db_file)]
        killed = False
        try:
            proc = subprocess.Popen(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                start_new_session=True,
            )
            time.sleep(kill_delay_ms / 1000.0)
            if proc.poll() is None:
                try:
                    os.killpg(os.getpgid(proc.pid), signal.SIGKILL)
                except OSError:
                    pass
                proc.wait(timeout=2.0)
                killed = True
            if proc.stdout:
                proc.stdout.close()
            if proc.stderr:
                proc.stderr.close()
        except Exception:
            pass

        # Post-crash check: verify db is not corrupted
        check_proc = subprocess.run(
            [str(self.config.kvdb_bin), "check", str(db_file)],
            capture_output=True,
            text=True,
            timeout=self.config.timeout,
        )
        check_passed = check_proc.returncode == 0 and "OK" in check_proc.stdout

        dump_proc = subprocess.run(
            [str(self.config.kvdb_bin), "dump", str(db_file)],
            capture_output=True,
            text=True,
            timeout=self.config.timeout,
        )
        dump_passed = dump_proc.returncode == 0

        duration = (time.monotonic() - start_time) * 1000.0
        passed = check_passed and dump_passed

        return IterationResult(
            iteration=iteration,
            seed=seed,
            mode="compaction",
            passed=passed,
            delay_ms=kill_delay_ms,
            in_flight_at_kill=1 if killed else 0,
            operations_started=1,
            acks_received=0,
            unresolved_ops=0,
            check_passed=check_passed,
            duration_ms=duration,
        )

"""Adversarial mode targeting recovery-time crash injection (Group B scenarios)."""

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


class RecoveryCrashRunner:
    """Stress tests crash recovery by killing the engine while it is replaying or truncating."""

    def __init__(self, config: RunConfig) -> None:
        self.config = config

    def run_nested_recovery_crash(
        self,
        iteration: int,
        db_file: Path,
        nested_kills: int = 3,
        replay_delay_ms: float = 2.0,
    ) -> IterationResult:
        start_time = time.monotonic()
        seed = self.config.seed + iteration

        # 1. Fire open and interrupt during replay
        kills_delivered = 0
        for k in range(nested_kills):
            cmd = [str(self.config.kvdb_bin), "open", str(db_file)]
            try:
                proc = subprocess.Popen(
                    cmd,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    start_new_session=True,
                )
                # Sleep tiny duration targeting active replay/recovery
                time.sleep(replay_delay_ms / 1000.0)
                if proc.poll() is None:
                    try:
                        os.killpg(os.getpgid(proc.pid), signal.SIGKILL)
                    except OSError:
                        pass
                    proc.wait(timeout=2.0)
                    kills_delivered += 1
                if proc.stdout:
                    proc.stdout.close()
                if proc.stderr:
                    proc.stderr.close()
            except Exception:
                pass

        # 2. Final recovery run allowed to complete
        open_cmd = [str(self.config.kvdb_bin), "open", str(db_file)]
        open_passed = False
        try:
            res = subprocess.run(
                open_cmd,
                capture_output=True,
                text=True,
                timeout=self.config.timeout,
            )
            open_passed = res.returncode == 0
        except Exception:
            open_passed = False

        # 3. Check and Dump
        check_passed = False
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
        actual_state = parse_raw_dump(dump_proc.stdout) if dump_passed else {}

        duration = (time.monotonic() - start_time) * 1000.0
        passed = open_passed and check_passed and dump_passed

        return IterationResult(
            iteration=iteration,
            seed=seed,
            mode="recovery-crash",
            passed=passed,
            delay_ms=replay_delay_ms,
            in_flight_at_kill=kills_delivered,
            operations_started=0,
            acks_received=0,
            unresolved_ops=0,
            check_passed=check_passed,
            duration_ms=duration,
        )

"""Adversarial mode targeting recovery-time crash injection (Group B scenarios)."""

from __future__ import annotations

import os
import random
import signal
import subprocess
import time
from pathlib import Path
from typing import Dict, List, Optional

from torture_tool.cli import RunConfig
from torture_tool.concurrency.dispatcher import ResponseDispatcher
from torture_tool.concurrency.writer import StdinWriter
from torture_tool.oracle.evaluator import EvaluationResult, OracleEvaluator
from torture_tool.oracle.journal import OracleJournal
from torture_tool.protocol.spec import AckResponse, PutRequest
from torture_tool.runner.iteration import IterationResult, parse_raw_dump


class RecoveryCrashRunner:
    """Stress tests crash recovery by killing the engine while it is replaying or truncating."""

    def __init__(self, config: RunConfig) -> None:
        self.config = config

    def run_nested_recovery_crash(
        self,
        iteration: int,
        db_file: Path,
        journal_path: Optional[Path] = None,
        nested_kills: int = 3,
        replay_delay_ms: float = 2.0,
    ) -> IterationResult:
        start_time = time.monotonic()
        seed = self.config.seed + iteration
        prng = random.Random(seed)

        # Ensure journal path exists
        if journal_path is None:
            journal_path = self.config.artifact_dir / f"oracle_recovery_iter_{iteration:04d}.jsonl"
        journal = OracleJournal(journal_path)

        # 1. Reset / initialize database for clean recovery cycle
        if db_file.exists():
            try:
                db_file.unlink()
                lock_file = Path(str(db_file) + ".lock")
                if lock_file.exists():
                    lock_file.unlink()
            except OSError:
                pass

        init_proc = subprocess.run(
            [str(self.config.kvdb_bin), "open", str(db_file)],
            capture_output=True,
            text=True,
            timeout=self.config.timeout,
        )
        if init_proc.returncode != 0:
            journal.close()
            return IterationResult(
                iteration=iteration,
                seed=seed,
                mode="recovery-crash",
                passed=False,
                delay_ms=0.0,
                in_flight_at_kill=0,
                operations_started=0,
                acks_received=0,
                unresolved_ops=0,
                check_passed=False,
                error_message=f"Failed to initialize database: {init_proc.stderr}",
            )

        # 2. Populate non-trivial log volume via streaming serve mode
        serve_cmd = [str(self.config.kvdb_bin), "serve", str(db_file)]
        proc = subprocess.Popen(
            serve_cmd,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            start_new_session=True,
        )
        dispatcher = ResponseDispatcher(proc.stdout)
        dispatcher.start()
        writer = StdinWriter(proc.stdin)

        num_keys = prng.randint(50, 150)
        ops_started = 0
        acks_received = 0

        for i in range(num_keys):
            op_id = f"rec-{iteration}-{i:04d}"
            key = f"key_{prng.randint(0, 30):04d}".encode("ascii")
            val_len = prng.randint(32, 256)
            val = prng.randbytes(val_len)

            req = PutRequest(op_id=op_id, key=key, value=val)
            journal.log_start(iteration, req)
            fut = dispatcher.register(op_id)
            ops_started += 1
            writer.write_request(req)

            try:
                resp = fut.result(timeout=self.config.timeout)
                if isinstance(resp, AckResponse):
                    acks_received += 1
                    journal.log_ack(iteration, op_id)
            except Exception:
                break

        dispatcher.stop()
        if proc.poll() is None:
            try:
                os.killpg(os.getpgid(proc.pid), signal.SIGTERM)
                proc.wait(timeout=2.0)
            except (OSError, subprocess.TimeoutExpired):
                try:
                    os.killpg(os.getpgid(proc.pid), signal.SIGKILL)
                    proc.wait(timeout=2.0)
                except OSError:
                    pass
        if proc.stdout:
            proc.stdout.close()
        if proc.stderr:
            proc.stderr.close()

        journal.close()

        # 3. Inject torn tail record to simulate uncommitted write during crash
        torn_bytes = prng.randbytes(prng.randint(8, 64))
        with open(db_file, "ab") as f:
            f.write(torn_bytes)

        # 4. Fire open and interrupt during replay (nested kills)
        kills_delivered = 0
        for k in range(nested_kills):
            cmd = [str(self.config.kvdb_bin), "open", str(db_file)]
            try:
                rec_proc = subprocess.Popen(
                    cmd,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    start_new_session=True,
                )
                jitter_delay = (replay_delay_ms + prng.uniform(0.0, 2.0)) / 1000.0
                time.sleep(jitter_delay)
                if rec_proc.poll() is None:
                    try:
                        os.killpg(os.getpgid(rec_proc.pid), signal.SIGKILL)
                    except OSError:
                        pass
                    rec_proc.wait(timeout=2.0)
                    kills_delivered += 1
                if rec_proc.stdout:
                    rec_proc.stdout.close()
                if rec_proc.stderr:
                    rec_proc.stderr.close()
            except Exception:
                pass

        # 5. Final recovery run allowed to complete
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

        # 6. Integrity check and state verification against Oracle
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

        events = list(OracleJournal.read_events(journal_path))
        eval_result = OracleEvaluator.evaluate(events, actual_state)

        duration = (time.monotonic() - start_time) * 1000.0
        passed = open_passed and check_passed and dump_passed and eval_result.passed and (acks_received > 0)

        err_msg = ""
        if not passed:
            err_msg = (
                f"Recovery check failed (open={open_passed}, check={check_passed}, "
                f"dump={dump_passed}, oracle={eval_result.passed}, acks={acks_received})"
            )
            if eval_result.violations:
                err_msg += f": {eval_result.violations[0].format_diagnostic()}"

        return IterationResult(
            iteration=iteration,
            seed=seed,
            mode="recovery-crash",
            passed=passed,
            delay_ms=replay_delay_ms,
            in_flight_at_kill=kills_delivered,
            operations_started=ops_started,
            acks_received=acks_received,
            unresolved_ops=0,
            check_passed=check_passed,
            duration_ms=duration,
            error_message=err_msg,
        )

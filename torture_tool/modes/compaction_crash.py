"""Adversarial mode targeting compaction crashes and atomic rename swaps (Group C scenarios)."""

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
from torture_tool.protocol.spec import AckResponse, DelRequest, PutRequest
from torture_tool.runner.iteration import IterationResult, parse_raw_dump


class CompactionCrashRunner:
    """Stress tests compaction by generating churn and killing the process during maintenance."""

    def __init__(self, config: RunConfig) -> None:
        self.config = config

    def run_compaction_crash(
        self,
        iteration: int,
        db_file: Path,
        journal_path: Optional[Path] = None,
        kill_delay_ms: float = 5.0,
    ) -> IterationResult:
        start_time = time.monotonic()
        seed = self.config.seed + iteration
        prng = random.Random(seed)

        if journal_path is None:
            journal_path = self.config.artifact_dir / f"oracle_compaction_iter_{iteration:04d}.jsonl"
        if journal_path.exists():
            try:
                journal_path.unlink()
            except OSError:
                pass
        journal = OracleJournal(journal_path)

        # 1. Reset / initialize database
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
                mode="compaction",
                passed=False,
                delay_ms=0.0,
                in_flight_at_kill=0,
                operations_started=0,
                acks_received=0,
                unresolved_ops=0,
                check_passed=False,
                error_message=f"Failed to initialize database: {init_proc.stderr}",
            )

        # 2. Generate heavy overwrite & delete churn on a small key space (creating obsolete log data)
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

        key_pool = [f"k_{k:02d}".encode("ascii") for k in range(20)]
        ops_started = 0
        acks_received = 0

        # Phase 2a: Write initial keys with sizable payloads
        for i, k in enumerate(key_pool):
            op_id = f"cmp-put-{iteration}-{i:03d}"
            v = prng.randbytes(prng.randint(128, 512))
            req = PutRequest(op_id=op_id, key=k, value=v)
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

        # Phase 2b: Overwrites and deletions to create obsolete records
        num_churn_ops = prng.randint(60, 120)
        for i in range(num_churn_ops):
            k = prng.choice(key_pool)
            op_id = f"cmp-churn-{iteration}-{i:03d}"
            # 60% chance overwrite, 40% chance delete
            if prng.random() < 0.6:
                v = prng.randbytes(prng.randint(128, 512))
                req = PutRequest(op_id=op_id, key=k, value=v)
            else:
                req = DelRequest(op_id=op_id, key=k)

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

        size_before = db_file.stat().st_size if db_file.exists() else 0

        # 3. Trigger compaction command with crash injection: ./kvdb compact <db_path>
        cmd = [str(self.config.kvdb_bin), "compact", str(db_file)]
        killed = False
        try:
            cmp_proc = subprocess.Popen(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                start_new_session=True,
            )
            # Add small random jitter around kill delay
            jitter = (kill_delay_ms + prng.uniform(0.0, 3.0)) / 1000.0
            time.sleep(jitter)
            if cmp_proc.poll() is None:
                try:
                    os.killpg(os.getpgid(cmp_proc.pid), signal.SIGKILL)
                except OSError:
                    pass
                cmp_proc.wait(timeout=2.0)
                killed = True
            if cmp_proc.stdout:
                cmp_proc.stdout.close()
            if cmp_proc.stderr:
                cmp_proc.stderr.close()
        except Exception:
            pass

        # 4. If killed mid-compaction, allow a clean compaction to verify completion
        clean_compact_passed = True
        if killed:
            clean_res = subprocess.run(
                [str(self.config.kvdb_bin), "compact", str(db_file)],
                capture_output=True,
                text=True,
                timeout=self.config.timeout,
            )
            clean_compact_passed = clean_res.returncode == 0

        size_after = db_file.stat().st_size if db_file.exists() else 0
        shrunk = (size_after <= size_before)

        # 5. Post-crash check & dump: verify db consistency against Oracle
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
        passed = (
            check_passed
            and dump_passed
            and clean_compact_passed
            and eval_result.passed
            and (acks_received > 0)
            and shrunk
        )

        err_msg = ""
        if not passed:
            err_msg = (
                f"Compaction check failed (check={check_passed}, dump={dump_passed}, "
                f"compact={clean_compact_passed}, oracle={eval_result.passed}, "
                f"acks={acks_received}, shrunk={shrunk})"
            )
            if eval_result.violations:
                err_msg += f": {eval_result.violations[0].format_diagnostic()}"

        return IterationResult(
            iteration=iteration,
            seed=seed,
            mode="compaction",
            passed=passed,
            delay_ms=kill_delay_ms,
            in_flight_at_kill=1 if killed else 0,
            operations_started=ops_started,
            acks_received=acks_received,
            unresolved_ops=0,
            check_passed=check_passed,
            duration_ms=duration,
            error_message=err_msg,
        )

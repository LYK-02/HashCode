"""Adversarial mode targeting disk-full quotas and write error boundaries (Group E scenarios)."""

from __future__ import annotations

import os
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
from torture_tool.process.controller import ChildProcessController
from torture_tool.protocol.spec import AckResponse, DelRequest, ErrResponse, PutRequest
from torture_tool.protocol.utils import bytes_to_hex
from torture_tool.runner.iteration import IterationResult, parse_raw_dump


class DiskFullRunner:
    """Verifies that exceeding --max-bytes quota safely rejects writes with ERR and prevents corruption."""

    def __init__(self, config: RunConfig) -> None:
        self.config = config

    def run_disk_full_quota_cycle(
        self,
        iteration: int,
        db_file: Path,
        journal_path: Optional[Path] = None,
        max_bytes: int = 500 * 1024,  # 500 KiB quota
    ) -> IterationResult:
        start_time = time.monotonic()
        seed = self.config.seed + iteration

        if journal_path is None:
            journal_path = self.config.artifact_dir / f"oracle_diskfull_iter_{iteration:04d}.jsonl"
        if journal_path.exists():
            try:
                journal_path.unlink()
            except OSError:
                pass
        journal = OracleJournal(journal_path)

        # Ensure fresh database for quota cycle
        if db_file.exists():
            try:
                db_file.unlink()
                lock_file = Path(str(db_file) + ".lock")
                if lock_file.exists():
                    lock_file.unlink()
            except OSError:
                pass

        # 1. Initialize database with --max-bytes quota parameter on open
        open_proc = subprocess.run(
            [str(self.config.kvdb_bin), "open", str(db_file), "--max-bytes", str(max_bytes)],
            capture_output=True,
            text=True,
            timeout=self.config.timeout,
        )
        if open_proc.returncode != 0:
            journal.close()
            return IterationResult(
                iteration=iteration,
                seed=seed,
                mode="disk-full",
                passed=False,
                delay_ms=0.0,
                in_flight_at_kill=0,
                operations_started=0,
                acks_received=0,
                unresolved_ops=0,
                check_passed=False,
                error_message=f"Failed to open database with --max-bytes: {open_proc.stderr}",
            )

        # 2. Start streaming serve mode
        cmd = [str(self.config.kvdb_bin), "serve", str(db_file)]
        ctrl = ChildProcessController(
            command=cmd,
            db_path=db_file,
            startup_timeout=self.config.timeout,
        )
        try:
            ctrl.start()
        except Exception as e:
            journal.close()
            return IterationResult(
                iteration=iteration,
                seed=seed,
                mode="disk-full",
                passed=False,
                delay_ms=0.0,
                in_flight_at_kill=0,
                operations_started=0,
                acks_received=0,
                unresolved_ops=0,
                check_passed=False,
                error_message=f"Failed to start serve: {e}",
            )

        dispatcher = ResponseDispatcher(ctrl.process.stdout)
        dispatcher.start()
        writer = StdinWriter(ctrl.process.stdin)

        # 3. Issue writes until quota rejection (ERR QUOTA_EXCEEDED) is received
        acks = 0
        errs = 0
        ops = 0
        payload_64k = b"x" * (64 * 1024)

        for i in range(1, 25):
            op_id = f"q-{iteration}-{i:03d}"
            req = PutRequest(op_id=op_id, key=f"key_{i}".encode("ascii"), value=payload_64k)
            journal.log_start(iteration, req)
            fut = dispatcher.register(op_id)
            ops += 1
            writer.write_request(req)

            try:
                resp = fut.result(timeout=5.0)
                if isinstance(resp, AckResponse):
                    acks += 1
                    journal.log_ack(iteration, op_id)
                elif isinstance(resp, ErrResponse):
                    errs += 1
                    journal.log_fail(iteration, op_id, resp.code, resp.message)
                    # Quota hit successfully!
                    break
            except Exception:
                break

        # 4. Crash process immediately after rejection to verify durability & absence of corruption
        ctrl.kill()
        dispatcher.stop()

        # 5. Verify database passes open (recovery), check, and dump against Oracle
        open_proc = subprocess.run(
            [str(self.config.kvdb_bin), "open", str(db_file)],
            capture_output=True,
            text=True,
            timeout=self.config.timeout,
        )
        open_passed = open_proc.returncode == 0

        check_proc = subprocess.run(
            [str(self.config.kvdb_bin), "check", str(db_file)],
            capture_output=True,
            text=True,
            timeout=self.config.timeout,
        )
        check_passed = open_passed and check_proc.returncode == 0 and "OK" in check_proc.stdout

        dump_proc = subprocess.run(
            [str(self.config.kvdb_bin), "dump", str(db_file)],
            capture_output=True,
            text=True,
            timeout=self.config.timeout,
        )
        dump_passed = dump_proc.returncode == 0
        actual_state = parse_raw_dump(dump_proc.stdout) if dump_passed else {}

        # 6. Reclaim space verification: delete keys, compact, and verify subsequent write succeeds
        reclaim_succeeded = False
        reclaim_proc = subprocess.Popen(
            [str(self.config.kvdb_bin), "serve", str(db_file)],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            start_new_session=True,
        )
        rec_disp = ResponseDispatcher(reclaim_proc.stdout)
        rec_disp.start()
        rec_writer = StdinWriter(reclaim_proc.stdin)

        # Delete previously ACKed keys
        for i in range(1, min(acks + 1, 4)):
            del_op_id = f"del-{iteration}-{i:03d}"
            del_req = DelRequest(op_id=del_op_id, key=f"key_{i}".encode("ascii"))
            journal.log_start(iteration, del_req)
            del_fut = rec_disp.register(del_op_id)
            rec_writer.write_request(del_req)
            try:
                del_resp = del_fut.result(timeout=5.0)
                if isinstance(del_resp, AckResponse):
                    journal.log_ack(iteration, del_op_id)
            except Exception:
                pass

        rec_disp.stop()
        if reclaim_proc.poll() is None:
            reclaim_proc.terminate()
            reclaim_proc.wait(timeout=2.0)
        if reclaim_proc.stdout:
            reclaim_proc.stdout.close()
        if reclaim_proc.stderr:
            reclaim_proc.stderr.close()

        # Run compaction to reclaim disk space
        cmp_res = subprocess.run(
            [str(self.config.kvdb_bin), "compact", str(db_file)],
            capture_output=True,
            text=True,
            timeout=self.config.timeout,
        )
        if cmp_res.returncode == 0:
            # Open serve again and verify a new write succeeds
            post_serve = subprocess.Popen(
                [str(self.config.kvdb_bin), "serve", str(db_file)],
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                start_new_session=True,
            )
            post_disp = ResponseDispatcher(post_serve.stdout)
            post_disp.start()
            post_writer = StdinWriter(post_serve.stdin)

            post_op_id = f"post-reclaim-{iteration}"
            post_req = PutRequest(op_id=post_op_id, key=b"reclaim_key", value=b"reclaimed_val")
            journal.log_start(iteration, post_req)
            post_fut = post_disp.register(post_op_id)
            post_writer.write_request(post_req)
            try:
                post_resp = post_fut.result(timeout=5.0)
                if isinstance(post_resp, AckResponse):
                    reclaim_succeeded = True
                    journal.log_ack(iteration, post_op_id)
            except Exception:
                pass

            post_disp.stop()
            if post_serve.poll() is None:
                post_serve.terminate()
                post_serve.wait(timeout=2.0)
            if post_serve.stdout:
                post_serve.stdout.close()
            if post_serve.stderr:
                post_serve.stderr.close()

        journal.close()

        # Final dump and Oracle evaluation
        final_dump = subprocess.run(
            [str(self.config.kvdb_bin), "dump", str(db_file)],
            capture_output=True,
            text=True,
            timeout=self.config.timeout,
        )
        final_actual = parse_raw_dump(final_dump.stdout) if final_dump.returncode == 0 else {}
        events = list(OracleJournal.read_events(journal_path))
        eval_result = OracleEvaluator.evaluate(events, final_actual)

        duration = (time.monotonic() - start_time) * 1000.0
        passed = (
            check_passed
            and dump_passed
            and (acks > 0)
            and (errs > 0)
            and reclaim_succeeded
            and eval_result.passed
        )

        err_msg = ""
        if not passed:
            err_msg = (
                f"Disk-full cycle failed (check={check_passed}, acks={acks}, "
                f"errs={errs}, reclaim={reclaim_succeeded}, oracle={eval_result.passed})"
            )
            if eval_result.violations:
                err_msg += f": {eval_result.violations[0].format_diagnostic()}"

        return IterationResult(
            iteration=iteration,
            seed=seed,
            mode="disk-full",
            passed=passed,
            delay_ms=0.0,
            in_flight_at_kill=1,
            operations_started=ops,
            acks_received=acks,
            unresolved_ops=0,
            check_passed=check_passed,
            duration_ms=duration,
            error_message=err_msg,
        )

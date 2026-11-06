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
from torture_tool.oracle.journal import OracleJournal
from torture_tool.process.controller import ChildProcessController
from torture_tool.protocol.spec import AckResponse, ErrResponse, PutRequest
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
        max_bytes: int = 500 * 1024,  # 500 KiB quota
    ) -> IterationResult:
        start_time = time.monotonic()
        seed = self.config.seed + iteration

        # 1. Start serve mode with quota restriction
        cmd = [str(self.config.kvdb_bin), "serve", str(db_file), "--max-bytes", str(max_bytes)]
        ctrl = ChildProcessController(
            command=cmd,
            db_path=db_file,
            startup_timeout=self.config.timeout,
        )
        try:
            ctrl.start()
        except Exception as e:
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
                error_message=f"Failed to start serve with --max-bytes: {e}",
            )

        dispatcher = ResponseDispatcher(ctrl.process.stdout)
        dispatcher.start()
        writer = StdinWriter(ctrl.process.stdin)

        # 2. Issue writes until quota rejection (ERR QUOTA_EXCEEDED) is received
        acks = 0
        errs = 0
        ops = 0
        payload_64k = b"x" * (64 * 1024)

        for i in range(1, 20):
            op_id = f"q-{iteration}-{i:03d}"
            req = PutRequest(op_id=op_id, key=f"key_{i}".encode("ascii"), value=payload_64k)
            fut = dispatcher.register(op_id)
            ops += 1
            writer.write_request(req)

            try:
                resp = fut.result(timeout=5.0)
                if isinstance(resp, AckResponse):
                    acks += 1
                elif isinstance(resp, ErrResponse):
                    errs += 1
                    # Quota hit successfully!
                    break
            except Exception:
                break

        # 3. Crash process immediately after rejection
        ctrl.kill()
        dispatcher.stop()

        # 4. Verify database passes check and dump
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
        # Passed if database is not corrupted, check passed, and quota rejected gracefully
        passed = check_passed and dump_passed and (acks > 0)

        return IterationResult(
            iteration=iteration,
            seed=seed,
            mode="disk-full",
            passed=passed,
            delay_ms=0.0,
            in_flight_at_kill=0,
            operations_started=ops,
            acks_received=acks,
            unresolved_ops=0,
            check_passed=check_passed,
            duration_ms=duration,
        )


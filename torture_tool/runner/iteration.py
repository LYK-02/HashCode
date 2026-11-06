"""Single crash/recovery iteration execution coordinator."""

from __future__ import annotations

import os
import shutil
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, List, Optional

from torture_tool.cli import RunConfig
from torture_tool.concurrency.dispatcher import ResponseDispatcher
from torture_tool.concurrency.pool import WorkerPool
from torture_tool.concurrency.worker import WorkerStats
from torture_tool.concurrency.writer import StdinWriter
from torture_tool.generator.prng import SeedManager
from torture_tool.generator.workload import WorkloadConfig
from torture_tool.oracle.evaluator import EvaluationResult, OracleEvaluator
from torture_tool.oracle.journal import OracleJournal
from torture_tool.process.controller import ChildProcessController
from torture_tool.protocol.utils import hex_to_bytes
from torture_tool.scheduler.crash_scheduler import CrashSchedule, CrashScheduler


@dataclass
class IterationResult:
    iteration: int
    seed: int
    mode: str
    passed: bool
    delay_ms: float
    in_flight_at_kill: int
    operations_started: int
    acks_received: int
    unresolved_ops: int
    check_passed: bool
    eval_result: Optional[EvaluationResult] = None
    error_message: Optional[str] = None
    duration_ms: float = 0.0


def parse_raw_dump(dump_text: str) -> Dict[bytes, bytes]:
    """Parse standard sorted dump output: '<hex_key> <hex_val>\\n'."""
    state: Dict[bytes, bytes] = {}
    for line in dump_text.splitlines():
        trimmed = line.strip()
        if not trimmed:
            continue
        parts = trimmed.split(" ")
        if len(parts) == 1:
            # Key with empty value
            k = hex_to_bytes(parts[0])
            state[k] = b""
        elif len(parts) >= 2:
            k = hex_to_bytes(parts[0])
            v = hex_to_bytes(parts[1])
            state[k] = v
    return state


class IterationRunner:
    """Coordinates a single complete crash-recovery test iteration."""

    def __init__(
        self,
        config: RunConfig,
        seed_manager: SeedManager,
        journal_path: Path,
        workload_config: Optional[WorkloadConfig] = None,
    ) -> None:
        self.config = config
        self.seed_manager = seed_manager
        self.journal_path = journal_path
        self.workload_config = workload_config or WorkloadConfig.for_mode(config.mode)

    def run_iteration(
        self,
        iteration: int,
        db_file: Path,
        iter_log_dir: Optional[Path] = None,
        reuse_database: bool = False,
    ) -> IterationResult:
        start_time = time.monotonic()
        seed = self.seed_manager.get_iteration_seed(iteration)

        if not reuse_database:
            if db_file.exists():
                try:
                    db_file.unlink()
                except OSError:
                    pass
            mock_state = db_file.with_name(db_file.name + ".state")
            if mock_state.exists():
                try:
                    mock_state.unlink()
                except OSError:
                    pass

        journal = OracleJournal(self.journal_path)
        journal.log_iteration_start(iteration)

        # 1. Setup subprocess controller for serve mode
        serve_cmd = [str(self.config.kvdb_bin), "serve", str(db_file)]
        ctrl = ChildProcessController(
            command=serve_cmd,
            db_path=db_file,
            log_dir=iter_log_dir,
            startup_timeout=self.config.timeout,
        )

        try:
            ctrl.start()
        except Exception as e:
            journal.log_iteration_end(iteration, "FAILED_STARTUP")
            journal.close()
            return IterationResult(
                iteration=iteration,
                seed=seed,
                mode=self.config.mode,
                passed=False,
                delay_ms=0.0,
                in_flight_at_kill=0,
                operations_started=0,
                acks_received=0,
                unresolved_ops=0,
                check_passed=False,
                error_message=f"Engine failed to start serve mode: {e}",
                duration_ms=(time.monotonic() - start_time) * 1000.0,
            )

        # 2. Attach dispatcher and writer
        dispatcher = ResponseDispatcher(ctrl.process.stdout)
        dispatcher.start()
        writer = StdinWriter(ctrl.process.stdin)

        # 3. Launch concurrent worker pool
        pool = WorkerPool(
            num_workers=self.config.workers,
            seed_manager=self.seed_manager,
            dispatcher=dispatcher,
            writer=writer,
            journal=journal,
            workload_config=self.workload_config,
            iteration=iteration,
            request_timeout=self.config.timeout,
        )
        pool.start()

        # 4. Schedule kill
        scheduler_prng = self.seed_manager.get_scheduler_prng(iteration)
        scheduler = CrashScheduler(prng=scheduler_prng)

        def count_inflight() -> int:
            return len(dispatcher.get_pending_op_ids())

        crash_info = scheduler.wait_and_kill(
            kill_func=ctrl.kill,
            get_inflight_count_func=count_inflight,
        )

        # 5. Stop workers and dispatcher
        worker_stats = pool.stop_and_join(timeout=2.0)
        dispatcher.stop()
        journal.close()

        total_ops = sum(s.operations_started for s in worker_stats)
        total_acks = sum(s.acks_received for s in worker_stats)
        total_unresolved = sum(s.inflight_at_stop for s in worker_stats)

        # 6. Post-recovery verification: Run kvdb check
        check_passed = False
        check_cmd = [str(self.config.kvdb_bin), "check", str(db_file)]
        try:
            check_proc = subprocess.run(
                check_cmd,
                capture_output=True,
                text=True,
                timeout=self.config.timeout,
            )
            check_passed = check_proc.returncode == 0 and "OK" in check_proc.stdout
        except Exception:
            check_passed = False

        # 7. Post-recovery verification: Run kvdb dump
        dump_state: Dict[bytes, bytes] = {}
        dump_passed = False
        dump_cmd = [str(self.config.kvdb_bin), "dump", str(db_file)]
        try:
            dump_proc = subprocess.run(
                dump_cmd,
                capture_output=True,
                text=True,
                timeout=self.config.timeout,
            )
            if dump_proc.returncode == 0:
                dump_state = parse_raw_dump(dump_proc.stdout)
                dump_passed = True
        except Exception:
            dump_passed = False

        # 8. Compare against independent oracle journal events
        journal_events = list(OracleJournal.read_events(self.journal_path))
        eval_result = OracleEvaluator.evaluate(journal_events, dump_state)

        passed = check_passed and dump_passed and eval_result.passed
        duration = (time.monotonic() - start_time) * 1000.0

        return IterationResult(
            iteration=iteration,
            seed=seed,
            mode=self.config.mode,
            passed=passed,
            delay_ms=crash_info.delay_ms,
            in_flight_at_kill=crash_info.in_flight_count,
            operations_started=total_ops,
            acks_received=total_acks,
            unresolved_ops=total_unresolved,
            check_passed=check_passed,
            eval_result=eval_result,
            duration_ms=duration,
        )

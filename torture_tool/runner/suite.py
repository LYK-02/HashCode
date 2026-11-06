"""Multi-iteration test suite runner and console loop coordinator."""

from __future__ import annotations

import sys
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import TYPE_CHECKING, List, Optional

from torture_tool.cli import RunConfig
from torture_tool.generator.prng import SeedManager
from torture_tool.reporting.console import ConsoleReporter
from torture_tool.reporting.packager import FailurePackager
from torture_tool.reporting.terminal_ui import TerminalUI
from torture_tool.runner.iteration import IterationResult, IterationRunner
from torture_tool.verifier.comparator import StateComparator

if TYPE_CHECKING:
    from torture_tool.modes.compaction_crash import CompactionCrashRunner
    from torture_tool.modes.disk_full import DiskFullRunner
    from torture_tool.modes.recovery_crash import RecoveryCrashRunner


@dataclass
class SuiteSummary:
    total_iterations: int
    completed_iterations: int = 0
    passed_iterations: int = 0
    failed_iterations: int = 0
    kills_with_inflight: int = 0
    total_operations: int = 0
    total_acks: int = 0
    total_duration_sec: float = 0.0
    first_failure: Optional[IterationResult] = None


class TestSuiteRunner:
    """Executes the complete multi-iteration crash/recovery campaign."""

    def __init__(self, config: RunConfig) -> None:
        self.config = config
        self.seed_manager = SeedManager(root_seed=config.seed)
        self.summary = SuiteSummary(total_iterations=config.iterations)

    def run(self) -> SuiteSummary:
        start_time = time.monotonic()
        run_dir = self.config.artifact_dir / f"run_seed_{self.config.seed}"
        run_dir.mkdir(parents=True, exist_ok=True)
        journal_path = run_dir / "oracle_events.jsonl"

        recovery_runner = None
        compaction_runner = None
        disk_full_runner = None
        if self.config.mode == "recovery-crash":
            from torture_tool.modes.recovery_crash import RecoveryCrashRunner

            recovery_runner = RecoveryCrashRunner(self.config)
        elif self.config.mode == "compaction":
            from torture_tool.modes.compaction_crash import CompactionCrashRunner

            compaction_runner = CompactionCrashRunner(self.config)
        elif self.config.mode == "disk-full":
            from torture_tool.modes.disk_full import DiskFullRunner

            disk_full_runner = DiskFullRunner(self.config)

        terminal_ui = TerminalUI(total_iterations=self.config.iterations, mode=self.config.mode)

        # Calibrate adaptive fsync latency across warmup operations
        calib_db = run_dir / "calib_warmup.db"
        from torture_tool.scheduler.crash_scheduler import calibrate_fsync_latency

        calibrated_fsync_ms = calibrate_fsync_latency(
            kvdb_bin=self.config.kvdb_bin,
            warmup_db=calib_db,
            warmup_ops=5,
        )

        for iter_num in range(1, self.config.iterations + 1):
            iter_log_dir = run_dir / f"iter_{iter_num:04d}"
            iter_journal_path = run_dir / f"oracle_iter_{iter_num:04d}.jsonl"

            if self.config.mode == "recovery-crash" and recovery_runner:
                res = recovery_runner.run_nested_recovery_crash(
                    iteration=iter_num,
                    db_file=self.config.db_path,
                )
            elif self.config.mode == "compaction" and compaction_runner:
                res = compaction_runner.run_compaction_crash(
                    iteration=iter_num,
                    db_file=self.config.db_path,
                )
            elif self.config.mode == "disk-full" and disk_full_runner:
                res = disk_full_runner.run_disk_full_quota_cycle(
                    iteration=iter_num,
                    db_file=self.config.db_path,
                )
            else:
                iter_runner = IterationRunner(
                    config=self.config,
                    seed_manager=self.seed_manager,
                    journal_path=iter_journal_path,
                    fsync_latency_ms=calibrated_fsync_ms,
                )

                res = iter_runner.run_iteration(
                    iteration=iter_num,
                    db_file=self.config.db_path,
                    iter_log_dir=iter_log_dir,
                )

            self.summary.completed_iterations += 1
            self.summary.total_operations += res.operations_started
            self.summary.total_acks += res.acks_received
            if res.in_flight_at_kill > 0:
                self.summary.kills_with_inflight += 1

            # Live throughput (ops/sec) and p99 latency calculation
            elapsed_suite = max(0.001, time.monotonic() - start_time)
            ops_sec = int(self.summary.total_acks / elapsed_suite)

            all_iter_lats = getattr(res, "latencies_ms", [])
            if all_iter_lats:
                sorted_lats = sorted(all_iter_lats)
                p99_idx = min(len(sorted_lats) - 1, int(0.99 * len(sorted_lats)))
                p99_ms = sorted_lats[p99_idx]
            else:
                p99_ms = max(0.1, res.duration_ms / max(1, res.operations_started))

            if res.passed:
                self.summary.passed_iterations += 1
                terminal_ui.update(
                    res,
                    total_acks=self.summary.total_acks,
                    in_flight_kills=self.summary.kills_with_inflight,
                    ops_sec=ops_sec,
                    p99_ms=p99_ms,
                )
            else:
                self.summary.failed_iterations += 1
                self.summary.first_failure = res
                terminal_ui.update(
                    res,
                    total_acks=self.summary.total_acks,
                    in_flight_kills=self.summary.kills_with_inflight,
                    ops_sec=ops_sec,
                    p99_ms=p99_ms,
                )

                # Preserve failure artifacts immediately!
                FailurePackager.preserve_failure(
                    iteration_result=res,
                    config=self.config,
                    db_path=self.config.db_path,
                    journal_path=iter_journal_path,
                )
                # First failure rule: Halt immediately!
                break

        terminal_ui.finish()

        self.summary.total_duration_sec = time.monotonic() - start_time
        ConsoleReporter.print_summary(self.summary)
        return self.summary

    def _print_iteration_status(self, res: IterationResult) -> None:
        verdict = "PASS" if res.passed else "FAIL"
        line = (
            f"[ITER {res.iteration:04d}/{self.config.iterations:04d}] "
            f"SEED={res.seed} MODE={res.mode} DELAY={res.delay_ms:.1f}ms "
            f"IN_FLIGHT={res.in_flight_at_kill} OPS={res.operations_started} "
            f"ACKS={res.acks_received} STATUS={verdict} ({res.duration_ms:.0f}ms)\n"
        )
        sys.stdout.write(line)
        sys.stdout.flush()

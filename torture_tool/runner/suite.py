"""Multi-iteration test suite runner and console loop coordinator."""

from __future__ import annotations

import sys
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import List, Optional

from torture_tool.cli import RunConfig
from torture_tool.generator.prng import SeedManager
from torture_tool.reporting.console import ConsoleReporter
from torture_tool.reporting.packager import FailurePackager
from torture_tool.runner.iteration import IterationResult, IterationRunner
from torture_tool.verifier.comparator import StateComparator


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

        for iter_num in range(1, self.config.iterations + 1):
            iter_log_dir = run_dir / f"iter_{iter_num:04d}"
            iter_journal_path = run_dir / f"oracle_iter_{iter_num:04d}.jsonl"

            iter_runner = IterationRunner(
                config=self.config,
                seed_manager=self.seed_manager,
                journal_path=iter_journal_path,
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

            if res.passed:
                self.summary.passed_iterations += 1
                ConsoleReporter.print_iteration(res, self.config.iterations)
            else:
                self.summary.failed_iterations += 1
                self.summary.first_failure = res
                ConsoleReporter.print_iteration(res, self.config.iterations)

                # Preserve failure artifacts immediately!
                FailurePackager.preserve_failure(
                    iteration_result=res,
                    config=self.config,
                    db_path=self.config.db_path,
                    journal_path=iter_journal_path,
                )
                # First failure rule: Halt immediately!
                break

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

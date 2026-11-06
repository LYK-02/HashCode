"""Console summary formatter and terminal output rendering."""

from __future__ import annotations

import sys
from typing import TYPE_CHECKING, List

if TYPE_CHECKING:
    from torture_tool.runner.iteration import IterationResult
    from torture_tool.runner.suite import SuiteSummary


class ConsoleReporter:
    """Formats human-readable terminal progress and final qualification summaries."""

    @classmethod
    def print_iteration(cls, res: IterationResult, total_iterations: int) -> None:
        verdict = "\033[92mPASS\033[0m" if res.passed else "\033[91mFAIL\033[0m"
        line = (
            f"[ITER {res.iteration:04d}/{total_iterations:04d}] "
            f"SEED={res.seed} MODE={res.mode} DELAY={res.delay_ms:.1f}ms "
            f"IN_FLIGHT={res.in_flight_at_kill} OPS={res.operations_started} "
            f"ACKS={res.acks_received} STATUS={verdict} ({res.duration_ms:.0f}ms)\n"
        )
        sys.stdout.write(line)
        sys.stdout.flush()

    @classmethod
    def print_summary(cls, summary: SuiteSummary) -> None:
        sys.stdout.write("\n" + "=" * 80 + "\n")
        sys.stdout.write("TORTURE TEST CAMPAIGN SUMMARY\n")
        sys.stdout.write("=" * 80 + "\n")
        sys.stdout.write(f"Total Configured Iterations : {summary.total_iterations}\n")
        sys.stdout.write(f"Completed Iterations        : {summary.completed_iterations}\n")
        sys.stdout.write(f"Passed Iterations           : {summary.passed_iterations}\n")
        sys.stdout.write(f"Failed Iterations           : {summary.failed_iterations}\n")
        sys.stdout.write(f"Kills With In-Flight Writes : {summary.kills_with_inflight}\n")
        sys.stdout.write(f"Total Operations Started    : {summary.total_operations}\n")
        sys.stdout.write(f"Total Durable ACKs          : {summary.total_acks}\n")
        sys.stdout.write(f"Total Elapsed Time          : {summary.total_duration_sec:.2f}s\n")
        sys.stdout.write("=" * 80 + "\n")

        if summary.failed_iterations > 0:
            sys.stdout.write("\033[91mRESULT: FAILED (Invariants Violated)\033[0m\n")
            if summary.first_failure and summary.first_failure.error_message:
                sys.stdout.write(f"\nFailure Reason: {summary.first_failure.error_message}\n")
            if summary.first_failure and summary.first_failure.eval_result:
                sys.stdout.write("\nFirst Discrepancy:\n")
                for v in summary.first_failure.eval_result.violations[:3]:
                    sys.stdout.write(f"  * {v.format_diagnostic()}\n")
        else:
            sys.stdout.write("\033[92mRESULT: 100% PASSED (All Core Invariants Preserved)\033[0m\n")
        sys.stdout.write("=" * 80 + "\n\n")
        sys.stdout.flush()

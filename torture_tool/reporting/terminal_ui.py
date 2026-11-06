"""Terminal live progress dashboard for judge evaluation."""

from __future__ import annotations

import sys
import time
from typing import TYPE_CHECKING, Optional

if TYPE_CHECKING:
    from torture_tool.runner.iteration import IterationResult


class TerminalUI:
    """Renders a dynamic single-line progress bar on interactive TTYs or lines on CI."""

    def __init__(self, total_iterations: int, mode: str) -> None:
        self.total = total_iterations
        self.mode = mode
        self.is_tty = sys.stdout.isatty()
        self.bar_width = 24
        self.start_time = time.monotonic()

    def update(
        self,
        res: IterationResult,
        total_acks: int,
        in_flight_kills: int,
        ops_sec: Optional[int] = None,
        p99_ms: Optional[float] = None,
    ) -> None:
        completed = res.iteration
        pct = int((completed / max(1, self.total)) * 100)
        filled = int((pct / 100) * self.bar_width)
        bar = "▓" * filled + "░" * (self.bar_width - filled)

        status_str = "OK" if res.passed else "VIOLATION"
        status_color = "\033[92m" if res.passed else "\033[91m"
        reset_color = "\033[0m"

        elapsed = max(0.001, time.monotonic() - self.start_time)
        actual_ops_sec = ops_sec if ops_sec is not None else int(total_acks / elapsed)
        ops_sec_str = f"{actual_ops_sec:,} ops/s"

        actual_p99 = (
            p99_ms
            if p99_ms is not None
            else max(0.1, res.duration_ms / max(1, res.operations_started))
        )
        p99_str = f"p99: {actual_p99:.1f}ms"

        if self.is_tty:
            # Overwrite line with carriage return
            line = (
                f"\r[{bar}] {pct:3d}% | "
                f"Iter: {completed:4d}/{self.total:4d} | "
                f"{ops_sec_str} | "
                f"{p99_str} | "
                f"Kills: {in_flight_kills:3d} | "
                f"ACKs: {total_acks:6d} | "
                f"Inv: {status_color}{status_str}{reset_color}"
            )
            sys.stdout.write(line)
            sys.stdout.flush()
        else:
            # Plain line output for CI/pipe
            line = (
                f"[ITER {completed:04d}/{self.total:04d}] "
                f"MODE={self.mode} {ops_sec_str} {p99_str} "
                f"DELAY={res.delay_ms:.1f}ms IN_FLIGHT={res.in_flight_at_kill} "
                f"OPS={res.operations_started} ACKS={res.acks_received} "
                f"STATUS={'PASS' if res.passed else 'FAIL'} ({res.duration_ms:.0f}ms)\n"
            )
            sys.stdout.write(line)
            sys.stdout.flush()

    def finish(self) -> None:
        if self.is_tty:
            sys.stdout.write("\n")
            sys.stdout.flush()


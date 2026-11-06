"""Deterministic crash delay scheduler and in-flight kill measurement."""

from __future__ import annotations

import random
import time
from dataclasses import dataclass
from typing import Callable, Optional


@dataclass(frozen=True)
class CrashSchedule:
    delay_ms: float
    in_flight_count: int
    timing_type: str  # "immediate", "random_ms", "write_synchronized"


class CrashScheduler:
    """Calculates deterministic millisecond delays and injects SIGKILL during active mutations."""

    def __init__(self, prng: random.Random) -> None:
        self.prng = prng

    def select_delay_ms(self, min_ms: float = 5.0, max_ms: float = 200.0) -> float:
        """Choose a seeded random sleep delay in milliseconds."""
        roll = self.prng.random()
        if roll < 0.10:
            # Immediate kill (0 to 3 ms) targeting engine startup/first packet
            return self.prng.uniform(0.1, 3.0)
        elif roll < 0.30:
            # Very short kill (3 to 20 ms)
            return self.prng.uniform(3.0, 20.0)
        else:
            # Medium to long kill window (20 to max_ms)
            return self.prng.uniform(20.0, max_ms)

    def wait_and_kill(
        self,
        kill_func: Callable[[], int],
        get_inflight_count_func: Callable[[], int],
        delay_ms: Optional[float] = None,
        min_ms: float = 5.0,
        max_ms: float = 200.0,
    ) -> CrashSchedule:
        """Sleep for the selected duration, record in-flight count, and execute raw kill."""
        actual_delay = (
            delay_ms if delay_ms is not None else self.select_delay_ms(min_ms, max_ms)
        )

        timing_type = "immediate" if actual_delay <= 3.0 else "random_ms"

        # Sleep the requested delay in seconds
        time.sleep(actual_delay / 1000.0)

        # Snapshot in-flight operations at the exact moment of kill
        in_flight_now = get_inflight_count_func()

        # Fire ungraceful kill (SIGKILL)
        kill_func()

        return CrashSchedule(
            delay_ms=actual_delay,
            in_flight_count=in_flight_now,
            timing_type=timing_type,
        )


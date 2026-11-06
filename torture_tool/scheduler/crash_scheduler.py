"""Deterministic crash delay scheduler and in-flight kill measurement."""

from __future__ import annotations

import random
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Callable, Optional


def calibrate_fsync_latency(
    kvdb_bin: str | Path,
    warmup_db: Path,
    warmup_ops: int = 5,
) -> float:
    """Measure the average round-trip write + fsync latency over warmup operations."""
    proc = None
    try:
        proc = subprocess.Popen(
            [str(kvdb_bin), "serve", str(warmup_db)],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        latencies: list[float] = []
        for i in range(warmup_ops):
            t0 = time.perf_counter()
            proc.stdin.write(f"PUT calib_{i} 6b{i} 76{i}\n")
            proc.stdin.flush()
            _ = proc.stdout.readline()
            lat_ms = (time.perf_counter() - t0) * 1000.0
            latencies.append(lat_ms)
        proc.kill()
        proc.wait(timeout=1.0)
        if warmup_db.exists():
            warmup_db.unlink(missing_ok=True)
        lock_file = Path(str(warmup_db) + ".lock")
        if lock_file.exists():
            lock_file.unlink(missing_ok=True)
        if latencies:
            return sum(latencies) / len(latencies)
    except Exception:
        pass
    finally:
        if proc:
            try:
                if proc.stdin:
                    proc.stdin.close()
                if proc.stdout:
                    proc.stdout.close()
                if proc.stderr:
                    proc.stderr.close()
            except Exception:
                pass
    return 2.0  # Safe default: 2.0 ms


@dataclass(frozen=True)
class CrashSchedule:
    delay_ms: float
    in_flight_count: int
    timing_type: str  # "immediate", "random_ms", "write_synchronized"


class CrashScheduler:
    """Calculates deterministic millisecond delays and injects SIGKILL during active mutations."""

    def __init__(
        self,
        prng: random.Random,
        fsync_latency_ms: Optional[float] = None,
    ) -> None:
        self.prng = prng
        self.fsync_latency_ms = fsync_latency_ms

    def select_delay_ms(
        self,
        min_ms: float = 5.0,
        max_ms: float = 200.0,
        fsync_ms: Optional[float] = None,
    ) -> float:
        """Choose a seeded random sleep delay in milliseconds.

        If fsync_ms is calibrated (e.g. T_fsync ~ 1-3ms), centers random delays around
        [0.5 * T_fsync, 1.5 * T_fsync] to maximize the probability of striking mid-syscall
        between pwrite() and File.Sync().
        """
        calibrated = fsync_ms if fsync_ms is not None else self.fsync_latency_ms
        roll = self.prng.random()
        if calibrated is not None and calibrated > 0.0:
            if roll < 0.15:
                # Immediate kill (0.1 to 3 ms) targeting engine startup
                return self.prng.uniform(0.1, 3.0)
            elif roll < 0.70:
                # Targeted fsync calibration window: [0.5 * T_fsync, 1.5 * T_fsync]
                low = max(0.1, 0.5 * calibrated)
                high = max(low + 0.1, 1.5 * calibrated)
                return self.prng.uniform(low, high)
            else:
                return self.prng.uniform(20.0, max_ms)

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


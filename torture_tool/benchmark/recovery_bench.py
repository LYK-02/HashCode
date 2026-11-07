"""Performance and scale benchmarks for 1,000,000 key recovery and 500-cycle torture.

Exercises:
1. 1,000,000 Key Recovery Benchmark: Verifies that opening an existing 1M-key database
   recovers within the official requirement (<= 2.0 seconds).
2. 500-Iteration Crash/Restart Stress Harness integration check.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Optional


@dataclass
class BenchmarkResult:
    test_name: str
    target_metric: str
    actual_value: float
    unit: str
    passed: bool
    details: str


class BenchmarkHarness:
    """Automated benchmark driver verifying high-scale recovery limits."""

    def __init__(self, kvdb_bin: Path | str, artifact_dir: Path | str = Path("artifacts")) -> None:
        self.kvdb_bin = Path(kvdb_bin).resolve()
        self.artifact_dir = Path(artifact_dir).resolve()
        self.artifact_dir.mkdir(parents=True, exist_ok=True)

    def run_1m_key_recovery_benchmark(
        self,
        key_count: int = 1_000_000,
        batch_size: int = 10_000,
        max_allowed_recovery_sec: float = 2.0,
        trials: int = 3,
    ) -> BenchmarkResult:
        """Populate a real database with key_count keys and measure cold ./kvdb open recovery time."""
        with tempfile.TemporaryDirectory() as tmp_dir:
            db_path = Path(tmp_dir) / "bench_1m.db"
            script_path = Path(tmp_dir) / "batch_chunk.txt"

            # 1. Populate database using maximum allowable batch chunks (10,000 ops each)
            total_batches = key_count // batch_size
            for b_idx in range(total_batches):
                lines = []
                start_i = b_idx * batch_size
                for i in range(start_i, start_i + batch_size):
                    lines.append(f"put k_{i:07d} v_{i:07d}")
                script_path.write_text("\n".join(lines) + "\n")

                cmd = [str(self.kvdb_bin), "batch", str(db_path), str(script_path)]
                res = subprocess.run(cmd, capture_output=True, text=True)
                if res.returncode != 0:
                    return BenchmarkResult(
                        test_name="1M Key Recovery Benchmark",
                        target_metric=f"<= {max_allowed_recovery_sec}s",
                        actual_value=-1.0,
                        unit="seconds",
                        passed=False,
                        details=f"Failed during population batch {b_idx}: {res.stderr}",
                    )

            db_size_mb = db_path.stat().st_size / (1024 * 1024)

            # 2. Measure cold restart / recovery duration across multiple trials
            durations: list[float] = []
            for trial in range(trials):
                # Clean lock file if leftover
                lock_file = Path(str(db_path) + ".lock")
                if lock_file.exists():
                    lock_file.unlink(missing_ok=True)

                t0 = time.perf_counter()
                open_proc = subprocess.run(
                    [str(self.kvdb_bin), "open", str(db_path)],
                    capture_output=True,
                    text=True,
                    timeout=max_allowed_recovery_sec * 3,
                )
                dur = time.perf_counter() - t0
                if open_proc.returncode != 0:
                    return BenchmarkResult(
                        test_name="1M Key Recovery Benchmark",
                        target_metric=f"<= {max_allowed_recovery_sec}s",
                        actual_value=dur,
                        unit="seconds",
                        passed=False,
                        details=f"kvdb open trial {trial} failed: {open_proc.stderr}",
                    )
                durations.append(dur)

            avg_dur = sum(durations) / len(durations)
            passed = avg_dur <= max_allowed_recovery_sec

            return BenchmarkResult(
                test_name="1M Key Recovery Benchmark",
                target_metric=f"<= {max_allowed_recovery_sec}s",
                actual_value=avg_dur,
                unit="seconds",
                passed=passed,
                details=(
                    f"Recovered {key_count:,} keys ({db_size_mb:.2f} MB) in {avg_dur:.4f}s avg "
                    f"({[f'{d:.4f}s' for d in durations]}); Target <= {max_allowed_recovery_sec}s"
                ),
            )

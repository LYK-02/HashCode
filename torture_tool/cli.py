"""Command-line interface parser and configuration dataclass for ./torture."""

from __future__ import annotations

import argparse
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import List, Optional

VALID_MODES = [
    "default",
    "large-values",
    "large-batches",
    "compaction",
    "recovery-crash",
    "disk-full",
    "self-test",
]


class ArgumentParserWithExitCode(argparse.ArgumentParser):
    """Custom parser ensuring exit code 2 on syntax/validation errors."""

    def error(self, message: str) -> None:
        self.print_usage(sys.stderr)
        sys.stderr.write(f"error: {message}\n")
        sys.exit(2)


@dataclass(frozen=True)
class RunConfig:
    seed: int
    iterations: int = 500
    mode: str = "default"
    workers: int = 2
    timeout: float = 30.0
    db_path: Path = Path("data.db")
    artifact_dir: Path = Path("artifacts")
    kvdb_bin: Path = Path("./kvdb")

    def __post_init__(self) -> None:
        object.__setattr__(self, "kvdb_bin", self.kvdb_bin.resolve())
        if self.iterations < 1:
            raise ValueError(f"Iterations must be >= 1, got {self.iterations}")
        if self.workers < 2:
            raise ValueError(f"Workers must be >= 2, got {self.workers}")
        if self.mode not in VALID_MODES:
            raise ValueError(f"Invalid mode '{self.mode}'. Must be one of {VALID_MODES}")
        if self.timeout <= 0:
            raise ValueError(f"Timeout must be > 0, got {self.timeout}")


def create_parser() -> argparse.ArgumentParser:
    """Build the command-line argument parser for ./torture."""
    parser = ArgumentParserWithExitCode(
        prog="torture",
        description="Independent adversarial crash and recovery torture tester for kvdb",
    )
    # Required seed
    parser.add_argument(
        "--seed",
        type=int,
        required=True,
        help="Mandatory integer seed for deterministic random generation",
    )
    # Optional flags
    parser.add_argument(
        "--iterations",
        type=int,
        default=500,
        help="Total number of crash/recovery iterations to execute (default: 500)",
    )
    parser.add_argument(
        "--mode",
        type=str,
        default="default",
        choices=VALID_MODES,
        help=f"Targeted stress mode (choices: {', '.join(VALID_MODES)}; default: default)",
    )
    parser.add_argument(
        "--workers",
        type=int,
        default=2,
        help="Number of concurrent worker client threads (minimum: 2, default: 2)",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=30.0,
        help="Per-operation / startup timeout in seconds (default: 30.0)",
    )
    parser.add_argument(
        "--db",
        type=Path,
        default=Path("data.db"),
        help="Target database file path (default: data.db)",
    )
    parser.add_argument(
        "--artifact-dir",
        type=Path,
        default=Path("artifacts"),
        help="Directory where logs and failure artifacts are saved (default: artifacts/)",
    )
    parser.add_argument(
        "--kvdb-bin",
        type=Path,
        default=Path("./kvdb"),
        help="Path to kvdb compiled binary (default: ./kvdb)",
    )
    return parser


def parse_args(args: Optional[List[str]] = None) -> RunConfig:
    """Parse command line arguments into a validated RunConfig."""
    parser = create_parser()
    parsed = parser.parse_args(args)

    if parsed.workers < 2:
        parser.error(f"--workers must be at least 2, got {parsed.workers}")
    if parsed.iterations < 1:
        parser.error(f"--iterations must be at least 1, got {parsed.iterations}")

    return RunConfig(
        seed=parsed.seed,
        iterations=parsed.iterations,
        mode=parsed.mode,
        workers=parsed.workers,
        timeout=parsed.timeout,
        db_path=parsed.db,
        artifact_dir=parsed.artifact_dir,
        kvdb_bin=parsed.kvdb_bin,
    )


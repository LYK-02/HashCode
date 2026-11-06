"""Specialized mode runners for targeted stress testing."""

from __future__ import annotations

from pathlib import Path
from typing import Optional

from torture_tool.cli import RunConfig
from torture_tool.generator.workload import WorkloadConfig
from torture_tool.runner.suite import SuiteSummary, TestSuiteRunner


class LargeValuesModeRunner:
    """Specialized driver executing extreme boundary value stress (up to 1 MiB)."""

    def __init__(self, config: RunConfig) -> None:
        self.config = RunConfig(
            seed=config.seed,
            iterations=config.iterations,
            mode="large-values",
            workers=config.workers,
            timeout=max(30.0, config.timeout),
            db_path=config.db_path,
            artifact_dir=config.artifact_dir,
            kvdb_bin=config.kvdb_bin,
        )

    def run(self) -> SuiteSummary:
        suite = TestSuiteRunner(self.config)
        return suite.run()


class LargeBatchesModeRunner:
    """Specialized driver executing massive atomic batch stress (up to 10,000 operations)."""

    def __init__(self, config: RunConfig) -> None:
        self.config = RunConfig(
            seed=config.seed,
            iterations=config.iterations,
            mode="large-batches",
            workers=config.workers,
            timeout=max(60.0, config.timeout),
            db_path=config.db_path,
            artifact_dir=config.artifact_dir,
            kvdb_bin=config.kvdb_bin,
        )

    def run(self) -> SuiteSummary:
        suite = TestSuiteRunner(self.config)
        return suite.run()


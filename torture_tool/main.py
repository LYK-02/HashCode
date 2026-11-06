"""Main entry point for ./torture CLI executable."""

from __future__ import annotations

import sys

from torture_tool.cli import parse_args
from torture_tool.runner.suite import TestSuiteRunner


def main() -> int:
    config = parse_args(sys.argv[1:])
    suite = TestSuiteRunner(config)
    summary = suite.run()
    if summary.failed_iterations > 0:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())


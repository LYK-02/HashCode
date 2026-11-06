"""Unit tests for Milestone 1 resilience hardening: worker exceptions, protocol errors, and CLI."""

from __future__ import annotations

import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import MagicMock, patch

from torture_tool.cli import RunConfig, parse_args
from torture_tool.concurrency.worker import WorkerStats
from torture_tool.generator.prng import SeedManager
from torture_tool.oracle.evaluator import EvaluationResult
from torture_tool.runner.iteration import IterationRunner
from torture_tool.scheduler.crash_scheduler import CrashSchedule


class TestCliParameterValidation(unittest.TestCase):
    """Test CLI parameter validation for invalid timeout and missing binaries."""

    def test_timeout_zero_exits_code_2(self):
        with self.assertRaises(SystemExit) as cm:
            parse_args(["--seed", "42", "--timeout", "0"])
        self.assertEqual(cm.exception.code, 2)

    def test_timeout_negative_exits_code_2(self):
        with self.assertRaises(SystemExit) as cm:
            parse_args(["--seed", "42", "--timeout", "-3.5"])
        self.assertEqual(cm.exception.code, 2)

    def test_missing_kvdb_bin_exits_code_2(self):
        with self.assertRaises(SystemExit) as cm:
            parse_args(["--seed", "42", "--kvdb-bin", "/path/that/does/not/exist/kvdb_bin"])
        self.assertEqual(cm.exception.code, 2)

    def test_torture_binary_timeout_zero_subprocess(self):
        """Run ./torture with --timeout 0 and verify exit code 2."""
        res = subprocess.run(
            [sys.executable, "-m", "torture_tool.main", "--seed", "1", "--timeout", "0"],
            capture_output=True,
            text=True,
            cwd=str(Path(__file__).resolve().parent.parent.parent),
        )
        self.assertEqual(res.returncode, 2)
        self.assertIn("error: --timeout must be greater than 0, got 0.0", res.stderr)


class TestIterationFailFast(unittest.TestCase):
    """Verify that worker exceptions and protocol violations cause iteration failure."""

    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory()
        self.db_path = Path(self.tmpdir.name) / "test.db"
        self.journal_path = Path(self.tmpdir.name) / "oracle.jsonl"
        self.config = RunConfig(
            seed=1001,
            iterations=1,
            workers=2,
            db_path=self.db_path,
            artifact_dir=Path(self.tmpdir.name) / "artifacts",
            kvdb_bin=Path(sys.executable),
        )
        self.seed_manager = SeedManager(root_seed=1001)

    def tearDown(self):
        self.tmpdir.cleanup()

    @patch("torture_tool.runner.iteration.OracleEvaluator.evaluate")
    @patch("torture_tool.runner.iteration.subprocess.run")
    @patch("torture_tool.runner.iteration.CrashScheduler")
    @patch("torture_tool.runner.iteration.WorkerPool")
    @patch("torture_tool.runner.iteration.StdinWriter")
    @patch("torture_tool.runner.iteration.ResponseDispatcher")
    @patch("torture_tool.runner.iteration.ChildProcessController")
    def test_iteration_fails_when_worker_exception_occurs(
        self,
        mock_ctrl_cls,
        mock_disp_cls,
        mock_writer_cls,
        mock_pool_cls,
        mock_sched_cls,
        mock_subproc_run,
        mock_eval,
    ):
        # Setup mocks so everything else succeeds
        mock_ctrl = mock_ctrl_cls.return_value
        mock_ctrl.process.stdout = MagicMock()
        mock_ctrl.process.stdin = MagicMock()

        mock_disp = mock_disp_cls.return_value
        mock_disp.protocol_errors = []
        mock_disp.unknown_responses = []

        # Worker encounters an exception (e.g. TimeoutError or unhandled exception)
        mock_pool = mock_pool_cls.return_value
        mock_pool.stop_and_join.return_value = [
            WorkerStats(worker_id="0", operations_started=10, acks_received=9, exceptions_encountered=1),
            WorkerStats(worker_id="1", operations_started=10, acks_received=10, exceptions_encountered=0),
        ]

        mock_sched = mock_sched_cls.return_value
        mock_sched.wait_and_kill.return_value = CrashSchedule(delay_ms=10.0, in_flight_count=1, timing_type="random_ms")

        # Database check and dump succeed
        check_proc = MagicMock(returncode=0, stdout="OK - all checksums valid\n")
        dump_proc = MagicMock(returncode=0, stdout="6b657931 76616c31\n")
        mock_subproc_run.side_effect = [check_proc, dump_proc]

        mock_eval.return_value = EvaluationResult(passed=True, violations=[])

        runner = IterationRunner(
            config=self.config,
            seed_manager=self.seed_manager,
            journal_path=self.journal_path,
        )
        res = runner.run_iteration(iteration=1, db_file=self.db_path)

        self.assertFalse(res.passed)
        self.assertIsNotNone(res.error_message)
        self.assertIn("WORKER_EXCEPTION_OR_TIMEOUT", res.error_message)
        self.assertIn("1 worker exception(s) encountered", res.error_message)

    @patch("torture_tool.runner.iteration.OracleEvaluator.evaluate")
    @patch("torture_tool.runner.iteration.subprocess.run")
    @patch("torture_tool.runner.iteration.CrashScheduler")
    @patch("torture_tool.runner.iteration.WorkerPool")
    @patch("torture_tool.runner.iteration.StdinWriter")
    @patch("torture_tool.runner.iteration.ResponseDispatcher")
    @patch("torture_tool.runner.iteration.ChildProcessController")
    def test_iteration_fails_when_protocol_errors_occur(
        self,
        mock_ctrl_cls,
        mock_disp_cls,
        mock_writer_cls,
        mock_pool_cls,
        mock_sched_cls,
        mock_subproc_run,
        mock_eval,
    ):
        mock_ctrl = mock_ctrl_cls.return_value
        mock_ctrl.process.stdout = MagicMock()
        mock_ctrl.process.stdin = MagicMock()

        # Protocol error logged by dispatcher
        mock_disp = mock_disp_cls.return_value
        mock_disp.protocol_errors = ["Parse error: unknown opcode FOOBAR"]
        mock_disp.unknown_responses = []

        mock_pool = mock_pool_cls.return_value
        mock_pool.stop_and_join.return_value = [
            WorkerStats(worker_id="0", operations_started=5, acks_received=5, exceptions_encountered=0),
            WorkerStats(worker_id="1", operations_started=5, acks_received=5, exceptions_encountered=0),
        ]

        mock_sched = mock_sched_cls.return_value
        mock_sched.wait_and_kill.return_value = CrashSchedule(delay_ms=5.0, in_flight_count=0, timing_type="random_ms")

        check_proc = MagicMock(returncode=0, stdout="OK\n")
        dump_proc = MagicMock(returncode=0, stdout="")
        mock_subproc_run.side_effect = [check_proc, dump_proc]

        mock_eval.return_value = EvaluationResult(passed=True, violations=[])

        runner = IterationRunner(
            config=self.config,
            seed_manager=self.seed_manager,
            journal_path=self.journal_path,
        )
        res = runner.run_iteration(iteration=1, db_file=self.db_path)

        self.assertFalse(res.passed)
        self.assertIsNotNone(res.error_message)
        self.assertIn("PROTOCOL_VIOLATION", res.error_message)

    @patch("torture_tool.runner.iteration.OracleEvaluator.evaluate")
    @patch("torture_tool.runner.iteration.subprocess.run")
    @patch("torture_tool.runner.iteration.CrashScheduler")
    @patch("torture_tool.runner.iteration.WorkerPool")
    @patch("torture_tool.runner.iteration.StdinWriter")
    @patch("torture_tool.runner.iteration.ResponseDispatcher")
    @patch("torture_tool.runner.iteration.ChildProcessController")
    def test_iteration_fails_when_unknown_responses_occur(
        self,
        mock_ctrl_cls,
        mock_disp_cls,
        mock_writer_cls,
        mock_pool_cls,
        mock_sched_cls,
        mock_subproc_run,
        mock_eval,
    ):
        mock_ctrl = mock_ctrl_cls.return_value
        mock_ctrl.process.stdout = MagicMock()
        mock_ctrl.process.stdin = MagicMock()

        # Unknown response logged by dispatcher
        mock_disp = mock_disp_cls.return_value
        mock_disp.protocol_errors = []
        mock_disp.unknown_responses = ["CORRUPT_BYTES_XYZ"]

        mock_pool = mock_pool_cls.return_value
        mock_pool.stop_and_join.return_value = [
            WorkerStats(worker_id="0", operations_started=5, acks_received=5, exceptions_encountered=0),
            WorkerStats(worker_id="1", operations_started=5, acks_received=5, exceptions_encountered=0),
        ]

        mock_sched = mock_sched_cls.return_value
        mock_sched.wait_and_kill.return_value = CrashSchedule(delay_ms=5.0, in_flight_count=0, timing_type="random_ms")

        check_proc = MagicMock(returncode=0, stdout="OK\n")
        dump_proc = MagicMock(returncode=0, stdout="")
        mock_subproc_run.side_effect = [check_proc, dump_proc]

        mock_eval.return_value = EvaluationResult(passed=True, violations=[])

        runner = IterationRunner(
            config=self.config,
            seed_manager=self.seed_manager,
            journal_path=self.journal_path,
        )
        res = runner.run_iteration(iteration=1, db_file=self.db_path)

        self.assertFalse(res.passed)
        self.assertIsNotNone(res.error_message)
        self.assertIn("PROTOCOL_VIOLATION", res.error_message)


    @patch("torture_tool.runner.iteration.OracleEvaluator.evaluate")
    @patch("torture_tool.runner.iteration.subprocess.run")
    @patch("torture_tool.runner.iteration.CrashScheduler")
    @patch("torture_tool.runner.iteration.WorkerPool")
    @patch("torture_tool.runner.iteration.StdinWriter")
    @patch("torture_tool.runner.iteration.ResponseDispatcher")
    @patch("torture_tool.runner.iteration.ChildProcessController")
    def test_iteration_succeeds_when_no_errors(
        self,
        mock_ctrl_cls,
        mock_disp_cls,
        mock_writer_cls,
        mock_pool_cls,
        mock_sched_cls,
        mock_subproc_run,
        mock_eval,
    ):
        mock_ctrl = mock_ctrl_cls.return_value
        mock_ctrl.process.stdout = MagicMock()
        mock_ctrl.process.stdin = MagicMock()

        mock_disp = mock_disp_cls.return_value
        mock_disp.protocol_errors = []
        mock_disp.unknown_responses = []

        mock_pool = mock_pool_cls.return_value
        mock_pool.stop_and_join.return_value = [
            WorkerStats(worker_id="0", operations_started=10, acks_received=10, exceptions_encountered=0),
            WorkerStats(worker_id="1", operations_started=10, acks_received=10, exceptions_encountered=0),
        ]

        mock_sched = mock_sched_cls.return_value
        mock_sched.wait_and_kill.return_value = CrashSchedule(delay_ms=5.0, in_flight_count=0, timing_type="random_ms")

        check_proc = MagicMock(returncode=0, stdout="OK - all checksums valid\n")
        dump_proc = MagicMock(returncode=0, stdout="6b657931 76616c31\n")
        mock_subproc_run.side_effect = [check_proc, dump_proc]

        mock_eval.return_value = EvaluationResult(passed=True, violations=[])

        runner = IterationRunner(
            config=self.config,
            seed_manager=self.seed_manager,
            journal_path=self.journal_path,
        )
        res = runner.run_iteration(iteration=1, db_file=self.db_path)

        self.assertTrue(res.passed)
        self.assertIsNone(res.error_message)

    @patch("torture_tool.runner.iteration.OracleEvaluator.evaluate")
    @patch("torture_tool.runner.iteration.subprocess.run")
    @patch("torture_tool.runner.iteration.CrashScheduler")
    @patch("torture_tool.runner.iteration.WorkerPool")
    @patch("torture_tool.runner.iteration.StdinWriter")
    @patch("torture_tool.runner.iteration.ResponseDispatcher")
    @patch("torture_tool.runner.iteration.ChildProcessController")
    def test_iteration_multiple_errors_aggregated(
        self,
        mock_ctrl_cls,
        mock_disp_cls,
        mock_writer_cls,
        mock_pool_cls,
        mock_sched_cls,
        mock_subproc_run,
        mock_eval,
    ):
        mock_ctrl = mock_ctrl_cls.return_value
        mock_ctrl.process.stdout = MagicMock()
        mock_ctrl.process.stdin = MagicMock()

        mock_disp = mock_disp_cls.return_value
        mock_disp.protocol_errors = ["Parse error: invalid packet"]
        mock_disp.unknown_responses = ["UNKNOWN_CORRUPT"]

        mock_pool = mock_pool_cls.return_value
        mock_pool.stop_and_join.return_value = [
            WorkerStats(worker_id="0", operations_started=5, acks_received=4, exceptions_encountered=2),
            WorkerStats(worker_id="1", operations_started=5, acks_received=5, exceptions_encountered=1),
        ]

        mock_sched = mock_sched_cls.return_value
        mock_sched.wait_and_kill.return_value = CrashSchedule(delay_ms=5.0, in_flight_count=0, timing_type="random_ms")

        check_proc = MagicMock(returncode=0, stdout="OK\n")
        dump_proc = MagicMock(returncode=0, stdout="")
        mock_subproc_run.side_effect = [check_proc, dump_proc]

        mock_eval.return_value = EvaluationResult(passed=True, violations=[])

        runner = IterationRunner(
            config=self.config,
            seed_manager=self.seed_manager,
            journal_path=self.journal_path,
        )
        res = runner.run_iteration(iteration=1, db_file=self.db_path)

        self.assertFalse(res.passed)
        self.assertIsNotNone(res.error_message)
        self.assertIn("WORKER_EXCEPTION_OR_TIMEOUT: 3 worker exception(s) encountered", res.error_message)
        self.assertIn("PROTOCOL_VIOLATION", res.error_message)


if __name__ == "__main__":
    unittest.main()

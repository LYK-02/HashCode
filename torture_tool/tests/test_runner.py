"""Unit and integration tests for Phase T5: Random Crash Injection Loop."""

import os
import shutil
import stat
import sys
import tempfile
import time
import unittest
from pathlib import Path

from torture_tool.cli import RunConfig
from torture_tool.generator.prng import SeedManager
from torture_tool.generator.workload import WorkloadConfig
from torture_tool.runner.iteration import (
    IterationResult,
    IterationRunner,
    parse_raw_dump,
)
from torture_tool.runner.suite import TestSuiteRunner
from torture_tool.scheduler.crash_scheduler import CrashSchedule, CrashScheduler


class TestCrashScheduler(unittest.TestCase):
    def test_deterministic_kill_delays(self):
        seed_mgr = SeedManager(root_seed=9999)
        sched1 = CrashScheduler(prng=seed_mgr.get_scheduler_prng(1))
        sched2 = CrashScheduler(prng=seed_mgr.get_scheduler_prng(1))

        delays1 = [sched1.select_delay_ms() for _ in range(50)]
        delays2 = [sched2.select_delay_ms() for _ in range(50)]

        self.assertEqual(delays1, delays2)

    def test_wait_and_kill_invocation(self):
        seed_mgr = SeedManager(root_seed=123)
        sched = CrashScheduler(prng=seed_mgr.get_scheduler_prng(1))

        killed = False

        def mock_kill():
            nonlocal killed
            killed = True
            return 0

        def mock_inflight():
            return 7

        schedule = sched.wait_and_kill(
            kill_func=mock_kill,
            get_inflight_count_func=mock_inflight,
            delay_ms=5.0,
        )

        self.assertTrue(killed)
        self.assertEqual(schedule.in_flight_count, 7)
        self.assertEqual(schedule.delay_ms, 5.0)


class TestFullIterationCrashLoop(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.mkdtemp()
        self.kvdb_mock_script = Path(self.tmpdir) / "kvdb"

        # Create a mock kvdb script implementing serve, check, dump
        # serve: keeps in-memory state or file state, responds with ACK op_id
        # check: prints "OK" and exits 0
        # dump: prints key_hex val_hex pairs
        content = (
            "#!/usr/bin/env python3\n"
            "import sys, os\n"
            "cmd = sys.argv[1] if len(sys.argv) > 1 else ''\n"
            "db_path = sys.argv[2] if len(sys.argv) > 2 else 'data.db'\n"
            "log_path = db_path + '.state'\n"
            "\n"
            "if cmd == 'serve':\n"
            "    state = {}\n"
            "    if os.path.exists(log_path):\n"
            "        with open(log_path, 'r') as f:\n"
            "            for l in f:\n"
            "                p = l.strip().split()\n"
            "                if len(p) == 2:\n"
            "                    if p[1] == 'DEL_TOMBSTONE': state.pop(p[0], None)\n"
            "                    else: state[p[0]] = p[1]\n"
            "    f_out = open(log_path, 'a', buffering=1)\n"
            "    for line in sys.stdin:\n"
            "        p = line.strip().split()\n"
            "        if not p: continue\n"
            "        op = p[0]\n"
            "        op_id = p[1]\n"
            "        if op == 'PUT':\n"
            "            k, v = p[2], p[3]\n"
            "            state[k] = v\n"
            "            f_out.write(f'{k} {v}\\n')\n"
            "            f_out.flush()\n"
            "            os.fsync(f_out.fileno())\n"
            "            sys.stdout.write(f'ACK {op_id}\\n')\n"
            "            sys.stdout.flush()\n"
            "        elif op == 'DEL':\n"
            "            k = p[2]\n"
            "            state.pop(k, None)\n"
            "            f_out.write(f'{k} DEL_TOMBSTONE\\n')\n"
            "            f_out.flush()\n"
            "            os.fsync(f_out.fileno())\n"
            "            sys.stdout.write(f'ACK {op_id}\\n')\n"
            "            sys.stdout.flush()\n"
            "        elif op == 'BATCH':\n"
            "            cnt = int(p[2])\n"
            "            batch_lines = []\n"
            "            for itm in p[3].split(','):\n"
            "                if ':' in itm:\n"
            "                    ik, iv = itm.split(':', 1)\n"
            "                    if iv:\n"
            "                        state[ik] = iv\n"
            "                        batch_lines.append(f'{ik} {iv}\\n')\n"
            "                    else:\n"
            "                        state.pop(ik, None)\n"
            "                        batch_lines.append(f'{ik} DEL_TOMBSTONE\\n')\n"
            "            f_out.write(''.join(batch_lines))\n"
            "            f_out.flush()\n"
            "            os.fsync(f_out.fileno())\n"
            "            sys.stdout.write(f'ACK {op_id}\\n')\n"
            "            sys.stdout.flush()\n"
            "elif cmd == 'check':\n"
            "    sys.stdout.write('OK: Database is consistent\\n')\n"
            "    sys.exit(0)\n"
            "elif cmd == 'dump':\n"
            "    state = {}\n"
            "    if os.path.exists(log_path):\n"
            "        with open(log_path, 'r') as f:\n"
            "            for l in f:\n"
            "                p = l.strip().split()\n"
            "                if len(p) == 2:\n"
            "                    if p[1] == 'DEL_TOMBSTONE': state.pop(p[0], None)\n"
            "                    else: state[p[0]] = p[1]\n"
            "    for k in sorted(state.keys()):\n"
            "        sys.stdout.write(f'{k} {state[k]}\\n')\n"
            "    sys.exit(0)\n"
            "else:\n"
            "    sys.stderr.write(f'Unknown command: {cmd}\\n')\n"
            "    sys.exit(1)\n"
        )
        self.kvdb_mock_script.write_text(content, encoding="utf-8")
        self.kvdb_mock_script.chmod(self.kvdb_mock_script.stat().st_mode | stat.S_IEXEC)

    def tearDown(self):
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def test_repeated_crash_recovery_iterations(self):
        """Execute 10 full crash/recovery cycles verifying mid-flight SIGKILL and oracle checks."""
        config = RunConfig(
            seed=777,
            iterations=10,
            workers=2,
            db_path=Path(self.tmpdir) / "test.db",
            artifact_dir=Path(self.tmpdir) / "artifacts",
            kvdb_bin=self.kvdb_mock_script,
        )

        suite = TestSuiteRunner(config=config)
        summary = suite.run()

        if summary.first_failure:
            print("First failure details:")
            print(f"Iteration: {summary.first_failure.iteration}")
            print(f"Check passed: {summary.first_failure.check_passed}")
            if summary.first_failure.eval_result:
                for v in summary.first_failure.eval_result.violations:
                    print("Violation:", v.format_diagnostic())

        self.assertEqual(summary.completed_iterations, 10)
        self.assertEqual(summary.passed_iterations, 10)
        self.assertEqual(summary.failed_iterations, 0)
        self.assertGreater(summary.total_operations, 50)
        self.assertGreater(summary.total_acks, 20)


if __name__ == "__main__":
    unittest.main()

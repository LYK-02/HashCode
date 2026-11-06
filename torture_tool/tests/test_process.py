"""Unit and integration tests for Phase T3: Subprocess and Process Lifetime Manager."""

import fcntl
import os
import signal
import sys
import tempfile
import time
import unittest
from pathlib import Path

from torture_tool.process.controller import (
    ChildProcessController,
    ProcessStartupError,
)
from torture_tool.process.stderr_drainer import StderrDrainer


class TestProcessController(unittest.TestCase):
    def test_spawn_and_sigkill(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            db_path.touch()

            # Python command simulating a running server looping on stdin
            cmd = [
                sys.executable,
                "-c",
                "import sys, time; sys.stdout.write('READY\\n'); sys.stdout.flush(); [line for line in sys.stdin]",
            ]

            ctrl = ChildProcessController(command=cmd, db_path=db_path)
            ctrl.start()

            self.assertTrue(ctrl.is_alive())
            pid = ctrl.pid
            self.assertIsNotNone(pid)

            # Terminate ungracefully with SIGKILL
            exit_code = ctrl.kill(timeout=2.0)
            self.assertFalse(ctrl.is_alive())
            # In POSIX, kill -9 results in exit status -9 or 137
            self.assertIn(exit_code, (-signal.SIGKILL, 128 + signal.SIGKILL))

    def test_stderr_flooding_does_not_deadlock(self):
        """Verify that emitting >1MB of stderr does not block or deadlock child writes."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            log_dir = Path(tmpdir) / "logs"

            # Script that emits 2 MB of data to stderr then echoes stdin to stdout
            script = (
                "import sys\n"
                "for _ in range(20000):\n"
                "    sys.stderr.write('x' * 100 + '\\n')\n"
                "sys.stderr.flush()\n"
                "sys.stdout.write('DONE_FLOOD\\n')\n"
                "sys.stdout.flush()\n"
                "line = sys.stdin.readline()\n"
                "sys.stdout.write('ECHO:' + line)\n"
                "sys.stdout.flush()\n"
            )

            cmd = [sys.executable, "-c", script]
            ctrl = ChildProcessController(command=cmd, db_path=db_path, log_dir=log_dir)
            ctrl.start()

            # Read readiness marker from stdout
            out_line = ctrl.process.stdout.readline().decode("utf-8")
            self.assertEqual(out_line.strip(), "DONE_FLOOD")

            # Write to stdin and verify reply
            ctrl.process.stdin.write(b"ping\n")
            ctrl.process.stdin.flush()
            echo_line = ctrl.process.stdout.readline().decode("utf-8")
            self.assertEqual(echo_line.strip(), "ECHO:ping")

            ctrl.kill()

            # Verify stderr log was written
            stderr_log = log_dir / "engine_stderr.log"
            self.assertTrue(stderr_log.exists())
            self.assertGreater(stderr_log.stat().st_size, 1000000)

    def test_immediate_crash_raises_startup_error(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            cmd = [
                sys.executable,
                "-c",
                "import sys; sys.stderr.write('Fatal lock contention\\n'); sys.exit(1)",
            ]

            ctrl = ChildProcessController(command=cmd, db_path=db_path)
            with self.assertRaises(ProcessStartupError) as cm:
                ctrl.start()
            self.assertIn("Fatal lock contention", str(cm.exception))

    def test_file_lock_released_after_sigkill(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "locked.db"
            db_path.touch()

            # Script that acquires exclusive flock and sleeps
            script = (
                "import sys, fcntl, time\n"
                f"f = open('{db_path}', 'r+b')\n"
                "fcntl.flock(f.fileno(), fcntl.LOCK_EX)\n"
                "sys.stdout.write('LOCKED\\n')\n"
                "sys.stdout.flush()\n"
                "time.sleep(60)\n"
            )

            cmd = [sys.executable, "-c", script]
            ctrl = ChildProcessController(command=cmd, db_path=db_path)
            ctrl.start()

            # Wait for lock confirmation
            line = ctrl.process.stdout.readline().decode("utf-8")
            self.assertEqual(line.strip(), "LOCKED")

            # At this point, the file is locked
            with self.assertRaises(BlockingIOError):
                with open(db_path, "r+b") as f:
                    fcntl.flock(f.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)

            # Kill process with SIGKILL
            ctrl.kill()

            # Verify file lock is immediately released by OS
            self.assertTrue(ctrl.verify_lock_released())


if __name__ == "__main__":
    unittest.main()


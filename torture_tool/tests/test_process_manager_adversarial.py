"""Tests for process lifecycle edge cases: early exits, crashes, and silent hangs.

Verifies:
1. Child process crashing immediately on startup is detected and raises ProcessStartupError.
2. Silent hanging child process is caught by timeout handling and does NOT falsely report PASS.
"""

from __future__ import annotations

import os
import shutil
import tempfile
import unittest
from pathlib import Path

from torture_tool.concurrency.dispatcher import ResponseDispatcher
from torture_tool.concurrency.writer import StdinWriter
from torture_tool.process.controller import (
    ChildProcessController,
    ProcessStartupError,
)
from torture_tool.protocol.spec import PingRequest


class TestProcessLifecycleAdversarial(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.mkdtemp()
        self.fake_kvdb = Path(__file__).parent / "mocks" / "fake_kvdb.py"
        self.db_path = Path(self.tmpdir) / "test.db"

    def tearDown(self):
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def test_immediate_crash_on_startup_detected(self):
        """Verify engine that exits immediately on startup raises ProcessStartupError."""
        ctrl = ChildProcessController(
            command=["python3", str(self.fake_kvdb), "serve", str(self.db_path)],
            db_path=self.db_path,
            log_dir=Path(self.tmpdir),
        )
        os.environ["FAKE_KVDB_EARLY_EXIT"] = "1"
        os.environ["FAKE_KVDB_EXIT_CODE"] = "139"
        try:
            with self.assertRaises(ProcessStartupError) as ctx:
                ctrl.start()
            self.assertIn("died immediately on startup", str(ctx.exception))
        finally:
            os.environ.pop("FAKE_KVDB_EARLY_EXIT", None)
            os.environ.pop("FAKE_KVDB_EXIT_CODE", None)
            ctrl.cleanup()

    def test_silent_hang_detected_by_timeout(self):
        """Verify engine that silently hangs on requests times out instead of succeeding."""
        ctrl = ChildProcessController(
            command=["python3", str(self.fake_kvdb), "serve", str(self.db_path)],
            db_path=self.db_path,
        )
        os.environ["FAKE_KVDB_SILENT_HANG"] = "1"
        try:
            ctrl.start()
            dispatcher = ResponseDispatcher(ctrl.process.stdout)
            dispatcher.start()
            writer = StdinWriter(ctrl.process.stdin)

            req = PingRequest(op_id="hang-ping")
            fut = dispatcher.register("hang-ping")
            writer.write_request(req)

            # Awaiting with timeout must raise TimeoutError, never return a fake ACK
            with self.assertRaises(TimeoutError):
                fut.result(timeout=0.2)

            dispatcher.stop()
            ctrl.kill()
            ctrl.cleanup()
        finally:
            os.environ.pop("FAKE_KVDB_SILENT_HANG", None)


if __name__ == "__main__":
    unittest.main()


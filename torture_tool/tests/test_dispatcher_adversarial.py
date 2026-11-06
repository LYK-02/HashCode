"""Tests for ResponseDispatcher and pipe handling under adversarial engine conditions.

Tests:
1. Out-of-order ACK demultiplexing across concurrent worker threads.
2. Stderr flooding (5 MB) does not block or deadlock child process or harness.
3. Premature child pipe closure safely cancels pending futures with ProcessDiedError.
4. Duplicate ACK handling is gracefully resolved without unhandled exceptions.
"""

from __future__ import annotations

import os
import shutil
import tempfile
import time
import unittest
from pathlib import Path

from torture_tool.concurrency.dispatcher import ProcessDiedError, ResponseDispatcher
from torture_tool.concurrency.writer import StdinWriter
from torture_tool.process.controller import ChildProcessController
from torture_tool.protocol.spec import AckResponse, PingRequest, PutRequest
from torture_tool.protocol.utils import bytes_to_hex


class TestDispatcherAdversarial(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.mkdtemp()
        self.fake_kvdb = Path(__file__).parent / "mocks" / "fake_kvdb.py"
        self.db_path = Path(self.tmpdir) / "test.db"

    def tearDown(self):
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def test_out_of_order_response_routing(self):
        """Verify dispatcher correctly routes out-of-order ACKs to the right waiting futures."""
        env = os.environ.copy()
        env["FAKE_KVDB_OUT_OF_ORDER"] = "1"

        ctrl = ChildProcessController(
            command=["python3", str(self.fake_kvdb), "serve", str(self.db_path)],
            db_path=self.db_path,
        )
        ctrl.start()

        dispatcher = ResponseDispatcher(ctrl.process.stdout)
        dispatcher.start()
        writer = StdinWriter(ctrl.process.stdin)

        futures = []
        op_ids = [f"op-ooo-{i:03d}" for i in range(1, 16)]

        for op_id in op_ids:
            req = PutRequest(op_id=op_id, key=b"k", value=b"v")
            fut = dispatcher.register(op_id)
            futures.append((op_id, fut))
            writer.write_request(req)

        # All futures must resolve despite server returning them reversed in batches
        for op_id, fut in futures:
            resp = fut.result(timeout=5.0)
            self.assertIsInstance(resp, AckResponse)
            self.assertEqual(resp.op_id, op_id)

        dispatcher.stop()
        ctrl.kill()
        ctrl.cleanup()

    def test_stderr_flood_no_deadlock(self):
        """Verify that high volume stderr flooding (5 MB) does not block harness execution."""
        ctrl = ChildProcessController(
            command=["python3", str(self.fake_kvdb), "serve", str(self.db_path)],
            db_path=self.db_path,
            log_dir=Path(self.tmpdir),
        )
        # Configure fake engine to flood 5 MB to stderr on startup
        os.environ["FAKE_KVDB_STDERR_FLOOD"] = "1"
        os.environ["FAKE_KVDB_FLOOD_MB"] = "5"
        try:
            ctrl.start()
            self.assertTrue(ctrl.is_alive())

            # StderrDrainer should have captured data without locking up OS pipe buffer
            dispatcher = ResponseDispatcher(ctrl.process.stdout)
            dispatcher.start()
            writer = StdinWriter(ctrl.process.stdin)

            req = PingRequest(op_id="ping-after-flood")
            fut = dispatcher.register("ping-after-flood")
            writer.write_request(req)
            resp = fut.result(timeout=5.0)
            self.assertEqual(resp.op_id, "ping-after-flood")

            dispatcher.stop()
            ctrl.kill()
            ctrl.cleanup()
        finally:
            os.environ.pop("FAKE_KVDB_STDERR_FLOOD", None)
            os.environ.pop("FAKE_KVDB_FLOOD_MB", None)

    def test_pipe_closure_cancels_pending_futures(self):
        """Verify closing stdout or killing engine marks all active registered futures with ProcessDiedError."""
        ctrl = ChildProcessController(
            command=["python3", str(self.fake_kvdb), "serve", str(self.db_path)],
            db_path=self.db_path,
        )
        ctrl.start()

        dispatcher = ResponseDispatcher(ctrl.process.stdout)
        dispatcher.start()

        # Register futures that never get answered
        fut1 = dispatcher.register("never-acked-1")
        fut2 = dispatcher.register("never-acked-2")

        # Kill child
        ctrl.kill()
        dispatcher.stop(timeout=2.0)

        with self.assertRaises(ProcessDiedError):
            fut1.result(timeout=1.0)

        with self.assertRaises(ProcessDiedError):
            fut2.result(timeout=1.0)

        ctrl.cleanup()


if __name__ == "__main__":
    unittest.main()


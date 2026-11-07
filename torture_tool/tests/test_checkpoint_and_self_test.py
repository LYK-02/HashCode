"""Integration and unit tests for deterministic checkpoint crash injection and self-test mode."""

import subprocess
import tempfile
import unittest
from pathlib import Path

from torture_tool.cli import RunConfig
from torture_tool.modes.self_test import SelfTestRunner
from torture_tool.oracle.evaluator import OracleEvaluator, ViolationType
from torture_tool.oracle.journal import JournalEvent
from torture_tool.protocol.spec import BatchItem, BatchRequest, PutRequest
from torture_tool.protocol.utils import bytes_to_hex
from torture_tool.scheduler.checkpoint import (
    CheckpointCrashInjector,
    EngineCheckpoint,
)


class TestCheckpointAndSelfTest(unittest.TestCase):
    def setUp(self):
        self.kvdb_bin = Path("./kvdb").resolve()

    def test_checkpoint_crash_pre_write(self):
        with tempfile.TemporaryDirectory() as tmp_dir:
            db_path = Path(tmp_dir) / "test_chk.db"
            injector = CheckpointCrashInjector(self.kvdb_bin, db_path)
            req = PutRequest(op_id="pw-1", key=b"k1", value=b"v1")
            res = injector.inject_checkpoint_crash(EngineCheckpoint.PRE_WRITE, req)
            self.assertTrue(res.success)
            self.assertEqual(res.bytes_sent, 0)
            # Database should be cleanly recoverable
            chk = subprocess.run([str(self.kvdb_bin), "check", str(db_path)], capture_output=True, text=True)
            self.assertEqual(chk.returncode, 0)
            self.assertIn("OK", chk.stdout)

    def test_checkpoint_crash_mid_payload(self):
        with tempfile.TemporaryDirectory() as tmp_dir:
            db_path = Path(tmp_dir) / "test_chk_mid.db"
            injector = CheckpointCrashInjector(self.kvdb_bin, db_path)
            req = PutRequest(op_id="mid-1", key=b"k1", value=b"v" * 1024)
            res = injector.inject_checkpoint_crash(EngineCheckpoint.MID_PAYLOAD, req)
            self.assertTrue(res.success)
            self.assertGreater(res.bytes_sent, 0)
            chk = subprocess.run([str(self.kvdb_bin), "check", str(db_path)], capture_output=True, text=True)
            self.assertEqual(chk.returncode, 0)
            self.assertIn("OK", chk.stdout)

    def test_checkpoint_crash_mid_batch(self):
        with tempfile.TemporaryDirectory() as tmp_dir:
            db_path = Path(tmp_dir) / "test_chk_batch.db"
            injector = CheckpointCrashInjector(self.kvdb_bin, db_path)
            items = tuple([BatchItem(key=f"b_{i}".encode("ascii"), value=f"v_{i}".encode("ascii")) for i in range(5)])
            req = BatchRequest(op_id="mb-1", items=items)
            res = injector.inject_checkpoint_crash(EngineCheckpoint.MID_BATCH, req)
            self.assertTrue(res.success)
            self.assertGreater(res.bytes_sent, 0)
            chk = subprocess.run([str(self.kvdb_bin), "check", str(db_path)], capture_output=True, text=True)
            self.assertEqual(chk.returncode, 0)
            self.assertIn("OK", chk.stdout)

    def test_checkpoint_crash_post_ack(self):
        with tempfile.TemporaryDirectory() as tmp_dir:
            db_path = Path(tmp_dir) / "test_chk_ack.db"
            injector = CheckpointCrashInjector(self.kvdb_bin, db_path)
            req = PutRequest(op_id="ack-1", key=b"k_ack", value=b"v_ack")
            res = injector.inject_checkpoint_crash(EngineCheckpoint.POST_ACK, req)
            self.assertTrue(res.success)
            # Post ACK must be durable!
            dump = subprocess.run([str(self.kvdb_bin), "dump", str(db_path)], capture_output=True, text=True)
            self.assertEqual(dump.returncode, 0)
            self.assertIn(b"k_ack".hex(), dump.stdout)

    def test_self_test_mode_runner(self):
        config = RunConfig(seed=12345, iterations=1, mode="self-test", kvdb_bin=self.kvdb_bin)
        runner = SelfTestRunner(config)
        res = runner.run_self_test(iteration=1)
        self.assertTrue(res.passed, f"SelfTestRunner failed: {res.error_message}")
        self.assertEqual(res.operations_started, 8)
        self.assertEqual(res.acks_received, 8)


if __name__ == "__main__":
    unittest.main()


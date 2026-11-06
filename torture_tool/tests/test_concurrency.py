"""Unit and integration tests for Phase T4: Response Dispatcher and Concurrent Workers."""

import concurrent.futures
import io
import queue
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from pathlib import Path

from torture_tool.concurrency.dispatcher import (
    DuplicateOperationIdError,
    ProcessDiedError,
    ResponseDispatcher,
)
from torture_tool.concurrency.pool import WorkerPool
from torture_tool.concurrency.worker import WorkerThread
from torture_tool.concurrency.writer import StdinWriter
from torture_tool.generator.prng import SeedManager
from torture_tool.generator.workload import WorkloadConfig, WorkloadGenerator
from torture_tool.oracle.journal import OracleJournal
from torture_tool.process.controller import ChildProcessController
from torture_tool.protocol.spec import AckResponse, ErrResponse, PutRequest


class TestDispatcher(unittest.TestCase):
    def test_out_of_order_resolution(self):
        # Create an OS pipe for testing dispatcher
        r_fd, w_fd = os.pipe()
        r_file = open(r_fd, "rb", buffering=0)
        w_file = open(w_fd, "wb", buffering=0)

        dispatcher = ResponseDispatcher(r_file)
        dispatcher.start()

        # Register two operations
        fut1 = dispatcher.register("op-1")
        fut2 = dispatcher.register("op-2")

        # Emit responses in reverse order
        w_file.write(b"ACK op-2 200\n")
        w_file.flush()
        w_file.write(b"ACK op-1 100\n")
        w_file.flush()

        resp2 = fut2.result(timeout=2.0)
        resp1 = fut1.result(timeout=2.0)

        self.assertIsInstance(resp2, AckResponse)
        self.assertEqual(resp2.op_id, "op-2")
        self.assertEqual(resp2.seq_num, 200)

        self.assertIsInstance(resp1, AckResponse)
        self.assertEqual(resp1.op_id, "op-1")
        self.assertEqual(resp1.seq_num, 100)

        dispatcher.stop()
        w_file.close()
        r_file.close()

    def test_duplicate_op_id_rejected(self):
        r_fd, w_fd = os.pipe()
        r_file = open(r_fd, "rb", buffering=0)
        w_file = open(w_fd, "wb", buffering=0)

        dispatcher = ResponseDispatcher(r_file)
        dispatcher.start()

        dispatcher.register("dup-op")
        with self.assertRaises(DuplicateOperationIdError):
            dispatcher.register("dup-op")

        dispatcher.stop()
        w_file.close()
        r_file.close()

    def test_pipe_close_fails_pending_with_process_died_error(self):
        r_fd, w_fd = os.pipe()
        r_file = open(r_fd, "rb", buffering=0)
        w_file = open(w_fd, "wb", buffering=0)

        dispatcher = ResponseDispatcher(r_file)
        dispatcher.start()

        fut = dispatcher.register("in-flight-op")
        # Abruptly close writer pipe (simulating process death)
        w_file.close()

        with self.assertRaises(ProcessDiedError):
            fut.result(timeout=2.0)

        dispatcher.stop()
        r_file.close()


import os


class TestConcurrentWorkersIntegration(unittest.TestCase):
    def test_concurrent_workers_against_echo_mock(self):
        """Simulate 4 concurrent workers issuing 200 operations to a child serve process."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            journal_path = Path(tmpdir) / "journal.jsonl"
            journal = OracleJournal(journal_path)

            # Python child script simulating kvdb serve (reads line, emits ACK after brief work)
            server_script = (
                "import sys, time\n"
                "for line in sys.stdin:\n"
                "    parts = line.strip().split()\n"
                "    if len(parts) >= 2:\n"
                "        op_id = parts[1]\n"
                "        sys.stdout.write(f'ACK {op_id}\\n')\n"
                "        sys.stdout.flush()\n"
            )

            cmd = [sys.executable, "-c", server_script]
            ctrl = ChildProcessController(command=cmd, db_path=db_path)
            ctrl.start()

            dispatcher = ResponseDispatcher(ctrl.process.stdout)
            dispatcher.start()

            writer = StdinWriter(ctrl.process.stdin)
            seed_mgr = SeedManager(root_seed=12345)
            config = WorkloadConfig.for_mode("default")

            pool = WorkerPool(
                num_workers=4,
                seed_manager=seed_mgr,
                dispatcher=dispatcher,
                writer=writer,
                journal=journal,
                workload_config=config,
                iteration=1,
                request_timeout=5.0,
            )

            # Start 4 workers
            pool.start()

            # Let workers run concurrently for 0.5s
            time.sleep(0.5)

            # Kill child process with SIGKILL while workers are actively writing!
            ctrl.kill()

            # Stop workers and dispatcher
            stats = pool.stop_and_join(timeout=2.0)
            dispatcher.stop()
            journal.close()

            # Verify aggregate execution
            total_ops = sum(s.operations_started for s in stats)
            total_acks = sum(s.acks_received for s in stats)
            total_inflight = sum(s.inflight_at_stop for s in stats)

            self.assertGreater(total_ops, 10)
            self.assertGreater(total_acks, 0)
            self.assertEqual(total_ops, total_acks + total_inflight)

    def test_worker_pool_shared_key_space_configuration(self):
        """WorkerPool properly delegates shared_key_pool parameter to worker generators."""
        seed_mgr = SeedManager(root_seed=999)
        config_shared = WorkloadConfig(shared_key_pool=True)
        config_partitioned = WorkloadConfig(shared_key_pool=False)

        # 1. Default shared pool configuration
        pool1 = WorkerPool(
            num_workers=2,
            seed_manager=seed_mgr,
            dispatcher=None,  # type: ignore
            writer=None,  # type: ignore
            journal=None,  # type: ignore
            workload_config=config_shared,
        )
        self.assertTrue(pool1.shared_key_pool)

        # 2. Partitioned pool configuration
        pool2 = WorkerPool(
            num_workers=2,
            seed_manager=seed_mgr,
            dispatcher=None,  # type: ignore
            writer=None,  # type: ignore
            journal=None,  # type: ignore
            workload_config=config_partitioned,
            shared_key_pool=False,
        )
        self.assertFalse(pool2.shared_key_pool)

    def test_stdin_writer_chunked_transmission_metrics(self):
        """StdinWriter correctly tracks metrics and writes large payloads in chunks."""
        r_fd, w_fd = os.pipe()
        r_file = open(r_fd, "rb", buffering=65536)
        w_file = open(w_fd, "wb", buffering=65536)

        chunk_sz = 32 * 1024
        writer = StdinWriter(w_file, chunk_size=chunk_sz)

        # Write a 16 KiB value (32 KiB hex fits within pipe buffer without blocking)
        large_val = b"x" * (16 * 1024)
        req = PutRequest(op_id="chunk-test-1", key=b"key1", value=large_val)
        wire_str = writer.write_request(req)

        self.assertEqual(writer.requests_sent, 1)
        self.assertEqual(writer.bytes_written, len(wire_str.encode("utf-8")))
        self.assertEqual(writer.lock_acquisitions, 1)
        self.assertGreaterEqual(writer.total_lock_wait_time, 0.0)
        self.assertGreaterEqual(writer.average_lock_wait_time, 0.0)

        # Read back from pipe
        line = r_file.readline().decode("utf-8")
        self.assertTrue(line.startswith("PUT chunk-test-1 "))

        writer.close()
        r_file.close()


if __name__ == "__main__":
    unittest.main()


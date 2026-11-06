"""Comprehensive unit and integration tests for Milestone 3:
Concurrency, Shared-Key Workloads, Oracle Race Resolution, and Writer Lock Contention.
"""

from __future__ import annotations

import io
import os
import sys
import tempfile
import threading
import time
import unittest
from pathlib import Path
from typing import Dict, List, Set

from torture_tool.concurrency.dispatcher import ResponseDispatcher
from torture_tool.concurrency.pool import WorkerPool
from torture_tool.concurrency.writer import StdinWriter
from torture_tool.generator.prng import SeedManager
from torture_tool.generator.workload import (
    WorkloadConfig,
    WorkloadGenerator,
    generate_key,
)
from torture_tool.oracle.evaluator import (
    EvaluationResult,
    InvariantViolation,
    OracleEvaluator,
    ViolationType,
)
from torture_tool.oracle.journal import JournalEvent, OracleJournal
from torture_tool.process.controller import ChildProcessController
from torture_tool.protocol.spec import (
    BatchRequest,
    MAX_RAW_VALUE_LEN,
    PutRequest,
)
from torture_tool.protocol.utils import bytes_to_hex


class TestSharedKeyPoolGeneration(unittest.TestCase):
    """Verify Finding C1: eliminate artificial key isolation across workers."""

    def test_shared_key_pool_omits_worker_prefix(self):
        """When shared_key_pool is True (default), keys do not include worker prefix."""
        mgr = SeedManager(root_seed=123)
        config = WorkloadConfig(shared_key_pool=True, key_pool_size=50)

        gen1 = WorkloadGenerator(worker_id="w1", prng=mgr.get_worker_prng(1), config=config)
        gen2 = WorkloadGenerator(worker_id="w2", prng=mgr.get_worker_prng(2), config=config)

        k1 = gen1.next_key()
        k2 = gen2.next_key()

        self.assertFalse(k1.startswith(b"w1_"), f"Unexpected worker prefix in shared key: {k1}")
        self.assertFalse(k2.startswith(b"w2_"), f"Unexpected worker prefix in shared key: {k2}")
        self.assertTrue(k1.startswith(b"k_"), f"Key should follow k_XXXX format: {k1}")
        self.assertTrue(k2.startswith(b"k_"), f"Key should follow k_XXXX format: {k2}")

    def test_partitioned_key_pool_preserves_worker_prefix(self):
        """When shared_key_pool is False, keys include worker prefix."""
        mgr = SeedManager(root_seed=123)
        config = WorkloadConfig(shared_key_pool=False, key_pool_size=50)

        gen1 = WorkloadGenerator(worker_id="w1", prng=mgr.get_worker_prng(1), config=config)
        k1 = gen1.next_key()
        self.assertTrue(k1.startswith(b"w1_k_"), f"Expected worker prefix: {k1}")

    def test_multi_worker_shared_key_collision_rate(self):
        """Multiple workers with a bounded key pool must generate high collision rates."""
        mgr = SeedManager(root_seed=999)
        pool_size = 15
        config = WorkloadConfig(shared_key_pool=True, key_pool_size=pool_size)

        num_workers = 4
        generators = [
            WorkloadGenerator(
                worker_id=f"w{i}",
                prng=mgr.get_worker_prng(f"w{i}"),
                config=config,
            )
            for i in range(1, num_workers + 1)
        ]

        # Each worker generates 50 operations
        keys_per_worker: Dict[str, Set[bytes]] = {}
        for gen in generators:
            keys = set()
            for _ in range(50):
                op = gen.next_operation()
                if isinstance(op, PutRequest):
                    keys.add(op.key)
                elif isinstance(op, BatchRequest):
                    for item in op.items:
                        keys.add(item.key)
                else:
                    keys.add(getattr(op, "key", b""))
            keys.discard(b"")
            keys_per_worker[gen.worker_id] = keys

        all_keys = set().union(*keys_per_worker.values())
        # Count keys mutated by at least 2 distinct workers
        colliding_keys = [
            k for k in all_keys
            if sum(1 for keys in keys_per_worker.values() if k in keys) >= 2
        ]

        collision_rate = len(colliding_keys) / len(all_keys)
        # Bounded pool of 15 keys across 4 workers generating 50 ops each must yield high collision
        self.assertGreater(collision_rate, 0.70, f"Expected >70% collision rate, got {collision_rate:.2%}")


class TestOracleSharedKeyInflightRaces(unittest.TestCase):
    """Verify OracleEvaluator handling of same-key concurrent mutations."""

    def setUp(self):
        self.k1 = b"shared_k1"
        self.k2 = b"shared_k2"
        self.k3 = b"shared_k3"
        self.v1_base = b"val_base_1"
        self.v2_base = b"val_base_2"
        self.v3_base = b"val_base_3"

        self.base_events = [
            JournalEvent(type="START", ts_ns=1, iteration=1, op_id="init_1", data={"kind": "PUT", "key_hex": self.k1.hex(), "val_hex": self.v1_base.hex()}),
            JournalEvent(type="ACK", ts_ns=2, iteration=1, op_id="init_1", data={"seq_num": 1}),
            JournalEvent(type="START", ts_ns=3, iteration=1, op_id="init_2", data={"kind": "PUT", "key_hex": self.k2.hex(), "val_hex": self.v2_base.hex()}),
            JournalEvent(type="ACK", ts_ns=4, iteration=1, op_id="init_2", data={"seq_num": 2}),
            JournalEvent(type="START", ts_ns=5, iteration=1, op_id="init_3", data={"kind": "PUT", "key_hex": self.k3.hex(), "val_hex": self.v3_base.hex()}),
            JournalEvent(type="ACK", ts_ns=6, iteration=1, op_id="init_3", data={"seq_num": 3}),
        ]

    def test_concurrent_inflight_put_put_identical_key(self):
        """Two concurrent in-flight PUTs targeting the exact same key."""
        v_p1 = b"val_p1"
        v_p2 = b"val_p2"
        events = list(self.base_events) + [
            JournalEvent(type="START", ts_ns=7, iteration=1, op_id="p1", data={"kind": "PUT", "key_hex": self.k1.hex(), "val_hex": v_p1.hex()}),
            JournalEvent(type="START", ts_ns=8, iteration=1, op_id="p2", data={"kind": "PUT", "key_hex": self.k1.hex(), "val_hex": v_p2.hex()}),
        ]

        # Case 1: Neither applied (base retained) -> Must pass
        res = OracleEvaluator.evaluate(events, {self.k1: self.v1_base, self.k2: self.v2_base, self.k3: self.v3_base})
        self.assertTrue(res.passed)

        # Case 2: P1 applied -> Must pass
        res = OracleEvaluator.evaluate(events, {self.k1: v_p1, self.k2: self.v2_base, self.k3: self.v3_base})
        self.assertTrue(res.passed)

        # Case 3: P2 applied -> Must pass
        res = OracleEvaluator.evaluate(events, {self.k1: v_p2, self.k2: self.v2_base, self.k3: self.v3_base})
        self.assertTrue(res.passed)

        # Case 4: Corrupt / torn bytes -> Must fail with VALUE_CORRUPTED
        res = OracleEvaluator.evaluate(events, {self.k1: b"torn_val", self.k2: self.v2_base, self.k3: self.v3_base})
        self.assertFalse(res.passed)
        self.assertTrue(any(v.violation_type == ViolationType.VALUE_CORRUPTED for v in res.violations))

    def test_concurrent_inflight_put_del_identical_key(self):
        """Concurrent in-flight PUT and DEL targeting the exact same key."""
        v_p1 = b"val_p1"
        events = list(self.base_events) + [
            JournalEvent(type="START", ts_ns=7, iteration=1, op_id="p1", data={"kind": "PUT", "key_hex": self.k1.hex(), "val_hex": v_p1.hex()}),
            JournalEvent(type="START", ts_ns=8, iteration=1, op_id="d1", data={"kind": "DEL", "key_hex": self.k1.hex()}),
        ]

        # Case 1: Neither applied -> Must pass
        res = OracleEvaluator.evaluate(events, {self.k1: self.v1_base, self.k2: self.v2_base, self.k3: self.v3_base})
        self.assertTrue(res.passed)

        # Case 2: PUT applied -> Must pass
        res = OracleEvaluator.evaluate(events, {self.k1: v_p1, self.k2: self.v2_base, self.k3: self.v3_base})
        self.assertTrue(res.passed)

        # Case 3: DEL applied (k1 absent) -> Must pass
        res = OracleEvaluator.evaluate(events, {self.k2: self.v2_base, self.k3: self.v3_base})
        self.assertTrue(res.passed)

        # Case 4: Torn bytes -> Must fail with VALUE_CORRUPTED
        res = OracleEvaluator.evaluate(events, {self.k1: b"torn_xyz", self.k2: self.v2_base, self.k3: self.v3_base})
        self.assertFalse(res.passed)
        self.assertTrue(any(v.violation_type == ViolationType.VALUE_CORRUPTED for v in res.violations))

    def test_concurrent_inflight_batch_and_put_identical_key(self):
        """Overlapping in-flight Batch B1 (k1, k2) and in-flight single PUT P1 (k1)."""
        v_b1 = b"val_batch_1"
        v_b2 = b"val_batch_2"
        v_p1 = b"val_single_p1"

        events = list(self.base_events) + [
            JournalEvent(type="START", ts_ns=7, iteration=1, op_id="b1", data={
                "kind": "BATCH",
                "items": [
                    {"k": self.k1.hex(), "v": v_b1.hex()},
                    {"k": self.k2.hex(), "v": v_b2.hex()},
                ]
            }),
            JournalEvent(type="START", ts_ns=8, iteration=1, op_id="p1", data={
                "kind": "PUT",
                "key_hex": self.k1.hex(),
                "val_hex": v_p1.hex()
            }),
        ]

        # Case 1: Both unapplied (k1=base, k2=base) -> Passes
        res1 = OracleEvaluator.evaluate(events, {self.k1: self.v1_base, self.k2: self.v2_base, self.k3: self.v3_base})
        self.assertTrue(res1.passed)

        # Case 2: Batch B1 applied 100%, P1 unapplied (k1=v_b1, k2=v_b2) -> Passes
        res2 = OracleEvaluator.evaluate(events, {self.k1: v_b1, self.k2: v_b2, self.k3: self.v3_base})
        self.assertTrue(res2.passed)

        # Case 3: Batch B1 unapplied, P1 applied (k1=v_p1, k2=v2_base) -> MUST PASS (resolved in M3)
        res3 = OracleEvaluator.evaluate(events, {self.k1: v_p1, self.k2: self.v2_base, self.k3: self.v3_base})
        self.assertTrue(res3.passed, f"Expected pass when B1 unapplied and P1 applied: {res3.violations}")

        # Case 4: Batch B1 applied 100%, P1 serialized after B1 and applied (k1=v_p1, k2=v_b2) -> MUST PASS (resolved in M3)
        res4 = OracleEvaluator.evaluate(events, {self.k1: v_p1, self.k2: v_b2, self.k3: self.v3_base})
        self.assertTrue(res4.passed, f"Expected pass when B1 applied then P1 applied: {res4.violations}")

        # Case 5: Partial batch application (k1 applied from B1, k2 unapplied from B1) -> MUST FAIL with BATCH_PARTIAL_APPLICATION
        res5 = OracleEvaluator.evaluate(events, {self.k1: v_b1, self.k2: self.v2_base, self.k3: self.v3_base})
        self.assertFalse(res5.passed)
        self.assertTrue(any(v.violation_type == ViolationType.BATCH_PARTIAL_APPLICATION for v in res5.violations))

        # Case 6: Torn batch item -> MUST FAIL with VALUE_CORRUPTED
        res6 = OracleEvaluator.evaluate(events, {self.k1: b"torn_b1", self.k2: v_b2, self.k3: self.v3_base})
        self.assertFalse(res6.passed)
        self.assertTrue(any(v.violation_type == ViolationType.VALUE_CORRUPTED for v in res6.violations))

    def test_concurrent_inflight_two_batches_identical_key(self):
        """Two concurrent in-flight batches B1 and B2 sharing key k1."""
        v_b1_1 = b"val_b1_k1"
        v_b1_2 = b"val_b1_k2"
        v_b2_1 = b"val_b2_k1"
        v_b2_3 = b"val_b2_k3"

        events = list(self.base_events) + [
            JournalEvent(type="START", ts_ns=7, iteration=1, op_id="b1", data={
                "kind": "BATCH",
                "items": [
                    {"k": self.k1.hex(), "v": v_b1_1.hex()},
                    {"k": self.k2.hex(), "v": v_b1_2.hex()},
                ]
            }),
            JournalEvent(type="START", ts_ns=8, iteration=1, op_id="b2", data={
                "kind": "BATCH",
                "items": [
                    {"k": self.k1.hex(), "v": v_b2_1.hex()},
                    {"k": self.k3.hex(), "v": v_b2_3.hex()},
                ]
            }),
        ]

        # Case 1: Both applied: B1 applied then B2 applied on k1 (k1=v_b2_1, k2=v_b1_2, k3=v_b2_3) -> PASS
        res1 = OracleEvaluator.evaluate(events, {self.k1: v_b2_1, self.k2: v_b1_2, self.k3: v_b2_3})
        self.assertTrue(res1.passed, f"Expected pass when B1 then B2 applied: {res1.violations}")

        # Case 2: Both applied: B2 applied then B1 applied on k1 (k1=v_b1_1, k2=v_b1_2, k3=v_b2_3) -> PASS
        res2 = OracleEvaluator.evaluate(events, {self.k1: v_b1_1, self.k2: v_b1_2, self.k3: v_b2_3})
        self.assertTrue(res2.passed, f"Expected pass when B2 then B1 applied: {res2.violations}")

        # Case 3: B1 applied, B2 unapplied (k1=v_b1_1, k2=v_b1_2, k3=v3_base) -> PASS
        res3 = OracleEvaluator.evaluate(events, {self.k1: v_b1_1, self.k2: v_b1_2, self.k3: self.v3_base})
        self.assertTrue(res3.passed, f"Expected pass when B1 applied and B2 unapplied: {res3.violations}")

        # Case 4: Neither applied -> PASS
        res4 = OracleEvaluator.evaluate(events, {self.k1: self.v1_base, self.k2: self.v2_base, self.k3: self.v3_base})
        self.assertTrue(res4.passed)

        # Case 5: B1 partially applied -> MUST FAIL
        res5 = OracleEvaluator.evaluate(events, {self.k1: v_b1_1, self.k2: self.v2_base, self.k3: self.v3_base})
        self.assertFalse(res5.passed)
        self.assertTrue(any(v.violation_type == ViolationType.BATCH_PARTIAL_APPLICATION for v in res5.violations))


class TestWriterContentionAndInterleaving(unittest.TestCase):
    """Verify Finding C3: StdinWriter lock optimization, contention tracking, and 1 MiB interleaving."""

    def test_stdin_writer_metrics_initialization(self):
        """Verify metric tracking initialization and properties."""
        r_fd, w_fd = os.pipe()
        w = open(w_fd, "wb", buffering=65536)
        writer = StdinWriter(w)

        self.assertEqual(writer.requests_sent, 0)
        self.assertEqual(writer.bytes_written, 0)
        self.assertEqual(writer.lock_acquisitions, 0)
        self.assertEqual(writer.total_lock_wait_time, 0.0)
        self.assertEqual(writer.average_lock_wait_time, 0.0)
        self.assertEqual(writer.max_lock_wait_time, 0.0)

        writer.close()
        os.close(r_fd)

    def test_low_lock_contention_and_thread_interleaving_during_1mib_payloads(self):
        """Transmit 1 MiB payloads concurrently with small requests; assert low lock wait and interleaving."""
        r_fd, w_fd = os.pipe()
        r = open(r_fd, "rb", buffering=65536)
        w = open(w_fd, "wb", buffering=65536)

        received_ops: List[str] = []
        stop_reader = threading.Event()

        def reader_loop():
            while not stop_reader.is_set():
                line = r.readline()
                if not line:
                    break
                decoded = line.decode("utf-8").strip()
                parts = decoded.split()
                if len(parts) >= 2:
                    received_ops.append(parts[1])

        rt = threading.Thread(target=reader_loop, daemon=True)
        rt.start()

        writer = StdinWriter(w)

        # Worker 1: 5 large 1 MiB payloads
        def worker_large():
            for i in range(5):
                req = PutRequest(
                    op_id=f"w_large-{i}",
                    key=b"large_key",
                    value=b"L" * (1024 * 1024),  # 1 MiB raw bytes -> 2 MiB wire hex
                )
                writer.write_request(req)
                time.sleep(0.002)

        # Worker 2: 20 small 64-byte payloads
        def worker_small():
            for i in range(20):
                req = PutRequest(
                    op_id=f"w_small-{i}",
                    key=b"small_key",
                    value=b"S" * 64,
                )
                writer.write_request(req)
                time.sleep(0.001)

        t1 = threading.Thread(target=worker_large)
        t2 = threading.Thread(target=worker_small)

        t0 = time.monotonic()
        t1.start()
        t2.start()
        t1.join()
        t2.join()

        # Allow pipe reader to drain
        time.sleep(0.05)
        stop_reader.set()
        writer.close()
        r.close()

        # 1. Verify all 25 requests were transmitted and received
        self.assertEqual(len(received_ops), 25)
        self.assertEqual(writer.requests_sent, 25)

        # 2. Verify low lock contention: average lock wait time must be <= 50 ms
        avg_wait_ms = writer.average_lock_wait_time * 1000.0
        self.assertLess(
            avg_wait_ms,
            50.0,
            f"Lock contention too high: average wait was {avg_wait_ms:.2f} ms",
        )

        # 3. Verify thread interleaving occurs (small requests interleaved between large requests)
        worker_ids = [op.split("-")[0] for op in received_ops]
        transitions = sum(1 for i in range(len(worker_ids) - 1) if worker_ids[i] != worker_ids[i + 1])
        self.assertGreater(
            transitions,
            4,
            f"Insufficient thread interleaving during 1 MiB writes: only {transitions} transitions observed",
        )


class TestLiveSharedKeyWorkerPoolStress(unittest.TestCase):
    """Integration test: Multi-worker pool running against fake_kvdb in shared key pool mode."""

    def test_concurrent_shared_key_pool_with_crash_and_recovery(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test_shared.db"
            journal_path = Path(tmpdir) / "journal_shared.jsonl"
            fake_kvdb = Path(__file__).parent / "mocks" / "fake_kvdb.py"

            journal = OracleJournal(journal_path)
            cmd = ["python3", str(fake_kvdb), "serve", str(db_path)]
            ctrl = ChildProcessController(command=cmd, db_path=db_path)
            ctrl.start()

            dispatcher = ResponseDispatcher(ctrl.process.stdout)
            dispatcher.start()

            writer = StdinWriter(ctrl.process.stdin)
            seed_mgr = SeedManager(root_seed=4242)
            # High collision configuration: small pool size across 4 workers
            config = WorkloadConfig(
                mode="default",
                key_pool_size=10,
                shared_key_pool=True,
                put_ratio=0.60,
                del_ratio=0.20,
                batch_ratio=0.20,
            )

            pool = WorkerPool(
                num_workers=4,
                seed_manager=seed_mgr,
                dispatcher=dispatcher,
                writer=writer,
                journal=journal,
                workload_config=config,
                iteration=1,
                request_timeout=5.0,
                shared_key_pool=True,
            )

            pool.start()
            # Allow active concurrent mutations against shared keys
            time.sleep(0.4)

            # Crash child process via SIGKILL
            ctrl.kill()

            stats = pool.stop_and_join(timeout=2.0)
            dispatcher.stop()
            journal.close()

            # Read recovered state via fake_kvdb dump
            import subprocess
            dump_proc = subprocess.run(
                ["python3", str(fake_kvdb), "dump", str(db_path)],
                capture_output=True,
                text=True,
                check=True,
            )

            from torture_tool.verifier.dump_parser import parse_dump_output
            actual_state = parse_dump_output(dump_proc.stdout)

            # Evaluate with independent oracle
            events = list(OracleJournal.read_events(journal_path))
            res = OracleEvaluator.evaluate(events=events, actual_state=actual_state)

            self.assertTrue(
                res.passed,
                f"Oracle evaluation failed on shared-key crash recovery: {[v.format_diagnostic() for v in res.violations]}",
            )
            self.assertGreater(res.total_acked_ops, 5)


if __name__ == "__main__":
    unittest.main()

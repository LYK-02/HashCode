"""Unit tests for Phase T1: CLI parsing, PRNG hierarchy, and deterministic workload generation."""

import argparse
import sys
import unittest
from pathlib import Path

from torture_tool.cli import RunConfig, create_parser, parse_args
from torture_tool.generator.prng import SeedManager, derive_subseed
from torture_tool.generator.workload import (
    WorkloadConfig,
    WorkloadGenerator,
    generate_key,
    generate_value,
)
from torture_tool.protocol.spec import (
    BatchRequest,
    DelRequest,
    MAX_RAW_VALUE_LEN,
    PutRequest,
)


class TestCli(unittest.TestCase):
    def test_mandatory_seed_present(self):
        config = parse_args(["--seed", "12345"])
        self.assertEqual(config.seed, 12345)
        self.assertEqual(config.iterations, 500)
        self.assertEqual(config.mode, "default")
        self.assertEqual(config.workers, 2)
        self.assertEqual(config.timeout, 30.0)
        self.assertEqual(config.db_path, Path("data.db"))

    def test_missing_seed_fails_with_exit_code_2(self):
        parser = create_parser()
        with self.assertRaises(SystemExit) as cm:
            parser.parse_args([])
        self.assertEqual(cm.exception.code, 2)

    def test_workers_less_than_two_rejected(self):
        with self.assertRaises(SystemExit) as cm:
            parse_args(["--seed", "123", "--workers", "1"])
        self.assertEqual(cm.exception.code, 2)

    def test_iterations_less_than_one_rejected(self):
        with self.assertRaises(SystemExit) as cm:
            parse_args(["--seed", "123", "--iterations", "0"])
        self.assertEqual(cm.exception.code, 2)

    def test_valid_custom_flags(self):
        config = parse_args(
            [
                "--seed",
                "999",
                "--iterations",
                "100",
                "--mode",
                "large-values",
                "--workers",
                "4",
                "--timeout",
                "15.5",
                "--db",
                "/tmp/test.db",
                "--artifact-dir",
                "my_artifacts",
                "--kvdb-bin",
                "/bin/kvdb",
            ]
        )
        self.assertEqual(config.seed, 999)
        self.assertEqual(config.iterations, 100)
        self.assertEqual(config.mode, "large-values")
        self.assertEqual(config.workers, 4)
        self.assertEqual(config.timeout, 15.5)
        self.assertEqual(config.db_path, Path("/tmp/test.db"))
        self.assertEqual(config.artifact_dir, Path("my_artifacts"))
        self.assertEqual(config.kvdb_bin, Path("/bin/kvdb"))

    def test_invalid_mode_rejected(self):
        with self.assertRaises(SystemExit) as cm:
            parse_args(["--seed", "123", "--mode", "nonexistent-mode"])
        self.assertEqual(cm.exception.code, 2)


class TestPrngHierarchy(unittest.TestCase):
    def test_deterministic_subseed_derivation(self):
        s1 = derive_subseed(12345, "worker", 1)
        s2 = derive_subseed(12345, "worker", 1)
        self.assertEqual(s1, s2)

    def test_different_domains_produce_distinct_seeds(self):
        s_worker = derive_subseed(12345, "worker", 0)
        s_scheduler = derive_subseed(12345, "scheduler", 0)
        self.assertNotEqual(s_worker, s_scheduler)

    def test_different_indices_produce_distinct_seeds(self):
        s1 = derive_subseed(12345, "worker", 1)
        s2 = derive_subseed(12345, "worker", 2)
        self.assertNotEqual(s1, s2)

    def test_seed_manager_stream_isolation(self):
        mgr1 = SeedManager(root_seed=42)
        mgr2 = SeedManager(root_seed=42)

        p1 = mgr1.get_worker_prng(1)
        p2 = mgr2.get_worker_prng(1)

        # Both streams must output identical sequence
        vals1 = [p1.random() for _ in range(50)]
        vals2 = [p2.random() for _ in range(50)]
        self.assertEqual(vals1, vals2)

        # Worker 2 stream must be different
        p3 = mgr1.get_worker_prng(2)
        vals3 = [p3.random() for _ in range(50)]
        self.assertNotEqual(vals1, vals3)


class TestWorkloadGeneration(unittest.TestCase):
    def test_reproducible_operations(self):
        mgr1 = SeedManager(root_seed=777)
        gen1 = WorkloadGenerator(worker_id="w1", prng=mgr1.get_worker_prng(1))

        mgr2 = SeedManager(root_seed=777)
        gen2 = WorkloadGenerator(worker_id="w1", prng=mgr2.get_worker_prng(1))

        ops1 = [gen1.next_operation() for _ in range(100)]
        ops2 = [gen2.next_operation() for _ in range(100)]

        for op1, op2 in zip(ops1, ops2):
            self.assertEqual(type(op1), type(op2))
            self.assertEqual(op1.op_id, op2.op_id)
            if isinstance(op1, PutRequest):
                self.assertEqual(op1.key, op2.key)
                self.assertEqual(op1.value, op2.value)
            elif isinstance(op1, DelRequest):
                self.assertEqual(op1.key, op2.key)
            elif isinstance(op1, BatchRequest):
                self.assertEqual(len(op1.items), len(op2.items))
                for item1, item2 in zip(op1.items, op2.items):
                    self.assertEqual(item1.key, item2.key)
                    self.assertEqual(item1.value, item2.value)

    def test_different_seeds_produce_different_workloads(self):
        mgr1 = SeedManager(root_seed=111)
        gen1 = WorkloadGenerator(worker_id="w1", prng=mgr1.get_worker_prng(1))

        mgr2 = SeedManager(root_seed=222)
        gen2 = WorkloadGenerator(worker_id="w1", prng=mgr2.get_worker_prng(1))

        ops1 = [gen1.next_operation() for _ in range(50)]
        ops2 = [gen2.next_operation() for _ in range(50)]

        # Payloads and keys should diverge
        keys1 = [getattr(op, "key", None) for op in ops1]
        keys2 = [getattr(op, "key", None) for op in ops2]
        self.assertNotEqual(keys1, keys2)

    def test_large_values_mode_boundaries(self):
        config = WorkloadConfig.for_mode("large-values")
        mgr = SeedManager(root_seed=123)
        gen = WorkloadGenerator(worker_id="w1", prng=mgr.get_worker_prng(1), config=config)

        large_put = gen.generate_put()
        self.assertGreaterEqual(len(large_put.value), 64 * 1024)
        self.assertLessEqual(len(large_put.value), MAX_RAW_VALUE_LEN)

    def test_large_batches_mode_boundaries(self):
        config = WorkloadConfig.for_mode("large-batches")
        mgr = SeedManager(root_seed=456)
        gen = WorkloadGenerator(worker_id="w1", prng=mgr.get_worker_prng(1), config=config)

        batch = gen.generate_batch(count=10000)
        self.assertEqual(len(batch.items), 10000)

    def test_empty_keys_never_generated(self):
        mgr = SeedManager(root_seed=999)
        prng = mgr.get_worker_prng(1)
        for _ in range(500):
            k = generate_key(prng, key_pool_size=100)
            self.assertGreater(len(k), 0)


if __name__ == "__main__":
    unittest.main()


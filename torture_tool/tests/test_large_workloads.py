"""Unit and integration tests for Phase T7: Large Values and Massive Batches."""

import hashlib
import random
import unittest

from torture_tool.generator.large_batches import generate_giant_batch
from torture_tool.generator.large_values import generate_large_payload
from torture_tool.generator.prng import SeedManager
from torture_tool.generator.workload import WorkloadConfig, WorkloadGenerator
from torture_tool.protocol.codec import encode_request, parse_request
from torture_tool.protocol.spec import (
    BatchRequest,
    MAX_RAW_VALUE_LEN,
    PutRequest,
)


class TestLargeValues(unittest.TestCase):
    def test_exact_1_mib_payload_generation(self):
        prng = random.Random(12345)
        data, digest = generate_large_payload(
            size_bytes=MAX_RAW_VALUE_LEN,
            pattern_type="noise",
            prng=prng,
        )
        self.assertEqual(len(data), MAX_RAW_VALUE_LEN)
        self.assertEqual(hashlib.sha256(data).hexdigest(), digest)

    def test_null_bytes_boundary_payload(self):
        prng = random.Random(42)
        data, digest = generate_large_payload(
            size_bytes=1024 * 512,  # 512 KiB
            pattern_type="nulls",
            prng=prng,
        )
        self.assertEqual(len(data), 512 * 1024)
        self.assertEqual(data, b"\x00" * (512 * 1024))
        self.assertEqual(hashlib.sha256(data).hexdigest(), digest)

    def test_delimiter_rich_binary_payload(self):
        prng = random.Random(99)
        data, digest = generate_large_payload(
            size_bytes=64 * 1024,
            pattern_type="delimiters",
            prng=prng,
        )
        self.assertIn(b",:\n\r", data)
        self.assertEqual(len(data), 64 * 1024)

    def test_wire_framing_for_large_values(self):
        """Verify that encoding and decoding a 1 MiB value maintains byte fidelity."""
        prng = random.Random(777)
        val, _ = generate_large_payload(1024 * 1024, "noise", prng)
        req = PutRequest(op_id="large-1", key=b"big_key", value=val)

        wire_line = encode_request(req)
        self.assertTrue(wire_line.startswith("PUT large-1 6269675f6b6579 "))
        self.assertTrue(wire_line.endswith("\n"))

        parsed = parse_request(wire_line)
        self.assertIsInstance(parsed, PutRequest)
        self.assertEqual(parsed.key, b"big_key")
        self.assertEqual(parsed.value, val)
        self.assertEqual(len(parsed.value), 1024 * 1024)


class TestLargeBatches(unittest.TestCase):
    def test_giant_batch_generation(self):
        prng = random.Random(888)
        batch_req, intent = generate_giant_batch(
            op_id="batch-10k",
            count=10000,
            key_pool_size=500,  # Ensure collisions & intra-batch overwrites
            prng=prng,
            min_val_size=4,
            max_val_size=16,
        )

        self.assertEqual(len(batch_req.items), 10000)
        # Verify intent map has all distinct keys
        self.assertLessEqual(len(intent), 500)
        self.assertGreater(len(intent), 100)

    def test_wire_framing_for_moderate_batch(self):
        """Verify framing and encoding of 1,000-op batch across pipe codec."""
        prng = random.Random(555)
        batch_req, _ = generate_giant_batch(
            op_id="batch-1k",
            count=1000,
            key_pool_size=100,
            prng=prng,
            min_val_size=4,
            max_val_size=8,
        )

        wire_line = encode_request(batch_req)
        self.assertTrue(wire_line.startswith("BATCH batch-1k 1000 "))

        parsed = parse_request(wire_line)
        self.assertIsInstance(parsed, BatchRequest)
        self.assertEqual(len(parsed.items), 1000)


if __name__ == "__main__":
    unittest.main()


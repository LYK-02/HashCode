"""Negative validation suite for the independent reference oracle.

Guarantees that the Oracle NEVER reports a false positive:
1. ACKED_WRITE_LOST: An acknowledged write is missing from post-recovery state -> FAIL.
2. VALUE_CORRUPTED: An acknowledged write has a 1-bit corrupted value -> FAIL.
3. BATCH_PARTIAL_APPLICATION: 3 of 4 items from an unacknowledged batch appear -> FAIL.
4. UNKNOWN_KEY_DETECTED: A key appears in actual state that was never issued -> FAIL.
5. FAILED_OP_APPLIED: An operation that explicitly failed (ERR) appears in state -> FAIL.
"""

from __future__ import annotations

import unittest

from torture_tool.oracle.evaluator import OracleEvaluator, ViolationType
from torture_tool.oracle.journal import JournalEvent
from torture_tool.protocol.utils import bytes_to_hex


class TestOracleNegativeViolations(unittest.TestCase):
    def test_missing_acked_write_fails_with_acked_write_lost(self):
        """If a write was ACKed, but is missing from actual state, oracle must fail with ACKED_WRITE_LOST."""
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="op-001",
                data={
                    "kind": "PUT",
                    "key_hex": bytes_to_hex(b"key_a"),
                    "val_hex": bytes_to_hex(b"val_a"),
                },
            ),
            JournalEvent(type="ACK", ts_ns=2000, iteration=1, op_id="op-001"),
        ]
        # Actual state has NO keys
        actual_state = {}

        res = OracleEvaluator.evaluate(events=events, actual_state=actual_state)
        self.assertFalse(res.passed)
        self.assertEqual(len(res.violations), 1)
        self.assertEqual(res.violations[0].violation_type, ViolationType.ACKED_WRITE_LOST)
        self.assertEqual(res.violations[0].key, b"key_a")

    def test_corrupted_value_fails_with_value_corrupted(self):
        """If a write was ACKed, but value has bit flips, oracle must fail with VALUE_CORRUPTED."""
        val = b"hello world 12345"
        corrupted_val = b"hello world 12344"  # 1-bit flip
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="op-002",
                data={
                    "kind": "PUT",
                    "key_hex": bytes_to_hex(b"key_b"),
                    "val_hex": bytes_to_hex(val),
                },
            ),
            JournalEvent(type="ACK", ts_ns=2000, iteration=1, op_id="op-002"),
        ]
        actual_state = {b"key_b": corrupted_val}

        res = OracleEvaluator.evaluate(events=events, actual_state=actual_state)
        self.assertFalse(res.passed)
        self.assertEqual(len(res.violations), 1)
        self.assertEqual(res.violations[0].violation_type, ViolationType.VALUE_CORRUPTED)
        self.assertEqual(res.violations[0].key, b"key_b")

    def test_partial_batch_application_fails_atomicity(self):
        """If an unacknowledged batch has only some of its mutations applied, oracle must fail with BATCH_PARTIAL_APPLICATION."""
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="batch-001",
                data={
                    "kind": "BATCH",
                    "items": [
                        {"k": bytes_to_hex(b"b_k1"), "v": bytes_to_hex(b"v1")},
                        {"k": bytes_to_hex(b"b_k2"), "v": bytes_to_hex(b"v2")},
                        {"k": bytes_to_hex(b"b_k3"), "v": bytes_to_hex(b"v3")},
                        {"k": bytes_to_hex(b"b_k4"), "v": bytes_to_hex(b"v4")},
                    ],
                },
            ),
            # NO ACK for batch (in-flight crash)
        ]
        # Only 3 of the 4 items applied
        actual_state = {
            b"b_k1": b"v1",
            b"b_k2": b"v2",
            b"b_k3": b"v3",
        }

        res = OracleEvaluator.evaluate(events=events, actual_state=actual_state)
        self.assertFalse(res.passed)
        self.assertTrue(
            any(v.violation_type == ViolationType.BATCH_PARTIAL_APPLICATION for v in res.violations)
        )

    def test_phantom_unknown_key_fails_with_unknown_key_detected(self):
        """If a key exists that was never referenced in any client request, oracle must fail with UNKNOWN_KEY_DETECTED."""
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="op-003",
                data={
                    "kind": "PUT",
                    "key_hex": bytes_to_hex(b"valid_key"),
                    "val_hex": bytes_to_hex(b"v"),
                },
            ),
            JournalEvent(type="ACK", ts_ns=2000, iteration=1, op_id="op-003"),
        ]
        # State contains phantom key
        actual_state = {
            b"valid_key": b"v",
            b"phantom_key": b"ghost_data",
        }

        res = OracleEvaluator.evaluate(events=events, actual_state=actual_state)
        self.assertFalse(res.passed)
        self.assertTrue(
            any(v.violation_type == ViolationType.UNKNOWN_KEY_DETECTED for v in res.violations)
        )

    def test_failed_operation_applied_fails(self):
        """If an operation received an explicit ERR response, but was applied anyway, oracle must fail with FAILED_OP_APPLIED."""
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="op-004",
                data={
                    "kind": "PUT",
                    "key_hex": bytes_to_hex(b"rejected_key"),
                    "val_hex": bytes_to_hex(b"v"),
                },
            ),
            JournalEvent(
                type="FAIL",
                ts_ns=2000,
                iteration=1,
                op_id="op-004",
                data={"code": "QUOTA_EXCEEDED"},
            ),
        ]
        # The key was mistakenly saved to disk anyway
        actual_state = {b"rejected_key": b"v"}

        res = OracleEvaluator.evaluate(events=events, actual_state=actual_state)
        self.assertFalse(res.passed)
        self.assertTrue(
            any(v.violation_type == ViolationType.FAILED_OP_APPLIED for v in res.violations)
        )


if __name__ == "__main__":
    unittest.main()

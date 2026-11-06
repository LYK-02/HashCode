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

    def test_inflight_put_corrupted_value_fails_with_value_corrupted(self):
        """If an in-flight PUT leaves torn or corrupted bytes on disk, oracle must fail with VALUE_CORRUPTED."""
        events = [
            # Prior acknowledged write
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="op-base",
                data={
                    "kind": "PUT",
                    "key_hex": bytes_to_hex(b"target_key"),
                    "val_hex": bytes_to_hex(b"prior_val"),
                },
            ),
            JournalEvent(type="ACK", ts_ns=1500, iteration=1, op_id="op-base"),
            # In-flight write (started but never ACKed before crash)
            JournalEvent(
                type="START",
                ts_ns=2000,
                iteration=1,
                op_id="op-inflight",
                data={
                    "kind": "PUT",
                    "key_hex": bytes_to_hex(b"target_key"),
                    "val_hex": bytes_to_hex(b"new_val"),
                },
            ),
        ]

        # Case 1: Corrupted/torn bytes on disk (neither prior_val nor new_val)
        res_corrupt = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"target_key": b"torn_corrupted_bytes"},
        )
        self.assertFalse(res_corrupt.passed)
        self.assertTrue(
            any(v.violation_type == ViolationType.VALUE_CORRUPTED for v in res_corrupt.violations)
        )

        # Case 2: New expected value on disk -> MUST PASS
        res_new = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"target_key": b"new_val"},
        )
        self.assertTrue(res_new.passed)

        # Case 3: Prior acknowledged value on disk -> MUST PASS
        res_prior = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"target_key": b"prior_val"},
        )
        self.assertTrue(res_prior.passed)

    def test_inflight_put_new_key_corrupted_value_fails(self):
        """If an in-flight PUT on a new key leaves torn bytes, oracle must fail with VALUE_CORRUPTED."""
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="op-new",
                data={
                    "kind": "PUT",
                    "key_hex": bytes_to_hex(b"fresh_key"),
                    "val_hex": bytes_to_hex(b"expected_full_value"),
                },
            ),
        ]

        # Case 1: Torn bytes on disk
        res_torn = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"fresh_key": b"expected_partial"},
        )
        self.assertFalse(res_torn.passed)
        self.assertTrue(
            any(v.violation_type == ViolationType.VALUE_CORRUPTED for v in res_torn.violations)
        )

        # Case 2: Expected value on disk -> MUST PASS
        res_valid = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"fresh_key": b"expected_full_value"},
        )
        self.assertTrue(res_valid.passed)

        # Case 3: Absent from disk -> MUST PASS (write never reached disk)
        res_absent = OracleEvaluator.evaluate(
            events=events,
            actual_state={},
        )
        self.assertTrue(res_absent.passed)

    def test_inflight_batch_torn_values_fails_with_value_corrupted(self):
        """If an in-flight batch leaves torn bytes for its keys, oracle must reject with VALUE_CORRUPTED."""
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="batch-torn",
                data={
                    "kind": "BATCH",
                    "items": [
                        {"k": bytes_to_hex(b"bk1"), "v": bytes_to_hex(b"bv1")},
                        {"k": bytes_to_hex(b"bk2"), "v": bytes_to_hex(b"bv2")},
                    ],
                },
            ),
        ]

        # Both keys have torn values on disk
        res_torn = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"bk1": b"torn_1", b"bk2": b"torn_2"},
        )
        self.assertFalse(res_torn.passed)
        self.assertTrue(
            any(v.violation_type == ViolationType.VALUE_CORRUPTED for v in res_torn.violations)
        )

    def test_inflight_batch_one_applied_one_torn_fails(self):
        """If an in-flight batch has one key applied and one key torn, oracle must flag violations."""
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="batch-partial-torn",
                data={
                    "kind": "BATCH",
                    "items": [
                        {"k": bytes_to_hex(b"bk1"), "v": bytes_to_hex(b"bv1")},
                        {"k": bytes_to_hex(b"bk2"), "v": bytes_to_hex(b"bv2")},
                    ],
                },
            ),
        ]

        # One applied, one torn
        res = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"bk1": b"bv1", b"bk2": b"torn_2"},
        )
        self.assertFalse(res.passed)
        violation_types = {v.violation_type for v in res.violations}
        self.assertIn(ViolationType.VALUE_CORRUPTED, violation_types)
        self.assertIn(ViolationType.BATCH_PARTIAL_APPLICATION, violation_types)

    def test_ack_seq_num_monotonic_replay_ordering(self):
        """Out-of-order ACK events with protocol seq_num must be replayed in monotonic sequence order."""
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="op-seq-1",
                data={
                    "kind": "PUT",
                    "key_hex": bytes_to_hex(b"same_key"),
                    "val_hex": bytes_to_hex(b"older_write_seq10"),
                },
            ),
            JournalEvent(
                type="START",
                ts_ns=1100,
                iteration=1,
                op_id="op-seq-2",
                data={
                    "kind": "PUT",
                    "key_hex": bytes_to_hex(b"same_key"),
                    "val_hex": bytes_to_hex(b"newer_write_seq20"),
                },
            ),
            # Demux logged ACK for op-seq-2 FIRST, then ACK for op-seq-1 SECOND:
            JournalEvent(
                type="ACK",
                ts_ns=2000,
                iteration=1,
                op_id="op-seq-2",
                data={"seq_num": 20},
            ),
            JournalEvent(
                type="ACK",
                ts_ns=2100,
                iteration=1,
                op_id="op-seq-1",
                data={"seq_num": 10},
            ),
        ]

        # The state machine must sort by seq_num: op-seq-1 (seq 10) replayed first,
        # then op-seq-2 (seq 20) replayed, leaving 'newer_write_seq20' as the final value.
        res_valid = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"same_key": b"newer_write_seq20"},
        )
        self.assertTrue(res_valid.passed)
        self.assertEqual(len(res_valid.violations), 0)

        # If it had not sorted by seq_num, 'older_write_seq10' would be thought of as newer!
        res_invalid = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"same_key": b"older_write_seq10"},
        )
        self.assertFalse(res_invalid.passed)
        self.assertEqual(res_invalid.violations[0].violation_type, ViolationType.VALUE_CORRUPTED)


if __name__ == "__main__":
    unittest.main()

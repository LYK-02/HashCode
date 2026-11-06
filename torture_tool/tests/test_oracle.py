"""Comprehensive unit test suite for Phase T2: Independent Reference Oracle."""

import json
import tempfile
import unittest
from pathlib import Path

from torture_tool.oracle.evaluator import (
    InvariantViolation,
    OracleEvaluator,
    ViolationType,
)
from torture_tool.oracle.journal import JournalEvent, OracleJournal
from torture_tool.oracle.state import ReferenceDatabaseState
from torture_tool.protocol.spec import (
    BatchItem,
    BatchRequest,
    DelRequest,
    PutRequest,
)
from torture_tool.protocol.utils import bytes_to_hex


class TestOracleJournal(unittest.TestCase):
    def test_journal_round_trip(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            j_path = Path(tmpdir) / "events.jsonl"
            journal = OracleJournal(j_path)

            journal.log_iteration_start(1)
            req1 = PutRequest(op_id="w1-1-01", key=b"keyA", value=b"valA")
            journal.log_start(1, req1)
            journal.log_ack(1, "w1-1-01", seq_num=10)

            req2 = DelRequest(op_id="w1-1-02", key=b"keyA")
            journal.log_start(1, req2)
            journal.log_fail(1, "w1-1-02", code="IO_ERROR", msg="Disk fail")

            journal.log_iteration_end(1, "PASS")
            journal.close()

            # Read back
            events = list(OracleJournal.read_events(j_path))
            self.assertEqual(len(events), 6)
            self.assertEqual(events[0].type, "ITER_START")
            self.assertEqual(events[1].type, "START")
            self.assertEqual(events[1].op_id, "w1-1-01")
            self.assertEqual(events[2].type, "ACK")
            self.assertEqual(events[2].op_id, "w1-1-01")
            self.assertEqual(events[3].type, "START")
            self.assertEqual(events[3].op_id, "w1-1-02")
            self.assertEqual(events[4].type, "FAIL")
            self.assertEqual(events[4].op_id, "w1-1-02")
            self.assertEqual(events[5].type, "ITER_END")


class TestOracleEvaluator(unittest.TestCase):
    def test_acknowledged_writes_strictly_verified(self):
        events = [
            JournalEvent("START", 100, 1, "op-1", {"kind": "PUT", "key_hex": bytes_to_hex(b"k1"), "val_hex": bytes_to_hex(b"v1")}),
            JournalEvent("ACK", 200, 1, "op-1"),
            JournalEvent("START", 300, 1, "op-2", {"kind": "PUT", "key_hex": bytes_to_hex(b"k2"), "val_hex": bytes_to_hex(b"v2")}),
            JournalEvent("ACK", 400, 1, "op-2"),
        ]

        # Valid recovered state matching exactly
        actual_valid = {b"k1": b"v1", b"k2": b"v2"}
        res = OracleEvaluator.evaluate(events, actual_valid)
        self.assertTrue(res.passed)
        self.assertEqual(len(res.violations), 0)

        # Missing ACKed key
        actual_missing = {b"k1": b"v1"}
        res_missing = OracleEvaluator.evaluate(events, actual_missing)
        self.assertFalse(res_missing.passed)
        self.assertEqual(res_missing.violations[0].violation_type, ViolationType.ACKED_WRITE_LOST)
        self.assertEqual(res_missing.violations[0].key, b"k2")

        # Corrupted value in ACKed key
        actual_corrupt = {b"k1": b"v1", b"k2": b"v_corrupt"}
        res_corrupt = OracleEvaluator.evaluate(events, actual_corrupt)
        self.assertFalse(res_corrupt.passed)
        self.assertEqual(res_corrupt.violations[0].violation_type, ViolationType.VALUE_CORRUPTED)
        self.assertEqual(res_corrupt.violations[0].key, b"k2")

    def test_unknown_key_detection(self):
        events = [
            JournalEvent("START", 100, 1, "op-1", {"kind": "PUT", "key_hex": bytes_to_hex(b"k1"), "val_hex": bytes_to_hex(b"v1")}),
            JournalEvent("ACK", 200, 1, "op-1"),
        ]
        # Actual state has an extra unknown key
        actual = {b"k1": b"v1", b"phantom_key": b"ghost"}
        res = OracleEvaluator.evaluate(events, actual)
        self.assertFalse(res.passed)
        self.assertEqual(res.violations[0].violation_type, ViolationType.UNKNOWN_KEY_DETECTED)
        self.assertEqual(res.violations[0].key, b"phantom_key")

    def test_inflight_single_op_ambiguity_handling(self):
        events = [
            JournalEvent("START", 100, 1, "op-1", {"kind": "PUT", "key_hex": bytes_to_hex(b"k1"), "val_hex": bytes_to_hex(b"v1")}),
            JournalEvent("ACK", 200, 1, "op-1"),
            # op-2 started but NEVER acked before crash
            JournalEvent("START", 300, 1, "op-2", {"kind": "PUT", "key_hex": bytes_to_hex(b"k2"), "val_hex": bytes_to_hex(b"v2")}),
        ]

        # Scenario A: op-2 reached disk before crash -> VALID
        res_a = OracleEvaluator.evaluate(events, {b"k1": b"v1", b"k2": b"v2"})
        self.assertTrue(res_a.passed)

        # Scenario B: op-2 was lost before disk commit -> VALID
        res_b = OracleEvaluator.evaluate(events, {b"k1": b"v1"})
        self.assertTrue(res_b.passed)

    def test_inflight_batch_all_or_none_atomicity(self):
        batch_items = [
            {"k": bytes_to_hex(b"bk1"), "v": bytes_to_hex(b"bv1")},
            {"k": bytes_to_hex(b"bk2"), "v": bytes_to_hex(b"bv2")},
            {"k": bytes_to_hex(b"bk3"), "v": bytes_to_hex(b"bv3")},
        ]
        events = [
            # In-flight batch never ACKed
            JournalEvent("START", 100, 1, "b-1", {"kind": "BATCH", "items": batch_items}),
        ]

        # Case 1: 100% applied -> VALID
        actual_all = {b"bk1": b"bv1", b"bk2": b"bv2", b"bk3": b"bv3"}
        self.assertTrue(OracleEvaluator.evaluate(events, actual_all).passed)

        # Case 2: 0% applied -> VALID
        actual_none = {}
        self.assertTrue(OracleEvaluator.evaluate(events, actual_none).passed)

        # Case 3: Partial application (2 out of 3 keys) -> STRICTLY INVALID!
        actual_partial = {b"bk1": b"bv1", b"bk2": b"bv2"}
        res_partial = OracleEvaluator.evaluate(events, actual_partial)
        self.assertFalse(res_partial.passed)
        self.assertEqual(res_partial.violations[0].violation_type, ViolationType.BATCH_PARTIAL_APPLICATION)
        self.assertEqual(res_partial.violations[0].op_id, "b-1")

    def test_overwrites_and_deletes_ordering(self):
        events = [
            JournalEvent("START", 100, 1, "op-1", {"kind": "PUT", "key_hex": bytes_to_hex(b"k"), "val_hex": bytes_to_hex(b"v1")}),
            JournalEvent("ACK", 150, 1, "op-1"),
            JournalEvent("START", 200, 1, "op-2", {"kind": "PUT", "key_hex": bytes_to_hex(b"k"), "val_hex": bytes_to_hex(b"v2")}),
            JournalEvent("ACK", 250, 1, "op-2"),
            JournalEvent("START", 300, 1, "op-3", {"kind": "DEL", "key_hex": bytes_to_hex(b"k")}),
            JournalEvent("ACK", 350, 1, "op-3"),
        ]

        # The key was deleted, so actual empty state is correct
        self.assertTrue(OracleEvaluator.evaluate(events, {}).passed)
        # If the key still exists with v1 or v2, it violates the delete ACK!
        res_stale = OracleEvaluator.evaluate(events, {b"k": b"v1"})
        self.assertFalse(res_stale.passed)


if __name__ == "__main__":
    unittest.main()


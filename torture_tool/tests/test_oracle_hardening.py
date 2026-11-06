"""Comprehensive hardening tests for Milestone 2: Independent Oracle & State Evaluation.

Empirically validates:
1. Finding O1: In-flight single PUT value verification (torn/corrupted bytes on disk fail with VALUE_CORRUPTED).
2. In-flight PUT valid values (V_expected and V_prior pass; absent passes for new key).
3. Finding O1/Vulnerability 2: In-flight batch atomicity and torn value detection (all-or-nothing strictly enforced).
4. Finding O2: Protocol seq_num monotonic replay ordering even when wire events demux out-of-order.
5. Finding R2: Strict dump parser integration rejecting duplicate keys and failing iterations with DUMP_PARSE_ERROR.
"""

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path
from unittest.mock import MagicMock, patch

from torture_tool.cli import RunConfig
from torture_tool.generator.prng import SeedManager
from torture_tool.oracle.evaluator import InvariantViolation, OracleEvaluator, ViolationType
from torture_tool.oracle.journal import JournalEvent, OracleJournal
from torture_tool.protocol.utils import bytes_to_hex
from torture_tool.runner.iteration import IterationResult, IterationRunner, parse_raw_dump
from torture_tool.verifier.dump_parser import DumpParseError, parse_dump_output


class TestOracleInFlightPUTHardening(unittest.TestCase):
    def test_inflight_put_corrupted_value_detected(self):
        """When an in-flight PUT targeting an existing key leaves corrupted/torn bytes on disk,
        the Oracle must strictly reject it with VALUE_CORRUPTED (closing Finding O1).
        """
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="op-base",
                data={
                    "kind": "PUT",
                    "key_hex": bytes_to_hex(b"account:101"),
                    "val_hex": bytes_to_hex(b"balance=500"),
                },
            ),
            JournalEvent(type="ACK", ts_ns=2000, iteration=1, op_id="op-base"),
            JournalEvent(
                type="START",
                ts_ns=3000,
                iteration=1,
                op_id="op-inflight-put",
                data={
                    "kind": "PUT",
                    "key_hex": bytes_to_hex(b"account:101"),
                    "val_hex": bytes_to_hex(b"balance=600"),
                },
            ),
        ]

        # Case 1: Corrupted/torn bytes
        res_corrupt = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"account:101": b"bal\x00\xff_torn"},
        )
        self.assertFalse(res_corrupt.passed)
        self.assertEqual(len(res_corrupt.violations), 1)
        self.assertEqual(res_corrupt.violations[0].violation_type, ViolationType.VALUE_CORRUPTED)
        self.assertEqual(res_corrupt.violations[0].key, b"account:101")

        # Case 2: Exactly V_expected -> Passes
        res_expected = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"account:101": b"balance=600"},
        )
        self.assertTrue(res_expected.passed)
        self.assertEqual(len(res_expected.violations), 0)

        # Case 3: Exactly V_prior -> Passes
        res_prior = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"account:101": b"balance=500"},
        )
        self.assertTrue(res_prior.passed)
        self.assertEqual(len(res_prior.violations), 0)

        # Case 4: Key vanished entirely -> Violates ACK durability (PUT cannot delete acknowledged key)
        res_missing = OracleEvaluator.evaluate(
            events=events,
            actual_state={},
        )
        self.assertFalse(res_missing.passed)
        self.assertEqual(res_missing.violations[0].violation_type, ViolationType.ACKED_WRITE_LOST)

    def test_inflight_put_new_key_torn_bytes_rejected(self):
        """When an in-flight PUT on a previously non-existent key leaves torn bytes,
        it must be rejected with VALUE_CORRUPTED.
        """
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="op-new-key",
                data={
                    "kind": "PUT",
                    "key_hex": bytes_to_hex(b"new_user"),
                    "val_hex": bytes_to_hex(b"full_profile_data_payload"),
                },
            ),
        ]

        # Incomplete / torn write on disk
        res_torn = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"new_user": b"full_profile_"},
        )
        self.assertFalse(res_torn.passed)
        self.assertEqual(res_torn.violations[0].violation_type, ViolationType.VALUE_CORRUPTED)
        self.assertEqual(res_torn.violations[0].key, b"new_user")

        # Complete expected value -> Passes
        res_complete = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"new_user": b"full_profile_data_payload"},
        )
        self.assertTrue(res_complete.passed)

        # Absent from disk -> Passes (write never reached disk before crash)
        res_absent = OracleEvaluator.evaluate(
            events=events,
            actual_state={},
        )
        self.assertTrue(res_absent.passed)


class TestOracleInFlightBatchHardening(unittest.TestCase):
    def test_inflight_batch_all_or_nothing_atomicity(self):
        """An unacknowledged batch must either be 100% applied or 0% applied."""
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="batch-atomic-01",
                data={
                    "kind": "BATCH",
                    "items": [
                        {"k": bytes_to_hex(b"batch_k1"), "v": bytes_to_hex(b"val_1")},
                        {"k": bytes_to_hex(b"batch_k2"), "v": bytes_to_hex(b"val_2")},
                        {"k": bytes_to_hex(b"batch_k3"), "v": bytes_to_hex(b"val_3")},
                    ],
                },
            ),
        ]

        # 100% applied
        self.assertTrue(
            OracleEvaluator.evaluate(
                events=events,
                actual_state={b"batch_k1": b"val_1", b"batch_k2": b"val_2", b"batch_k3": b"val_3"},
            ).passed
        )

        # 0% applied
        self.assertTrue(
            OracleEvaluator.evaluate(
                events=events,
                actual_state={},
            ).passed
        )

        # Partial: 1 of 3
        res_partial_1 = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"batch_k1": b"val_1"},
        )
        self.assertFalse(res_partial_1.passed)
        self.assertTrue(
            any(v.violation_type == ViolationType.BATCH_PARTIAL_APPLICATION for v in res_partial_1.violations)
        )

        # Partial: 2 of 3
        res_partial_2 = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"batch_k1": b"val_1", b"batch_k2": b"val_2"},
        )
        self.assertFalse(res_partial_2.passed)
        self.assertTrue(
            any(v.violation_type == ViolationType.BATCH_PARTIAL_APPLICATION for v in res_partial_2.violations)
        )

    def test_inflight_batch_all_keys_torn_fails_evaluation(self):
        """Vulnerability 2 check: If an in-flight batch leaves torn bytes for multiple keys,
        it must NOT bypass verification (previously applied_keys == 0, absent_keys == total).
        """
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="batch-torn-all",
                data={
                    "kind": "BATCH",
                    "items": [
                        {"k": bytes_to_hex(b"k_a"), "v": bytes_to_hex(b"valid_value_a")},
                        {"k": bytes_to_hex(b"k_b"), "v": bytes_to_hex(b"valid_value_b")},
                    ],
                },
            ),
        ]

        # Both keys have torn bytes
        res = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"k_a": b"torn_garbage_a", b"k_b": b"torn_garbage_b"},
        )
        self.assertFalse(res.passed)
        self.assertTrue(
            any(v.violation_type == ViolationType.VALUE_CORRUPTED for v in res.violations)
        )

    def test_inflight_batch_mixed_applied_and_torn_fails(self):
        """If one batch mutation applied cleanly while another was torn, both
        VALUE_CORRUPTED and BATCH_PARTIAL_APPLICATION must be flagged.
        """
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="batch-mixed",
                data={
                    "kind": "BATCH",
                    "items": [
                        {"k": bytes_to_hex(b"k_a"), "v": bytes_to_hex(b"valid_value_a")},
                        {"k": bytes_to_hex(b"k_b"), "v": bytes_to_hex(b"valid_value_b")},
                    ],
                },
            ),
        ]

        res = OracleEvaluator.evaluate(
            events=events,
            actual_state={b"k_a": b"valid_value_a", b"k_b": b"torn_garbage_b"},
        )
        self.assertFalse(res.passed)
        types = {v.violation_type for v in res.violations}
        self.assertIn(ViolationType.VALUE_CORRUPTED, types)
        self.assertIn(ViolationType.BATCH_PARTIAL_APPLICATION, types)


class TestOracleSequenceNumberReplay(unittest.TestCase):
    def test_ack_replayed_in_monotonic_seq_num_order(self):
        """Finding O2: Evaluator must order ACK events by protocol seq_num
        prior to state machine replay to eliminate out-of-order demux anomalies.
        """
        events = [
            JournalEvent(
                type="START",
                ts_ns=100,
                iteration=1,
                op_id="write-seq-1",
                data={"kind": "PUT", "key_hex": bytes_to_hex(b"shared_key"), "val_hex": bytes_to_hex(b"old_v1")},
            ),
            JournalEvent(
                type="START",
                ts_ns=200,
                iteration=1,
                op_id="write-seq-2",
                data={"kind": "PUT", "key_hex": bytes_to_hex(b"shared_key"), "val_hex": bytes_to_hex(b"mid_v2")},
            ),
            JournalEvent(
                type="START",
                ts_ns=300,
                iteration=1,
                op_id="write-seq-3",
                data={"kind": "PUT", "key_hex": bytes_to_hex(b"shared_key"), "val_hex": bytes_to_hex(b"new_v3")},
            ),
            # Demux logged ACKs in scrambled order: seq 30, then seq 10, then seq 20
            JournalEvent(type="ACK", ts_ns=400, iteration=1, op_id="write-seq-3", data={"seq_num": 30}),
            JournalEvent(type="ACK", ts_ns=450, iteration=1, op_id="write-seq-1", data={"seq_num": 10}),
            JournalEvent(type="ACK", ts_ns=500, iteration=1, op_id="write-seq-2", data={"seq_num": 20}),
        ]

        # The authoritative monotonic order is seq 10 -> 20 -> 30, so final value is 'new_v3'.
        res_v3 = OracleEvaluator.evaluate(events=events, actual_state={b"shared_key": b"new_v3"})
        self.assertTrue(res_v3.passed)

        # Any other value from older sequence numbers violates durability and monotonic ordering.
        res_v2 = OracleEvaluator.evaluate(events=events, actual_state={b"shared_key": b"mid_v2"})
        self.assertFalse(res_v2.passed)

        res_v1 = OracleEvaluator.evaluate(events=events, actual_state={b"shared_key": b"old_v1"})
        self.assertFalse(res_v1.passed)


class TestStrictDumpParserIntegration(unittest.TestCase):
    def test_parse_raw_dump_rejects_duplicate_keys(self):
        """Finding R2: parse_raw_dump must raise DumpParseError on duplicate keys."""
        dump_with_duplicates = "6161 76616c31\n6161 76616c32\n"  # 'aa' 'val1', 'aa' 'val2'
        with self.assertRaises(DumpParseError) as ctx:
            parse_raw_dump(dump_with_duplicates)
        self.assertIn("duplicate key detected in dump", str(ctx.exception))

    def test_parse_dump_output_normal(self):
        dump_valid = "6b31 7631\n6b32 7632\n"
        result = parse_raw_dump(dump_valid)
        self.assertEqual(result, {b"k1": b"v1", b"k2": b"v2"})

    @patch("subprocess.run")
    def test_iteration_runner_fails_on_dump_duplicate_key(self, mock_run):
        """IterationRunner must fail the iteration with DUMP_PARSE_ERROR when
        the engine dump output contains duplicate keys.
        """
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp_path = Path(tmpdir)
            db_file = tmp_path / "test.db"
            journal_path = tmp_path / "journal.jsonl"

            # Create dummy binary and journal
            db_file.touch()
            journal = OracleJournal(journal_path)
            journal.log_iteration_start(1)
            journal.close()

            config = RunConfig(
                seed=42,
                iterations=1,
                mode="default",
                workers=2,
                timeout=5.0,
                db_path=db_file,
                artifact_dir=tmp_path / "artifacts",
                kvdb_bin=tmp_path / "kvdb",
            )
            config.kvdb_bin.touch()

            # Mock subprocess.run for check and dump:
            def fake_subprocess_run(cmd, *args, **kwargs):
                cmd_str = [str(c) for c in cmd]
                mock_proc = MagicMock()
                mock_proc.returncode = 0
                if "check" in cmd_str:
                    mock_proc.stdout = "OK\n"
                elif "dump" in cmd_str:
                    # Return dump with duplicate keys!
                    mock_proc.stdout = "61 01\n61 02\n"
                return mock_proc

            mock_run.side_effect = fake_subprocess_run

            runner = IterationRunner(
                config=config,
                seed_manager=SeedManager(42),
                journal_path=journal_path,
            )

            # Mock ChildProcessController, WorkerPool, CrashScheduler
            with patch("torture_tool.runner.iteration.ChildProcessController") as mock_ctrl, \
                 patch("torture_tool.runner.iteration.ResponseDispatcher") as mock_disp, \
                 patch("torture_tool.runner.iteration.StdinWriter"), \
                 patch("torture_tool.runner.iteration.WorkerPool") as mock_pool, \
                 patch("torture_tool.runner.iteration.CrashScheduler") as mock_sched:

                mock_ctrl_instance = MagicMock()
                mock_ctrl_instance.process.stdout = MagicMock()
                mock_ctrl_instance.process.stdin = MagicMock()
                mock_ctrl.return_value = mock_ctrl_instance

                mock_disp_instance = MagicMock()
                mock_disp_instance.protocol_errors = []
                mock_disp_instance.unknown_responses = []
                mock_disp_instance.get_pending_op_ids.return_value = []
                mock_disp.return_value = mock_disp_instance

                mock_pool_instance = MagicMock()
                mock_pool_instance.stop_and_join.return_value = []
                mock_pool.return_value = mock_pool_instance

                mock_sched_instance = MagicMock()
                crash_info = MagicMock()
                crash_info.delay_ms = 10.0
                crash_info.in_flight_count = 0
                mock_sched_instance.wait_and_kill.return_value = crash_info
                mock_sched.return_value = mock_sched_instance

                result = runner.run_iteration(
                    iteration=1,
                    db_file=db_file,
                    reuse_database=True,
                )

                self.assertFalse(result.passed)
                self.assertIsNotNone(result.error_message)
                self.assertIn("DUMP_PARSE_ERROR", result.error_message)
                self.assertIn("duplicate key detected in dump", result.error_message)


if __name__ == "__main__":
    unittest.main()

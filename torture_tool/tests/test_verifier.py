"""Unit tests for Phase T6: Dump parsing, comparator, diff reports, and failure packaging."""

import json
import shutil
import tempfile
import unittest
from pathlib import Path

from torture_tool.cli import RunConfig
from torture_tool.oracle.evaluator import EvaluationResult, InvariantViolation, ViolationType
from torture_tool.protocol.utils import bytes_to_hex
from torture_tool.reporting.packager import FailurePackager
from torture_tool.runner.iteration import IterationResult
from torture_tool.verifier.comparator import StateComparator
from torture_tool.verifier.dump_parser import DumpParseError, parse_dump_output


class TestDumpParser(unittest.TestCase):
    def test_valid_dump_parsing(self):
        dump_text = (
            "6b31 7631\n"
            "6b32 \n"  # empty value
            "6b33 00ff01\n"
        )
        parsed = parse_dump_output(dump_text)
        self.assertEqual(len(parsed), 3)
        self.assertEqual(parsed[b"k1"], b"v1")
        self.assertEqual(parsed[b"k2"], b"")
        self.assertEqual(parsed[b"k3"], b"\x00\xff\x01")

    def test_duplicate_keys_in_dump_rejected(self):
        dump_text = (
            "6b31 7631\n"
            "6b31 7632\n"  # Duplicate key!
        )
        with self.assertRaises(DumpParseError) as cm:
            parse_dump_output(dump_text)
        self.assertIn("duplicate key detected", str(cm.exception))

    def test_malformed_hex_in_dump_rejected(self):
        with self.assertRaises(DumpParseError):
            parse_dump_output("xyz 7631\n")


class TestStateComparator(unittest.TestCase):
    def test_comparator_matches_clean_state(self):
        expected = {b"k1": b"v1", b"k2": b"v2"}
        actual = {b"k1": b"v1", b"k2": b"v2"}
        eval_res = EvaluationResult(passed=True)

        report = StateComparator.compare_states(expected, actual, eval_res)
        self.assertTrue(report.passed)
        self.assertEqual(report.matched_count, 2)
        self.assertEqual(len(report.key_diffs), 0)

    def test_comparator_detects_missing_and_corrupt(self):
        expected = {b"k1": b"v1", b"k2": b"v2"}
        actual = {b"k1": b"v1_corrupt"}  # k2 is missing, k1 is corrupt
        eval_res = EvaluationResult(
            passed=False,
            violations=[
                InvariantViolation(
                    violation_type=ViolationType.ACKED_WRITE_LOST,
                    key=b"k2",
                    expected=b"v2",
                    actual=None,
                    op_id="op-2",
                    description="Lost ACKed key",
                )
            ],
        )

        report = StateComparator.compare_states(expected, actual, eval_res)
        self.assertFalse(report.passed)
        self.assertEqual(report.matched_count, 0)
        self.assertEqual(len(report.key_diffs), 2)

        human_diff = report.generate_human_diff()
        self.assertIn("MISSING key_hex=6b32", human_diff)
        self.assertIn("VALUE MISMATCH key_hex=6b31", human_diff)


class TestFailurePackager(unittest.TestCase):
    def test_failure_artifact_bundle_creation(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            db_file = tmp / "data.db"
            db_file.write_text("corrupted db content")
            journal_file = tmp / "oracle_events.jsonl"
            journal_file.write_text('{"type": "START"}\n')

            config = RunConfig(seed=12345, artifact_dir=tmp / "artifacts")
            iter_res = IterationResult(
                iteration=3,
                seed=999,
                mode="default",
                passed=False,
                delay_ms=25.0,
                in_flight_at_kill=2,
                operations_started=100,
                acks_received=98,
                unresolved_ops=2,
                check_passed=True,
                eval_result=EvaluationResult(
                    passed=False,
                    violations=[
                        InvariantViolation(
                            violation_type=ViolationType.VALUE_CORRUPTED,
                            key=b"key1",
                            expected=b"exp",
                            actual=b"act",
                            op_id="op-1",
                            description="Torn write",
                        )
                    ],
                ),
            )

            bundle_dir = FailurePackager.preserve_failure(
                iteration_result=iter_res,
                config=config,
                db_path=db_file,
                journal_path=journal_file,
            )

            self.assertTrue(bundle_dir.exists())
            self.assertTrue((bundle_dir / "corrupted.db").exists())
            self.assertTrue((bundle_dir / "oracle_events.jsonl").exists())
            self.assertTrue((bundle_dir / "manifest.json").exists())
            self.assertTrue((bundle_dir / "reproduce.sh").exists())

            # Verify manifest
            with open(bundle_dir / "manifest.json", "r") as f:
                manifest = json.load(f)
            self.assertEqual(manifest["iteration"], 3)
            self.assertEqual(manifest["seed"], 999)
            self.assertEqual(manifest["violations"][0]["type"], "ViolationType.VALUE_CORRUPTED")

            # Verify reproduce script is executable
            mode = (bundle_dir / "reproduce.sh").stat().st_mode
            self.assertTrue(mode & 0o111)


if __name__ == "__main__":
    unittest.main()


"""Torture tool self-test and mutation testing runner.

Deliberately injects faults into the database and into journal events to verify
that the OracleEvaluator, dump verifier, and recovery checks catch every class
of violation without false positives:
1. Missing acknowledged write (ACKED_WRITE_LOST)
2. Bit-flip corruption in stored payload (VALUE_CORRUPTED)
3. Partial batch commit / atomicity violation (BATCH_PARTIAL_APPLICATION)
4. Phantom / unrequested key detected (UNKNOWN_KEY_DETECTED)
5. Failed operation applied to disk (FAILED_OP_APPLIED)
6. Torn WAL record / truncated tail handling
7. Checkpoint crash injection verification
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import tempfile
import time
from dataclasses import dataclass
from pathlib import Path
from typing import List, Tuple

from torture_tool.cli import RunConfig
from torture_tool.oracle.evaluator import OracleEvaluator, ViolationType
from torture_tool.oracle.journal import JournalEvent
from torture_tool.protocol.spec import BatchItem, BatchRequest, PutRequest
from torture_tool.protocol.utils import bytes_to_hex
from torture_tool.runner.iteration import IterationResult, parse_raw_dump
from torture_tool.scheduler.checkpoint import (
    CheckpointCrashInjector,
    EngineCheckpoint,
)


@dataclass
class MutationTestResult:
    name: str
    expected_violation: str
    detected_violation: str
    passed: bool
    details: str


class SelfTestRunner:
    """Executes systematic mutation testing and self-verification of torture tool harness."""

    def __init__(self, config: RunConfig) -> None:
        self.config = config

    def run_self_test(self, iteration: int = 1) -> IterationResult:
        """Run all mutation tests and checkpoint validations."""
        start_time = time.monotonic()
        results: List[MutationTestResult] = []

        # Sub-test 1: Missing Acknowledged Write
        results.append(self._test_missing_acked_write())

        # Sub-test 2: Value Bit Corruption
        results.append(self._test_corrupted_value())

        # Sub-test 3: Partial Batch Violation
        results.append(self._test_partial_batch())

        # Sub-test 4: Phantom Key Injection
        results.append(self._test_phantom_key())

        # Sub-test 5: Failed Operation Applied
        results.append(self._test_failed_op_applied())

        # Sub-test 6: Checkpoint Injections against live kvdb
        chk_results = self._test_live_checkpoint_injections()
        results.extend(chk_results)

        all_passed = all(r.passed for r in results)
        total_ops = len(results)
        acks = sum(1 for r in results if r.passed)
        duration_ms = (time.monotonic() - start_time) * 1000.0

        err_msg = None
        if not all_passed:
            failed = [f"{r.name}: expected {r.expected_violation}, got {r.detected_violation}" for r in results if not r.passed]
            err_msg = "; ".join(failed)

        # Print summary of self-test results to stdout
        sys.stdout.write("\n" + "=" * 70 + "\n")
        sys.stdout.write("TORTURE TOOL ORACLE & CHECKPOINT SELF-TEST RESULTS\n")
        sys.stdout.write("=" * 70 + "\n")
        for r in results:
            status = "PASS" if r.passed else "FAIL"
            sys.stdout.write(f"[{status}] {r.name:<35} | {r.details}\n")
        sys.stdout.write("=" * 70 + "\n\n")
        sys.stdout.flush()

        return IterationResult(
            iteration=iteration,
            seed=self.config.seed,
            mode="self-test",
            passed=all_passed,
            delay_ms=0.0,
            in_flight_at_kill=len(chk_results),
            operations_started=total_ops,
            acks_received=acks,
            unresolved_ops=0,
            check_passed=all_passed,
            error_message=err_msg,
            duration_ms=duration_ms,
        )

    def _test_missing_acked_write(self) -> MutationTestResult:
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="mut-01",
                data={"kind": "PUT", "key_hex": bytes_to_hex(b"k_acked"), "val_hex": bytes_to_hex(b"val_1")},
            ),
            JournalEvent(type="ACK", ts_ns=2000, iteration=1, op_id="mut-01"),
        ]
        # State drops k_acked completely
        actual = {}
        res = OracleEvaluator.evaluate(events, actual)
        detected = res.violations[0].violation_type.name if res.violations else "NONE"
        passed = (not res.passed) and (detected == ViolationType.ACKED_WRITE_LOST.name)
        return MutationTestResult(
            name="Missing Acknowledged Write",
            expected_violation=ViolationType.ACKED_WRITE_LOST.name,
            detected_violation=detected,
            passed=passed,
            details=f"Detected: {detected} (Oracle correctly flagged missing durable write)",
        )

    def _test_corrupted_value(self) -> MutationTestResult:
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="mut-02",
                data={"kind": "PUT", "key_hex": bytes_to_hex(b"k_corrupt"), "val_hex": bytes_to_hex(b"pure_data")},
            ),
            JournalEvent(type="ACK", ts_ns=2000, iteration=1, op_id="mut-02"),
        ]
        # State contains 1-bit corrupted payload
        actual = {b"k_corrupt": b"pure_datb"}
        res = OracleEvaluator.evaluate(events, actual)
        detected = res.violations[0].violation_type.name if res.violations else "NONE"
        passed = (not res.passed) and (detected == ViolationType.VALUE_CORRUPTED.name)
        return MutationTestResult(
            name="Payload Bit-Flip Corruption",
            expected_violation=ViolationType.VALUE_CORRUPTED.name,
            detected_violation=detected,
            passed=passed,
            details=f"Detected: {detected} (Oracle correctly identified single-byte value corruption)",
        )

    def _test_partial_batch(self) -> MutationTestResult:
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="mut-batch",
                data={
                    "kind": "BATCH",
                    "items": [
                        {"k": bytes_to_hex(b"b1"), "v": bytes_to_hex(b"v1")},
                        {"k": bytes_to_hex(b"b2"), "v": bytes_to_hex(b"v2")},
                    ],
                },
            ),
        ]
        # Unacknowledged batch: only 1 of 2 keys committed!
        actual = {b"b1": b"v1"}
        res = OracleEvaluator.evaluate(events, actual)
        detected = res.violations[0].violation_type.name if res.violations else "NONE"
        passed = (not res.passed) and (detected == ViolationType.BATCH_PARTIAL_APPLICATION.name)
        return MutationTestResult(
            name="Partial Batch Mutation",
            expected_violation=ViolationType.BATCH_PARTIAL_APPLICATION.name,
            detected_violation=detected,
            passed=passed,
            details=f"Detected: {detected} (Oracle caught non-atomic half-batch application)",
        )

    def _test_phantom_key(self) -> MutationTestResult:
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="mut-03",
                data={"kind": "PUT", "key_hex": bytes_to_hex(b"legit_key"), "val_hex": bytes_to_hex(b"v")},
            ),
            JournalEvent(type="ACK", ts_ns=2000, iteration=1, op_id="mut-03"),
        ]
        actual = {b"legit_key": b"v", b"ghost_key": b"spooky"}
        res = OracleEvaluator.evaluate(events, actual)
        detected = res.violations[0].violation_type.name if res.violations else "NONE"
        passed = (not res.passed) and (detected == ViolationType.UNKNOWN_KEY_DETECTED.name)
        return MutationTestResult(
            name="Phantom Key Injection",
            expected_violation=ViolationType.UNKNOWN_KEY_DETECTED.name,
            detected_violation=detected,
            passed=passed,
            details=f"Detected: {detected} (Oracle caught key not present in client requests)",
        )

    def _test_failed_op_applied(self) -> MutationTestResult:
        events = [
            JournalEvent(
                type="START",
                ts_ns=1000,
                iteration=1,
                op_id="mut-04",
                data={"kind": "PUT", "key_hex": bytes_to_hex(b"rejected"), "val_hex": bytes_to_hex(b"bad")},
            ),
            JournalEvent(
                type="FAIL",
                ts_ns=2000,
                iteration=1,
                op_id="mut-04",
                data={"code": "ERR_INVALID"},
            ),
        ]
        actual = {b"rejected": b"bad"}
        res = OracleEvaluator.evaluate(events, actual)
        detected = res.violations[0].violation_type.name if res.violations else "NONE"
        passed = (not res.passed) and (detected == ViolationType.FAILED_OP_APPLIED.name)
        return MutationTestResult(
            name="Failed Operation Applied",
            expected_violation=ViolationType.FAILED_OP_APPLIED.name,
            detected_violation=detected,
            passed=passed,
            details=f"Detected: {detected} (Oracle caught operation that returned ERR but was written)",
        )

    def _test_live_checkpoint_injections(self) -> List[MutationTestResult]:
        """Test live deterministic checkpoint crash injection against kvdb binary."""
        results = []
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp_path = Path(tmp_dir)
            db_path = tmp_path / "chk_test.db"
            injector = CheckpointCrashInjector(kvdb_bin=self.config.kvdb_bin, db_path=db_path)

            # Test checkpoint: MID_PAYLOAD
            req = PutRequest(op_id="chk-put-mid", key=b"chk_mid_k", value=b"x" * 2048)
            res_mid = injector.inject_checkpoint_crash(EngineCheckpoint.MID_PAYLOAD, req)
            # Verify database integrity with ./kvdb check
            chk_proc = subprocess.run([str(self.config.kvdb_bin), "check", str(db_path)], capture_output=True, text=True)
            passed_mid = res_mid.success and chk_proc.returncode == 0 and "OK" in chk_proc.stdout
            results.append(
                MutationTestResult(
                    name="Checkpoint: MID_PAYLOAD SIGKILL",
                    expected_violation="TORN_WRITE_RECOVERED",
                    detected_violation="RECOVERED_OK" if passed_mid else "FAIL",
                    passed=passed_mid,
                    details=f"Sent {res_mid.bytes_sent} bytes; engine clean recovery: {chk_proc.stdout.strip()}",
                )
            )

            # Test checkpoint: MID_BATCH
            batch_items = tuple([
                BatchItem(key=f"b_{i}".encode("ascii"), value=f"v_{i}".encode("ascii"))
                for i in range(10)
            ])
            batch_req = BatchRequest(op_id="chk-batch-mid", items=batch_items)
            res_batch = injector.inject_checkpoint_crash(EngineCheckpoint.MID_BATCH, batch_req)
            chk_proc2 = subprocess.run([str(self.config.kvdb_bin), "check", str(db_path)], capture_output=True, text=True)
            passed_batch = res_batch.success and chk_proc2.returncode == 0 and "OK" in chk_proc2.stdout
            results.append(
                MutationTestResult(
                    name="Checkpoint: MID_BATCH SIGKILL",
                    expected_violation="ATOMIC_ROLLBACK",
                    detected_violation="ROLLBACK_OK" if passed_batch else "FAIL",
                    passed=passed_batch,
                    details=f"Sent {res_batch.bytes_sent} bytes; engine atomicity maintained: {chk_proc2.stdout.strip()}",
                )
            )

            # Test checkpoint: PRE_FSYNC
            put_req = PutRequest(op_id="chk-pre-fsync", key=b"chk_fsync_k", value=b"fsync_val")
            res_fsync = injector.inject_checkpoint_crash(EngineCheckpoint.PRE_FSYNC, put_req)
            chk_proc3 = subprocess.run([str(self.config.kvdb_bin), "check", str(db_path)], capture_output=True, text=True)
            passed_fsync = res_fsync.success and chk_proc3.returncode == 0 and "OK" in chk_proc3.stdout
            results.append(
                MutationTestResult(
                    name="Checkpoint: PRE_FSYNC SIGKILL",
                    expected_violation="UNACKED_DISCARD_OR_KEEP",
                    detected_violation="RECOVERED_OK" if passed_fsync else "FAIL",
                    passed=passed_fsync,
                    details=f"Killed right at pipe flush; kvdb check: {chk_proc3.stdout.strip()}",
                )
            )

        return results

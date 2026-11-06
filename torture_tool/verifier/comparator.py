"""Deep state comparator and differential analysis between oracle and actual database."""

from __future__ import annotations

import hashlib
from dataclasses import dataclass, field
from typing import Dict, List, Optional

from torture_tool.oracle.evaluator import EvaluationResult, InvariantViolation
from torture_tool.oracle.journal import JournalEvent
from torture_tool.protocol.utils import bytes_to_hex


@dataclass
class KeyDiff:
    key: bytes
    status: str  # "MATCH", "MISSING", "EXTRA", "CORRUPTED"
    expected_val: Optional[bytes] = None
    actual_val: Optional[bytes] = None
    expected_hash: Optional[str] = None
    actual_hash: Optional[str] = None


@dataclass
class DiffReport:
    passed: bool
    total_expected: int
    total_actual: int
    matched_count: int
    violations: List[InvariantViolation] = field(default_factory=list)
    key_diffs: List[KeyDiff] = field(default_factory=list)

    def generate_human_diff(self, max_diff_lines: int = 50) -> str:
        lines: List[str] = []
        lines.append("=" * 80)
        lines.append(f"DATABASE VERIFICATION REPORT: {'PASSED' if self.passed else 'FAILED'}")
        lines.append(
            f"Expected Live Keys: {self.total_expected} | Actual Keys in Dump: {self.total_actual} | Matched: {self.matched_count}"
        )
        lines.append("=" * 80)

        if self.violations:
            lines.append("\nCRITICAL INVARIANT VIOLATIONS:")
            for idx, v in enumerate(self.violations, 1):
                lines.append(f"  {idx}. {v.format_diagnostic()}")

        if self.key_diffs:
            lines.append("\nKEY-LEVEL DISCREPANCIES (First few):")
            for d in self.key_diffs[:max_diff_lines]:
                k_hex = bytes_to_hex(d.key)
                if d.status == "MISSING":
                    lines.append(f"  [-] MISSING key_hex={k_hex}")
                    lines.append(f"      Expected: len={len(d.expected_val or b'')} sha256={d.expected_hash}")
                elif d.status == "EXTRA":
                    lines.append(f"  [+] EXTRA/PHANTOM key_hex={k_hex}")
                    lines.append(f"      Actual:   len={len(d.actual_val or b'')} sha256={d.actual_hash}")
                elif d.status == "CORRUPTED":
                    lines.append(f"  [!] VALUE MISMATCH key_hex={k_hex}")
                    lines.append(f"      Expected: len={len(d.expected_val or b'')} sha256={d.expected_hash}")
                    lines.append(f"      Actual:   len={len(d.actual_val or b'')} sha256={d.actual_hash}")

        lines.append("=" * 80)
        return "\n".join(lines)


class StateComparator:
    """Performs deep cryptographic and byte-level diffing between oracle model and dump."""

    @classmethod
    def compare_states(
        cls,
        expected_live: Dict[bytes, bytes],
        actual_state: Dict[bytes, bytes],
        eval_result: EvaluationResult,
    ) -> DiffReport:
        key_diffs: List[KeyDiff] = []
        matched_count = 0

        # Check all expected keys
        for k, exp_v in expected_live.items():
            exp_hash = hashlib.sha256(exp_v).hexdigest()
            if k not in actual_state:
                key_diffs.append(
                    KeyDiff(
                        key=k,
                        status="MISSING",
                        expected_val=exp_v,
                        expected_hash=exp_hash,
                    )
                )
            else:
                act_v = actual_state[k]
                act_hash = hashlib.sha256(act_v).hexdigest()
                if exp_v != act_v:
                    key_diffs.append(
                        KeyDiff(
                            key=k,
                            status="CORRUPTED",
                            expected_val=exp_v,
                            actual_val=act_v,
                            expected_hash=exp_hash,
                            actual_hash=act_hash,
                        )
                    )
                else:
                    matched_count += 1

        # Check for extra keys in actual state
        for k, act_v in actual_state.items():
            if k not in expected_live:
                act_hash = hashlib.sha256(act_v).hexdigest()
                key_diffs.append(
                    KeyDiff(
                        key=k,
                        status="EXTRA",
                        actual_val=act_v,
                        actual_hash=act_hash,
                    )
                )

        passed = eval_result.passed and len(eval_result.violations) == 0

        return DiffReport(
            passed=passed,
            total_expected=len(expected_live),
            total_actual=len(actual_state),
            matched_count=matched_count,
            violations=eval_result.violations,
            key_diffs=key_diffs,
        )


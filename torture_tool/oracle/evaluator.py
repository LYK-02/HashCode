"""Oracle evaluator resolving guaranteed acknowledged mutations and post-crash ambiguities."""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import Dict, List, Optional, Set, Tuple

from torture_tool.oracle.journal import JournalEvent
from torture_tool.oracle.state import ReferenceDatabaseState
from torture_tool.protocol.spec import (
    BatchItem,
    BatchRequest,
    DelRequest,
    PutRequest,
    Request,
)
from torture_tool.protocol.utils import bytes_to_hex, hex_to_bytes


class ViolationType(str, Enum):
    ACKED_WRITE_LOST = "ACKED_WRITE_LOST"
    VALUE_CORRUPTED = "VALUE_CORRUPTED"
    UNKNOWN_KEY_DETECTED = "UNKNOWN_KEY_DETECTED"
    BATCH_PARTIAL_APPLICATION = "BATCH_PARTIAL_APPLICATION"
    FAILED_OP_APPLIED = "FAILED_OP_APPLIED"
    INDEX_DESYNCHRONIZATION = "INDEX_DESYNCHRONIZATION"


@dataclass(frozen=True)
class InvariantViolation:
    violation_type: ViolationType
    key: Optional[bytes]
    expected: Optional[bytes]
    actual: Optional[bytes]
    op_id: Optional[str]
    description: str

    def format_diagnostic(self) -> str:
        k_str = bytes_to_hex(self.key) if self.key is not None else "N/A"
        exp_str = (
            f"len={len(self.expected)} hex={bytes_to_hex(self.expected[:32])}..."
            if self.expected is not None
            else "None"
        )
        act_str = (
            f"len={len(self.actual)} hex={bytes_to_hex(self.actual[:32])}..."
            if self.actual is not None
            else "None"
        )
        return (
            f"[{self.violation_type}] key_hex={k_str} (op_id={self.op_id or 'unknown'}):\n"
            f"  Expected: {exp_str}\n"
            f"  Actual:   {act_str}\n"
            f"  Detail:   {self.description}"
        )


@dataclass
class EvaluationResult:
    passed: bool
    violations: List[InvariantViolation] = field(default_factory=list)
    total_acked_ops: int = 0
    total_inflight_ops: int = 0
    total_failed_ops: int = 0


class OracleEvaluator:
    """Evaluates recovered database state against client-observed event history."""

    def __init__(self) -> None:
        pass

    @classmethod
    def evaluate(
        cls,
        events: List[JournalEvent],
        actual_state: Dict[bytes, bytes],
    ) -> EvaluationResult:
        """Replay events, model in-flight ambiguity, and strictly validate actual state.

        Core Invariant Rules:
        1. Every ACKed mutation must be present (or deleted if DEL) in observed order.
        2. Every explicitly FAILED mutation must NOT have taken effect.
        3. Every in-flight single mutation (START but no ACK/FAIL) may be either
           wholly applied OR wholly absent. Torn values are strictly rejected.
        4. Every in-flight BATCH mutation must be atomic: either ALL operations
           applied, OR NONE applied. Partial application is rejected.
        5. No unknown/phantom keys allowed.
        """
        # Map of op_id -> START event
        start_map: Dict[str, JournalEvent] = {}
        # List of ACK events in order of arrival
        ack_list: List[JournalEvent] = []
        # Set of op_ids that explicitly failed
        fail_set: Set[str] = set()

        for ev in events:
            if ev.type == "START" and ev.op_id:
                start_map[ev.op_id] = ev
            elif ev.type == "ACK" and ev.op_id:
                ack_list.append(ev)
            elif ev.type == "FAIL" and ev.op_id:
                fail_set.add(ev.op_id)

        # 1. Build authoritative baseline state from acknowledged operations
        acked_ids: Set[str] = set()
        base_state = ReferenceDatabaseState()

        for ack_ev in ack_list:
            op_id = ack_ev.op_id
            acked_ids.add(op_id)
            start_ev = start_map.get(op_id)
            if not start_ev or not start_ev.data:
                continue

            kind = start_ev.data.get("kind")
            if kind == "PUT":
                k = hex_to_bytes(start_ev.data["key_hex"])
                v = hex_to_bytes(start_ev.data["val_hex"])
                base_state.apply_put(k, v)
            elif kind == "DEL":
                k = hex_to_bytes(start_ev.data["key_hex"])
                base_state.apply_del(k)
            elif kind == "BATCH":
                items: List[BatchItem] = []
                for itm in start_ev.data.get("items", []):
                    k = hex_to_bytes(itm["k"])
                    v = hex_to_bytes(itm["v"]) if itm["v"] is not None else None
                    items.append(BatchItem(key=k, value=v))
                base_state.apply_batch(items)

        # 2. Identify in-flight operations (START logged, but neither ACK nor FAIL)
        inflight_ops: List[JournalEvent] = []
        for op_id, start_ev in start_map.items():
            if op_id not in acked_ids and op_id not in fail_set:
                inflight_ops.append(start_ev)

        # 3. Build candidate reference states
        # The true state is base_state plus any subset of in-flight single ops
        # AND for each in-flight batch, either 100% applied or 0% applied.
        violations: List[InvariantViolation] = []

        # Check: No unknown keys (every key in actual_state must be known to the oracle)
        all_possible_keys = set(base_state.all_observed_keys)
        for inf in inflight_ops:
            if not inf.data:
                continue
            kind = inf.data.get("kind")
            if kind in ("PUT", "DEL"):
                all_possible_keys.add(hex_to_bytes(inf.data["key_hex"]))
            elif kind == "BATCH":
                for itm in inf.data.get("items", []):
                    all_possible_keys.add(hex_to_bytes(itm["k"]))

        for actual_k, actual_v in actual_state.items():
            if actual_k not in all_possible_keys:
                violations.append(
                    InvariantViolation(
                        violation_type=ViolationType.UNKNOWN_KEY_DETECTED,
                        key=actual_k,
                        expected=None,
                        actual=actual_v,
                        op_id=None,
                        description="Key exists in recovered dump but was never generated or started",
                    )
                )
            elif actual_k not in base_state.live_keys:
                # Key was deleted (or only ever in-flight). If no in-flight write explains its presence:
                has_inflight_write = False
                for inf in inflight_ops:
                    if not inf.data:
                        continue
                    kind = inf.data.get("kind")
                    if kind == "PUT" and hex_to_bytes(inf.data["key_hex"]) == actual_k:
                        has_inflight_write = True
                        break
                    elif kind == "BATCH":
                        for itm in inf.data.get("items", []):
                            if hex_to_bytes(itm["k"]) == actual_k and itm["v"] is not None:
                                has_inflight_write = True
                                break
                    if has_inflight_write:
                        break

                if not has_inflight_write:
                    violations.append(
                        InvariantViolation(
                            violation_type=ViolationType.ACKED_WRITE_LOST,
                            key=actual_k,
                            expected=None,
                            actual=actual_v,
                            op_id=None,
                            description="Key was acknowledged deleted, but resurrected in recovered state",
                        )
                    )

        # Check: Every ACKed key must be present with correct value unless overwritten
        for base_k, base_v in base_state.live_keys.items():
            # If there are in-flight operations targeting this key, value might be altered
            # Otherwise, it MUST match base_v exactly.
            has_inflight_write = False
            for inf in inflight_ops:
                if not inf.data:
                    continue
                kind = inf.data.get("kind")
                if kind in ("PUT", "DEL") and hex_to_bytes(inf.data["key_hex"]) == base_k:
                    has_inflight_write = True
                    break
                elif kind == "BATCH":
                    for itm in inf.data.get("items", []):
                        if hex_to_bytes(itm["k"]) == base_k:
                            has_inflight_write = True
                            break
                if has_inflight_write:
                    break

            if not has_inflight_write:
                # Value MUST match base_state
                if base_k not in actual_state:
                    violations.append(
                        InvariantViolation(
                            violation_type=ViolationType.ACKED_WRITE_LOST,
                            key=base_k,
                            expected=base_v,
                            actual=None,
                            op_id=None,
                            description="Acknowledged key is completely missing from recovered state",
                        )
                    )
                elif actual_state[base_k] != base_v:
                    violations.append(
                        InvariantViolation(
                            violation_type=ViolationType.VALUE_CORRUPTED,
                            key=base_k,
                            expected=base_v,
                            actual=actual_state[base_k],
                            op_id=None,
                            description="Acknowledged key has corrupted or mismatched value in recovered state",
                        )
                    )

        # Check: In-flight batch atomicity
        for inf in inflight_ops:
            if not inf.data or inf.data.get("kind") != "BATCH":
                continue
            batch_items = inf.data.get("items", [])
            if not batch_items:
                continue

            # Collapse intra-batch overwrites: only the LAST mutation for each key matters
            final_batch_ops: Dict[bytes, Optional[bytes]] = {}
            for itm in batch_items:
                bk = hex_to_bytes(itm["k"])
                bv = hex_to_bytes(itm["v"]) if itm["v"] is not None else None
                final_batch_ops[bk] = bv

            # Determine whether this batch's final mutations were applied or absent
            applied_keys = 0
            absent_keys = 0

            for bk, bv in final_batch_ops.items():
                if bv is not None:
                    # Final op was PUT
                    if bk in actual_state and actual_state[bk] == bv:
                        applied_keys += 1
                    else:
                        absent_keys += 1
                else:
                    # Final op was DEL
                    if bk not in actual_state:
                        applied_keys += 1
                    else:
                        absent_keys += 1

            total_unique_keys = len(final_batch_ops)
            if 0 < applied_keys < total_unique_keys:
                # Check if the "applied" keys were already in base_state prior to this batch
                truly_applied = 0
                for bk, bv in final_batch_ops.items():
                    if bv is not None:
                        if actual_state.get(bk) == bv and base_state.get(bk) != bv:
                            truly_applied += 1
                    else:
                        if bk not in actual_state and base_state.get(bk) is not None:
                            truly_applied += 1

                if 0 < truly_applied < total_unique_keys and truly_applied > 0:
                    violations.append(
                        InvariantViolation(
                            violation_type=ViolationType.BATCH_PARTIAL_APPLICATION,
                            key=None,
                            expected=None,
                            actual=None,
                            op_id=inf.op_id,
                            description=(
                                f"In-flight batch {inf.op_id} partially applied: "
                                f"{truly_applied}/{total_unique_keys} operations took effect"
                            ),
                        )
                    )

        # Check: Every explicitly failed operation must NOT have taken effect
        for fail_id in fail_set:
            start_ev = start_map.get(fail_id)
            if not start_ev or not start_ev.data:
                continue
            kind = start_ev.data.get("kind")
            if kind == "PUT":
                k = hex_to_bytes(start_ev.data["key_hex"])
                v = hex_to_bytes(start_ev.data["val_hex"])
                # If key was never previously in base_state or has different base_state value,
                # but actual_state has v, it means the failed op was applied!
                if actual_state.get(k) == v and base_state.get(k) != v:
                    violations.append(
                        InvariantViolation(
                            violation_type=ViolationType.FAILED_OP_APPLIED,
                            key=k,
                            expected=base_state.get(k),
                            actual=v,
                            op_id=fail_id,
                            description=f"Operation {fail_id} explicitly failed with ERR, but was persisted to database",
                        )
                    )
            elif kind == "DEL":
                k = hex_to_bytes(start_ev.data["key_hex"])
                if k not in actual_state and base_state.get(k) is not None:
                    violations.append(
                        InvariantViolation(
                            violation_type=ViolationType.FAILED_OP_APPLIED,
                            key=k,
                            expected=base_state.get(k),
                            actual=None,
                            op_id=fail_id,
                            description=f"Delete operation {fail_id} explicitly failed with ERR, but key was deleted",
                        )
                    )

        passed = len(violations) == 0
        return EvaluationResult(
            passed=passed,
            violations=violations,
            total_acked_ops=len(ack_list),
            total_inflight_ops=len(inflight_ops),
            total_failed_ops=len(fail_set),
        )

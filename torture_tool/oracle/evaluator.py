"""Oracle evaluator resolving guaranteed acknowledged mutations and post-crash ambiguities."""

from __future__ import annotations

from collections import deque
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
    BATCH_PARTIALLY_APPLIED = "BATCH_PARTIAL_APPLICATION"
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
        # Order ack_list by protocol seq_num (when present) prior to state machine replay
        # to ensure deterministic, monotonic state transitions even if wire events were demuxed out of order.
        ack_list.sort(
            key=lambda ev: (
                0 if (ev.data and ev.data.get("seq_num") is not None) else 1,
                ev.data.get("seq_num") if (ev.data and ev.data.get("seq_num") is not None) else 0,
            )
        )

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

        # Index in-flight mutations per key:
        # key -> list of (op_id, expected_val_or_None)
        inflight_mutations_by_key: Dict[bytes, List[Tuple[str, Optional[bytes]]]] = {}
        inflight_del_keys: Set[bytes] = set()

        for inf in inflight_ops:
            if not inf.data:
                continue
            kind = inf.data.get("kind")
            op_id = inf.op_id or "unknown"
            if kind == "PUT":
                k = hex_to_bytes(inf.data["key_hex"])
                v = hex_to_bytes(inf.data["val_hex"])
                inflight_mutations_by_key.setdefault(k, []).append((op_id, v))
            elif kind == "DEL":
                k = hex_to_bytes(inf.data["key_hex"])
                inflight_mutations_by_key.setdefault(k, []).append((op_id, None))
                inflight_del_keys.add(k)
            elif kind == "BATCH":
                items = inf.data.get("items", [])
                batch_final: Dict[bytes, Optional[bytes]] = {}
                for itm in items:
                    bk = hex_to_bytes(itm["k"])
                    bv = hex_to_bytes(itm["v"]) if itm["v"] is not None else None
                    batch_final[bk] = bv
                for bk, bv in batch_final.items():
                    inflight_mutations_by_key.setdefault(bk, []).append((op_id, bv))
                    if bv is None:
                        inflight_del_keys.add(bk)

        # 3. Build candidate reference states and evaluate violations
        violations: List[InvariantViolation] = []

        # Check 1: No unknown keys (every key in actual_state must be known to the oracle)
        all_possible_keys = set(base_state.all_observed_keys)
        all_possible_keys.update(inflight_mutations_by_key.keys())

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
                # Key was NOT live in base_state (either deleted previously, or only in-flight).
                expected_puts = [
                    v for op_id, v in inflight_mutations_by_key.get(actual_k, [])
                    if v is not None
                ]
                if not expected_puts:
                    # Key was acknowledged deleted or never created with a live value, but appeared in actual_state
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
                else:
                    # Key was targeted by in-flight write(s).
                    # actual_v MUST match one of the expected values! Any torn/corrupted bytes are rejected.
                    if actual_v not in expected_puts:
                        first_op = inflight_mutations_by_key[actual_k][0][0]
                        violations.append(
                            InvariantViolation(
                                violation_type=ViolationType.VALUE_CORRUPTED,
                                key=actual_k,
                                expected=expected_puts[0] if len(expected_puts) == 1 else None,
                                actual=actual_v,
                                op_id=first_op,
                                description=(
                                    f"In-flight write for key was persisted with corrupted or torn value on disk: "
                                    f"got {len(actual_v)} bytes, expected {len(expected_puts[0])} bytes"
                                ),
                            )
                        )

        # Check 2: Every ACKed key must be present with correct value unless overwritten
        for base_k, base_v in base_state.live_keys.items():
            expected_puts = [
                v for op_id, v in inflight_mutations_by_key.get(base_k, [])
                if v is not None
            ]
            can_be_deleted = base_k in inflight_del_keys

            if base_k not in actual_state:
                # Key is missing from recovered state.
                # Only permitted if an in-flight DEL or batch delete targeted this key.
                if not can_be_deleted:
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
            else:
                actual_v = actual_state[base_k]
                # Key is present.
                # Its value MUST match either prior acknowledged value (base_v)
                # OR one of the in-flight expected values!
                if actual_v != base_v:
                    if actual_v not in expected_puts:
                        first_op = (
                            inflight_mutations_by_key[base_k][0][0]
                            if base_k in inflight_mutations_by_key
                            else None
                        )
                        violations.append(
                            InvariantViolation(
                                violation_type=ViolationType.VALUE_CORRUPTED,
                                key=base_k,
                                expected=expected_puts[0] if len(expected_puts) == 1 else base_v,
                                actual=actual_v,
                                op_id=first_op,
                                description=(
                                    "Acknowledged key has corrupted or mismatched value in recovered state: "
                                    "does not match acknowledged base value or any in-flight write"
                                ),
                            )
                        )

        # Check 3: In-flight batch atomicity with multi-worker concurrent race resolution
        inflight_batches = [
            inf for inf in inflight_ops
            if inf.data and inf.data.get("kind") == "BATCH" and inf.data.get("items")
        ]

        if inflight_batches:
            # 1. Parse all in-flight operations into structured representation:
            # (op_id, kind, key_map) where key_map is Dict[bytes, Optional[bytes]]
            parsed_inflight: List[Tuple[str, str, Dict[bytes, Optional[bytes]]]] = []
            for inf in inflight_ops:
                if not inf.data:
                    continue
                knd = inf.data.get("kind")
                oid = inf.op_id or "unknown"
                if knd == "PUT":
                    k = hex_to_bytes(inf.data["key_hex"])
                    v = hex_to_bytes(inf.data["val_hex"])
                    parsed_inflight.append((oid, "PUT", {k: v}))
                elif knd == "DEL":
                    k = hex_to_bytes(inf.data["key_hex"])
                    parsed_inflight.append((oid, "DEL", {k: None}))
                elif knd == "BATCH":
                    b_items = inf.data.get("items", [])
                    final_ops: Dict[bytes, Optional[bytes]] = {}
                    for itm in b_items:
                        bk = hex_to_bytes(itm["k"])
                        bv = hex_to_bytes(itm["v"]) if itm["v"] is not None else None
                        final_ops[bk] = bv
                    parsed_inflight.append((oid, "BATCH", final_ops))

            # 2. Group into connected components by shared keys
            n_ops = len(parsed_inflight)
            adj: Dict[int, List[int]] = {i: [] for i in range(n_ops)}
            for i in range(n_ops):
                keys_i = set(parsed_inflight[i][2].keys())
                for j in range(i + 1, n_ops):
                    keys_j = set(parsed_inflight[j][2].keys())
                    if not keys_i.isdisjoint(keys_j):
                        adj[i].append(j)
                        adj[j].append(i)

            visited_ops: Set[int] = set()
            components: List[List[int]] = []
            for i in range(n_ops):
                if i not in visited_ops:
                    comp = []
                    q = [i]
                    visited_ops.add(i)
                    while q:
                        curr = q.pop()
                        comp.append(curr)
                        for neighbor in adj[curr]:
                            if neighbor not in visited_ops:
                                visited_ops.add(neighbor)
                                q.append(neighbor)
                    components.append(comp)

            # 3. Evaluate each component that contains at least one BATCH
            for comp in components:
                comp_batches = [
                    parsed_inflight[idx] for idx in comp
                    if parsed_inflight[idx][1] == "BATCH"
                ]
                if not comp_batches:
                    # Single PUTs and DELs have no multi-key atomicity constraints
                    continue

                comp_ops = [parsed_inflight[idx] for idx in comp]
                comp_keys: Set[bytes] = set()
                for _, _, kmap in comp_ops:
                    comp_keys.update(kmap.keys())

                sorted_comp_keys = sorted(comp_keys)
                target_state = {k: actual_state.get(k) for k in sorted_comp_keys}
                init_state = {k: base_state.get(k) for k in sorted_comp_keys}

                # BFS to verify whether ANY linearizable sequence applying a subset of operations
                # in this component matches target_state while preserving batch atomicity.
                def find_valid_execution() -> bool:
                    if init_state == target_state:
                        return True

                    def state_to_tuple(st: Dict[bytes, Optional[bytes]]) -> tuple:
                        return tuple(st[k] for k in sorted_comp_keys)

                    queue = deque([(init_state, frozenset())])
                    visited_states = {(state_to_tuple(init_state), frozenset())}
                    max_states = 10000

                    while queue and len(visited_states) < max_states:
                        curr_st, used = queue.popleft()

                        for op_idx, (oid, knd, kmap) in enumerate(comp_ops):
                            if op_idx in used:
                                continue

                            # Apply operation as an atomic unit
                            nxt_st = curr_st.copy()
                            for k, v in kmap.items():
                                nxt_st[k] = v

                            if nxt_st == target_state:
                                return True

                            nxt_used = used | {op_idx}
                            st_tup = state_to_tuple(nxt_st)
                            state_key = (st_tup, nxt_used)
                            if state_key not in visited_states:
                                visited_states.add(state_key)
                                queue.append((nxt_st, nxt_used))

                    return False

                if not find_valid_execution():
                    # Batch atomicity was broken by partial application or torn items
                    for b_oid, _, _ in comp_batches:
                        violations.append(
                            InvariantViolation(
                                violation_type=ViolationType.BATCH_PARTIAL_APPLICATION,
                                key=None,
                                expected=None,
                                actual=None,
                                op_id=b_oid,
                                description=(
                                    f"In-flight batch {b_oid} partially applied: "
                                    f"could not be resolved to an atomic all-or-nothing state"
                                ),
                            )
                        )

        # Check 4: Every explicitly failed operation must NOT have taken effect
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
            elif kind == "BATCH":
                batch_items = start_ev.data.get("items", [])
                for itm in batch_items:
                    k = hex_to_bytes(itm["k"])
                    v = hex_to_bytes(itm["v"]) if itm["v"] is not None else None
                    if v is not None:
                        if actual_state.get(k) == v and base_state.get(k) != v:
                            violations.append(
                                InvariantViolation(
                                    violation_type=ViolationType.FAILED_OP_APPLIED,
                                    key=k,
                                    expected=base_state.get(k),
                                    actual=v,
                                    op_id=fail_id,
                                    description=f"Batch operation {fail_id} explicitly failed with ERR, but key was persisted to database",
                                )
                            )
                    else:
                        if k not in actual_state and base_state.get(k) is not None:
                            violations.append(
                                InvariantViolation(
                                    violation_type=ViolationType.FAILED_OP_APPLIED,
                                    key=k,
                                    expected=base_state.get(k),
                                    actual=None,
                                    op_id=fail_id,
                                    description=f"Batch delete operation {fail_id} explicitly failed with ERR, but key was deleted",
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

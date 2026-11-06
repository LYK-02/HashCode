"""In-memory reference database state and transaction history."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Dict, List, Optional, Set, Tuple

from torture_tool.protocol.spec import (
    BatchItem,
    BatchRequest,
    DelRequest,
    PutRequest,
    Request,
)


@dataclass
class ReferenceDatabaseState:
    """Represents the authoritative client-observed key-value state."""

    # Map of live keys to their byte values
    live_keys: Dict[bytes, bytes] = field(default_factory=dict)
    # Set of all keys ever written in the session
    all_observed_keys: Set[bytes] = field(default_factory=set)

    def apply_put(self, key: bytes, value: bytes) -> None:
        self.live_keys[key] = value
        self.all_observed_keys.add(key)

    def apply_del(self, key: bytes) -> None:
        self.live_keys.pop(key, None)
        self.all_observed_keys.add(key)

    def apply_batch(self, items: Sequence[BatchItem]) -> None:
        for item in items:
            if item.value is not None:
                self.apply_put(item.key, item.value)
            else:
                self.apply_del(item.key)

    def apply_request(self, req: Request) -> None:
        if isinstance(req, PutRequest):
            self.apply_put(req.key, req.value)
        elif isinstance(req, DelRequest):
            self.apply_del(req.key)
        elif isinstance(req, BatchRequest):
            self.apply_batch(req.items)

    def clone(self) -> ReferenceDatabaseState:
        return ReferenceDatabaseState(
            live_keys=dict(self.live_keys),
            all_observed_keys=set(self.all_observed_keys),
        )

    def get(self, key: bytes) -> Optional[bytes]:
        return self.live_keys.get(key)


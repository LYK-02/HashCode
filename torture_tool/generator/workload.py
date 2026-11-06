"""Deterministic workload generators for ./torture."""

from __future__ import annotations

import random
from dataclasses import dataclass
from typing import List, Optional, Sequence, Tuple

from torture_tool.protocol.spec import (
    BatchItem,
    BatchRequest,
    DelRequest,
    MAX_RAW_VALUE_LEN,
    PingRequest,
    PutRequest,
    Request,
)


@dataclass(frozen=True)
class WorkloadConfig:
    """Configures distributions and limits for workload generation."""

    mode: str = "default"
    key_pool_size: int = 200
    min_val_size: int = 1
    max_val_size: int = 1024
    put_ratio: float = 0.60
    del_ratio: float = 0.20
    batch_ratio: float = 0.20
    min_batch_size: int = 2
    max_batch_size: int = 20
    shared_key_pool: bool = False

    @classmethod
    def for_mode(cls, mode: str, shared_key_pool: bool = False) -> WorkloadConfig:
        """Return tuned workload configuration preset based on mode."""
        if mode == "default":
            return cls(
                mode="default",
                key_pool_size=200,
                min_val_size=0,
                max_val_size=4096,
                put_ratio=0.60,
                del_ratio=0.20,
                batch_ratio=0.20,
                min_batch_size=2,
                max_batch_size=30,
                shared_key_pool=shared_key_pool,
            )
        elif mode == "large-values":
            return cls(
                mode="large-values",
                key_pool_size=50,
                min_val_size=64 * 1024,  # 64 KiB
                max_val_size=MAX_RAW_VALUE_LEN,  # 1 MiB
                put_ratio=0.80,
                del_ratio=0.10,
                batch_ratio=0.10,
                min_batch_size=2,
                max_batch_size=5,
                shared_key_pool=shared_key_pool,
            )
        elif mode == "large-batches":
            return cls(
                mode="large-batches",
                key_pool_size=1000,
                min_val_size=16,
                max_val_size=256,
                put_ratio=0.10,
                del_ratio=0.10,
                batch_ratio=0.80,
                min_batch_size=100,
                max_batch_size=10000,
                shared_key_pool=shared_key_pool,
            )
        elif mode == "compaction":
            # High churn on small key space: heavy deletes and overwrites
            return cls(
                mode="compaction",
                key_pool_size=30,
                min_val_size=128,
                max_val_size=2048,
                put_ratio=0.50,
                del_ratio=0.35,
                batch_ratio=0.15,
                min_batch_size=5,
                max_batch_size=25,
                shared_key_pool=shared_key_pool,
            )
        elif mode == "recovery-crash":
            return cls(
                mode="recovery-crash",
                key_pool_size=500,
                min_val_size=32,
                max_val_size=1024,
                put_ratio=0.70,
                del_ratio=0.15,
                batch_ratio=0.15,
                min_batch_size=10,
                max_batch_size=50,
                shared_key_pool=shared_key_pool,
            )
        elif mode == "disk-full":
            return cls(
                mode="disk-full",
                key_pool_size=100,
                min_val_size=1024,
                max_val_size=64 * 1024,
                put_ratio=0.75,
                del_ratio=0.10,
                batch_ratio=0.15,
                min_batch_size=5,
                max_batch_size=20,
                shared_key_pool=shared_key_pool,
            )
        else:
            raise ValueError(f"Unknown workload mode: '{mode}'")


def generate_key(prng: random.Random, key_pool_size: int) -> bytes:
    """Generate a key deterministically from a bounded pool or raw entropy."""
    idx = prng.randint(0, key_pool_size - 1)
    # Generate structured key representation: "key_<idx>" with zero-padded hex
    return f"k_{idx:04x}".encode("ascii")


def generate_value(prng: random.Random, min_size: int, max_size: int) -> bytes:
    """Generate random payload bytes covering binary, nulls, and boundary sizes."""
    if max_size <= 0:
        return b""

    size = prng.randint(min_size, max_size)
    if size == 0:
        return b""

    # Choose value archetype
    archetype = prng.random()
    if archetype < 0.20:
        # All null bytes
        return b"\x00" * size
    elif archetype < 0.35:
        # Repeating pattern with special characters
        pattern = b"val_\xff\xfe\x00\n\r:,_"
        repeats = (size // len(pattern)) + 1
        return (pattern * repeats)[:size]
    elif archetype < 0.50:
        # Ascii text
        chars = b"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
        return bytes(prng.choice(chars) for _ in range(size))
    else:
        # High entropy random bytes
        return prng.randbytes(size)


class WorkloadGenerator:
    """Stateful deterministic workload generator bound to an isolated PRNG stream."""

    def __init__(
        self,
        worker_id: str | int,
        prng: random.Random,
        config: Optional[WorkloadConfig] = None,
        iteration: int = 1,
        shared_key_pool: Optional[bool] = None,
    ) -> None:
        self.worker_id = str(worker_id)
        self.prng = prng
        self.config = config or WorkloadConfig.for_mode("default")
        self.iteration = iteration
        self._op_seq = 0
        if shared_key_pool is not None:
            self.shared_key_pool = shared_key_pool
        else:
            self.shared_key_pool = getattr(self.config, "shared_key_pool", False)

    def next_op_id(self) -> str:
        """Generate globally unique operation ID formatted as '<worker_id>-<iteration>-<seq>'."""
        self._op_seq += 1
        return f"{self.worker_id}-{self.iteration}-{self._op_seq:06d}"

    def generate_put(self, key: Optional[bytes] = None) -> PutRequest:
        op_id = self.next_op_id()
        k = key or self.next_key()
        v = generate_value(self.prng, self.config.min_val_size, self.config.max_val_size)
        return PutRequest(op_id=op_id, key=k, value=v)

    def generate_del(self, key: Optional[bytes] = None) -> DelRequest:
        op_id = self.next_op_id()
        k = key or self.next_key()
        return DelRequest(op_id=op_id, key=k)

    def next_key(self) -> bytes:
        """Derive a key from the key pool.

        When shared_key_pool is True, workers share the exact same key space
        (e.g. k_0000, k_0001) stimulating concurrent same-key races.
        When False, keys are partitioned per worker_id (e.g. w1_k_0000).
        """
        raw_key = generate_key(self.prng, self.config.key_pool_size)
        if self.shared_key_pool:
            return raw_key
        return f"{self.worker_id}_{raw_key.decode('ascii')}".encode("ascii")

    def generate_batch(self, count: Optional[int] = None) -> BatchRequest:
        op_id = self.next_op_id()
        num_items = count or self.prng.randint(
            self.config.min_batch_size, self.config.max_batch_size
        )
        items: List[BatchItem] = []
        for _ in range(num_items):
            k = self.next_key()
            is_del = self.prng.random() < 0.25
            if is_del:
                items.append(BatchItem(key=k, value=None))
            else:
                # In BATCH grammar, <key>: indicates deletion, so batch PUT values are at least 1 byte
                min_sz = max(1, self.config.min_val_size)
                v = generate_value(self.prng, min_sz, self.config.max_val_size)
                items.append(BatchItem(key=k, value=v))
        return BatchRequest(op_id=op_id, items=tuple(items))

    def next_operation(self) -> Request:
        """Generate the next deterministic request based on configured ratios."""
        roll = self.prng.random()
        if roll < self.config.put_ratio:
            return self.generate_put()
        elif roll < (self.config.put_ratio + self.config.del_ratio):
            return self.generate_del()
        else:
            return self.generate_batch()


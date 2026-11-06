"""Deterministic seed expansion and isolated PRNG stream management."""

from __future__ import annotations

import hashlib
import random
from dataclasses import dataclass
from typing import Dict


def derive_subseed(root_seed: int, domain: str, index: int | str = 0) -> int:
    """Derive a deterministic 64-bit unsigned integer subseed from a root seed.

    Uses SHA-256 to guarantee that different domain tags (e.g. 'worker', 'scheduler')
    and indices produce statistically independent, non-overlapping PRNG streams.
    """
    raw_material = f"{root_seed}:{domain}:{index}".encode("utf-8")
    digest = hashlib.sha256(raw_material).digest()
    # Extract first 8 bytes as big-endian unsigned 64-bit int
    subseed = int.from_bytes(digest[:8], byteorder="big", signed=False)
    return subseed


@dataclass(frozen=True)
class SeedManager:
    """Manages hierarchical PRNG creation for torture runs, ensuring thread isolation."""

    root_seed: int

    def get_iteration_seed(self, iteration: int) -> int:
        """Derive seed for a specific crash/recovery iteration."""
        return derive_subseed(self.root_seed, "iteration", iteration)

    def get_scheduler_prng(self, iteration: int = 0) -> random.Random:
        """Create isolated PRNG instance for the crash delay scheduler."""
        seed = derive_subseed(self.root_seed, "scheduler", iteration)
        return random.Random(seed)

    def get_worker_prng(self, worker_id: int | str, iteration: int = 0) -> random.Random:
        """Create isolated PRNG instance for a specific worker thread.

        Each worker gets its own independent instance to avoid lock contention
        and prevent thread preemption races from breaking repeatability.
        """
        seed = derive_subseed(self.root_seed, f"worker:{worker_id}", iteration)
        return random.Random(seed)

    def get_substream(self, domain: str, index: int | str = 0) -> random.Random:
        """Generic isolated stream generator."""
        seed = derive_subseed(self.root_seed, domain, index)
        return random.Random(seed)


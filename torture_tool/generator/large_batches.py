"""Synthesizer for massive atomic batches up to 10,000 operations."""

from __future__ import annotations

import random
from typing import Dict, List, Optional, Tuple

from torture_tool.protocol.spec import BatchItem, BatchRequest


def generate_giant_batch(
    op_id: str,
    count: int,
    key_pool_size: int,
    prng: random.Random,
    min_val_size: int = 16,
    max_val_size: int = 256,
) -> Tuple[BatchRequest, Dict[bytes, Optional[bytes]]]:
    """Generate a batch with up to 10,000 operations and return (BatchRequest, resolved_intent).

    Includes intra-batch overwrites, deletes, and duplicate keys.
    'resolved_intent' maps each affected key to its final state resulting from the batch.
    """
    total_ops = max(1, min(10000, count))
    items: List[BatchItem] = []
    final_intent: Dict[bytes, Optional[bytes]] = {}

    for _ in range(total_ops):
        # Pick key from bounded pool to encourage collisions within batch
        idx = prng.randint(0, key_pool_size - 1)
        k = f"k_{idx:05d}".encode("ascii")

        # 75% PUT, 25% DEL
        is_del = prng.random() < 0.25
        if is_del:
            val = None
        else:
            sz = prng.randint(min_val_size, max_val_size)
            val = prng.randbytes(sz)

        items.append(BatchItem(key=k, value=val))
        final_intent[k] = val

    return BatchRequest(op_id=op_id, items=tuple(items)), final_intent


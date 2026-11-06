"""Synthesizer for boundary and maximum sized values up to 1 MiB."""

from __future__ import annotations

import hashlib
import random
from typing import Tuple

from torture_tool.protocol.spec import MAX_RAW_VALUE_LEN


def generate_large_payload(
    size_bytes: int,
    pattern_type: str,
    prng: random.Random,
) -> Tuple[bytes, str]:
    """Generate a large binary payload with exact size and return (data, sha256_hash).

    Pattern types:
    - 'nulls': pure zero bytes (b'\\x00' * N)
    - 'noise': seeded random binary bytes
    - 'delimiters': repeating protocol delimiters (b',:\\n\\r' * N)
    - 'high_entropy': non-UTF-8 random bytes
    """
    size = min(size_bytes, MAX_RAW_VALUE_LEN)
    if size <= 0:
        return b"", hashlib.sha256(b"").hexdigest()

    if pattern_type == "nulls":
        data = b"\x00" * size
    elif pattern_type == "delimiters":
        pattern = b",:\n\r\x00\xff"
        repeats = (size // len(pattern)) + 1
        data = (pattern * repeats)[:size]
    elif pattern_type == "noise":
        # Deterministic byte generation from prng
        data = prng.randbytes(size)
    else:
        # Default high entropy mix with invalid utf8 bytes
        prefix = b"\xff\xfe\x80\xc0"
        repeats = (size // len(prefix)) + 1
        data = (prefix * repeats)[:size]

    digest = hashlib.sha256(data).hexdigest()
    return data, digest


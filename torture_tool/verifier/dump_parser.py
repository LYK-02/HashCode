"""Deterministic binary-safe parser for kvdb dump output."""

from __future__ import annotations

from typing import Dict

from torture_tool.protocol.spec import ProtocolParseError
from torture_tool.protocol.utils import hex_to_bytes


class DumpParseError(Exception):
    """Raised when kvdb dump output violates format or contains duplicate/malformed keys."""

    pass


def parse_dump_output(dump_text: str, verify_sorted: bool = False) -> Dict[bytes, bytes]:
    """Parse standard sorted dump output: '<hex_key> <hex_val>\\n' into Dict[bytes, bytes].

    Strictly validates:
    1. Rejects duplicate keys (index corruption in engine).
    2. Validates hex digits for keys and values.
    3. Handles empty values correctly.
    4. Optionally validates lexicographical key order.
    """
    state: Dict[bytes, bytes] = {}
    last_key: bytes | None = None

    for line_num, line in enumerate(dump_text.splitlines(), start=1):
        trimmed = line.strip()
        if not trimmed:
            continue

        parts = trimmed.split(" ")
        if len(parts) == 1:
            # Key with empty value: '<hex_key>'
            k_hex = parts[0]
            v_hex = ""
        elif len(parts) == 2:
            k_hex = parts[0]
            v_hex = parts[1]
        else:
            raise DumpParseError(
                f"Line {line_num}: malformed dump entry (expected at most 2 tokens, got {len(parts)}): '{line}'"
            )

        try:
            k = hex_to_bytes(k_hex)
        except ProtocolParseError as e:
            raise DumpParseError(f"Line {line_num}: invalid hex key '{k_hex}': {e}") from e

        try:
            v = hex_to_bytes(v_hex)
        except ProtocolParseError as e:
            raise DumpParseError(f"Line {line_num}: invalid hex value '{v_hex}': {e}") from e

        if k in state:
            raise DumpParseError(
                f"Line {line_num}: duplicate key detected in dump: '{k_hex}'. Index is corrupted!"
            )

        if verify_sorted and last_key is not None:
            if k <= last_key:
                raise DumpParseError(
                    f"Line {line_num}: keys not strictly sorted: {k!r} <= {last_key!r}"
                )

        state[k] = v
        last_key = k

    return state


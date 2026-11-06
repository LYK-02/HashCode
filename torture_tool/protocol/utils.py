"""Binary-safe hex encoding/decoding utilities for ./torture protocol."""

from __future__ import annotations

import binascii
from typing import Optional

from torture_tool.protocol.spec import (
    HEX_PATTERN,
    MAX_HEX_VALUE_LEN,
    MAX_RAW_VALUE_LEN,
    ProtocolParseError,
    ProtocolValidationError,
)


def bytes_to_hex(data: bytes) -> str:
    """Convert raw bytes to lowercase hexadecimal string."""
    if not isinstance(data, (bytes, bytearray)):
        raise TypeError(f"Expected bytes, got {type(data).__name__}")
    return binascii.hexlify(data).decode("ascii")


def hex_to_bytes(hex_str: str) -> bytes:
    """Convert hexadecimal string to raw bytes with strict validation."""
    if not isinstance(hex_str, str):
        raise ProtocolParseError(f"Expected hex string, got {type(hex_str).__name__}")
    if len(hex_str) % 2 != 0:
        raise ProtocolParseError(
            f"Invalid hex string: odd length ({len(hex_str)} characters)"
        )
    if not HEX_PATTERN.match(hex_str):
        raise ProtocolParseError("Invalid hex string: contains non-hexadecimal characters")
    try:
        return binascii.unhexlify(hex_str)
    except Exception as e:
        raise ProtocolParseError(f"Failed to decode hex: {e}") from e


def validate_key(key: bytes) -> None:
    """Validate that a key is non-empty bytes."""
    if not isinstance(key, bytes):
        raise ProtocolValidationError(f"Key must be bytes, got {type(key).__name__}")
    if len(key) == 0:
        raise ProtocolValidationError("Key cannot be empty (0 bytes)")


def validate_value(value: bytes) -> None:
    """Validate that value bytes do not exceed maximum permitted payload size."""
    if not isinstance(value, bytes):
        raise ProtocolValidationError(f"Value must be bytes, got {type(value).__name__}")
    if len(value) > MAX_RAW_VALUE_LEN:
        raise ProtocolValidationError(
            f"Value size {len(value)} exceeds maximum allowable limit of {MAX_RAW_VALUE_LEN} bytes"
        )


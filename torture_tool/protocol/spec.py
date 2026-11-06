"""Protocol specification, constants, tokens, and data models for ./torture."""

from __future__ import annotations

import re
from dataclasses import dataclass
from enum import Enum
from typing import List, Optional, Tuple, Union

# Wire constants
MAX_RAW_VALUE_LEN = 1024 * 1024  # 1 MiB
MAX_HEX_VALUE_LEN = MAX_RAW_VALUE_LEN * 2  # 2 MiB in hex chars
MAX_LINE_LENGTH = MAX_HEX_VALUE_LEN + 1024 * 1024  # Framing headroom (up to ~3 MiB for 10k batch)

# Identifiers and tokens
OP_ID_PATTERN = re.compile(r"^[a-zA-Z0-9_\-\.]+$")
HEX_PATTERN = re.compile(r"^[0-9a-fA-F]*$")


class ErrorCode(str, Enum):
    """Standardized error codes emitted by the engine."""

    QUOTA_EXCEEDED = "QUOTA_EXCEEDED"
    INVALID_REQUEST = "INVALID_REQUEST"
    PAYLOAD_TOO_LARGE = "PAYLOAD_TOO_LARGE"
    IO_ERROR = "IO_ERROR"
    LOCKED = "LOCKED"
    INTERNAL_ERROR = "INTERNAL_ERROR"


class ProtocolError(Exception):
    """Base class for all protocol encoding and parsing failures."""

    pass


class ProtocolParseError(ProtocolError):
    """Raised when an incoming wire message is malformed or violates grammar."""

    pass


class ProtocolValidationError(ProtocolError):
    """Raised when request payload or parameters violate constraints."""

    pass


@dataclass(frozen=True)
class PutRequest:
    op_id: str
    key: bytes
    value: bytes

    def __post_init__(self) -> None:
        if not OP_ID_PATTERN.match(self.op_id):
            raise ProtocolValidationError(f"Invalid op_id format: '{self.op_id}'")
        if not isinstance(self.key, bytes) or len(self.key) == 0:
            raise ProtocolValidationError("Key must be non-empty bytes")
        if not isinstance(self.value, bytes):
            raise ProtocolValidationError("Value must be bytes")
        if len(self.value) > MAX_RAW_VALUE_LEN:
            raise ProtocolValidationError(
                f"Value length {len(self.value)} exceeds maximum of {MAX_RAW_VALUE_LEN} bytes"
            )


@dataclass(frozen=True)
class DelRequest:
    op_id: str
    key: bytes

    def __post_init__(self) -> None:
        if not OP_ID_PATTERN.match(self.op_id):
            raise ProtocolValidationError(f"Invalid op_id format: '{self.op_id}'")
        if not isinstance(self.key, bytes) or len(self.key) == 0:
            raise ProtocolValidationError("Key must be non-empty bytes")


@dataclass(frozen=True)
class BatchItem:
    key: bytes
    value: Optional[bytes]  # None indicates deletion

    def __post_init__(self) -> None:
        if not isinstance(self.key, bytes) or len(self.key) == 0:
            raise ProtocolValidationError("BatchItem key must be non-empty bytes")
        if self.value is not None and not isinstance(self.value, bytes):
            raise ProtocolValidationError("BatchItem value must be bytes or None")
        if self.value is not None and len(self.value) > MAX_RAW_VALUE_LEN:
            raise ProtocolValidationError(
                f"BatchItem value length {len(self.value)} exceeds limit {MAX_RAW_VALUE_LEN}"
            )


@dataclass(frozen=True)
class BatchRequest:
    op_id: str
    items: Tuple[BatchItem, ...]

    def __post_init__(self) -> None:
        if not OP_ID_PATTERN.match(self.op_id):
            raise ProtocolValidationError(f"Invalid op_id format: '{self.op_id}'")
        if not isinstance(self.items, (list, tuple)) or len(self.items) == 0:
            raise ProtocolValidationError("BatchRequest must contain at least 1 item")
        if len(self.items) > 10000:
            raise ProtocolValidationError("BatchRequest cannot exceed 10000 items")


@dataclass(frozen=True)
class PingRequest:
    op_id: str

    def __post_init__(self) -> None:
        if not OP_ID_PATTERN.match(self.op_id):
            raise ProtocolValidationError(f"Invalid op_id format: '{self.op_id}'")


Request = Union[PutRequest, DelRequest, BatchRequest, PingRequest]


@dataclass(frozen=True)
class AckResponse:
    op_id: str
    seq_num: Optional[int] = None

    def __post_init__(self) -> None:
        if not OP_ID_PATTERN.match(self.op_id):
            raise ProtocolParseError(f"Invalid op_id format in ACK: '{self.op_id}'")
        if self.seq_num is not None and self.seq_num < 0:
            raise ProtocolParseError(f"Sequence number cannot be negative: {self.seq_num}")


@dataclass(frozen=True)
class ErrResponse:
    op_id: str
    error_code: str
    message: str

    def __post_init__(self) -> None:
        if not OP_ID_PATTERN.match(self.op_id):
            raise ProtocolParseError(f"Invalid op_id format in ERR: '{self.op_id}'")
        if not self.error_code:
            raise ProtocolParseError("Error code cannot be empty in ERR response")


@dataclass(frozen=True)
class PongResponse:
    op_id: str

    def __post_init__(self) -> None:
        if not OP_ID_PATTERN.match(self.op_id):
            raise ProtocolParseError(f"Invalid op_id format in PONG: '{self.op_id}'")


Response = Union[AckResponse, ErrResponse, PongResponse]


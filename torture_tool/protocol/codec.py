"""Wire codecs: request encoder and response parser for ./torture protocol."""

from __future__ import annotations

from typing import List

from torture_tool.protocol.spec import (
    AckResponse,
    BatchItem,
    BatchRequest,
    DelRequest,
    ErrResponse,
    MAX_LINE_LENGTH,
    PingRequest,
    PongResponse,
    ProtocolParseError,
    ProtocolValidationError,
    PutRequest,
    Request,
    Response,
)
from torture_tool.protocol.utils import bytes_to_hex, hex_to_bytes


def encode_request(request: Request) -> str:
    """Encode a typed request into a wire-formatted protocol string with trailing newline."""
    if isinstance(request, PutRequest):
        hex_key = bytes_to_hex(request.key)
        hex_val = bytes_to_hex(request.value)
        return f"PUT {request.op_id} {hex_key} {hex_val}\n"

    elif isinstance(request, DelRequest):
        hex_key = bytes_to_hex(request.key)
        return f"DEL {request.op_id} {hex_key}\n"

    elif isinstance(request, BatchRequest):
        encoded_items: List[str] = []
        for item in request.items:
            h_key = bytes_to_hex(item.key)
            if item.value is not None:
                h_val = bytes_to_hex(item.value)
                encoded_items.append(f"{h_key}:{h_val}")
            else:
                encoded_items.append(f"{h_key}:")
        items_str = ",".join(encoded_items)
        return f"BATCH {request.op_id} {len(request.items)} {items_str}\n"

    elif isinstance(request, PingRequest):
        return f"PING {request.op_id}\n"

    else:
        raise ProtocolValidationError(f"Unsupported request type: {type(request).__name__}")


def parse_response(line: str) -> Response:
    """Parse an incoming engine response line into a strongly typed Response object.

    Handles ACK, ERR, and PONG. Strips trailing newline/carriage return.
    Strictly validates tokens and lengths.
    """
    if not isinstance(line, str):
        raise ProtocolParseError(f"Expected str line, got {type(line).__name__}")

    raw_line = line.rstrip("\r\n")
    if not raw_line:
        raise ProtocolParseError("Empty response line received")

    if len(raw_line) > MAX_LINE_LENGTH:
        raise ProtocolParseError(
            f"Response line exceeds maximum allowed length: {len(raw_line)} > {MAX_LINE_LENGTH}"
        )

    parts = raw_line.split(" ", 2)
    tag = parts[0]

    if tag == "ACK":
        if len(parts) < 2:
            raise ProtocolParseError("Malformed ACK line: missing op_id")
        op_id = parts[1]
        seq_num = None
        if len(parts) == 3:
            # Check for sequence number
            rest = parts[2].strip()
            if rest:
                try:
                    seq_num = int(rest)
                except ValueError as e:
                    raise ProtocolParseError(
                        f"Malformed ACK line: sequence number is not an integer: '{rest}'"
                    ) from e
        return AckResponse(op_id=op_id, seq_num=seq_num)

    elif tag == "ERR":
        if len(parts) < 3:
            raise ProtocolParseError("Malformed ERR line: expected 'ERR <op_id> <code> [msg]'")
        op_id = parts[1]
        rest = parts[2].split(" ", 1)
        code = rest[0]
        message = rest[1] if len(rest) > 1 else ""
        return ErrResponse(op_id=op_id, error_code=code, message=message)

    elif tag == "PONG":
        if len(parts) < 2:
            raise ProtocolParseError("Malformed PONG line: missing op_id")
        if len(parts) > 2:
            raise ProtocolParseError("Malformed PONG line: unexpected extra tokens")
        return PongResponse(op_id=parts[1])

    else:
        raise ProtocolParseError(f"Unknown response tag '{tag}' in line: '{raw_line}'")


def parse_request(line: str) -> Request:
    """Parse an incoming wire request line (useful for engine mocks and golden tests)."""
    raw_line = line.rstrip("\r\n")
    if not raw_line:
        raise ProtocolParseError("Empty request line")

    parts = raw_line.split(" ")
    if len(parts) < 2:
        raise ProtocolParseError(f"Invalid request line (too few tokens): '{raw_line}'")

    cmd = parts[0]
    op_id = parts[1]

    if cmd == "PUT":
        if len(parts) != 4:
            raise ProtocolParseError(f"Malformed PUT line: expected 4 tokens, got {len(parts)}")
        key = hex_to_bytes(parts[2])
        val = hex_to_bytes(parts[3])
        return PutRequest(op_id=op_id, key=key, value=val)

    elif cmd == "DEL":
        if len(parts) != 3:
            raise ProtocolParseError(f"Malformed DEL line: expected 3 tokens, got {len(parts)}")
        key = hex_to_bytes(parts[2])
        return DelRequest(op_id=op_id, key=key)

    elif cmd == "BATCH":
        if len(parts) != 4:
            raise ProtocolParseError(f"Malformed BATCH line: expected 4 tokens, got {len(parts)}")
        try:
            count = int(parts[2])
        except ValueError as e:
            raise ProtocolParseError(f"Invalid batch count: '{parts[2]}'") from e

        items_str = parts[3]
        if not items_str:
            raise ProtocolParseError("Batch items list cannot be empty string")

        raw_items = items_str.split(",")
        if len(raw_items) != count:
            raise ProtocolParseError(
                f"Batch count mismatch: header specifies {count}, but found {len(raw_items)} items"
            )

        items: List[BatchItem] = []
        for raw_item in raw_items:
            if ":" not in raw_item:
                raise ProtocolParseError(f"Malformed batch item without colon: '{raw_item}'")
            k_hex, v_hex = raw_item.split(":", 1)
            k = hex_to_bytes(k_hex)
            v = hex_to_bytes(v_hex) if len(v_hex) > 0 else None
            items.append(BatchItem(key=k, value=v))

        return BatchRequest(op_id=op_id, items=tuple(items))

    elif cmd == "PING":
        if len(parts) != 2:
            raise ProtocolParseError(f"Malformed PING line: expected 2 tokens, got {len(parts)}")
        return PingRequest(op_id=op_id)

    else:
        raise ProtocolParseError(f"Unknown request command '{cmd}'")


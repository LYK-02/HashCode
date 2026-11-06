"""Thread-safe serialized stdin writer for child process pipe communication."""

from __future__ import annotations

import io
import threading
from typing import Optional

from torture_tool.concurrency.dispatcher import ConcurrencyError
from torture_tool.protocol.codec import encode_request
from torture_tool.protocol.spec import Request


class StdinWriter:
    """Thread-safe writer ensuring atomic line transmission across child stdin pipe."""

    def __init__(self, stdin_stream: io.BufferedWriter) -> None:
        self.stdin_stream = stdin_stream
        self._lock = threading.Lock()
        self.requests_sent = 0
        self.bytes_written = 0
        self._is_closed = False

    def write_request(self, request: Request) -> str:
        """Encode and atomically write a request to stdin under lock.

        Returns the wire string that was sent.
        """
        wire_str = encode_request(request)
        wire_bytes = wire_str.encode("utf-8")

        with self._lock:
            if self._is_closed:
                raise ConcurrencyError("Cannot write to closed stdin writer")
            try:
                self.stdin_stream.write(wire_bytes)
                self.stdin_stream.flush()
                self.requests_sent += 1
                self.bytes_written += len(wire_bytes)
            except (BrokenPipeError, OSError) as e:
                self._is_closed = True
                raise ConcurrencyError(f"Failed to write request {request.op_id} to stdin: {e}") from e

        return wire_str

    def close(self) -> None:
        with self._lock:
            self._is_closed = True
            try:
                if not self.stdin_stream.closed:
                    self.stdin_stream.close()
            except OSError:
                pass


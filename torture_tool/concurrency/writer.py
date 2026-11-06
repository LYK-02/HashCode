"""Thread-safe serialized stdin writer for child process pipe communication."""

from __future__ import annotations

import io
import threading
import time
from typing import Optional

from torture_tool.concurrency.dispatcher import ConcurrencyError
from torture_tool.protocol.codec import encode_request
from torture_tool.protocol.spec import Request

DEFAULT_CHUNK_SIZE = 64 * 1024  # 64 KiB matches POSIX pipe buffer capacity


class StdinWriter:
    """Thread-safe writer ensuring atomic line transmission across child stdin pipe.

    Optimized for low lock contention and high concurrency:
    - Encodes payloads outside the mutex to keep CPU serialization off the critical path.
    - Writes large payloads using zero-copy memoryview chunking matching pipe capacity.
    - Tracks lock wait time and contention metrics for empirical diagnostics.
    - Cooperatively yields CPU quantum after lock release to eliminate thread starvation.
    """

    def __init__(
        self,
        stdin_stream: io.BufferedWriter,
        chunk_size: int = DEFAULT_CHUNK_SIZE,
    ) -> None:
        self.stdin_stream = stdin_stream
        self.chunk_size = chunk_size
        self._lock = threading.Lock()
        self.requests_sent = 0
        self.bytes_written = 0
        self.total_lock_wait_time = 0.0
        self.max_lock_wait_time = 0.0
        self.lock_acquisitions = 0
        self._is_closed = False

    @property
    def average_lock_wait_time(self) -> float:
        """Return average time in seconds worker threads spent waiting to acquire the lock."""
        if self.lock_acquisitions == 0:
            return 0.0
        return self.total_lock_wait_time / self.lock_acquisitions

    def write_request(self, request: Request) -> str:
        """Encode and atomically write a request to stdin under lock.

        Returns the wire string that was sent.
        """
        # Format and encode payload outside of lock to avoid serializing CPU formatting
        wire_str = encode_request(request)
        wire_bytes = wire_str.encode("utf-8")

        t0 = time.monotonic()
        with self._lock:
            wait_time = time.monotonic() - t0
            self.total_lock_wait_time += wait_time
            if wait_time > self.max_lock_wait_time:
                self.max_lock_wait_time = wait_time
            self.lock_acquisitions += 1

            if self._is_closed:
                raise ConcurrencyError("Cannot write to closed stdin writer")
            try:
                # Chunked buffer transmission using memoryview to avoid redundant allocations
                mv = memoryview(wire_bytes)
                total_len = len(wire_bytes)
                chunk_sz = self.chunk_size
                offset = 0
                while offset < total_len:
                    end = min(offset + chunk_sz, total_len)
                    self.stdin_stream.write(mv[offset:end])
                    offset = end
                self.stdin_stream.flush()
                self.requests_sent += 1
                self.bytes_written += total_len
            except (BrokenPipeError, OSError) as e:
                self._is_closed = True
                raise ConcurrencyError(f"Failed to write request {request.op_id} to stdin: {e}") from e

        # Cooperatively yield CPU quantum to prevent thread starvation during tight loops
        time.sleep(0)

        return wire_str

    def close(self) -> None:
        with self._lock:
            self._is_closed = True
            try:
                if not self.stdin_stream.closed:
                    self.stdin_stream.close()
            except OSError:
                pass


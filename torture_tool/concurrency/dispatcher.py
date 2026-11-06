"""Thread-safe response dispatcher demultiplexing child stdout to worker futures."""

from __future__ import annotations

import concurrent.futures
import io
import threading
from typing import Dict, List, Optional, Tuple

from torture_tool.protocol.codec import parse_response
from torture_tool.protocol.spec import (
    AckResponse,
    ErrResponse,
    PongResponse,
    ProtocolParseError,
    Response,
)


class ConcurrencyError(Exception):
    """Base error for harness concurrency and dispatch failures."""

    pass


class DuplicateOperationIdError(ConcurrencyError):
    """Raised when registering an operation ID that is already pending."""

    pass


class ProcessDiedError(ConcurrencyError):
    """Raised when child process terminates before providing a response."""

    pass


class ResponseDispatcher:
    """Demultiplexes lines from child stdout into individual worker futures by op_id.

    Runs a single background thread reading child stdout line-by-line.
    Workers register their op_id before sending a request and wait on the returned Future.
    """

    def __init__(self, stdout_stream: io.BufferedReader) -> None:
        self.stdout_stream = stdout_stream
        self._pending: Dict[str, concurrent.futures.Future[Response]] = {}
        self._lock = threading.Lock()
        self._thread: Optional[threading.Thread] = None
        self._is_stopped = False
        self.unknown_responses: List[str] = []
        self.protocol_errors: List[str] = []

    def start(self) -> None:
        """Start the background stdout reader/dispatcher thread."""
        self._thread = threading.Thread(
            target=self._reader_loop,
            name="ResponseDispatcherThread",
            daemon=True,
        )
        self._thread.start()

    def register(self, op_id: str) -> concurrent.futures.Future[Response]:
        """Register a pending operation ID and return a Future that resolves with its Response."""
        with self._lock:
            if self._is_stopped:
                raise ProcessDiedError("Dispatcher has stopped; process is dead")
            if op_id in self._pending:
                raise DuplicateOperationIdError(f"Operation ID '{op_id}' is already registered")
            fut: concurrent.futures.Future[Response] = concurrent.futures.Future()
            self._pending[op_id] = fut
            return fut

    def deregister(self, op_id: str) -> Optional[concurrent.futures.Future[Response]]:
        """Remove a pending future (e.g. on client timeout)."""
        with self._lock:
            return self._pending.pop(op_id, None)

    def get_pending_op_ids(self) -> List[str]:
        """Return a snapshot of all currently unresolved operation IDs."""
        with self._lock:
            return list(self._pending.keys())

    def _reader_loop(self) -> None:
        try:
            for raw_line in iter(self.stdout_stream.readline, b""):
                if not raw_line:
                    break
                line = raw_line.decode("utf-8", errors="replace")
                line_str = line.strip()
                if not line_str:
                    continue

                try:
                    resp = parse_response(line_str)
                except ProtocolParseError as e:
                    with self._lock:
                        self.protocol_errors.append(f"Parse error: {e} in '{line_str}'")
                    continue

                target_fut: Optional[concurrent.futures.Future[Response]] = None
                with self._lock:
                    target_fut = self._pending.pop(resp.op_id, None)
                    if target_fut is None:
                        self.unknown_responses.append(line_str)

                if target_fut is not None and not target_fut.done():
                    target_fut.set_result(resp)

        except (ValueError, OSError):
            # Stream closed or pipe broken
            pass
        finally:
            self._fail_all_pending("Child process stdout closed (EOF or killed)")

    def _fail_all_pending(self, reason: str) -> None:
        """Mark all unresolved pending futures with ProcessDiedError when child dies."""
        with self._lock:
            self._is_stopped = True
            remaining = list(self._pending.values())
            self._pending.clear()

        err = ProcessDiedError(reason)
        for fut in remaining:
            if not fut.done():
                fut.set_exception(err)

    def stop(self, timeout: float = 2.0) -> None:
        """Signal stop and join reader thread."""
        self._fail_all_pending("Dispatcher explicitly stopped")
        if self._thread and self._thread.is_alive():
            self._thread.join(timeout=timeout)


"""Asynchronous non-blocking stderr drainer preventing OS pipe-buffer deadlocks."""

from __future__ import annotations

import collections
import io
import threading
from pathlib import Path
from typing import Deque, List, Optional


class StderrDrainer:
    """Consumes child stderr in a background thread to prevent 64 KiB pipe deadlocks.

    Buffers the last N lines in memory and optionally writes all output to a log file.
    """

    def __init__(
        self,
        stderr_stream: io.BufferedReader,
        log_file_path: Optional[Path] = None,
        max_buffer_lines: int = 1000,
    ) -> None:
        self.stderr_stream = stderr_stream
        self.log_file_path = log_file_path
        self.max_buffer_lines = max_buffer_lines
        self.ring_buffer: Deque[str] = collections.deque(maxlen=max_buffer_lines)
        self._lock = threading.Lock()
        self._thread: Optional[threading.Thread] = None
        self._stop_event = threading.Event()
        self._log_file = None

        if self.log_file_path:
            self.log_file_path.parent.mkdir(parents=True, exist_ok=True)
            self._log_file = open(self.log_file_path, "a", encoding="utf-8", buffering=1)

    def start(self) -> None:
        self._thread = threading.Thread(
            target=self._drain_loop,
            name="StderrDrainerThread",
            daemon=True,
        )
        self._thread.start()

    def _drain_loop(self) -> None:
        try:
            for raw_line in iter(self.stderr_stream.readline, b""):
                if not raw_line:
                    break
                line = raw_line.decode("utf-8", errors="replace")
                with self._lock:
                    self.ring_buffer.append(line)
                    if self._log_file and not self._log_file.closed:
                        self._log_file.write(line)
                        self._log_file.flush()
        except (ValueError, OSError):
            # Stream closed or process terminated
            pass
        finally:
            if self._log_file and not self._log_file.closed:
                try:
                    self._log_file.flush()
                    self._log_file.close()
                except OSError:
                    pass

    def stop(self, timeout: float = 2.0) -> None:
        self._stop_event.set()
        if self._thread and self._thread.is_alive():
            self._thread.join(timeout=timeout)

    def get_recent_lines(self) -> List[str]:
        with self._lock:
            return list(self.ring_buffer)

    def get_recent_text(self) -> str:
        return "".join(self.get_recent_lines())


"""Process controller managing lifecycle, pipes, SIGKILL, and cleanup for kvdb serve."""

from __future__ import annotations

import atexit
import fcntl
import os
import signal
import subprocess
import time
from pathlib import Path
from typing import List, Optional

from torture_tool.process.stderr_drainer import StderrDrainer


class ProcessError(Exception):
    """Base error for child process operations."""

    pass


class ProcessStartupError(ProcessError):
    """Raised when child fails to start or dies immediately."""

    pass


class ProcessTimeoutError(ProcessError):
    """Raised when process operation times out."""

    pass


# Global tracking to prevent leaked children on abnormal harness exit
_ACTIVE_CONTROLLERS: List["ChildProcessController"] = []


def _cleanup_all_active_processes() -> None:
    for ctrl in list(_ACTIVE_CONTROLLERS):
        try:
            ctrl.cleanup()
        except Exception:
            pass


atexit.register(_cleanup_all_active_processes)


class ChildProcessController:
    """Manages the lifecycle of a single `./kvdb serve <db_path>` child process."""

    def __init__(
        self,
        command: List[str],
        db_path: Path,
        log_dir: Optional[Path] = None,
        startup_timeout: float = 10.0,
    ) -> None:
        self.command = command
        self.db_path = Path(db_path)
        self.log_dir = Path(log_dir) if log_dir else None
        self.startup_timeout = startup_timeout

        self.process: Optional[subprocess.Popen] = None
        self.stderr_drainer: Optional[StderrDrainer] = None
        self._is_killed = False

    @property
    def pid(self) -> Optional[int]:
        return self.process.pid if self.process else None

    def start(self) -> None:
        """Spawn the child process with piped stdin, stdout, and drained stderr."""
        if self.is_alive():
            raise ProcessError("Child process is already running")

        stderr_log = self.log_dir / "engine_stderr.log" if self.log_dir else None

        try:
            self.process = subprocess.Popen(
                self.command,
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                bufsize=0,  # Unbuffered binary streams
                start_new_session=True,  # Isolate in its own process group
            )
        except OSError as e:
            raise ProcessStartupError(f"Failed to execute command '{self.command}': {e}") from e

        # Register for emergency atexit cleanup
        _ACTIVE_CONTROLLERS.append(self)

        # Start stderr drainer immediately
        if self.process.stderr:
            self.stderr_drainer = StderrDrainer(
                stderr_stream=self.process.stderr,
                log_file_path=stderr_log,
            )
            self.stderr_drainer.start()

        # Check for immediate crash on startup
        start_wait = time.monotonic()
        exit_code = None
        while time.monotonic() - start_wait < 0.25:
            exit_code = self.process.poll()
            if exit_code is not None:
                break
            time.sleep(0.02)

        if exit_code is not None:
            recent_err = self.stderr_drainer.get_recent_text() if self.stderr_drainer else ""
            if self.stderr_drainer:
                self.stderr_drainer.stop(timeout=1.0)
            for stream in (self.process.stdin, self.process.stdout, self.process.stderr):
                if stream and not stream.closed:
                    try:
                        stream.close()
                    except OSError:
                        pass
            self.cleanup()
            raise ProcessStartupError(
                f"Process '{self.command[0]}' died immediately on startup with exit code {exit_code}.\n"
                f"Stderr output:\n{recent_err}"
            )

        self._is_killed = False

    def is_alive(self) -> bool:
        """Check whether child process is currently running."""
        if not self.process:
            return False
        return self.process.poll() is None

    def kill(self, timeout: float = 5.0) -> int:
        """Ungracefully terminate child with POSIX SIGKILL (kill -9) and reap exit code."""
        if not self.process:
            return 0

        self._is_killed = True
        pid = self.process.pid

        # Send raw SIGKILL to the process group
        try:
            pgid = os.getpgid(pid)
            os.killpg(pgid, signal.SIGKILL)
        except (ProcessLookupError, PermissionError):
            # Fallback to direct process signal if process group unavailable
            try:
                os.kill(pid, signal.SIGKILL)
            except ProcessLookupError:
                pass

        # Reap child to prevent zombies and ensure kernel resources (flock) are freed
        try:
            exit_code = self.process.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            # Force secondary direct kill
            try:
                os.kill(pid, signal.SIGKILL)
            except OSError:
                pass
            exit_code = self.process.wait(timeout=timeout)

        # Stop stderr drainer
        if self.stderr_drainer:
            self.stderr_drainer.stop(timeout=1.0)

        # Explicitly close remaining pipe streams to prevent resource warnings
        for stream in (self.process.stdin, self.process.stdout, self.process.stderr):
            if stream and not stream.closed:
                try:
                    stream.close()
                except OSError:
                    pass

        # Deregister from active controllers
        if self in _ACTIVE_CONTROLLERS:
            _ACTIVE_CONTROLLERS.remove(self)

        return exit_code

    def cleanup(self) -> None:
        """Defensive kill and cleanup guaranteed safe to call multiple times."""
        if self.is_alive():
            try:
                self.kill(timeout=2.0)
            except Exception:
                pass

        if self in _ACTIVE_CONTROLLERS:
            _ACTIVE_CONTROLLERS.remove(self)

    def verify_lock_released(self, timeout: float = 2.0) -> bool:
        """Verify the process released its kernel file lock (flock) on db_path."""
        if not self.db_path.exists():
            return True

        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            try:
                with open(self.db_path, "r+b") as f:
                    fcntl.flock(f.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
                    fcntl.flock(f.fileno(), fcntl.LOCK_UN)
                return True
            except (BlockingIOError, OSError):
                time.sleep(0.05)
        return False

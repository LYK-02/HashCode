"""Secondary independent OS process simulating competing CLI clients exercising flock(LOCK_EX) collisions."""

from __future__ import annotations

import subprocess
import threading
import time
from pathlib import Path
from typing import Optional

from torture_tool.oracle.journal import OracleJournal
from torture_tool.protocol.spec import PutRequest


class CompetingCLIClient(threading.Thread):
    """Spawns secondary independent OS processes running kvdb put/get to exercise file-level flock collisions."""

    def __init__(
        self,
        kvdb_bin: str | Path,
        db_path: str | Path,
        journal: Optional[OracleJournal] = None,
        iteration: int = 1,
        interval_ms: float = 15.0,
    ) -> None:
        super().__init__(name="CompetingCLIClient", daemon=True)
        self.kvdb_bin = str(kvdb_bin)
        self.db_path = str(db_path)
        self.journal = journal
        self.iteration = iteration
        self.interval_sec = max(0.005, interval_ms / 1000.0)
        self._stop_event = threading.Event()
        self._current_proc: Optional[subprocess.Popen] = None
        self._proc_lock = threading.Lock()

        self.attempts = 0
        self.lock_collisions = 0
        self.successes = 0

    def stop(self) -> None:
        self._stop_event.set()
        with self._proc_lock:
            if self._current_proc and self._current_proc.poll() is None:
                try:
                    self._current_proc.terminate()
                except Exception:
                    pass

    def run(self) -> None:
        while not self._stop_event.is_set():
            time.sleep(self.interval_sec)
            if self._stop_event.is_set():
                break

            self.attempts += 1
            op_id = f"cli_{self.iteration}_{self.attempts}"
            key = f"cli_{self.iteration}_{self.attempts}"
            val = f"v_{self.iteration}_{self.attempts}"

            if self.journal:
                try:
                    req = PutRequest(
                        op_id=op_id,
                        key=key.encode("utf-8"),
                        value=val.encode("utf-8"),
                    )
                    self.journal.log_start(self.iteration, req)
                except Exception:
                    pass

            try:
                cmd = [self.kvdb_bin, "put", self.db_path, key, val]
                proc = subprocess.Popen(
                    cmd,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                )
                with self._proc_lock:
                    self._current_proc = proc

                stdout, stderr = proc.communicate(timeout=1.0)

                with self._proc_lock:
                    self._current_proc = None

                if proc.returncode == 0:
                    self.successes += 1
                    if self.journal:
                        self.journal.log_ack(self.iteration, op_id, seq_num=0)
                elif proc.returncode > 0 and ("lock" in (stderr or "").lower() or "locked" in (stderr or "").lower()):
                    self.lock_collisions += 1
                    if self.journal:
                        self.journal.log_fail(self.iteration, op_id, "LOCKED")
                else:
                    self.lock_collisions += 1
                    # In-flight interruption (e.g. signal termination on kill): leave unresolved in journal
            except Exception:
                with self._proc_lock:
                    self._current_proc = None

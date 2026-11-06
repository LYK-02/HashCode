"""Append-only thread-safe JSONL event journal for independent oracle."""

from __future__ import annotations

import json
import os
import threading
import time
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any, Dict, Iterator, List, Optional

from torture_tool.protocol.spec import (
    BatchRequest,
    DelRequest,
    PingRequest,
    PutRequest,
    Request,
)
from torture_tool.protocol.utils import bytes_to_hex, hex_to_bytes


@dataclass(frozen=True)
class JournalEvent:
    type: str  # "START", "ACK", "FAIL", "ITER_START", "ITER_END"
    ts_ns: int
    iteration: int
    op_id: Optional[str] = None
    data: Optional[Dict[str, Any]] = None

    def to_json_line(self) -> str:
        d: Dict[str, Any] = {
            "type": self.type,
            "ts_ns": self.ts_ns,
            "iteration": self.iteration,
        }
        if self.op_id is not None:
            d["op_id"] = self.op_id
        if self.data is not None:
            d.update(self.data)
        return json.dumps(d, separators=(",", ":")) + "\n"

    @classmethod
    def from_json_line(cls, line: str) -> JournalEvent:
        obj = json.loads(line)
        t = obj.pop("type")
        ts = obj.pop("ts_ns")
        it = obj.pop("iteration")
        op_id = obj.pop("op_id", None)
        return cls(type=t, ts_ns=ts, iteration=it, op_id=op_id, data=obj if obj else None)


class OracleJournal:
    """Thread-safe append-only journal capturing operation events to disk."""

    def __init__(self, journal_path: Path, sync_every_write: bool = False) -> None:
        self.journal_path = Path(journal_path)
        self.sync_every_write = sync_every_write
        self._lock = threading.Lock()
        self.journal_path.parent.mkdir(parents=True, exist_ok=True)
        self._file = open(self.journal_path, "a", encoding="utf-8", buffering=1)  # line buffered

    def _append(self, event: JournalEvent) -> None:
        line = event.to_json_line()
        with self._lock:
            self._file.write(line)
            self._file.flush()
            if self.sync_every_write:
                os.fsync(self._file.fileno())

    def log_iteration_start(self, iteration: int) -> None:
        event = JournalEvent(
            type="ITER_START",
            ts_ns=time.monotonic_ns(),
            iteration=iteration,
        )
        self._append(event)

    def log_iteration_end(self, iteration: int, status: str) -> None:
        event = JournalEvent(
            type="ITER_END",
            ts_ns=time.monotonic_ns(),
            iteration=iteration,
            data={"status": status},
        )
        self._append(event)

    def log_start(self, iteration: int, request: Request) -> None:
        """Record intent before transmitting across the pipe."""
        ts = time.monotonic_ns()
        op_id = request.op_id

        if isinstance(request, PutRequest):
            data = {
                "kind": "PUT",
                "key_hex": bytes_to_hex(request.key),
                "val_hex": bytes_to_hex(request.value),
            }
        elif isinstance(request, DelRequest):
            data = {
                "kind": "DEL",
                "key_hex": bytes_to_hex(request.key),
            }
        elif isinstance(request, BatchRequest):
            items_data = []
            for item in request.items:
                items_data.append(
                    {
                        "k": bytes_to_hex(item.key),
                        "v": bytes_to_hex(item.value) if item.value is not None else None,
                    }
                )
            data = {
                "kind": "BATCH",
                "items": items_data,
            }
        elif isinstance(request, PingRequest):
            data = {"kind": "PING"}
        else:
            data = {"kind": "UNKNOWN"}

        self._append(JournalEvent(type="START", ts_ns=ts, iteration=iteration, op_id=op_id, data=data))

    def log_ack(self, iteration: int, op_id: str, seq_num: Optional[int] = None) -> None:
        """Record durable ACK only after response parsed from engine stdout."""
        ts = time.monotonic_ns()
        data = {}
        if seq_num is not None:
            data["seq_num"] = seq_num
        self._append(JournalEvent(type="ACK", ts_ns=ts, iteration=iteration, op_id=op_id, data=data))

    def log_fail(self, iteration: int, op_id: str, code: str, msg: str = "") -> None:
        """Record explicit failure response ERR from engine."""
        ts = time.monotonic_ns()
        data = {"code": code, "msg": msg}
        self._append(JournalEvent(type="FAIL", ts_ns=ts, iteration=iteration, op_id=op_id, data=data))

    def close(self) -> None:
        with self._lock:
            if not self._file.closed:
                self._file.flush()
                os.fsync(self._file.fileno())
                self._file.close()

    @classmethod
    def read_events(cls, path: Path) -> Iterator[JournalEvent]:
        """Read all events from an existing journal file."""
        with open(path, "r", encoding="utf-8") as f:
            for line in f:
                stripped = line.strip()
                if stripped:
                    yield JournalEvent.from_json_line(stripped)


"""Concurrent worker client thread issuing operations to kvdb serve child."""

from __future__ import annotations

import concurrent.futures
import threading
import time
from dataclasses import dataclass, field
from typing import List, Optional

from torture_tool.concurrency.dispatcher import (
    ProcessDiedError,
    ResponseDispatcher,
)
from torture_tool.concurrency.writer import StdinWriter
from torture_tool.generator.workload import WorkloadGenerator
from torture_tool.oracle.journal import OracleJournal
from torture_tool.protocol.spec import (
    AckResponse,
    ErrResponse,
    PongResponse,
    Request,
    Response,
)


@dataclass
class WorkerStats:
    worker_id: str
    operations_started: int = 0
    acks_received: int = 0
    errors_received: int = 0
    inflight_at_stop: int = 0
    exceptions_encountered: int = 0
    latencies_ms: List[float] = field(default_factory=list)


class WorkerThread(threading.Thread):
    """Independent worker thread generating requests and handling response futures."""

    def __init__(
        self,
        worker_id: str | int,
        workload_generator: WorkloadGenerator,
        dispatcher: ResponseDispatcher,
        writer: StdinWriter,
        journal: OracleJournal,
        iteration: int = 1,
        request_timeout: float = 10.0,
    ) -> None:
        super().__init__(name=f"Worker-{worker_id}", daemon=True)
        self.worker_id = str(worker_id)
        self.generator = workload_generator
        self.dispatcher = dispatcher
        self.writer = writer
        self.journal = journal
        self.iteration = iteration
        self.request_timeout = request_timeout

        self._stop_event = threading.Event()
        self.stats = WorkerStats(worker_id=self.worker_id)
        self.last_in_flight_op: Optional[Request] = None

    def stop(self) -> None:
        """Signal worker to stop generating new requests."""
        self._stop_event.set()

    def run(self) -> None:
        while not self._stop_event.is_set():
            try:
                req = self.generator.next_operation()
                op_id = req.op_id
                self.last_in_flight_op = req

                # 1. Register future before transmitting
                t_op_start = time.perf_counter()
                fut = self.dispatcher.register(op_id)

                # 2. Log START event to independent oracle journal
                self.journal.log_start(self.iteration, req)
                self.stats.operations_started += 1

                # 3. Write request atomically to stdin
                try:
                    self.writer.write_request(req)
                except Exception:
                    # Write failed (e.g. Broken pipe right as child was SIGKILLed)
                    self.dispatcher.deregister(op_id)
                    self.stats.inflight_at_stop += 1
                    break

                # 4. Await response resolution or process death
                try:
                    resp: Response = fut.result(timeout=self.request_timeout)
                    self.last_in_flight_op = None

                    if isinstance(resp, AckResponse):
                        self.stats.acks_received += 1
                        lat_ms = (time.perf_counter() - t_op_start) * 1000.0
                        self.stats.latencies_ms.append(lat_ms)
                        self.journal.log_ack(self.iteration, op_id, seq_num=resp.seq_num)
                    elif isinstance(resp, ErrResponse):
                        self.stats.errors_received += 1
                        self.journal.log_fail(
                            self.iteration, op_id, code=resp.error_code, msg=resp.message
                        )
                    elif isinstance(resp, PongResponse):
                        pass

                except ProcessDiedError:
                    # Normal when child is terminated by crash injection (SIGKILL)
                    self.stats.inflight_at_stop += 1
                    break
                except concurrent.futures.TimeoutError:
                    # Timeout waiting for response
                    self.stats.inflight_at_stop += 1
                    self.stats.exceptions_encountered += 1
                    break

            except Exception:
                self.stats.exceptions_encountered += 1
                break

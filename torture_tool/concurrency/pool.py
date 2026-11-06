"""Worker pool managing the lifecycle of concurrent client threads."""

from __future__ import annotations

import time
from typing import List

from torture_tool.concurrency.dispatcher import ResponseDispatcher
from torture_tool.concurrency.worker import WorkerStats, WorkerThread
from torture_tool.concurrency.writer import StdinWriter
from torture_tool.generator.prng import SeedManager
from torture_tool.generator.workload import WorkloadConfig, WorkloadGenerator
from torture_tool.oracle.journal import OracleJournal


class WorkerPool:
    """Orchestrates start, stop, and aggregation of N concurrent worker threads."""

    def __init__(
        self,
        num_workers: int,
        seed_manager: SeedManager,
        dispatcher: ResponseDispatcher,
        writer: StdinWriter,
        journal: OracleJournal,
        workload_config: WorkloadConfig,
        iteration: int = 1,
        request_timeout: float = 10.0,
    ) -> None:
        self.num_workers = max(2, num_workers)
        self.seed_manager = seed_manager
        self.dispatcher = dispatcher
        self.writer = writer
        self.journal = journal
        self.workload_config = workload_config
        self.iteration = iteration
        self.request_timeout = request_timeout

        self.workers: List[WorkerThread] = []

    def start(self) -> None:
        """Instantiate and start all concurrent worker threads."""
        self.workers = []
        for i in range(1, self.num_workers + 1):
            w_id = f"w{i}"
            prng = self.seed_manager.get_worker_prng(w_id, iteration=self.iteration)
            gen = WorkloadGenerator(
                worker_id=w_id,
                prng=prng,
                config=self.workload_config,
                iteration=self.iteration,
            )
            w = WorkerThread(
                worker_id=w_id,
                workload_generator=gen,
                dispatcher=self.dispatcher,
                writer=self.writer,
                journal=self.journal,
                iteration=self.iteration,
                request_timeout=self.request_timeout,
            )
            self.workers.append(w)
            w.start()

    def stop_and_join(self, timeout: float = 2.0) -> List[WorkerStats]:
        """Signal all workers to stop and join their threads."""
        for w in self.workers:
            w.stop()

        deadline = time.monotonic() + timeout
        for w in self.workers:
            rem = max(0.01, deadline - time.monotonic())
            w.join(timeout=rem)

        return [w.stats for w in self.workers]

    def get_aggregate_stats(self) -> WorkerStats:
        total = WorkerStats(worker_id="aggregate")
        for w in self.workers:
            total.operations_started += w.stats.operations_started
            total.acks_received += w.stats.acks_received
            total.errors_received += w.stats.errors_received
            total.inflight_at_stop += w.stats.inflight_at_stop
            total.exceptions_encountered += w.stats.exceptions_encountered
        return total


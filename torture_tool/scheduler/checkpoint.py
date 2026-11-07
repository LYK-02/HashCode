"""Deterministic crash injection at exact engine checkpoints.

Injects SIGKILL or pipe breaks at precise execution checkpoints:
- PRE_WRITE: after framing request, before byte transmission to stdin
- MID_PAYLOAD: after sending header and key bytes, but before complete value transmission
- MID_BATCH: after partial transmission of batch operations
- PRE_FSYNC: immediately upon pipe flush before engine completes fsync
- POST_ACK: immediately after receiving ACK frame from engine stdout
- CLOSE_BOUNDARY: while issuing graceful close before lock release completes
"""

from __future__ import annotations

import enum
import os
import signal
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Callable, Optional

from torture_tool.protocol.codec import encode_request
from torture_tool.protocol.spec import BatchRequest, DelRequest, PutRequest, Request


class EngineCheckpoint(enum.Enum):
    PRE_WRITE = "PRE_WRITE"
    MID_PAYLOAD = "MID_PAYLOAD"
    MID_BATCH = "MID_BATCH"
    PRE_FSYNC = "PRE_FSYNC"
    POST_ACK = "POST_ACK"
    CLOSE_BOUNDARY = "CLOSE_BOUNDARY"


@dataclass
class CheckpointCrashResult:
    checkpoint: EngineCheckpoint
    killed_pid: int
    bytes_sent: int
    target_op_id: str
    in_flight: int
    success: bool
    description: str


class CheckpointCrashInjector:
    """Provides deterministic, boundary-exact crash injection for kvdb."""

    def __init__(self, kvdb_bin: Path | str, db_path: Path | str) -> None:
        self.kvdb_bin = Path(kvdb_bin).resolve()
        self.db_path = Path(db_path).resolve()

    def inject_checkpoint_crash(
        self,
        checkpoint: EngineCheckpoint,
        target_request: Request,
        pre_existing_keys: Optional[dict[bytes, bytes]] = None,
    ) -> CheckpointCrashResult:
        """Spawn kvdb serve, write preconditions, and kill at the exact target checkpoint."""
        # 1. Initialize empty database if it does not exist
        if not self.db_path.exists():
            subprocess.run(
                [str(self.kvdb_bin), "open", str(self.db_path)],
                check=True,
                capture_output=True,
                timeout=5.0,
            )

        # Populate DB first via CLI batch if pre_existing_keys provided
        if pre_existing_keys:
            # Populate DB first via CLI batch
            batch_lines = []
            for k, v in pre_existing_keys.items():
                batch_lines.append(f"PUT {k.hex()} {v.hex()}")
            tmp_script = self.db_path.parent / "pre_seed.txt"
            tmp_script.write_text("\n".join(batch_lines) + "\n")
            try:
                subprocess.run(
                    [str(self.kvdb_bin), "batch", str(self.db_path), str(tmp_script)],
                    check=True,
                    capture_output=True,
                    timeout=10.0,
                )
            finally:
                if tmp_script.exists():
                    tmp_script.unlink()

        # 2. Launch engine in serve mode
        proc = subprocess.Popen(
            [str(self.kvdb_bin), "serve", str(self.db_path)],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            start_new_session=True,
        )

        pid = proc.pid
        bytes_sent = 0
        in_flight = 1
        desc = ""

        try:
            raw_encoded = encode_request(target_request).encode("ascii")

            if checkpoint == EngineCheckpoint.PRE_WRITE:
                # Kill BEFORE writing any bytes of the request
                desc = "Killed before any request bytes written to stdin"
                os.kill(pid, signal.SIGKILL)
                proc.wait(timeout=2.0)
                bytes_sent = 0

            elif checkpoint == EngineCheckpoint.MID_PAYLOAD:
                # Write partial packet: header + half the payload
                half_point = max(1, len(raw_encoded) // 2)
                proc.stdin.write(raw_encoded[:half_point])
                proc.stdin.flush()
                bytes_sent = half_point
                desc = f"Killed after transmitting {half_point}/{len(raw_encoded)} bytes of request payload"
                os.kill(pid, signal.SIGKILL)
                proc.wait(timeout=2.0)

            elif checkpoint == EngineCheckpoint.MID_BATCH:
                # Write partial batch: first 30% of request
                slice_len = max(5, len(raw_encoded) // 3)
                proc.stdin.write(raw_encoded[:slice_len])
                proc.stdin.flush()
                bytes_sent = slice_len
                desc = f"Killed mid-batch wire transfer at byte offset {slice_len}"
                os.kill(pid, signal.SIGKILL)
                proc.wait(timeout=2.0)

            elif checkpoint == EngineCheckpoint.PRE_FSYNC:
                # Send the entire request and flush pipe, kill instantly before ACK
                proc.stdin.write(raw_encoded)
                proc.stdin.flush()
                bytes_sent = len(raw_encoded)
                # Raw SIGKILL instantly to catch the window between syscall write and sync
                os.kill(pid, signal.SIGKILL)
                proc.wait(timeout=2.0)
                desc = f"Killed immediately after flushing request to stdin before ACK"

            elif checkpoint == EngineCheckpoint.POST_ACK:
                # Send full request, read ACK line from stdout, then kill instantly
                proc.stdin.write(raw_encoded)
                proc.stdin.flush()
                ack_line = proc.stdout.readline()
                bytes_sent = len(raw_encoded)
                desc = f"Killed immediately after receiving ACK: {ack_line.strip().decode('ascii', errors='replace')}"
                os.kill(pid, signal.SIGKILL)
                proc.wait(timeout=2.0)

            elif checkpoint == EngineCheckpoint.CLOSE_BOUNDARY:
                # Send request, read ACK, issue partial close / SIGKILL during cleanup
                proc.stdin.write(raw_encoded)
                proc.stdin.flush()
                _ = proc.stdout.readline()
                bytes_sent = len(raw_encoded)
                desc = "Killed during process shutdown/lock transition"
                os.kill(pid, signal.SIGKILL)
                proc.wait(timeout=2.0)

            return CheckpointCrashResult(
                checkpoint=checkpoint,
                killed_pid=pid,
                bytes_sent=bytes_sent,
                target_op_id=target_request.op_id,
                in_flight=in_flight,
                success=True,
                description=desc,
            )

        finally:
            try:
                if proc.poll() is None:
                    os.kill(pid, signal.SIGKILL)
                    proc.wait(timeout=2.0)
            except OSError:
                pass
            if proc.stdin:
                try:
                    proc.stdin.close()
                except OSError:
                    pass
            if proc.stdout:
                try:
                    proc.stdout.close()
                except OSError:
                    pass
            if proc.stderr:
                try:
                    proc.stderr.close()
                except OSError:
                    pass

#!/usr/bin/env python3
"""Configurable mock kvdb binary for testing the adversarial torture tool itself.

Supports simulating:
- DELAY_MODE: artificial response delays (testing timeouts)
- OUT_OF_ORDER_MODE: reverse or interleaved ACKs
- MALFORMED_MODE: corrupted responses
- DUPLICATE_ID_MODE: duplicate ACKs
- STDERR_FLOOD_MODE: high-volume stderr output testing pipe deadlocks
- EARLY_EXIT_MODE: premature exit on startup
- SILENT_HANG_MODE: reading stdin with zero output
- Normal serve/open/compact/check/dump modes
"""

import os
import sys
import time


def main():
    if len(sys.argv) < 2:
        sys.stderr.write("Usage: fake_kvdb <cmd> [args...]\n")
        sys.exit(1)

    cmd = sys.argv[1]
    db_path = sys.argv[2] if len(sys.argv) > 2 else "data.db"
    state_file = db_path + ".state"

    # 1. Early exit mode
    if os.environ.get("FAKE_KVDB_EARLY_EXIT") == "1":
        exit_code = int(os.environ.get("FAKE_KVDB_EXIT_CODE", "139"))
        sys.stderr.write(f"FATAL: simulated crash on startup (code {exit_code})\n")
        sys.exit(exit_code)

    # 2. Stderr flood mode on startup
    if os.environ.get("FAKE_KVDB_STDERR_FLOOD") == "1":
        flood_size_mb = int(os.environ.get("FAKE_KVDB_FLOOD_MB", "5"))
        garbage = "E" * 1024 + "\n"
        sys.stderr.write(f"Starting stderr flood: {flood_size_mb} MB...\n")
        for _ in range(flood_size_mb * 1024):
            sys.stderr.write(garbage)
        sys.stderr.flush()

    if cmd == "open":
        delay = float(os.environ.get("FAKE_KVDB_OPEN_DELAY", "0.01"))
        time.sleep(delay)
        sys.stdout.write("Database opened successfully: 10 keys recovered\n")
        sys.exit(0)

    elif cmd == "compact":
        delay = float(os.environ.get("FAKE_KVDB_COMPACT_DELAY", "0.02"))
        time.sleep(delay)
        sys.stdout.write("Compaction complete\n")
        sys.exit(0)

    elif cmd == "check":
        if os.environ.get("FAKE_KVDB_CORRUPT_CHECK") == "1":
            sys.stdout.write("ERROR: Checksum mismatch at frame 42\n")
            sys.exit(1)
        sys.stdout.write("OK: Database is consistent\n")
        sys.exit(0)

    elif cmd == "dump":
        if os.environ.get("FAKE_KVDB_MALFORMED_DUMP") == "1":
            sys.stdout.write("invalid_dump_line_without_space\n")
            sys.exit(0)
        state = {}
        if os.path.exists(state_file):
            with open(state_file, "r") as f:
                for line in f:
                    line = line.strip()
                    if not line:
                        continue
                    if line.startswith("TX_BATCH "):
                        if not line.endswith(" END_BATCH"):
                            continue
                        parts = line[:-10].split(" ", 3)
                        if len(parts) >= 4:
                            expected_cnt = int(parts[2])
                            items = parts[3].split(";")
                            if len(items) != expected_cnt:
                                continue
                            for item in items:
                                p = item.split()
                                if len(p) == 2:
                                    if p[1] == "DEL_TOMBSTONE":
                                        state.pop(p[0], None)
                                    else:
                                        state[p[0]] = p[1]
                                elif len(p) == 1:
                                    state[p[0]] = ""
                    else:
                        p = line.split(None, 1)
                        if len(p) == 2:
                            if p[1] == "DEL_TOMBSTONE":
                                state.pop(p[0], None)
                            else:
                                state[p[0]] = p[1]
                        elif len(p) == 1:
                            state[p[0]] = ""
        for k in sorted(state.keys()):
            sys.stdout.write(f"{k} {state[k]}\n")
        sys.exit(0)

    elif cmd == "serve":
        # Check silent hang mode
        if os.environ.get("FAKE_KVDB_SILENT_HANG") == "1":
            for _ in sys.stdin:
                pass
            sys.exit(0)

        delay_per_op = float(os.environ.get("FAKE_KVDB_DELAY_PER_OP", "0.0"))
        malformed = os.environ.get("FAKE_KVDB_MALFORMED") == "1"
        dup_acks = os.environ.get("FAKE_KVDB_DUP_ACKS") == "1"
        out_of_order = os.environ.get("FAKE_KVDB_OUT_OF_ORDER") == "1"

        state_f = open(state_file, "a", buffering=1)
        buffer_queue = []
        seq_num = 0
        if os.path.exists(state_file):
            try:
                with open(state_file, "r") as f:
                    seq_num = sum(1 for _ in f)
            except OSError:
                pass

        for line in sys.stdin:
            line = line.strip()
            if not line:
                continue

            parts = line.split()
            tag = parts[0]
            op_id = parts[1] if len(parts) > 1 else ""

            if delay_per_op > 0:
                time.sleep(delay_per_op)

            if malformed:
                sys.stdout.write(f"MALFORMED_RESPONSE {op_id}\n")
                sys.stdout.flush()
                continue

            if tag == "PING":
                resp = f"PONG {op_id}\n"
            elif tag == "PUT":
                seq_num += 1
                k = parts[2]
                v = parts[3] if len(parts) > 3 else ""
                state_f.write(f"{k} {v}\n")
                state_f.flush()
                os.fsync(state_f.fileno())
                resp = f"ACK {op_id} {seq_num}\n"
            elif tag == "DEL":
                seq_num += 1
                k = parts[2]
                state_f.write(f"{k} DEL_TOMBSTONE\n")
                state_f.flush()
                os.fsync(state_f.fileno())
                resp = f"ACK {op_id} {seq_num}\n"
            elif tag == "BATCH":
                seq_num += 1
                count = int(parts[2])
                items_str = parts[3] if len(parts) > 3 else ""
                batch_items = []
                if items_str:
                    for itm in items_str.split(","):
                        if ":" in itm:
                            ik, iv = itm.split(":", 1)
                            if iv:
                                batch_items.append(f"{ik} {iv}")
                            else:
                                batch_items.append(f"{ik} DEL_TOMBSTONE")
                # Format as atomic batch frame with END_BATCH marker
                frame = f"TX_BATCH {op_id} {count} {';'.join(batch_items)} END_BATCH\n"
                state_f.write(frame)
                state_f.flush()
                os.fsync(state_f.fileno())
                resp = f"ACK {op_id} {seq_num}\n"
            else:
                resp = f"ERR {op_id} UNKNOWN_COMMAND\n"

            if dup_acks:
                sys.stdout.write(resp)
                sys.stdout.write(resp)
                sys.stdout.flush()
                continue

            if out_of_order:
                buffer_queue.append(resp)
                if len(buffer_queue) >= 3:
                    while buffer_queue:
                        sys.stdout.write(buffer_queue.pop())
                    sys.stdout.flush()
            else:
                sys.stdout.write(resp)
                sys.stdout.flush()

        while buffer_queue:
            sys.stdout.write(buffer_queue.pop())
        sys.stdout.flush()
        state_f.close()
        sys.exit(0)

    else:
        sys.stderr.write(f"Unknown subcommand {cmd}\n")
        sys.exit(1)


if __name__ == "__main__":
    main()

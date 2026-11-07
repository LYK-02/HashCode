"""Unit and integration tests for Phase T8: Maintenance Modes (Recovery, Compaction, Disk-Full)."""

import os
import shutil
import stat
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path

from torture_tool.cli import RunConfig
from torture_tool.modes.compaction_crash import CompactionCrashRunner
from torture_tool.modes.disk_full import DiskFullRunner
from torture_tool.modes.recovery_crash import RecoveryCrashRunner


class TestMaintenanceModes(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.mkdtemp()
        self.kvdb_mock_script = Path(self.tmpdir) / "kvdb"

        # Mock kvdb supporting open, serve, check, dump, compact, and --max-bytes
        content = (
            "#!/usr/bin/env python3\n"
            "import sys, os, time\n"
            "cmd = sys.argv[1] if len(sys.argv) > 1 else ''\n"
            "db_path = sys.argv[2] if len(sys.argv) > 2 else 'data.db'\n"
            "log_path = db_path + '.state'\n"
            "quota_cfg_path = db_path + '.quota'\n"
            "\n"
            "if '--max-bytes' in sys.argv:\n"
            "    idx = sys.argv.index('--max-bytes')\n"
            "    with open(quota_cfg_path, 'w') as f_q:\n"
            "        f_q.write(sys.argv[idx + 1])\n"
            "\n"
            "max_bytes = 100000000\n"
            "if os.path.exists(quota_cfg_path):\n"
            "    with open(quota_cfg_path, 'r') as f_q:\n"
            "        max_bytes = int(f_q.read().strip())\n"
            "\n"
            "if cmd == 'open':\n"
            "    time.sleep(0.01)\n"
            "    sys.stdout.write('Database opened successfully: 42 keys recovered in 0.005s\\n')\n"
            "    sys.exit(0)\n"
            "elif cmd == 'compact':\n"
            "    time.sleep(0.01)\n"
            "    # Compact shrinks state file by discarding deleted keys\n"
            "    state = {}\n"
            "    if os.path.exists(log_path):\n"
            "        with open(log_path, 'r') as f:\n"
            "            for l in f:\n"
            "                p = l.strip().split()\n"
            "                if len(p) == 2:\n"
            "                    if p[1] == '__DEL__': state.pop(p[0], None)\n"
            "                    else: state[p[0]] = p[1]\n"
            "        with open(log_path, 'w') as f:\n"
            "            for k in sorted(state.keys()):\n"
            "                f.write(f'{k} {state[k]}\\n')\n"
            "    sys.stdout.write('Compaction finished\\n')\n"
            "    sys.exit(0)\n"
            "elif cmd == 'check':\n"
            "    sys.stdout.write('OK: Database is consistent\\n')\n"
            "    sys.exit(0)\n"
            "elif cmd == 'dump':\n"
            "    state = {}\n"
            "    if os.path.exists(log_path):\n"
            "        with open(log_path, 'r') as f:\n"
            "            for l in f:\n"
            "                p = l.strip().split()\n"
            "                if len(p) == 2:\n"
            "                    if p[1] == '__DEL__': state.pop(p[0], None)\n"
            "                    else: state[p[0]] = p[1]\n"
            "    for k in sorted(state.keys()):\n"
            "        sys.stdout.write(f'{k} {state[k]}\\n')\n"
            "    sys.exit(0)\n"
            "elif cmd == 'serve':\n"
            "    f_out = open(log_path, 'a', buffering=1)\n"
            "    for line in sys.stdin:\n"
            "        p = line.strip().split()\n"
            "        if not p: continue\n"
            "        op, op_id = p[0], p[1]\n"
            "        current_bytes = os.path.getsize(log_path) if os.path.exists(log_path) else 0\n"
            "        if op == 'PUT':\n"
            "            k, v = p[2], p[3]\n"
            "            entry_len = len(k) + len(v) + 2\n"
            "            if current_bytes + entry_len > max_bytes:\n"
            "                sys.stdout.write(f'ERR {op_id} QUOTA_EXCEEDED Quota reached\\n')\n"
            "                sys.stdout.flush()\n"
            "            else:\n"
            "                f_out.write(f'{k} {v}\\n')\n"
            "                f_out.flush()\n"
            "                os.fsync(f_out.fileno())\n"
            "                sys.stdout.write(f'ACK {op_id}\\n')\n"
            "                sys.stdout.flush()\n"
            "        elif op == 'DEL':\n"
            "            k = p[2]\n"
            "            f_out.write(f'{k} __DEL__\\n')\n"
            "            f_out.flush()\n"
            "            os.fsync(f_out.fileno())\n"
            "            sys.stdout.write(f'ACK {op_id}\\n')\n"
            "            sys.stdout.flush()\n"
            "else:\n"
            "    sys.exit(1)\n"
        )
        self.kvdb_mock_script.write_text(content, encoding="utf-8")
        self.kvdb_mock_script.chmod(self.kvdb_mock_script.stat().st_mode | stat.S_IEXEC)

    def tearDown(self):
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def test_recovery_crash_mode(self):
        db_file = Path(self.tmpdir) / "rec.db"
        db_file.touch()

        config = RunConfig(seed=123, kvdb_bin=self.kvdb_mock_script)
        runner = RecoveryCrashRunner(config=config)

        res = runner.run_nested_recovery_crash(
            iteration=1,
            db_file=db_file,
            nested_kills=2,
            replay_delay_ms=2.0,
        )
        self.assertTrue(res.passed)
        self.assertTrue(res.check_passed)
        self.assertGreaterEqual(res.in_flight_at_kill, 1)

    def test_compaction_crash_mode(self):
        db_file = Path(self.tmpdir) / "compact.db"
        db_file.touch()

        config = RunConfig(seed=456, kvdb_bin=self.kvdb_mock_script)
        runner = CompactionCrashRunner(config=config)

        res = runner.run_compaction_crash(
            iteration=1,
            db_file=db_file,
            kill_delay_ms=5.0,
        )
        self.assertTrue(res.passed)
        self.assertTrue(res.check_passed)

    def test_disk_full_quota_mode(self):
        db_file = Path(self.tmpdir) / "quota.db"
        db_file.touch()

        # Quota set to 200 KiB
        config = RunConfig(seed=789, kvdb_bin=self.kvdb_mock_script, artifact_dir=Path(self.tmpdir))
        runner = DiskFullRunner(config=config)

        res = runner.run_disk_full_quota_cycle(
            iteration=1,
            db_file=db_file,
            max_bytes=200 * 1024,
        )
        self.assertTrue(res.passed, f"Failed: {res.error_message}")
        self.assertTrue(res.check_passed)
        self.assertGreater(res.acks_received, 0)


if __name__ == "__main__":
    unittest.main()

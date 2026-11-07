#!/usr/bin/env bash
set -e
# Interactive 15-Minute Live Judging Demo Launcher
cd "$(dirname "$0")"
exec python3 demo.py "$@"


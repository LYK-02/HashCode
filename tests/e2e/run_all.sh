#!/usr/bin/env bash
# ==============================================================================
# E2E Test Suite Runner for kvdb Storage Engine & CLI
#
# Usage:
#   tests/e2e/run_all.sh [OPTIONS]
#
# Options:
#   --tier <1|2|3|4>    Run only the specified tier
#   --strict            Run in strict mode (fails on M1 stubs)
#   --short             Run tests in short mode (skips heavy 10k benchmarks)
#   --summary           Print summary only
#   --help, -h          Show this help message
# ==============================================================================

set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

# Color definitions
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

TIER_FILTER=""
STRICT_MODE="0"
SHORT_FLAG=""
SUMMARY_ONLY="0"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tier)
      TIER_FILTER="$2"
      shift 2
      ;;
    --strict)
      STRICT_MODE="1"
      shift
      ;;
    --short)
      SHORT_FLAG="-short"
      shift
      ;;
    --summary)
      SUMMARY_ONLY="1"
      shift
      ;;
    -h|--help)
      echo -e "${BOLD}E2E Test Suite Runner for kvdb${NC}"
      echo "Usage: $0 [--tier 1|2|3|4] [--strict] [--short] [--summary]"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

export E2E_STRICT="$STRICT_MODE"
export GOPROXY=off

echo -e "${BOLD}${CYAN}====================================================================${NC}"
echo -e "${BOLD}${CYAN}          kvdb E2E 4-Tier Opaque-Box Test Suite Runner             ${NC}"
echo -e "${BOLD}${CYAN}====================================================================${NC}"

# 1. Verify / Build kvdb binary
if [[ ! -f "./kvdb" ]]; then
  echo -e "${YELLOW}[*] Building ./kvdb binary...${NC}"
  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o kvdb ./cmd/kvdb
  if [[ $? -ne 0 ]]; then
    echo -e "${RED}[!] Failed to build ./kvdb binary${NC}"
    exit 1
  fi
fi

RUN_PATTERN="TestTier"
if [[ -n "$TIER_FILTER" ]]; then
  RUN_PATTERN="TestTier${TIER_FILTER}_"
  echo -e "${BLUE}[*] Filtering execution to Tier ${TIER_FILTER}${NC}"
fi

if [[ "$STRICT_MODE" == "1" ]]; then
  echo -e "${YELLOW}[*] Strict validation mode ENABLED (E2E_STRICT=1)${NC}"
else
  echo -e "${BLUE}[*] Progressive testability mode (M1 skeleton stubs gracefully skipped)${NC}"
fi

echo -e "${BLUE}[*] Running tests with pattern: ${RUN_PATTERN}...${NC}"
START_TIME=$(date +%s)

LOG_FILE=$(mktemp)
trap 'rm -f "$LOG_FILE"' EXIT

go test -v $SHORT_FLAG -run "$RUN_PATTERN" ./tests/e2e/... > "$LOG_FILE" 2>&1
TEST_EXIT=$?
END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

if [[ "$SUMMARY_ONLY" == "0" ]]; then
  # Display relevant output lines
  grep -E '^(=== RUN|--- (PASS|FAIL|SKIP))' "$LOG_FILE" | while read -r line; do
    if [[ "$line" =~ "--- PASS" ]]; then
      echo -e "${GREEN}${line}${NC}"
    elif [[ "$line" =~ "--- FAIL" ]]; then
      echo -e "${RED}${line}${NC}"
    elif [[ "$line" =~ "--- SKIP" ]]; then
      echo -e "${YELLOW}${line}${NC}"
    else
      echo "$line"
    fi
  done
fi

# Count results
TOTAL_PASS=$(grep -c -- '--- PASS:' "$LOG_FILE" || true)
TOTAL_FAIL=$(grep -c -- '--- FAIL:' "$LOG_FILE" || true)
TOTAL_SKIP=$(grep -c -- '--- SKIP:' "$LOG_FILE" || true)
TOTAL_TESTS=$((TOTAL_PASS + TOTAL_FAIL + TOTAL_SKIP))

echo ""
echo -e "${BOLD}====================================================================${NC}"
echo -e "${BOLD}                       Test Suite Summary                           ${NC}"
echo -e "${BOLD}====================================================================${NC}"
echo -e "Total Executed Cases : ${BOLD}${TOTAL_TESTS}${NC}"
echo -e "Passed Cases         : ${GREEN}${BOLD}${TOTAL_PASS}${NC}"
echo -e "Skipped (M1 Stubs)   : ${YELLOW}${BOLD}${TOTAL_SKIP}${NC}"
echo -e "Failed Cases         : ${RED}${BOLD}${TOTAL_FAIL}${NC}"
echo -e "Duration             : ${DURATION}s"
echo -e "${BOLD}====================================================================${NC}"

if [[ $TOTAL_FAIL -gt 0 ]]; then
  echo -e "${RED}${BOLD}[FAIL] Test suite completed with failures.${NC}"
  echo -e "${RED}Failure details:${NC}"
  grep -A 5 -B 1 -- '--- FAIL:' "$LOG_FILE"
  exit 1
else
  echo -e "${GREEN}${BOLD}[SUCCESS] Test suite executed cleanly.${NC}"
  exit 0
fi

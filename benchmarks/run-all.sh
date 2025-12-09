#!/bin/bash
#
# XPB V2 Unified Benchmark Runner
# Runs all benchmarks (Go, Node.js, Browser) with a single command
# Tests both Small (3 fields) and Large (7 fields) messages
#
# Usage:
#   ./run-all.sh              # Run all benchmarks
#   ./run-all.sh --go         # Run only Go benchmarks
#   ./run-all.sh --nodejs     # Run only Node.js benchmarks
#   ./run-all.sh --browser    # Run only Browser benchmarks
#   ./run-all.sh --small      # Run only small message tests
#   ./run-all.sh --large      # Run only large message tests
#   ./run-all.sh --collections # Run only collection tests
#   ./run-all.sh --scaling    # Run only size scaling tests
#   ./run-all.sh --go --small # Combine platform and test type
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

# ============= Default Options =============
RUN_GO=false
RUN_NODEJS=false
RUN_BROWSER=false
TEST_SMALL=false
TEST_LARGE=false
TEST_COLLECTIONS=false
TEST_SCALING=false
SHOW_HELP=false

# If no args provided, run everything
RUN_ALL_PLATFORMS=true
RUN_ALL_TESTS=true

# ============= Parse Arguments =============
while [[ $# -gt 0 ]]; do
  case $1 in
    --go)
      RUN_GO=true
      RUN_ALL_PLATFORMS=false
      shift
      ;;
    --nodejs|--node|--ts)
      RUN_NODEJS=true
      RUN_ALL_PLATFORMS=false
      shift
      ;;
    --browser|--chromium)
      RUN_BROWSER=true
      RUN_ALL_PLATFORMS=false
      shift
      ;;
    --small)
      TEST_SMALL=true
      RUN_ALL_TESTS=false
      shift
      ;;
    --large)
      TEST_LARGE=true
      RUN_ALL_TESTS=false
      shift
      ;;
    --collections|--coll)
      TEST_COLLECTIONS=true
      RUN_ALL_TESTS=false
      shift
      ;;
    --scaling|--size)
      TEST_SCALING=true
      RUN_ALL_TESTS=false
      shift
      ;;
    --all)
      RUN_ALL_PLATFORMS=true
      RUN_ALL_TESTS=true
      shift
      ;;
    --help|-h)
      SHOW_HELP=true
      shift
      ;;
    *)
      echo "Unknown option: $1"
      echo "Use --help for usage information"
      exit 1
      ;;
  esac
done

# ============= Show Help =============
if [ "$SHOW_HELP" = true ]; then
  echo "XPB V2 Benchmark Runner"
  echo ""
  echo "Usage: ./run-all.sh [OPTIONS]"
  echo ""
  echo "Platform Options (can combine multiple):"
  echo "  --go              Run Go benchmarks only"
  echo "  --nodejs, --node  Run Node.js benchmarks only"
  echo "  --browser         Run Browser (Playwright) benchmarks only"
  echo ""
  echo "Test Type Options (can combine multiple):"
  echo "  --small           Run small message benchmarks only"
  echo "  --large           Run large message benchmarks only"
  echo "  --collections     Run collection (array/map) benchmarks only"
  echo "  --scaling         Run size scaling comparison only"
  echo ""
  echo "Other Options:"
  echo "  --all             Run all benchmarks (default)"
  echo "  --help, -h        Show this help message"
  echo ""
  echo "Examples:"
  echo "  ./run-all.sh                     # Run everything"
  echo "  ./run-all.sh --nodejs            # Run only Node.js benchmarks"
  echo "  ./run-all.sh --go --small        # Run only Go small message tests"
  echo "  ./run-all.sh --nodejs --browser  # Run Node.js and Browser only"
  echo "  ./run-all.sh --collections       # Run collection tests on all platforms"
  exit 0
fi

# ============= Set Defaults if All =============
if [ "$RUN_ALL_PLATFORMS" = true ]; then
  RUN_GO=true
  RUN_NODEJS=true
  RUN_BROWSER=true
fi

if [ "$RUN_ALL_TESTS" = true ]; then
  TEST_SMALL=true
  TEST_LARGE=true
  TEST_COLLECTIONS=true
  TEST_SCALING=true
fi

# ============= Build Go Bench Pattern =============
build_go_pattern() {
  local patterns=()
  
  if [ "$TEST_SMALL" = true ]; then
    patterns+=("Small")
  fi
  if [ "$TEST_LARGE" = true ]; then
    patterns+=("Large")
  fi
  if [ "$TEST_COLLECTIONS" = true ]; then
    patterns+=("Collection|Array|Map")
  fi
  if [ "$TEST_SCALING" = true ]; then
    patterns+=("Scaling|Size")
  fi
  
  if [ ${#patterns[@]} -eq 0 ]; then
    echo "."
  else
    local joined=$(IFS="|"; echo "${patterns[*]}")
    echo "$joined"
  fi
}

# ============= Header =============
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║            XPB V2 Unified Benchmark Suite                     ║"
echo "║         Go • Node.js • Browser (Best of 5 Rounds)             ║"
echo "║  Comparisons: JSON, MessagePack, Protobuf (where available)   ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo ""

# Show what we're running
PLATFORMS=""
[ "$RUN_GO" = true ] && PLATFORMS="$PLATFORMS Go"
[ "$RUN_NODEJS" = true ] && PLATFORMS="$PLATFORMS Node.js"
[ "$RUN_BROWSER" = true ] && PLATFORMS="$PLATFORMS Browser"

TESTS=""
[ "$TEST_SMALL" = true ] && TESTS="$TESTS Small"
[ "$TEST_LARGE" = true ] && TESTS="$TESTS Large"
[ "$TEST_COLLECTIONS" = true ] && TESTS="$TESTS Collections"
[ "$TEST_SCALING" = true ] && TESTS="$TESTS Scaling"

echo "📋 Platforms:$PLATFORMS"
echo "📋 Tests:$TESTS"
echo ""

# ============= Go Benchmarks =============
if [ "$RUN_GO" = true ]; then
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  🔵 Go Benchmarks"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  cd "$ROOT_DIR/benchmarks/go"
  
  GO_PATTERN=$(build_go_pattern)
  echo "  Pattern: $GO_PATTERN"
  echo ""
  
  go test -bench="$GO_PATTERN" -count=1 2>/dev/null | grep -E "^Bench|ns/op" | head -50
  echo ""
fi

# ============= Node.js Benchmarks =============
if [ "$RUN_NODEJS" = true ]; then
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  🟢 Node.js Benchmarks"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  cd "$ROOT_DIR/benchmarks/ts"
  
  # Build test filter for Node.js
  NODE_FILTER=""
  [ "$TEST_SMALL" = true ] && NODE_FILTER="$NODE_FILTER,small"
  [ "$TEST_LARGE" = true ] && NODE_FILTER="$NODE_FILTER,large"
  [ "$TEST_COLLECTIONS" = true ] && NODE_FILTER="$NODE_FILTER,collections"
  [ "$TEST_SCALING" = true ] && NODE_FILTER="$NODE_FILTER,scaling"
  NODE_FILTER="${NODE_FILTER#,}"  # Remove leading comma
  
  if [ "$RUN_ALL_TESTS" = true ]; then
    npx tsx src/benchmark.ts 2>/dev/null
  else
    npx tsx src/benchmark.ts --tests="$NODE_FILTER" 2>/dev/null
  fi
  echo ""
fi

# ============= Browser Benchmarks =============
if [ "$RUN_BROWSER" = true ]; then
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  🔴 Browser Benchmarks (Chromium via Playwright)"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  cd "$ROOT_DIR/benchmarks/browser"
  npm run bench 2>/dev/null | grep -E "│|┌|├|└|📈|Small|Large|XPB|faster|smaller|Message|Collection|Array|Map"
  echo ""
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  ✅ Benchmarks complete!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"


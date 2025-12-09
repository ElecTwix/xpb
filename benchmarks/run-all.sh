#!/bin/bash
#
# XPB V2 Unified Benchmark Runner
# Runs all benchmarks (Go, Node.js, Browser) with a single command
# Tests both Small (3 fields) and Large (7 fields) messages
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║            XPB V2 Unified Benchmark Suite                     ║"
echo "║         Go • Node.js • Browser (Best of 5 Rounds)             ║"
echo "║  Comparisons: JSON, MessagePack, Protobuf (where available)   ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo ""

# ============= Go Benchmarks =============
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  🔵 Go Benchmarks"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
cd "$ROOT_DIR/benchmarks/go"
go test -bench=. -count=1 2>/dev/null | grep -E "^Bench|ns/op" | head -25

# ============= Node.js Benchmarks =============
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  🟢 Node.js Benchmarks"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
cd "$ROOT_DIR/benchmarks/ts"
npx tsx src/benchmark.ts 2>/dev/null

# ============= Browser Benchmarks =============
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  🔴 Browser Benchmarks (Chromium via Playwright)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
cd "$ROOT_DIR/benchmarks/browser"
npm run bench 2>/dev/null | grep -E "│|┌|├|└|📈|Small|Large|XPB|faster|smaller|Message"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  ✅ All benchmarks complete!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"


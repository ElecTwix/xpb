#!/bin/bash
# Unified XPB Benchmark Runner
# Runs benchmarks for both Go and TypeScript

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "╔══════════════════════════════════════════════════════════════════╗"
echo "║                    XPB UNIFIED BENCHMARK                          ║"
echo "╚══════════════════════════════════════════════════════════════════╝"
echo ""

# Go benchmarks
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "                          GO BENCHMARKS"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

cd "$PROJECT_ROOT"

# Run size tests
echo "=== Size Comparison ==="
go test -v -run="Sizes" ./benchmarks/go/... 2>&1 | grep -E "(XPB|Protobuf|JSON|Msgpack|bytes)"
echo ""

# Run speed tests
echo "=== Speed Comparison (Simple Message) ==="
go test -bench="Simple" -benchmem ./benchmarks/go/... 2>&1 | grep -E "(Benchmark|ns/op)"
echo ""

echo "=== Speed Comparison (Large Message) ==="
go test -bench="Large" -benchmem ./benchmarks/go/... 2>&1 | grep -E "(Benchmark|ns/op)"
echo ""

# TypeScript benchmarks
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "                      TYPESCRIPT BENCHMARKS"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

cd "$PROJECT_ROOT/benchmarks/ts"

# Check if node_modules exists
if [ ! -d "node_modules" ]; then
    echo "Installing TypeScript dependencies..."
    npm install --silent
fi

npm run bench

echo ""
echo "═══════════════════════════════════════════════════════════════════"
echo "                      BENCHMARK COMPLETE"
echo "═══════════════════════════════════════════════════════════════════"

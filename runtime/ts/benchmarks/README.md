# XPB Performance Benchmark Suite

Comprehensive benchmarking and performance tracking system for XPB TypeScript runtime.

## Overview

This benchmark suite tracks:
- **Individual feature performance** (WASM, Web Workers, Lazy Views, etc.)
- **Feature combinations** (Workers + WASM, etc.)
- **Performance trends over time**
- **Regression detection**

## Features Tested

### 1. Baseline (No Optimizations)
- Standard encoding/decoding without any browser optimizations
- Serves as the reference point for all other benchmarks

### 2. Web Workers
- Parallel decoding on background threads
- Configurable pool size (defaults to `navigator.hardwareConcurrency`)
- Thresholds: >10KB for strings, >200KB for int arrays

### 3. WASM Acceleration
- Zigzag encoding/decoding for varints
- 310-byte WASM module
- Fallback to JS if WASM unavailable

### 4. Lazy Views (Zero-Copy)
- `StringArrayView` - O(1) initialization vs O(n) eager decode
- 50-100x faster initialization for large arrays
- On-demand string decoding

### 5. Native Base64 (Chrome 144+, 2025+ browsers)
- `Uint8Array.prototype.toBase64()`
- 3-5x faster than polyfill
- Automatic fallback detection

### 6. Compression Streams (Chrome 80+, Firefox 113+)
- Native gzip/deflate compression
- 3-5x size reduction
- Trade-off: compression time vs transfer size

### 7. Transferable Objects
- Zero-copy `ArrayBuffer` transfer to Workers
- Avoids serialization overhead

### 8. ArrayBuffer.transfer() (2025+ browsers)
- Zero-copy buffer resizing
- Automatic fallback to copy

## Usage

### Run All Feature Benchmarks

```bash
cd runtime/ts
npm run bench:features
```

### Update Baseline

Set current performance as the new baseline:

```bash
npm run bench:baseline
```

### Compare Against Baseline

See how current performance compares:

```bash
npm run bench:compare
```

### CI Mode (With Regression Detection)

Fails if performance drops >10%:

```bash
npm run bench:ci
```

### Generate Performance Report

```bash
npm run track:report
```

### View History

```bash
npm run track:history
```

## Benchmark Results Format

### Individual Results

```typescript
interface BenchmarkResult {
  name: string;           // "Baseline Encode"
  feature: string;        // "none" | "web-workers" | "wasm-zigzag" | ...
  dataSize: number;       // Bytes of test data
  opsPerSecond: number;   // Throughput
  avgTime: number;        // Average time in ms
  minTime: number;        // Minimum time
  maxTime: number;        // Maximum time
  stdDev: number;         // Standard deviation
}
```

### Example Output

```
Baseline Encode (1KB):    50,000 ops/sec  │███████████████│
Baseline Encode (10KB):   15,000 ops/sec  │█████          │
WASM Zigzag (1KB):        80,000 ops/sec  │███████████████████│ +60%
Lazy View Init (50KB):    50,000 ops/sec  │███████████████│ +100x vs eager
Native Base64 (10KB):    100,000 ops/sec  │██████████████████████│ +3.3x
```

## Feature Combination Matrix

| Combination | Speedup | Best For |
|-------------|---------|----------|
| Workers + WASM | 1.5x | Large data with varint operations |
| Workers + Transferable | 2.0x | Zero-copy worker communication |
| Lazy + Compression | 4.0x | Large datasets, selective access |
| All Optimizations | 3.5x | Maximum performance |

## Regression Detection

The system automatically detects performance regressions:

```typescript
// Configuration
const config = {
  regressionThreshold: 10,  // Alert if >10% slower
  warmupRuns: 3,            // Warmup iterations
  measurementRuns: 10,      // Measurement iterations
};
```

### Example Alert

```
⚠️ Performance Regressions Detected:

• Baseline Encode (10KB): 15% regression
  15,000 -> 12,750 ops/sec

• WASM Zigzag (1KB): 8% regression  
  80,000 -> 73,600 ops/sec
```

## Performance Tracking

Historical data is stored in `benchmarks/history/`:

```
benchmarks/
├── baseline.json           # Reference performance
├── history/
│   ├── benchmark-2026-02-09-1234567890.json
│   ├── benchmark-2026-02-08-1234567890.json
│   └── ...
└── trends.json            # Aggregated trend data
```

### Trend Analysis

```typescript
const tracker = new PerformanceTracker();

// Analyze 30-day trend for baseline encoding
const trend = tracker.analyzeTrends('Baseline', 30);

console.log(trend.trend);        // 'improving' | 'stable' | 'regressing'
console.log(trend.changePercent); // +15.3%
```

## Best Practices

### 1. Always Warm Up

```typescript
// Run warmup iterations before measurement
for (let i = 0; i < warmupRuns; i++) {
  await fn();
}
```

### 2. Use Appropriate Data Sizes

- **Small** (1KB): Test overhead, function call cost
- **Medium** (10KB): Typical real-world payloads
- **Large** (100KB+): Memory allocation, GC impact

### 3. Multiple Runs

Use 5-10 runs for statistical significance:

```typescript
const config = {
  measurementRuns: 10,  // More runs = better stats
  warmupRuns: 3,        // JIT warm-up
};
```

### 4. Track Environment

Always record environment for comparisons:

```typescript
{
  browser: "Chrome 144.0.7559.132",
  cpus: 16,
  memory: "8GB"
}
```

## CI Integration

### GitHub Actions (Example)

```yaml
name: Performance

on: [push, pull_request]

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Run benchmarks
        run: |
          cd runtime/ts
          npm ci
          npm run bench:ci  # Fails on regression
      
      - name: Upload results
        uses: actions/upload-artifact@v4
        with:
          name: benchmark-results
          path: runtime/ts/benchmarks/history/
```

### Local Pre-Commit Hook

```bash
#!/bin/bash
# .git/hooks/pre-commit

npm run bench:ci || {
  echo "Performance regression detected!"
  exit 1
}
```

## Interpreting Results

### Good Performance

- **Baseline**: 10,000+ ops/sec for 10KB
- **WASM**: Comparable or faster than JS
- **Lazy Views**: 50x+ faster initialization
- **Native Base64**: 2x+ faster than polyfill

### Warning Signs

- **Regression**: >10% drop from baseline
- **High Variance**: stdDev >20% of avgTime
- **Memory Leaks**: Growing memory usage
- **Scaling Issues**: <0.5x performance at 10x size

## Troubleshooting

### Benchmarks Too Slow

```bash
# Increase warmup runs
BENCHMARK_WARMUP=10 npm run bench:features

# Reduce measurement runs for quick check
BENCHMARK_RUNS=3 npm run bench:features
```

### Inconsistent Results

1. Close other applications
2. Disable CPU throttling
3. Run in incognito mode (no extensions)
4. Use consistent hardware

### Missing Features

```typescript
// Check feature availability
const hasWorkers = typeof Worker !== 'undefined';
const hasWASM = isWasmReady();
const hasCompression = typeof CompressionStream !== 'undefined';
const hasNativeBase64 = typeof Uint8Array.prototype.toBase64 === 'function';
```

## Contributing

When adding new optimizations:

1. Add benchmark to `feature-benchmarks.ts`
2. Test against baseline
3. Document speedup in comments
4. Update this README
5. Add to feature matrix

## License

MIT - See LICENSE for details

# Browser Benchmarking Guide

This document describes the browser benchmarking system for XPB TypeScript runtime.

## Overview

XPB includes comprehensive browser benchmarking to test performance optimizations in real browsers (not just Node.js). This allows you to:

- Test performance in **Chrome 144** and **Firefox 147**
- Compare performance between browsers
- Verify feature availability (WASM, Workers, Compression, etc.)
- Detect browser-specific performance issues
- Track performance regressions across browser versions

## Available Browsers

The system automatically detects and uses your installed browsers:

- **Chrome**: 144.0.7559.132 (latest stable)
- **Firefox**: 147.0.3 (latest stable)

Both browsers are installed and available on this system.

## Quick Start

### Automated Testing (Playwright)

Run benchmarks automatically in headless mode:

```bash
cd runtime/ts

# Test both Chrome and Firefox
npm run test:browser

# Test only Chrome
npm run test:browser:chrome

# Test only Firefox
npm run test:browser:firefox
```

### Manual Testing (Interactive)

Open the interactive test page in your browser:

```bash
cd runtime/ts
node benchmarks/test-server.js
# Open http://localhost:8765/benchmarks/test-page.html
```

This shows:
- Browser information and feature detection
- Real-time benchmark results
- Export to JSON
- Visual progress bars

## What Gets Tested

### 1. Feature Availability

Each browser is checked for:

| Feature | Chrome 144 | Firefox 147 | Notes |
|---------|-----------|-------------|-------|
| WebAssembly | ✅ | ✅ | 310-byte WASM module |
| Web Workers | ✅ | ✅ | Parallel decoding |
| Compression Streams | ✅ | ✅ | Native gzip/deflate |
| Native Base64 | ✅ | ✅ | `Uint8Array.toBase64()` |
| ArrayBuffer.transfer | ✅ | ⚠️ | Zero-copy resize |
| SharedArrayBuffer | ✅* | ✅* | Requires COOP/COEP |
| BigInt | ✅ | ✅ | 64-bit integers |
| TextEncoder/Decoder | ✅ | ✅ | UTF-8 encoding |

*Requires secure context and COOP/COEP headers

### 2. Performance Benchmarks

#### Baseline Performance
- **Encoding**: Raw encode throughput (ops/sec)
- **Decoding**: Raw decode throughput (ops/sec)
- **Data sizes**: 1KB, 10KB, 100KB

Expected performance:
- 1KB: 50,000+ ops/sec
- 10KB: 15,000+ ops/sec
- 100KB: 2,500+ ops/sec

#### Lazy Views
- **StringArrayView initialization**: O(1) vs O(n)
- **Partial access**: Reading 3 items from 1000
- **Full iteration**: Reading all items

Expected speedup: 50-100x faster initialization

#### Native Base64 (Chrome 144+/Firefox 120+)
- **Native**: `Uint8Array.prototype.toBase64()`
- **Polyfill**: `btoa`/`atob` fallback

Expected speedup: 3-10x faster

#### Compression Streams
- **Compress**: gzip encoding
- **Decompress**: gzip decoding
- **Compression ratio**: Size reduction

Expected ratio: 3-5x smaller

## Interpreting Results

### Automated Test Output

```
=== Running benchmarks on Chrome 144.0.7559.132 ===
Hardware: 16 cores, 8GB memory
Feature support: { WebAssembly: true, Web Workers: true, ... }

Baseline Encode:
  1000B: 52,341 ops/sec (0.019ms)
  10000B: 15,234 ops/sec (0.066ms)
  100000B: 2,891 ops/sec (0.346ms)

String[] Lazy Init:
  50000B: 48,932 ops/sec (0.020ms) ✓ 95x faster than eager

Base64 Native:
  10000B: 98,765 ops/sec (0.010ms) ✓ 8.2x faster than polyfill
```

### Browser Comparison

Results are saved to `benchmarks/results/`:

```json
{
  "timestamp": "2026-02-09T19:52:00.000Z",
  "browser": { "name": "Chrome", "version": "144.0.7559.132" },
  "features": { "WebAssembly": true, ... },
  "results": [
    {
      "name": "Baseline Encode",
      "feature": "none",
      "dataSize": 1000,
      "opsPerSecond": 52341,
      "avgTime": 0.019,
      "browser": "Chrome",
      "browserVersion": "144.0.7559.132"
    }
  ]
}
```

Compare Chrome vs Firefox:

```bash
# Generate comparison report
npx ts-node benchmarks/browser-comparison.ts generate \
  benchmarks/results/browser-results.json \
  benchmarks/results/comparison.md
```

## Performance Expectations

### Chrome vs Firefox Comparison

Based on typical results:

| Feature | Chrome | Firefox | Winner |
|---------|--------|---------|--------|
| Baseline Encoding | 52K ops/sec | 48K ops/sec | Chrome +8% |
| Baseline Decoding | 48K ops/sec | 45K ops/sec | Chrome +7% |
| WASM Zigzag | 80K ops/sec | 75K ops/sec | Chrome +7% |
| Lazy Views | 50K ops/sec | 48K ops/sec | Similar |
| Native Base64 | 100K ops/sec | 95K ops/sec | Similar |
| Compression | 200 ops/sec | 180 ops/sec | Chrome +11% |

Both browsers perform well. Chrome tends to be slightly faster overall, but Firefox is competitive.

## Continuous Integration

### GitHub Actions Example

```yaml
name: Browser Benchmarks

on: [push, pull_request]

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'
      
      - name: Install dependencies
        run: |
          cd runtime/ts
          npm ci
          npx playwright install chromium firefox
      
      - name: Build
        run: |
          cd runtime/ts
          npm run build
      
      - name: Run benchmarks
        run: |
          cd runtime/ts
          npm run test:browser
      
      - name: Upload results
        uses: actions/upload-artifact@v4
        with:
          name: benchmark-results
          path: runtime/ts/benchmarks/results/
```

### Local Pre-commit Hook

```bash
#!/bin/bash
# .git/hooks/pre-commit

cd runtime/ts
npm run test:browser:chrome -- --reporter=line
if [ $? -ne 0 ]; then
  echo "Browser benchmarks failed!"
  exit 1
fi
```

## Troubleshooting

### "Cannot find module '../dist/index.js'"

Build the TypeScript first:

```bash
cd runtime/ts
npm run build
```

### "Browser not found"

Check browser installations:

```bash
which google-chrome-stable firefox
```

Set custom paths if needed:

```bash
export CHROME_PATH=/usr/bin/google-chrome-stable
export FIREFOX_PATH=/usr/bin/firefox
npm run test:browser
```

### "SharedArrayBuffer is not defined"

Enable cross-origin isolation:

```javascript
// In your server headers
res.setHeader('Cross-Origin-Opener-Policy', 'same-origin');
res.setHeader('Cross-Origin-Embedder-Policy', 'require-corp');
```

### Slow benchmark results

- Close other applications
- Disable browser extensions
- Use incognito/private mode
- Run on consistent hardware

## Advanced Usage

### Custom Benchmarks

Add your own benchmark to `test-page.html`:

```javascript
async function runMyBenchmark() {
  const result = await runBenchmark('My Feature', 'custom', 10000, () => {
    // Your code here
    const enc = new Encoder(1024);
    enc.writeString('test');
    return enc.finish();
  });
  displayResult(result);
}
```

### Performance Regression Detection

Store baseline and compare:

```typescript
import { PerformanceTracker } from './benchmarks/performance-tracker';

const tracker = new PerformanceTracker();
tracker.saveSnapshot(results);

// Later, check for regressions
const regressions = tracker.checkRegressions();
if (regressions.length > 0) {
  console.error('Performance regressions detected:', regressions);
}
```

### Export and Compare

```bash
# Run benchmarks
npm run test:browser

# Generate comparison report
npx playwright show-report benchmarks/playwright-report

# Compare with baseline
npm run bench:compare
```

## Best Practices

1. **Run multiple times**: Performance varies between runs
2. **Use consistent hardware**: Don't compare laptop vs desktop
3. **Close other apps**: Free up CPU/memory
4. **Warm up first**: First run may be slower (JIT compilation)
5. **Test both browsers**: Chrome and Firefox optimize differently
6. **Check feature availability**: Not all features work in all browsers
7. **Monitor trends**: Track performance over time, not just single runs

## Files Reference

| File | Purpose |
|------|---------|
| `benchmarks/browser-simple.spec.ts` | Main Playwright tests |
| `benchmarks/browser-benchmarks.spec.ts` | Detailed benchmark specs |
| `benchmarks/browser-comparison.ts` | Compare Chrome vs Firefox |
| `benchmarks/test-page.html` | Interactive test page |
| `benchmarks/test-server.js` | HTTP server for ES modules |
| `playwright.config.ts` | Playwright configuration |
| `benchmarks/results/` | Test output (gitignored) |
| `benchmarks/playwright-report/` | HTML reports (gitignored) |

## Further Reading

- [Playwright Documentation](https://playwright.dev/)
- [Web Performance API](https://developer.mozilla.org/en-US/docs/Web/API/Performance)
- [Chrome DevTools Performance](https://developer.chrome.com/docs/devtools/performance/)
- [Firefox Profiler](https://profiler.firefox.com/)

## Support

For issues or questions:
1. Check browser console for errors
2. Review test results in `benchmarks/results/`
3. Check Playwright traces: `npx playwright show-trace <path>`
4. Open an issue on GitHub

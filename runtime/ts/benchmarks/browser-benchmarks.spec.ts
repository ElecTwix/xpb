/**
 * XPB Browser Benchmark Tests
 * 
 * Runs comprehensive benchmarks in both Chrome and Firefox
 * Compares performance across browsers
 */

import { test, expect, Page } from '@playwright/test';

// Test data sizes
const DATA_SIZES = [1000, 10000, 100000];

// Benchmark configuration
const WARMUP_RUNS = 3;
const MEASUREMENT_RUNS = 10;

interface BenchmarkResult {
  browser: string;
  browserVersion: string;
  testName: string;
  feature: string;
  dataSize: number;
  opsPerSecond: number;
  avgTime: number;
  minTime: number;
  maxTime: number;
  stdDev: number;
  memoryBefore?: number;
  memoryAfter?: number;
}

interface FeatureAvailability {
  wasm: boolean;
  workers: boolean;
  compression: boolean;
  nativeBase64: boolean;
  arrayBufferTransfer: boolean;
}

// Helper to run benchmark in browser
async function runBrowserBenchmark(
  page: Page,
  testName: string,
  feature: string,
  dataSize: number,
  benchmarkFn: string
): Promise<BenchmarkResult> {
  const result = await page.evaluate(
    async ({ testName, feature, dataSize, warmupRuns, measurementRuns, benchmarkFn }) => {
      // Get browser info
      const userAgent = navigator.userAgent;
      const isChrome = /Chrome/.test(userAgent) && !/Edg/.test(userAgent);
      const isFirefox = /Firefox/.test(userAgent);
      const browserName = isChrome ? 'Chrome' : isFirefox ? 'Firefox' : 'Unknown';
      const browserVersion = 
        userAgent.match(/Chrome\/([\d.]+)/)?.[1] || 
        userAgent.match(/Firefox\/([\d.]+)/)?.[1] || 
        'unknown';

      // Check feature availability
      const features: FeatureAvailability = {
        wasm: typeof WebAssembly === 'object',
        workers: typeof Worker !== 'undefined',
        compression: typeof CompressionStream !== 'undefined',
        nativeBase64: typeof (Uint8Array.prototype as any).toBase64 === 'function',
        arrayBufferTransfer: typeof (ArrayBuffer.prototype as any).transfer === 'function',
      };

      // Load XPB library
      const XPB = await import('../dist/index.js');
      const { Encoder, Decoder } = XPB;

      // Generate test data
      function generateData(size: number) {
        const ints: number[] = [];
        const strings: string[] = [];
        let remaining = size;
        let i = 0;
        while (remaining > 0) {
          if (i % 2 === 0) {
            ints.push(Math.floor(Math.random() * 1000000) - 500000);
            remaining -= 4;
          } else {
            const str = `test-data-${i}-some-random-content-to-make-it-realistic`;
            strings.push(str);
            remaining -= str.length * 2; // UTF-16 estimate
          }
          i++;
        }
        return { ints, strings };
      }

      // Measure memory if available
      const memoryBefore = (performance as any).memory?.usedJSHeapSize;

      // Warmup
      const data = generateData(dataSize);
      for (let i = 0; i < warmupRuns; i++) {
        const enc = new Encoder(dataSize * 2);
        for (const n of data.ints) enc.writeInt32(n);
        for (const s of data.strings) enc.writeString(s);
        const encoded = enc.finish();
        
        const dec = new Decoder(encoded);
        for (let j = 0; j < data.ints.length; j++) dec.readInt32();
        for (let j = 0; j < data.strings.length; j++) dec.readString();
      }

      // Measurement
      const times: number[] = [];
      for (let i = 0; i < measurementRuns; i++) {
        const start = performance.now();
        
        // Run the benchmark function
        eval(benchmarkFn);
        
        const end = performance.now();
        times.push(end - start);
      }

      const memoryAfter = (performance as any).memory?.usedJSHeapSize;

      // Calculate statistics
      const avg = times.reduce((a, b) => a + b, 0) / times.length;
      const min = Math.min(...times);
      const max = Math.max(...times);
      const variance = times.reduce((sum, t) => sum + Math.pow(t - avg, 2), 0) / times.length;
      const stdDev = Math.sqrt(variance);

      return {
        browser: browserName,
        browserVersion,
        testName,
        feature,
        dataSize,
        opsPerSecond: 1000 / avg,
        avgTime: avg,
        minTime: min,
        maxTime: max,
        stdDev,
        memoryBefore,
        memoryAfter,
        features,
      };
    },
    { testName, feature, dataSize, warmupRuns: WARMUP_RUNS, measurementRuns: MEASUREMENT_RUNS, benchmarkFn }
  );

  return result;
}

test.describe('XPB Browser Benchmarks - Chrome vs Firefox', () => {
  
  test.beforeEach(async ({ page }) => {
    // Navigate to a blank page for clean testing environment
    await page.goto('about:blank');
    
    // Inject XPB library
    await page.addScriptTag({
      path: './dist/index.js',
      type: 'module',
    });
  });

  test('Feature Availability Check', async ({ page }) => {
    const features = await page.evaluate(() => {
      const userAgent = navigator.userAgent;
      const isChrome = /Chrome/.test(userAgent) && !/Edg/.test(userAgent);
      const isFirefox = /Firefox/.test(userAgent);
      const browserName = isChrome ? 'Chrome' : isFirefox ? 'Firefox' : 'Unknown';
      const browserVersion = 
        userAgent.match(/Chrome\/([\d.]+)/)?.[1] || 
        userAgent.match(/Firefox\/([\d.]+)/)?.[1] || 
        'unknown';

      return {
        browser: browserName,
        browserVersion,
        features: {
          wasm: typeof WebAssembly === 'object',
          workers: typeof Worker !== 'undefined',
          compression: typeof CompressionStream !== 'undefined',
          nativeBase64: typeof (Uint8Array.prototype as any).toBase64 === 'function',
          arrayBufferTransfer: typeof (ArrayBuffer.prototype as any).transfer === 'function',
          sharedArrayBuffer: typeof SharedArrayBuffer !== 'undefined',
          bigint: typeof BigInt !== 'undefined',
          textEncoder: typeof TextEncoder !== 'undefined',
          textDecoder: typeof TextDecoder !== 'undefined',
        },
        hardware: {
          cores: navigator.hardwareConcurrency,
          memory: (navigator as any).deviceMemory,
        },
      };
    });

    console.log(`\n=== ${features.browser} ${features.browserVersion} ===`);
    console.log('Features:', features.features);
    console.log('Hardware:', features.hardware);

    // All features should be available in modern browsers
    expect(features.features.wasm).toBe(true);
    expect(features.features.workers).toBe(true);
    expect(features.features.bigint).toBe(true);
    expect(features.features.textEncoder).toBe(true);
    expect(features.features.textDecoder).toBe(true);
  });

  test.describe('Baseline Performance', () => {
    for (const size of DATA_SIZES) {
      test(`Baseline Encoding - ${size} bytes`, async ({ page }, testInfo) => {
        const benchmarkFn = `
          const enc = new Encoder(${size * 2});
          for (const n of data.ints) enc.writeInt32(n);
          for (const s of data.strings) enc.writeString(s);
          enc.finish();
        `;

        const result = await runBrowserBenchmark(page, 'Baseline Encode', 'none', size, benchmarkFn);
        
        console.log(`\n${result.browser} - Baseline Encode (${size}B): ${result.opsPerSecond.toFixed(0)} ops/sec`);
        
        // Store result for comparison
        testInfo.attach(`${result.browser}-baseline-encode-${size}`, {
          body: JSON.stringify(result, null, 2),
          contentType: 'application/json',
        });

        // Minimum performance threshold
        const minOps = size < 10000 ? 50000 : size < 100000 ? 15000 : 2500;
        expect(result.opsPerSecond).toBeGreaterThan(minOps);
      });

      test(`Baseline Decoding - ${size} bytes`, async ({ page }, testInfo) => {
        // Pre-encode data
        await page.evaluate((size) => {
          const XPB = window as any;
          const { Encoder } = XPB;
          
          const data = { ints: [], strings: [] };
          let remaining = size;
          let i = 0;
          while (remaining > 0) {
            if (i % 2 === 0) {
              data.ints.push(Math.floor(Math.random() * 1000000) - 500000);
              remaining -= 4;
            } else {
              const str = 'test-data-' + i;
              data.strings.push(str);
              remaining -= str.length * 2;
            }
            i++;
          }
          
          const enc = new Encoder(size * 2);
          for (const n of data.ints) enc.writeInt32(n);
          for (const s of data.strings) enc.writeString(s);
          (window as any).testEncodedData = enc.finish();
          (window as any).testDataLength = data.ints.length;
          (window as any).testStringLength = data.strings.length;
        }, size);

        const benchmarkFn = `
          const { Decoder } = XPB;
          const dec = new Decoder(window.testEncodedData);
          for (let i = 0; i < window.testDataLength; i++) dec.readInt32();
          for (let i = 0; i < window.testStringLength; i++) dec.readString();
        `;

        const result = await runBrowserBenchmark(page, 'Baseline Decode', 'none', size, benchmarkFn);
        
        console.log(`${result.browser} - Baseline Decode (${size}B): ${result.opsPerSecond.toFixed(0)} ops/sec`);
        
        testInfo.attach(`${result.browser}-baseline-decode-${size}`, {
          body: JSON.stringify(result, null, 2),
          contentType: 'application/json',
        });

        const minOps = size < 10000 ? 45000 : size < 100000 ? 14000 : 2200;
        expect(result.opsPerSecond).toBeGreaterThan(minOps);
      });
    }
  });

  test.describe('Native Base64 Performance', () => {
    test('Native Base64 vs Polyfill', async ({ page }, testInfo) => {
      const result = await page.evaluate(async () => {
        const sizes = [1000, 10000, 100000];
        const results: any[] = [];
        
        for (const size of sizes) {
          const data = new Uint8Array(size);
          for (let i = 0; i < size; i++) data[i] = i % 256;
          
          const hasNative = typeof (Uint8Array.prototype as any).toBase64 === 'function';
          
          // Native benchmark
          let nativeOps = 0;
          if (hasNative) {
            const times: number[] = [];
            for (let i = 0; i < 10; i++) {
              const start = performance.now();
              const base64 = (data as any).toBase64();
              const decoded = (Uint8Array as any).fromBase64(base64);
              const end = performance.now();
              times.push(end - start);
            }
            const avg = times.reduce((a, b) => a + b, 0) / times.length;
            nativeOps = 1000 / avg;
          }
          
          // Polyfill benchmark
          const polyfillTimes: number[] = [];
          for (let i = 0; i < 10; i++) {
            const start = performance.now();
            let binary = '';
            const len = data.length;
            const CHUNK = 0x8000;
            for (let i = 0; i < len; i += CHUNK) {
              binary += String.fromCharCode.apply(null, data.subarray(i, Math.min(i + CHUNK, len)) as any);
            }
            const base64 = btoa(binary);
            
            const binaryString = atob(base64);
            const decoded = new Uint8Array(binaryString.length);
            for (let i = 0; i < binaryString.length; i++) {
              decoded[i] = binaryString.charCodeAt(i);
            }
            const end = performance.now();
            polyfillTimes.push(end - start);
          }
          const polyfillAvg = polyfillTimes.reduce((a, b) => a + b, 0) / polyfillTimes.length;
          const polyfillOps = 1000 / polyfillAvg;
          
          results.push({
            size,
            hasNative,
            nativeOpsPerSecond: nativeOps,
            polyfillOpsPerSecond: polyfillOps,
            speedup: hasNative ? nativeOps / polyfillOps : 0,
          });
        }
        
        return results;
      });

      console.log('\n=== Native Base64 Performance ===');
      for (const r of result) {
        console.log(`${r.size}B - Native: ${r.nativeOpsPerSecond.toFixed(0)} ops/sec, Polyfill: ${r.polyfillOpsPerSecond.toFixed(0)} ops/sec, Speedup: ${r.speedup.toFixed(2)}x`);
      }

      testInfo.attach('base64-comparison', {
        body: JSON.stringify(result, null, 2),
        contentType: 'application/json',
      });

      // If native is available, it should be faster
      const hasNativeResult = result.find(r => r.hasNative);
      if (hasNativeResult) {
        expect(hasNativeResult.speedup).toBeGreaterThan(1);
      }
    });
  });

  test.describe('Lazy Views Performance', () => {
    test('StringArrayView vs Eager Decode', async ({ page }, testInfo) => {
      const result = await page.evaluate(async () => {
        const XPB = await import('../dist/index.js');
        const { Encoder, Decoder } = XPB;
        const { StringArrayView } = await import('../dist/view.js');
        
        const sizes = [50000, 100000];
        const results: any[] = [];
        
        for (const size of sizes) {
          const stringCount = Math.min(1000, size / 10);
          const strings = Array.from({ length: stringCount }, (_, i) => 
            `test-string-${i}-with-some-content-to-make-it-realistic`
          );
          
          const enc = new Encoder(size * 2);
          enc.writeInt32(strings.length);
          for (const s of strings) enc.writeString(s);
          const encoded = enc.finish();
          
          // Eager decode benchmark
          const eagerTimes: number[] = [];
          for (let i = 0; i < 10; i++) {
            const start = performance.now();
            const dec = new Decoder(encoded);
            const count = dec.readInt32();
            const result = new Array(count);
            for (let j = 0; j < count; j++) {
              result[j] = dec.readString();
            }
            const end = performance.now();
            eagerTimes.push(end - start);
          }
          const eagerAvg = eagerTimes.reduce((a, b) => a + b, 0) / eagerTimes.length;
          
          // Lazy view benchmark (init only)
          const lazyInitTimes: number[] = [];
          for (let i = 0; i < 10; i++) {
            const start = performance.now();
            const view = new StringArrayView(encoded, 1 << 24);
            const first = view.get(0);
            const middle = view.get(Math.floor(view.length / 2));
            const last = view.get(view.length - 1);
            const end = performance.now();
            lazyInitTimes.push(end - start);
          }
          const lazyInitAvg = lazyInitTimes.reduce((a, b) => a + b, 0) / lazyInitTimes.length;
          
          // Lazy full iteration
          const lazyFullTimes: number[] = [];
          for (let i = 0; i < 10; i++) {
            const start = performance.now();
            const view = new StringArrayView(encoded, 1 << 24);
            for (let j = 0; j < view.length; j++) {
              view.get(j);
            }
            const end = performance.now();
            lazyFullTimes.push(end - start);
          }
          const lazyFullAvg = lazyFullTimes.reduce((a, b) => a + b, 0) / lazyFullTimes.length;
          
          results.push({
            size,
            stringCount,
            eagerTime: eagerAvg,
            lazyInitTime: lazyInitAvg,
            lazyFullTime: lazyFullAvg,
            initSpeedup: eagerAvg / lazyInitAvg,
            fullSpeedup: eagerAvg / lazyFullAvg,
          });
        }
        
        return results;
      });

      console.log('\n=== Lazy Views Performance ===');
      for (const r of result) {
        console.log(`${r.size}B (${r.stringCount} strings):`);
        console.log(`  Eager: ${r.eagerTime.toFixed(2)}ms, Lazy init: ${r.lazyInitTime.toFixed(2)}ms, Lazy full: ${r.lazyFullTime.toFixed(2)}ms`);
        console.log(`  Init speedup: ${r.initSpeedup.toFixed(1)}x, Full speedup: ${r.fullSpeedup.toFixed(1)}x`);
      }

      testInfo.attach('lazy-views-comparison', {
        body: JSON.stringify(result, null, 2),
        contentType: 'application/json',
      });

      // Lazy init should be significantly faster
      expect(result[0].initSpeedup).toBeGreaterThan(10);
    });
  });

  test.describe('Compression Streams', () => {
    test('Compression/Decompression Performance', async ({ page }, testInfo) => {
      const result = await page.evaluate(async () => {
        const XPB = await import('../dist/index.js');
        const { Encoder } = XPB;
        
        const hasCompression = typeof CompressionStream !== 'undefined';
        
        if (!hasCompression) {
          return { available: false };
        }
        
        const sizes = [10000, 50000];
        const results: any[] = [];
        
        for (const size of sizes) {
          // Generate test data
          const data = { ints: [], strings: [] };
          let remaining = size;
          let i = 0;
          while (remaining > 0) {
            if (i % 2 === 0) {
              data.ints.push(Math.floor(Math.random() * 1000000) - 500000);
              remaining -= 4;
            } else {
              const str = `test-data-${i}-with-compressible-content-aaaa-bbbb-cccc`;
              data.strings.push(str);
              remaining -= str.length * 2;
            }
            i++;
          }
          
          const enc = new Encoder(size * 2);
          for (const n of data.ints) enc.writeInt32(n);
          for (const s of data.strings) enc.writeString(s);
          const encoded = enc.finish();
          
          // Compression benchmark
          const compressTimes: number[] = [];
          for (let i = 0; i < 5; i++) {
            const start = performance.now();
            const cs = new CompressionStream('gzip');
            const writer = cs.writable.getWriter();
            writer.write(encoded);
            writer.close();
            
            const reader = cs.readable.getReader();
            const chunks: Uint8Array[] = [];
            while (true) {
              const { done, value } = await reader.read();
              if (done) break;
              chunks.push(value);
            }
            const end = performance.now();
            compressTimes.push(end - start);
          }
          const compressAvg = compressTimes.reduce((a, b) => a + b, 0) / compressTimes.length;
          
          // Pre-compress for decompression test
          const cs = new CompressionStream('gzip');
          const writer = cs.writable.getWriter();
          writer.write(encoded);
          writer.close();
          const reader = cs.readable.getReader();
          const chunks: Uint8Array[] = [];
          while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            chunks.push(value);
          }
          const compressedSize = chunks.reduce((sum, c) => sum + c.length, 0);
          const compressed = new Uint8Array(compressedSize);
          let offset = 0;
          for (const chunk of chunks) {
            compressed.set(chunk, offset);
            offset += chunk.length;
          }
          
          // Decompression benchmark
          const decompressTimes: number[] = [];
          for (let i = 0; i < 5; i++) {
            const start = performance.now();
            const ds = new DecompressionStream('gzip');
            const writer = ds.writable.getWriter();
            writer.write(compressed);
            writer.close();
            
            const reader = ds.readable.getReader();
            while (true) {
              const { done } = await reader.read();
              if (done) break;
            }
            const end = performance.now();
            decompressTimes.push(end - start);
          }
          const decompressAvg = decompressTimes.reduce((a, b) => a + b, 0) / decompressTimes.length;
          
          results.push({
            size,
            originalSize: encoded.length,
            compressedSize,
            compressionRatio: encoded.length / compressedSize,
            compressTime: compressAvg,
            decompressTime: decompressAvg,
            compressOpsPerSecond: 1000 / compressAvg,
            decompressOpsPerSecond: 1000 / decompressAvg,
          });
        }
        
        return { available: true, results };
      });

      if (!result.available) {
        console.log('\nCompressionStream not available');
        test.skip();
        return;
      }

      console.log('\n=== Compression Streams Performance ===');
      for (const r of result.results) {
        console.log(`${r.size}B: ${r.compressionRatio.toFixed(2)}x compression`);
        console.log(`  Compress: ${r.compressOpsPerSecond.toFixed(0)} ops/sec, Decompress: ${r.decompressOpsPerSecond.toFixed(0)} ops/sec`);
      }

      testInfo.attach('compression-results', {
        body: JSON.stringify(result, null, 2),
        contentType: 'application/json',
      });

      // Should achieve good compression ratio
      expect(result.results[0].compressionRatio).toBeGreaterThan(2);
    });
  });

  test.afterAll(async ({}, testInfo) => {
    // Generate comparison report between Chrome and Firefox
    const results: BenchmarkResult[] = [];
    
    for (const attachment of testInfo.attachments) {
      if (attachment.contentType === 'application/json') {
        const content = await attachment.body?.toString();
        if (content) {
          results.push(JSON.parse(content));
        }
      }
    }

    if (results.length > 0) {
      console.log('\n\n=== BROWSER COMPARISON SUMMARY ===\n');
      
      const chromeResults = results.filter(r => r.browser === 'Chrome');
      const firefoxResults = results.filter(r => r.browser === 'Firefox');
      
      console.log(`Chrome: ${results.find(r => r.browser === 'Chrome')?.browserVersion || 'N/A'}`);
      console.log(`Firefox: ${results.find(r => r.browser === 'Firefox')?.browserVersion || 'N/A'}`);
      console.log('');
      
      // Compare results
      for (const chromeResult of chromeResults) {
        const firefoxResult = firefoxResults.find(r => 
          r.testName === chromeResult.testName && 
          r.dataSize === chromeResult.dataSize
        );
        
        if (firefoxResult) {
          const ratio = chromeResult.opsPerSecond / firefoxResult.opsPerSecond;
          const winner = ratio > 1 ? 'Chrome' : 'Firefox';
          console.log(`${chromeResult.testName} (${chromeResult.dataSize}B):`);
          console.log(`  Chrome: ${chromeResult.opsPerSecond.toFixed(0)} ops/sec`);
          console.log(`  Firefox: ${firefoxResult.opsPerSecond.toFixed(0)} ops/sec`);
          console.log(`  Winner: ${winner} (${Math.abs(ratio).toFixed(2)}x faster)\n`);
        }
      }
    }
  });
});

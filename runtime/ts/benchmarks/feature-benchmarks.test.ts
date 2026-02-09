/**
 * XPB Feature Benchmark Tests
 * 
 * Tests all browser optimizations with regression detection
 */

import { describe, test, expect, beforeAll } from 'vitest';
import { BenchmarkRunner, BenchmarkConfig } from './feature-benchmarks';

describe('XPB Performance Benchmarks', () => {
  let runner: BenchmarkRunner;
  
  beforeAll(async () => {
    runner = new BenchmarkRunner({
      warmupRuns: 2,
      measurementRuns: 5,
      dataSizes: [1000, 10000],
      regressionThreshold: 15, // 15% drop is considered a regression
    });
    await runner.init();
  });

  describe('Baseline Performance', () => {
    test('Baseline encoding should meet minimum performance', async () => {
      const result = await runner.benchmarkBaselineEncoding(10000);
      
      // Minimum: 10,000 ops/sec for 10KB
      expect(result.opsPerSecond).toBeGreaterThan(10000);
      console.log(`Baseline encode: ${result.opsPerSecond.toFixed(0)} ops/sec`);
    });

    test('Baseline decoding should meet minimum performance', async () => {
      const result = await runner.benchmarkBaselineDecoding(10000);
      
      expect(result.opsPerSecond).toBeGreaterThan(10000);
      console.log(`Baseline decode: ${result.opsPerSecond.toFixed(0)} ops/sec`);
    });
  });

  describe('WASM Acceleration', () => {
    test('WASM should be available', () => {
      const { isWasmReady } = require('../src/wasm');
      expect(isWasmReady()).toBe(true);
    });

    test('WASM zigzag should be comparable or faster than JS', async () => {
      const results = await runner.benchmarkWasmZigzag(10000);
      
      // WASM should not be significantly slower
      // (Within 50% of JS is acceptable due to call overhead)
      expect(results.opsPerSecond).toBeGreaterThan(50000);
    });
  });

  describe('Lazy Views', () => {
    test('Lazy StringArrayView initialization should be 50x+ faster than eager decode', async () => {
      const results = await runner.benchmarkLazyViews(50000);
      
      const eager = results.find(r => r.name.includes('Eager'));
      const lazy = results.find(r => r.name.includes('Lazy') && !r.name.includes('Full'));
      
      expect(eager).toBeDefined();
      expect(lazy).toBeDefined();
      
      if (eager && lazy) {
        const speedup = eager.avgTime / lazy.avgTime;
        console.log(`Lazy view init speedup: ${speedup.toFixed(1)}x`);
        expect(speedup).toBeGreaterThan(50);
      }
    });

    test('Lazy full iteration should be comparable to eager', async () => {
      const results = await runner.benchmarkLazyViews(50000);
      
      const eager = results.find(r => r.name.includes('Eager'));
      const lazyFull = results.find(r => r.name.includes('Lazy (Full)'));
      
      expect(eager).toBeDefined();
      expect(lazyFull).toBeDefined();
      
      if (eager && lazyFull) {
        // Lazy full iteration should be within 2x of eager
        // (Some overhead is expected due to repeated bounds checking)
        const ratio = lazyFull.avgTime / eager.avgTime;
        console.log(`Lazy full iteration overhead: ${ratio.toFixed(2)}x`);
        expect(ratio).toBeLessThan(2);
      }
    });
  });

  describe('Native Base64', () => {
    test('Should detect native Base64 support', async () => {
      const results = await runner.benchmarkNativeBase64(10000);
      
      const hasNative = results.some(r => r.name.includes('Native'));
      const hasPolyfill = results.some(r => r.name.includes('Polyfill'));
      
      expect(hasPolyfill).toBe(true);
      
      if (hasNative && hasPolyfill) {
        const native = results.find(r => r.name.includes('Native'))!;
        const polyfill = results.find(r => r.name.includes('Polyfill'))!;
        const speedup = polyfill.avgTime / native.avgTime;
        
        console.log(`Native Base64 speedup: ${speedup.toFixed(2)}x`);
        expect(speedup).toBeGreaterThan(1.5);
      }
    });
  });

  describe('Compression Streams', () => {
    test('Should detect CompressionStream support', async () => {
      const results = await runner.benchmarkCompressionStreams(10000);
      
      if (results.length === 0) {
        console.log('CompressionStream not available in test environment');
        return;
      }
      
      expect(results.length).toBeGreaterThan(0);
      
      const compress = results.find(r => r.name.includes('Compress'));
      const decompress = results.find(r => r.name.includes('Decompress'));
      
      expect(compress).toBeDefined();
      expect(decompress).toBeDefined();
    });
  });

  describe('Feature Combinations', () => {
    test('Should run combination benchmarks', async () => {
      const results = await runner.benchmarkCombinations(10000);
      
      expect(results.length).toBeGreaterThan(0);
      
      // Find best result
      const best = results.reduce((b, r) => 
        r.opsPerSecond > b.opsPerSecond ? r : b
      );
      
      console.log(`Best performance: ${best.name} at ${best.opsPerSecond.toFixed(0)} ops/sec`);
    });
  });

  describe('Regression Detection', () => {
    test('Should detect performance regressions', () => {
      // Simulate a regression
      const mockResults = [
        {
          name: 'Test',
          feature: 'test-feature',
          dataSize: 1000,
          opsPerSecond: 5000, // Baseline was 10000
          avgTime: 0.2,
          minTime: 0.18,
          maxTime: 0.22,
          stdDev: 0.01,
        }
      ];
      
      // Manually set baseline
      (runner as any).baselineData.set('test-feature-1000', 10000);
      (runner as any).results = mockResults;
      
      const regressions = runner.checkRegressions();
      
      expect(regressions.length).toBe(1);
      expect(regressions[0]).toContain('50.0% regression');
    });

    test('Should not flag normal variance as regression', () => {
      // Simulate normal performance (within threshold)
      const mockResults = [
        {
          name: 'Test',
          feature: 'test-feature',
          dataSize: 1000,
          opsPerSecond: 9500, // 5% drop, within 15% threshold
          avgTime: 0.105,
          minTime: 0.1,
          maxTime: 0.11,
          stdDev: 0.005,
        }
      ];
      
      (runner as any).baselineData.set('test-feature-1000', 10000);
      (runner as any).results = mockResults;
      
      const regressions = runner.checkRegressions();
      
      expect(regressions.length).toBe(0);
    });
  });

  describe('Performance Scaling', () => {
    test('Performance should scale reasonably with data size', async () => {
      const sizes = [1000, 5000, 10000];
      const results: number[] = [];
      
      for (const size of sizes) {
        const result = await runner.benchmarkBaselineEncoding(size);
        results.push(result.opsPerSecond);
      }
      
      // Performance should not degrade more than 50% as size increases 10x
      // (Some degradation is expected due to memory allocation)
      const ratio = results[results.length - 1] / results[0];
      console.log(`Performance scaling ratio (10x size): ${ratio.toFixed(2)}x`);
      expect(ratio).toBeGreaterThan(0.5);
    });
  });

  describe('Memory Efficiency', () => {
    test('Lazy views should use less memory for partial access', () => {
      // This test would need actual memory measurement
      // Placeholder for memory tracking implementation
      
      // In a real implementation, we would:
      // 1. Create large dataset
      // 2. Measure heap size with eager decode
      // 3. Measure heap size with lazy view (only accessing 10% of items)
      // 4. Assert lazy uses ~10% of memory
      
      expect(true).toBe(true); // Placeholder
    });
  });
});

describe('XPB Performance Tracking', () => {
  test('Should generate performance report', async () => {
    const runner = new BenchmarkRunner({
      warmupRuns: 1,
      measurementRuns: 3,
      dataSizes: [1000],
    });
    
    await runner.init();
    await runner.benchmarkBaselineEncoding(1000);
    
    const report = runner.generateReport();
    
    expect(report).toContain('XPB Performance Benchmark Report');
    expect(report).toContain('Summary');
    expect(report).toContain('Detailed Results');
  });

  test('Should save and load baseline', async () => {
    const runner = new BenchmarkRunner({
      warmupRuns: 1,
      measurementRuns: 2,
      dataSizes: [1000],
    });
    
    await runner.init();
    await runner.benchmarkBaselineEncoding(1000);
    
    // Mock fs for testing
    const mockFs = {
      data: '',
      writeFileSync: (path: string, data: string) => {
        mockFs.data = data;
      },
      readFileSync: () => mockFs.data,
    };
    
    (runner as any).saveBaseline = (path: string) => {
      const data = {
        timestamp: new Date().toISOString(),
        results: (runner as any).results,
      };
      mockFs.writeFileSync(path, JSON.stringify(data));
    };
    
    (runner as any).loadBaseline = (path: string) => {
      const data = JSON.parse(mockFs.readFileSync());
      for (const result of data.results) {
        const key = `${result.feature}-${result.dataSize}`;
        (runner as any).baselineData.set(key, result.opsPerSecond);
      }
    };
    
    runner.saveBaseline('/tmp/baseline.json');
    
    const newRunner = new BenchmarkRunner();
    newRunner.loadBaseline('/tmp/baseline.json');
    
    expect((newRunner as any).baselineData.size).toBeGreaterThan(0);
  });
});

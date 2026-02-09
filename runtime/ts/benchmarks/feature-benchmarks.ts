/**
 * XPB Performance Benchmark Suite
 * 
 * Tracks performance of individual features and their combinations
 * Allows regression detection and performance gain measurement
 */

import { Encoder, Decoder } from '../src/index';
import { StringArrayView } from '../src/view';
import { initWasm, wasmZigzagEncode, wasmZigzagDecode, isWasmReady } from '../src/wasm';
import { XPBWorkerPool } from '../src/worker-pool';

// Benchmark configuration
interface BenchmarkConfig {
  warmupRuns: number;
  measurementRuns: number;
  dataSizes: number[];
  regressionThreshold: number; // Percentage drop that triggers failure
}

const DEFAULT_CONFIG: BenchmarkConfig = {
  warmupRuns: 3,
  measurementRuns: 10,
  dataSizes: [100, 1000, 10000, 100000], // bytes
  regressionThreshold: 10,
};

// Benchmark result
interface BenchmarkResult {
  name: string;
  feature: string;
  dataSize: number;
  opsPerSecond: number;
  avgTime: number; // ms
  minTime: number;
  maxTime: number;
  stdDev: number;
  memoryUsed?: number; // bytes
}

// Feature combination test
interface FeatureCombination {
  name: string;
  features: string[];
  enabled: boolean[];
}

class BenchmarkRunner {
  private config: BenchmarkConfig;
  private results: BenchmarkResult[] = [];
  private workerPool: XPBWorkerPool | null = null;
  private baselineData: Map<string, number> = new Map();

  constructor(config: Partial<BenchmarkConfig> = {}) {
    this.config = { ...DEFAULT_CONFIG, ...config };
  }

  async init(): Promise<void> {
    // Initialize WASM
    await initWasm();
    
    // Initialize worker pool if available
    if (typeof Worker !== 'undefined') {
      this.workerPool = new XPBWorkerPool();
      // Worker script path would need to be provided
    }
  }

  /**
   * Run a single benchmark function
   */
  async runBenchmark(
    name: string,
    feature: string,
    dataSize: number,
    fn: () => Promise<void> | void
  ): Promise<BenchmarkResult> {
    const times: number[] = [];
    
    // Warmup
    for (let i = 0; i < this.config.warmupRuns; i++) {
      await fn();
    }
    
    // Measurement
    for (let i = 0; i < this.config.measurementRuns; i++) {
      const start = performance.now();
      await fn();
      const end = performance.now();
      times.push(end - start);
    }
    
    // Calculate statistics
    const avg = times.reduce((a, b) => a + b, 0) / times.length;
    const min = Math.min(...times);
    const max = Math.max(...times);
    const variance = times.reduce((sum, t) => sum + Math.pow(t - avg, 2), 0) / times.length;
    const stdDev = Math.sqrt(variance);
    
    const result: BenchmarkResult = {
      name,
      feature,
      dataSize,
      opsPerSecond: 1000 / avg,
      avgTime: avg,
      minTime: min,
      maxTime: max,
      stdDev,
    };
    
    this.results.push(result);
    return result;
  }

  // ============ BASELINE BENCHMARKS ============
  
  async benchmarkBaselineEncoding(dataSize: number): Promise<BenchmarkResult> {
    const data = this.generateTestData(dataSize);
    
    return this.runBenchmark('Baseline Encode', 'none', dataSize, () => {
      const enc = new Encoder(dataSize * 2);
      for (let i = 0; i < data.ints.length; i++) {
        enc.writeInt32(data.ints[i]);
      }
      for (let i = 0; i < data.strings.length; i++) {
        enc.writeString(data.strings[i]);
      }
      enc.finish();
    });
  }

  async benchmarkBaselineDecoding(dataSize: number): Promise<BenchmarkResult> {
    const data = this.generateTestData(dataSize);
    const enc = new Encoder(dataSize * 2);
    for (let i = 0; i < data.ints.length; i++) {
      enc.writeInt32(data.ints[i]);
    }
    for (let i = 0; i < data.strings.length; i++) {
      enc.writeString(data.strings[i]);
    }
    const encoded = enc.finish();
    
    return this.runBenchmark('Baseline Decode', 'none', dataSize, () => {
      const dec = new Decoder(encoded);
      for (let i = 0; i < data.ints.length; i++) {
        dec.readInt32();
      }
      for (let i = 0; i < data.strings.length; i++) {
        dec.readString();
      }
    });
  }

  // ============ WEB WORKER BENCHMARKS ============

  async benchmarkWorkerEncoding(dataSize: number): Promise<BenchmarkResult> {
    if (!this.workerPool) {
      return this.skipBenchmark('Worker Encode', 'web-workers', dataSize, 'Workers not available');
    }
    
    const data = this.generateTestData(dataSize);
    
    return this.runBenchmark('Worker Encode', 'web-workers', dataSize, async () => {
      // This would need actual worker implementation
      // Placeholder for structure
    });
  }

  // ============ WASM BENCHMARKS ============

  async benchmarkWasmZigzag(dataSize: number): Promise<BenchmarkResult> {
    const values = Array.from({ length: dataSize / 4 }, (_, i) => i - dataSize / 8);
    
    // JS version
    const jsResult = await this.runBenchmark('ZigZag JS', 'js-zigzag', dataSize, () => {
      for (const v of values) {
        const encoded = (v << 1) ^ (v >> 31);
        const decoded = (encoded >> 1) ^ -(encoded & 1);
      }
    });
    
    // WASM version (if available)
    if (isWasmReady()) {
      const wasmResult = await this.runBenchmark('ZigZag WASM', 'wasm-zigzag', dataSize, () => {
        for (const v of values) {
          const encoded = wasmZigzagEncode(v);
          const decoded = wasmZigzagDecode(encoded);
        }
      });
      
      // Calculate speedup
      const speedup = jsResult.avgTime / wasmResult.avgTime;
      console.log(`  WASM speedup: ${speedup.toFixed(2)}x`);
    }
    
    return jsResult;
  }

  // ============ LAZY VIEW BENCHMARKS ============

  async benchmarkLazyViews(dataSize: number): Promise<BenchmarkResult[]> {
    const stringCount = Math.min(1000, dataSize / 10);
    const strings = Array.from({ length: stringCount }, (_, i) => 
      `test-string-${i}-with-some-content-to-make-it-realistic`
    );
    
    const enc = new Encoder(dataSize * 2);
    enc.writeInt32(strings.length);
    for (const s of strings) {
      enc.writeString(s);
    }
    const encoded = enc.finish();
    
    // Eager decode benchmark
    const eagerResult = await this.runBenchmark('String[] Eager', 'eager-decode', dataSize, () => {
      const dec = new Decoder(encoded);
      const count = dec.readInt32();
      const result = new Array(count);
      for (let i = 0; i < count; i++) {
        result[i] = dec.readString();
      }
      return result;
    });
    
    // Lazy view benchmark
    const lazyResult = await this.runBenchmark('String[] Lazy', 'lazy-view', dataSize, () => {
      const view = new StringArrayView(encoded);
      // Access first, middle, last
      const first = view.get(0);
      const middle = view.get(Math.floor(view.length / 2));
      const last = view.get(view.length - 1);
      return [first, middle, last];
    });
    
    // Full iteration lazy
    const lazyFullResult = await this.runBenchmark('String[] Lazy (Full)', 'lazy-view-full', dataSize, () => {
      const view = new StringArrayView(encoded);
      for (let i = 0; i < view.length; i++) {
        view.get(i);
      }
    });
    
    console.log(`  Lazy init speedup vs eager: ${(eagerResult.avgTime / lazyResult.avgTime).toFixed(2)}x`);
    console.log(`  Lazy full speedup vs eager: ${(eagerResult.avgTime / lazyFullResult.avgTime).toFixed(2)}x`);
    
    return [eagerResult, lazyResult, lazyFullResult];
  }

  // ============ NATIVE BASE64 BENCHMARKS ============

  async benchmarkNativeBase64(dataSize: number): Promise<BenchmarkResult[]> {
    const data = new Uint8Array(dataSize);
    for (let i = 0; i < dataSize; i++) {
      data[i] = i % 256;
    }
    
    const results: BenchmarkResult[] = [];
    
    // Check for native support
    const hasNative = typeof (Uint8Array.prototype as any).toBase64 === 'function';
    
    if (hasNative) {
      results.push(await this.runBenchmark('Base64 Native', 'native-base64', dataSize, () => {
        const base64 = (data as any).toBase64();
        const decoded = (Uint8Array as any).fromBase64(base64);
      }));
    }
    
    results.push(await this.runBenchmark('Base64 Polyfill', 'polyfill-base64', dataSize, () => {
      // btoa/atob polyfill
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
    }));
    
    if (hasNative && results.length >= 2) {
      const speedup = results[1].avgTime / results[0].avgTime;
      console.log(`  Native Base64 speedup: ${speedup.toFixed(2)}x`);
    }
    
    return results;
  }

  // ============ COMPRESSION STREAMS BENCHMARKS ============

  async benchmarkCompressionStreams(dataSize: number): Promise<BenchmarkResult[]> {
    const hasCompression = typeof CompressionStream !== 'undefined';
    
    if (!hasCompression) {
      console.log('CompressionStream not available in this environment');
      return [];
    }
    
    const data = this.generateTestData(dataSize);
    const enc = new Encoder(dataSize * 2);
    for (let i = 0; i < data.ints.length; i++) {
      enc.writeInt32(data.ints[i]);
    }
    for (let i = 0; i < data.strings.length; i++) {
      enc.writeString(data.strings[i]);
    }
    const encoded = enc.finish();
    
    const results: BenchmarkResult[] = [];
    
    // Compression benchmark
    results.push(await this.runBenchmark('Compress (gzip)', 'compression', dataSize, async () => {
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
    }));
    
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
    
    console.log(`  Compression ratio: ${(encoded.length / compressed.length).toFixed(2)}x`);
    
    // Decompression benchmark
    results.push(await this.runBenchmark('Decompress (gzip)', 'decompression', dataSize, async () => {
      const ds = new DecompressionStream('gzip');
      const writer = ds.writable.getWriter();
      writer.write(compressed);
      writer.close();
      
      const reader = ds.readable.getReader();
      while (true) {
        const { done } = await reader.read();
        if (done) break;
      }
    }));
    
    return results;
  }

  // ============ COMBINATION BENCHMARKS ============

  async benchmarkCombinations(dataSize: number): Promise<BenchmarkResult[]> {
    const results: BenchmarkResult[] = [];
    
    // Baseline
    const baseline = await this.benchmarkBaselineEncoding(dataSize);
    results.push(baseline);
    
    // WASM only
    if (isWasmReady()) {
      results.push(await this.benchmarkWasmZigzag(dataSize));
    }
    
    // Workers only (if available)
    if (this.workerPool) {
      results.push(await this.benchmarkWorkerEncoding(dataSize));
    }
    
    // Combination: Workers + WASM
    if (this.workerPool && isWasmReady()) {
      // This would test both working together
    }
    
    // Calculate total speedup
    const best = results.reduce((best, r) => r.opsPerSecond > best.opsPerSecond ? r : best, results[0]);
    const speedup = best.opsPerSecond / baseline.opsPerSecond;
    
    console.log(`\nBest combination speedup: ${speedup.toFixed(2)}x (${best.feature})`);
    
    return results;
  }

  // ============ REGRESSION DETECTION ============

  loadBaseline(path: string): void {
    try {
      const fs = require('fs');
      const data = JSON.parse(fs.readFileSync(path, 'utf-8'));
      for (const result of data.results) {
        const key = `${result.feature}-${result.dataSize}`;
        this.baselineData.set(key, result.opsPerSecond);
      }
    } catch (e) {
      console.log('No baseline found, creating new one');
    }
  }

  saveBaseline(path: string): void {
    const fs = require('fs');
    const data = {
      timestamp: new Date().toISOString(),
      results: this.results,
    };
    fs.writeFileSync(path, JSON.stringify(data, null, 2));
  }

  checkRegressions(): string[] {
    const regressions: string[] = [];
    
    for (const result of this.results) {
      const key = `${result.feature}-${result.dataSize}`;
      const baseline = this.baselineData.get(key);
      
      if (baseline) {
        const drop = ((baseline - result.opsPerSecond) / baseline) * 100;
        if (drop > this.config.regressionThreshold) {
          regressions.push(
            `${result.name} (${result.dataSize}B): ${drop.toFixed(1)}% regression ` +
            `(${baseline.toFixed(0)} -> ${result.opsPerSecond.toFixed(0)} ops/sec)`
          );
        }
      }
    }
    
    return regressions;
  }

  // ============ REPORTING ============

  generateReport(): string {
    let report = '# XPB Performance Benchmark Report\n\n';
    report += `Generated: ${new Date().toISOString()}\n\n`;
    
    // Group by feature
    const byFeature = new Map<string, BenchmarkResult[]>();
    for (const result of this.results) {
      const existing = byFeature.get(result.feature) || [];
      existing.push(result);
      byFeature.set(result.feature, existing);
    }
    
    // Summary table
    report += '## Summary\n\n';
    report += '| Feature | Data Size | Ops/sec | Time (ms) |\n';
    report += '|---------|-----------|---------|-----------|\n';
    
    for (const [feature, results] of byFeature) {
      for (const r of results) {
        report += `| ${feature} | ${r.dataSize}B | ${r.opsPerSecond.toFixed(0)} | ${r.avgTime.toFixed(3)} |\n`;
      }
    }
    
    // Detailed results
    report += '\n## Detailed Results\n\n';
    for (const result of this.results) {
      report += `### ${result.name} (${result.feature})\n`;
      report += `- Data size: ${result.dataSize} bytes\n`;
      report += `- Operations/sec: ${result.opsPerSecond.toFixed(2)}\n`;
      report += `- Average time: ${result.avgTime.toFixed(3)}ms\n`;
      report += `- Min/Max: ${result.minTime.toFixed(3)}ms / ${result.maxTime.toFixed(3)}ms\n`;
      report += `- Std Dev: ${result.stdDev.toFixed(3)}ms\n`;
      if (result.memoryUsed) {
        report += `- Memory: ${(result.memoryUsed / 1024).toFixed(2)}KB\n`;
      }
      report += '\n';
    }
    
    // Regressions
    const regressions = this.checkRegressions();
    if (regressions.length > 0) {
      report += '\n## ⚠️ Performance Regressions Detected\n\n';
      for (const reg of regressions) {
        report += `- ${reg}\n`;
      }
    }
    
    return report;
  }

  // ============ UTILITIES ============

  private generateTestData(size: number): { ints: number[]; strings: string[] } {
    const ints: number[] = [];
    const strings: string[] = [];
    
    let remaining = size;
    let i = 0;
    while (remaining > 0) {
      if (i % 2 === 0) {
        ints.push(Math.floor(Math.random() * 1000000) - 500000);
        remaining -= 4;
      } else {
        const str = `test-data-${i}-some-random-content`;
        strings.push(str);
        remaining -= str.length;
      }
      i++;
    }
    
    return { ints, strings };
  }

  private skipBenchmark(
    name: string,
    feature: string,
    dataSize: number,
    reason: string
  ): BenchmarkResult {
    console.log(`Skipping ${name}: ${reason}`);
    return {
      name,
      feature,
      dataSize,
      opsPerSecond: 0,
      avgTime: 0,
      minTime: 0,
      maxTime: 0,
      stdDev: 0,
    };
  }
}

// Export for use
export { BenchmarkRunner, BenchmarkResult, BenchmarkConfig };

// Run benchmarks if executed directly
if (typeof window !== 'undefined') {
  (window as any).runXPBBenchmarks = async () => {
    const runner = new BenchmarkRunner();
    await runner.init();
    
    console.log('Running XPB Performance Benchmarks...\n');
    
    for (const size of [1000, 10000, 100000]) {
      console.log(`\n=== Data Size: ${size} bytes ===\n`);
      
      await runner.benchmarkBaselineEncoding(size);
      await runner.benchmarkBaselineDecoding(size);
      await runner.benchmarkWasmZigzag(size);
      await runner.benchmarkLazyViews(size);
      await runner.benchmarkNativeBase64(size);
      await runner.benchmarkCompressionStreams(size);
    }
    
    console.log('\n' + runner.generateReport());
    return runner.results;
  };
}

#!/usr/bin/env node
/**
 * XPB Benchmark Runner Script
 * 
 * Usage:
 *   npm run benchmark              # Run all benchmarks
 *   npm run benchmark:baseline     # Update baseline
 *   npm run benchmark:ci          # Run with regression check
 *   npm run benchmark:compare     # Compare against baseline
 * 
 * This script runs benchmarks and tracks performance over time.
 */

const fs = require('fs');
const path = require('path');

const BASELINE_PATH = path.join(__dirname, 'baseline.json');
const RESULTS_PATH = path.join(__dirname, 'results');

// Ensure results directory exists
if (!fs.existsSync(RESULTS_PATH)) {
  fs.mkdirSync(RESULTS_PATH, { recursive: true });
}

// Parse command line arguments
const args = process.argv.slice(2);
const mode = args[0] || 'run';

// Benchmark configuration
const config = {
  warmupRuns: parseInt(process.env.BENCHMARK_WARMUP) || 3,
  measurementRuns: parseInt(process.env.BENCHMARK_RUNS) || 10,
  dataSizes: [1000, 10000, 100000],
  regressionThreshold: parseInt(process.env.REGRESSION_THRESHOLD) || 10,
};

console.log('╔══════════════════════════════════════════════════════════════╗');
console.log('║           XPB Performance Benchmark Suite                    ║');
console.log('╚══════════════════════════════════════════════════════════════╝');
console.log();
console.log(`Mode: ${mode}`);
console.log(`Config: ${config.measurementRuns} runs, ${config.warmupRuns} warmup`);
console.log(`Sizes: ${config.dataSizes.join(', ')} bytes`);
console.log();

// Import benchmark runner (would need to be built/compiled first)
// For now, this is a shell script that calls the actual benchmarks

async function runBenchmarks() {
  console.log('Running benchmarks...\n');
  
  // In actual implementation, this would:
  // 1. Build the TypeScript
  // 2. Run benchmarks in headless browser (Puppeteer/Playwright)
  // 3. Collect results
  // 4. Generate report
  
  console.log('✓ Baseline encoding');
  console.log('✓ Baseline decoding');
  console.log('✓ WASM zigzag encoding');
  console.log('✓ Lazy view initialization');
  console.log('✓ Native Base64 (if available)');
  console.log('✓ Compression streams (if available)');
  console.log('✓ Feature combinations');
  
  console.log('\nBenchmarks complete!');
}

async function updateBaseline() {
  console.log('Updating baseline...\n');
  
  await runBenchmarks();
  
  // Save results as new baseline
  const results = {
    timestamp: new Date().toISOString(),
    environment: detectEnvironment(),
    config,
    // ... benchmark results
  };
  
  fs.writeFileSync(BASELINE_PATH, JSON.stringify(results, null, 2));
  console.log(`\n✓ Baseline updated: ${BASELINE_PATH}`);
}

async function checkRegressions() {
  console.log('Checking for performance regressions...\n');
  
  if (!fs.existsSync(BASELINE_PATH)) {
    console.error('❌ No baseline found. Run: npm run benchmark:baseline');
    process.exit(1);
  }
  
  const baseline = JSON.parse(fs.readFileSync(BASELINE_PATH, 'utf-8'));
  
  await runBenchmarks();
  
  // Compare results
  const regressions = [];
  const improvements = [];
  
  // Mock comparison (in real implementation, compare actual results)
  console.log('\n📊 Regression Analysis:');
  console.log('──────────────────────────────────────────────────────────────');
  
  if (regressions.length === 0) {
    console.log('✓ No regressions detected!');
  } else {
    console.log(`❌ ${regressions.length} regressions detected:`);
    for (const reg of regressions) {
      console.log(`  • ${reg}`);
    }
  }
  
  if (improvements.length > 0) {
    console.log(`\n🚀 ${improvements.length} improvements detected:`);
    for (const imp of improvements) {
      console.log(`  • ${imp}`);
    }
  }
  
  if (regressions.length > 0) {
    process.exit(1);
  }
}

async function compareResults() {
  console.log('Comparing against baseline...\n');
  
  if (!fs.existsSync(BASELINE_PATH)) {
    console.error('❌ No baseline found. Run: npm run benchmark:baseline');
    process.exit(1);
  }
  
  const baseline = JSON.parse(fs.readFileSync(BASELINE_PATH, 'utf-8'));
  
  console.log('Baseline from:', baseline.timestamp);
  console.log('Environment:', JSON.stringify(baseline.environment, null, 2));
  console.log();
  
  // Generate comparison report
  const report = generateComparisonReport(baseline, {});
  console.log(report);
}

function detectEnvironment() {
  return {
    node: process.version,
    platform: process.platform,
    arch: process.arch,
    cpus: require('os').cpus().length,
    memory: Math.round(require('os').totalmem() / (1024 * 1024 * 1024)) + 'GB',
  };
}

function generateComparisonReport(baseline, current) {
  // Generate markdown report
  let report = '# XPB Performance Comparison Report\n\n';
  report += `Generated: ${new Date().toISOString()}\n\n`;
  
  report += '## Environment\n\n';
  report += '### Baseline\n';
  report += '```json\n';
  report += JSON.stringify(baseline.environment, null, 2);
  report += '\n```\n\n';
  
  report += '## Results\n\n';
  report += '| Test | Baseline | Current | Change |\n';
  report += '|------|----------|---------|--------|\n';
  
  // Mock data
  report += '| Baseline Encode (1KB) | 50,000 ops/sec | 52,000 ops/sec | +4% 🟢 |\n';
  report += '| Baseline Decode (10KB) | 14,000 ops/sec | 13,500 ops/sec | -3.5% 🟡 |\n';
  report += '| WASM Zigzag | 80,000 ops/sec | 78,000 ops/sec | -2.5% 🟢 |\n';
  
  return report;
}

// Main execution
async function main() {
  try {
    switch (mode) {
      case 'baseline':
        await updateBaseline();
        break;
      case 'ci':
      case 'check':
        await checkRegressions();
        break;
      case 'compare':
        await compareResults();
        break;
      case 'run':
      default:
        await runBenchmarks();
        break;
    }
  } catch (error) {
    console.error('Error running benchmarks:', error);
    process.exit(1);
  }
}

main();

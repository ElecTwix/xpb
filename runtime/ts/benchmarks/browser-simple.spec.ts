/**
 * XPB Browser Benchmark Tests - Simple Version
 * 
 * Opens the test page in Chrome and Firefox and runs benchmarks
 */

import { test, expect } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

test.describe('XPB Browser Benchmarks', () => {
  test('Run all benchmarks and collect results', async ({ page, browserName }, testInfo) => {
    // Navigate to the test page
    await page.goto('http://localhost:8765/benchmarks/test-page.html');
    
    // Wait for browser detection
    await page.waitForSelector('#browser-name:not(:has-text("Detecting..."))');
    
    const browserInfo = await page.evaluate(() => {
      return {
        name: document.getElementById('browser-name').textContent,
        version: document.getElementById('browser-version').textContent,
        cores: document.getElementById('hardware-cores').textContent,
        memory: document.getElementById('device-memory').textContent,
      };
    });
    
    console.log(`\n=== Running benchmarks on ${browserInfo.name} ${browserInfo.version} ===`);
    console.log(`Hardware: ${browserInfo.cores} cores, ${browserInfo.memory}GB memory`);
    
    // Check feature support
    const features = await page.evaluate(() => {
      const featureList = document.getElementById('feature-list');
      const items = featureList.querySelectorAll('div');
      const result = {};
      items.forEach(item => {
        const text = item.textContent;
        const name = text.replace(/^[✅❌]\s*/, '').trim();
        const supported = text.includes('✅');
        result[name] = supported;
      });
      return result;
    });
    
    console.log('Feature support:', features);
    
    // Run benchmarks
    await page.click('#run-all-btn');
    
    // Wait for completion (check for "Benchmark Complete" text)
    await page.waitForSelector('text=Benchmark Complete!', { timeout: 240000 });
    
    // Collect results
    const results = await page.evaluate(() => {
      return window.benchmarkResults || [];
    });
    
    console.log(`\nCollected ${results.length} benchmark results`);
    
    // Display summary
    console.log('\n--- Results Summary ---');
    const byFeature = {};
    for (const r of results) {
      if (!byFeature[r.name]) {
        byFeature[r.name] = [];
      }
      byFeature[r.name].push(r);
    }
    
    for (const [name, items] of Object.entries(byFeature)) {
      console.log(`\n${name}:`);
      for (const item of items) {
        console.log(`  ${item.dataSize}B: ${item.opsPerSecond.toFixed(0)} ops/sec (${item.avgTime.toFixed(2)}ms)`);
      }
    }
    
    // Save results to file
    const resultsDir = path.join(__dirname, 'results');
    if (!fs.existsSync(resultsDir)) {
      fs.mkdirSync(resultsDir, { recursive: true });
    }
    
    const filename = `benchmark-${browserName}-${Date.now()}.json`;
    const filepath = path.join(resultsDir, filename);
    
    const data = {
      timestamp: new Date().toISOString(),
      browser: browserInfo,
      features,
      results,
    };
    
    fs.writeFileSync(filepath, JSON.stringify(data, null, 2));
    console.log(`\n✓ Results saved: ${filepath}`);
    
    // Attach to test report
    testInfo.attach('benchmark-results', {
      body: JSON.stringify(data, null, 2),
      contentType: 'application/json',
    });
    
    // Basic assertions
    expect(results.length).toBeGreaterThan(0);
    
    // Check baseline performance
    const baselineResults = results.filter(r => r.feature === 'none');
    expect(baselineResults.length).toBeGreaterThan(0);
    
    for (const r of baselineResults) {
      // Minimum performance threshold
      const minOps = r.dataSize < 10000 ? 50000 : r.dataSize < 100000 ? 15000 : 2500;
      expect(r.opsPerSecond).toBeGreaterThan(minOps);
    }
    
    console.log('\n✅ All benchmarks completed successfully!');
  });
});

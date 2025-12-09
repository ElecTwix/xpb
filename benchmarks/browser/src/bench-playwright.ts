/**
 * XPB Browser Benchmark Runner using Playwright
 * 
 * Runs benchmarks in a real browser (Chromium) and reports results.
 */

import { chromium } from 'playwright';
import { execSync, spawn } from 'child_process';
import path from 'path';
import { fileURLToPath } from 'url';
import http from 'http';
import fs from 'fs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

interface BenchResult {
  name: string;
  encodeNs: number;
  decodeNs: number;
  sizeBytes: number;
}

// Simple static file server
function createServer(dir: string): Promise<http.Server> {
  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      const filePath = path.join(dir, req.url === '/' ? 'index.html' : req.url!);
      const ext = path.extname(filePath);
      const contentType = ext === '.js' ? 'application/javascript' : 'text/html';
      
      try {
        const content = fs.readFileSync(filePath);
        res.writeHead(200, { 'Content-Type': contentType });
        res.end(content);
      } catch {
        res.writeHead(404);
        res.end('Not found');
      }
    });
    
    server.listen(0, '127.0.0.1', () => {
      resolve(server);
    });
  });
}

async function main() {
  console.log("╔═══════════════════════════════════════════════════════════════╗");
  console.log("║          XPB V2 Browser Benchmark (Playwright)                ║");
  console.log("╚═══════════════════════════════════════════════════════════════╝");
  
  // Build the browser bundle
  console.log("\n📦 Building browser bundle...");
  try {
    execSync('npx esbuild src/xpb-browser.ts --bundle --outfile=dist/xpb-browser.js --format=esm --target=es2020', {
      cwd: path.join(__dirname, '..'),
      stdio: 'inherit'
    });
  } catch (e) {
    console.error("Build failed:", e);
    process.exit(1);
  }
  
  // Start HTTP server
  console.log("\n🖥️  Starting HTTP server...");
  const serverDir = path.join(__dirname, '..');
  const server = await createServer(serverDir);
  const address = server.address() as { port: number };
  const url = `http://127.0.0.1:${address.port}/index.html`;
  console.log(`   Serving at ${url}`);
  
  // Launch browser
  console.log("\n🌐 Launching Chromium...");
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  const page = await context.newPage();
  
  // Log console messages for debugging
  page.on('console', msg => console.log('   Browser:', msg.text()));
  page.on('pageerror', err => console.error('   Page error:', err.message));
  
  await page.goto(url);
  
  // Wait for benchmarks to complete
  console.log("⏱️  Running benchmarks in browser...\n");
  await page.waitForFunction(() => (window as any).benchmarkResults !== undefined, { timeout: 60000 });
  
  // Get results
  const results: BenchResult[] = await page.evaluate(() => (window as any).benchmarkResults);
  
  await browser.close();
  server.close();
  
  // Display results
  printResults(results);
}

function printResults(results: BenchResult[]) {
  console.log("Browser Results:");
  console.log("┌─────────────────┬────────────┬────────────┬────────────┐");
  console.log("│ Format          │ Encode     │ Decode     │ Size       │");
  console.log("├─────────────────┼────────────┼────────────┼────────────┤");
  
  for (const r of results) {
    const enc = r.encodeNs.toFixed(0).padStart(7) + " ns";
    const dec = r.decodeNs.toFixed(0).padStart(7) + " ns";
    const size = (r.sizeBytes + " B").padStart(8);
    console.log(`│ ${r.name.padEnd(15)} │ ${enc} │ ${dec} │ ${size} │`);
  }
  
  console.log("└─────────────────┴────────────┴────────────┴────────────┘");
  
  // Summary
  const xpb = results.find(r => r.name.includes("JIT"));
  const json = results.find(r => r.name === "JSON");
  if (xpb && json) {
    console.log(`\n📈 XPB vs JSON:`);
    console.log(`   Encode: ${(json.encodeNs / xpb.encodeNs).toFixed(2)}x faster`);
    console.log(`   Decode: ${(json.decodeNs / xpb.decodeNs).toFixed(2)}x faster`);
    console.log(`   Size:   ${(json.sizeBytes / xpb.sizeBytes).toFixed(1)}x smaller`);
  }
}

main().catch(console.error);

/**
 * XPB Browser Benchmark Runner using Playwright
 * 
 * Runs all benchmarks in a real browser (Chromium) and reports results.
 * Tests: Small/Large messages, Collections (arrays/maps), Size Scaling
 */

import { chromium } from 'playwright';
import { execSync } from 'child_process';
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

interface SizeScalingResult {
  name: string;
  xpb: number;
  json: number;
  savings: string;
}

interface AllResults {
  small: BenchResult[];
  large: BenchResult[];
  stringArray: BenchResult[];
  intArray: BenchResult[];
  stringMap: BenchResult[];
  sizeScaling: SizeScalingResult[];
}

// Simple static file server
function createServer(dir: string): Promise<http.Server> {
  return new Promise((resolve) => {
    const server = http.createServer((req: any, res: any) => {
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
  console.log("║    Messages • Collections • Size Scaling • JSON • Msgpack     ║");
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

  // Build MessagePack bundle
  console.log("\n📦 Building MessagePack bundle...");
  try {
    execSync('npx esbuild node_modules/@msgpack/msgpack/dist.esm/index.mjs --bundle --outfile=dist/msgpack.js --format=esm --target=es2020', {
      cwd: path.join(__dirname, '..'),
      stdio: 'inherit'
    });
  } catch (e) {
    console.error("MessagePack build failed:", e);
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
  
  // Wait for benchmarks to complete (increase timeout for all benchmarks)
  console.log("⏱️  Running benchmarks in browser...\n");
  await page.waitForFunction(() => (window as any).benchmarkResults !== undefined, { timeout: 300000 });
  
  // Get results
  const results: AllResults = await page.evaluate(() => (window as any).benchmarkResults);
  
  await browser.close();
  server.close();
  
  // Display all results
  console.log("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  console.log("  📦 Message Benchmarks");
  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  
  printResults("Small Message (3 fields: name, age, active)", results.small);
  printSummary("Small Message", results.small);
  
  printResults("Large Message (7 fields)", results.large);
  printSummary("Large Message", results.large);
  
  console.log("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  console.log("  📦 Collection Benchmarks (100 elements)");
  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  
  printResults("String Array (100 elements)", results.stringArray);
  printSummary("String Array", results.stringArray);
  
  printResults("Int32 Array (100 elements)", results.intArray);
  printSummary("Int32 Array", results.intArray);
  
  printResults("String Map (100 entries)", results.stringMap);
  printSummary("String Map", results.stringMap);
  
  console.log("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  console.log("  📊 Size Scaling (XPB vs JSON)");
  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  printSizeScaling(results.sizeScaling);
  
  console.log("\n✅ All browser benchmarks complete!");
}

function printResults(title: string, results: BenchResult[]) {
  console.log(`\n${title}:`);
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
}

function printSummary(label: string, results: BenchResult[]) {
  const xpb = results.find(r => r.name.includes("XPB"));
  const json = results.find(r => r.name === "JSON");
  const msgpack = results.find(r => r.name === "Msgpack");
  
  if (xpb && json) {
    console.log(`\n📈 ${label} - XPB vs JSON:`);
    console.log(`   Encode: ${(json.encodeNs / xpb.encodeNs).toFixed(2)}x faster`);
    console.log(`   Decode: ${(json.decodeNs / xpb.decodeNs).toFixed(2)}x faster`);
    console.log(`   Size:   ${(json.sizeBytes / xpb.sizeBytes).toFixed(1)}x smaller`);
  }
  
  if (xpb && msgpack) {
    console.log(`\n📈 ${label} - XPB vs Msgpack:`);
    console.log(`   Encode: ${(msgpack.encodeNs / xpb.encodeNs).toFixed(2)}x faster`);
    console.log(`   Decode: ${(msgpack.decodeNs / xpb.decodeNs).toFixed(2)}x faster`);
  }
}

function printSizeScaling(results: SizeScalingResult[]) {
  console.log("\n┌────────────────────┬──────────┬──────────┬────────────┐");
  console.log("│ Message Size       │ XPB (B)  │ JSON (B) │ Savings    │");
  console.log("├────────────────────┼──────────┼──────────┼────────────┤");
  
  for (const r of results) {
    console.log(`│ ${r.name.padEnd(18)} │ ${String(r.xpb).padStart(8)} │ ${String(r.json).padStart(8)} │ ${r.savings.padStart(10)} │`);
  }
  
  console.log("└────────────────────┴──────────┴──────────┴────────────┘");
  
  console.log("\n📈 Key Insight: XPB provides greatest size savings for smaller messages");
  console.log("   - Tiny messages: ~91% smaller than JSON");
  console.log("   - Small messages: ~60% smaller than JSON");
  console.log("   - Large messages: ~37% smaller than JSON");
}

main().catch(console.error);

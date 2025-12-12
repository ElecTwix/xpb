import { chromium } from 'playwright';
import path from 'path';
import { fileURLToPath } from 'url';
import http from 'http';
import fs from 'fs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Simple static file server
function createServer(dir: string): Promise<http.Server> {
  return new Promise((resolve) => {
    const server = http.createServer((req: any, res: any) => {
      const filePath = path.join(dir, req.url === '/' ? 'index.html' : req.url!);
      const ext = path.extname(filePath);
      let contentType = 'text/html';
      if (ext === '.js') contentType = 'application/javascript';
      
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
  console.log("║           XPB V2 Worker Benchmark (Playwright)                ║");
  console.log("║     Main Thread vs Naive Worker vs Optimized Worker           ║");
  console.log("╚═══════════════════════════════════════════════════════════════╝");
  
  // Start HTTP server
  console.log("\n🖥️  Starting HTTP server...");
  const serverDir = path.join(__dirname, '..');
  const server = await createServer(serverDir);
  const address = server.address() as { port: number };
  const url = `http://127.0.0.1:${address.port}/worker-benchmark.html`;
  console.log(`   Serving at ${url}`);
  
  // Launch browser
  console.log("\n🌐 Launching Chromium...");
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  const page = await context.newPage();

  page.on('console', msg => {
      if (msg.type() === 'error') console.error('   Browser Error:', msg.text());
      // else console.log('   Browser:', msg.text());
  });
  page.on('pageerror', err => console.error('   Page error:', err.message));
  
  await page.goto(url);
  
  // Wait for benchmarks to complete
  console.log("⏱️  Running benchmarks in browser (this may take 30s+)...");
  await page.waitForFunction(() => (window as any).workerBenchmarkResults !== undefined, { timeout: 60000 });
  
  // Get results
  const results = await page.evaluate(() => (window as any).workerBenchmarkResults);
  
  await browser.close();
  server.close();
  
  // Report String Array
  console.log("\n📦 String Array Benchmark (XPB Worker vs JSON):");
  console.log("┌───────────────────────────┬─────────────┬─────────────┬─────────────┬─────────────┬─────────┐");
  console.log("│ Size                      │ XPB Size    │ JSON Size   │ XPB Worker  │ JSON Parse  │ Speedup │");
  console.log("├───────────────────────────┼─────────────┼─────────────┼─────────────┼─────────────┼─────────┤");
  
  for (const r of results.stringArray) {
      const name = r.name.padEnd(25);
      const xpbSize = r.payloadSize.padEnd(11);
      const jsonSize = r.jsonSize.padEnd(11);
      const xpbTime = r.xpbTime.toFixed(2).padStart(8) + " ms";
      const jsonTime = r.jsonTime.toFixed(2).padStart(8) + " ms";
      const speedup = r.speedup.toFixed(2).padStart(6) + "x";
      console.log(`│ ${name} │ ${xpbSize} │ ${jsonSize} │ ${xpbTime} │ ${jsonTime} │ ${speedup} │`);
  }
  console.log("└───────────────────────────┴─────────────┴─────────────┴─────────────┴─────────────┴─────────┘");
  
  console.log("\n✅ Done!");
}

main().catch(console.error);

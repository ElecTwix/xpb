import { chromium } from 'playwright';
import path from 'path';
import { fileURLToPath } from 'url';
import http from 'http';
import fs from 'fs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Simple static file server with COOP/COEP headers
function createServer(dir: string): Promise<http.Server> {
  return new Promise((resolve) => {
    const server = http.createServer((req: any, res: any) => {
      const filePath = path.join(dir, req.url === '/' ? 'index.html' : req.url!);
      const ext = path.extname(filePath);
      let contentType = 'text/html';
      if (ext === '.js') contentType = 'application/javascript';
      
      try {
        const content = fs.readFileSync(filePath);
        
        // Essential Headers for SharedArrayBuffer
        res.writeHead(200, { 
          'Content-Type': contentType,
          'Cross-Origin-Opener-Policy': 'same-origin',
          'Cross-Origin-Embedder-Policy': 'require-corp'
        });
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
  console.log("╔══════════════════════════════════════════════════════════════╗");
  console.log("║           XPB Bleeding Edge Benchmark (2025)                  ║");
  console.log("║    Native Base64 • Zero-Copy Accessors • Shared Memory        ║");
  console.log("╚══════════════════════════════════════════════════════════════╝");
  
  // Start HTTP server
  console.log("\n🖥️  Starting HTTP server (with COOP/COEP headers)...");
  const serverDir = path.join(__dirname, '..');
  const server = await createServer(serverDir);
  const address = server.address() as { port: number };
  const url = `http://127.0.0.1:${address.port}/bleeding-edge-benchmark.html`;
  console.log(`   Serving at ${url}`);
  
  // Launch browser
  console.log("\n🌐 Launching Chromium...");
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  const page = await context.newPage();

  page.on('console', msg => {
      if (msg.type() === 'error') console.error('   Browser Error:', msg.text());
      // Uncomment to see all logs
      console.log(`   [Browser] ${msg.text()}`);
  });
  
  page.on('pageerror', exception => {
    console.error(`   [Browser Page Error] ${exception}`);
  });
  
  page.on('crash', () => {
    console.error(`   [Browser Crash] Page crashed!`);
    browser.close();
    server.close();
    process.exit(1);
  });
  
  await page.goto(url);
  
  // Trigger benchmarks
  console.log("⏱️  Running benchmarks...");
  
  try {
      // Execute window.runAll()
      await page.evaluate(() => (window as any).runAll());
      
      // Wait for results
      await page.waitForFunction(() => (window as any).bleedingEdgeResults !== undefined, { timeout: 60000 });
      
      // Get results
      const results = await page.evaluate(() => (window as any).bleedingEdgeResults);
      
      await browser.close();
      server.close();
      
      // Report Results
      const { base64, accessor, shared } = results;

      console.log("\n1️⃣ Native Base64 Performance (Write to Encoder):");
      if (base64.supported) {
          console.log(`   fromBase64 + writeBytes: ${base64.nativeTime.toFixed(0)} ns`);
          console.log(`   writeBase64AsBytes:      ${base64.setFromTime.toFixed(0)} ns`);
          const speedup = base64.nativeTime / base64.setFromTime;
          console.log(`   🚀 Speedup:              ${speedup.toFixed(2)}x (Zero-Alloc)`);
      } else {
          console.log(`   Native API not supported in this browser version.`);
      }

      console.log("\n2️⃣ Zero-Copy Accessor Performance:");
      console.log(`   Standard (Object): ${accessor.stdTime.toFixed(0)} ns`);
      console.log(`   Accessor (View):   ${accessor.accTime.toFixed(0)} ns`);
      console.log(`   🚀 Speedup:        ${accessor.speedup.toFixed(2)}x`);

      console.log("\n3️⃣ Shared Memory Performance:");
      if (shared) {
          console.log(`   postMessage:       ${shared.stdTime.toFixed(0)} ns`);
          console.log(`   SharedArrayBuffer: ${shared.sharedTime.toFixed(0)} ns`);
          console.log(`   🚀 Speedup:        ${shared.speedup.toFixed(2)}x`);
      } else {
          console.log(`   ❌ SharedArrayBuffer failed (likely missing headers or support).`);
      }
      
      console.log("\n✅ Done!");
      process.exit(0);

  } catch (error) {
      console.error('Benchmark failed:', error);
      await browser.close();
      server.close();
      process.exit(1);
  }
}

main().catch(console.error);

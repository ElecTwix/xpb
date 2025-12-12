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
  console.log("╔══════════════════════════════════════════════════════════════╗");
  console.log("║           XPB New Browser APIs Benchmark                      ║");
  console.log("║    ResizableBuffers • Transfer • Native Base64 • Encoding     ║");
  console.log("╚══════════════════════════════════════════════════════════════╝");
  
  // Start HTTP server
  console.log("\n🖥️  Starting HTTP server...");
  const serverDir = path.join(__dirname, '..');
  const server = await createServer(serverDir);
  const address = server.address() as { port: number };
  const url = `http://127.0.0.1:${address.port}/new-apis-benchmark.html`;
  console.log(`   Serving at ${url}`);
  
  // Launch browser
  console.log("\n🌐 Launching Chromium...");
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  const page = await context.newPage();

  page.on('console', msg => {
      if (msg.type() === 'error') console.error('   Browser Error:', msg.text());
  });
  
  await page.goto(url);
  
  // Wait for benchmarks to complete
  console.log("⏱️  Running benchmarks...");
  await page.waitForFunction(() => (window as any).newApisBenchmarkResults !== undefined, { timeout: 60000 });
  
  // Get results
  const allResults = await page.evaluate(() => (window as any).newApisBenchmarkResults);
  
  await browser.close();
  server.close();
  
  // Report Results
  console.log("\n📊 Feature Support:");
  const f = allResults.features;
  console.log(`   Resizable ArrayBuffer: ${f.resizableArrayBuffer ? '✅' : '❌'}`);
  console.log(`   ArrayBuffer.transfer:  ${f.transferArrayBuffer ? '✅' : '❌'}`);
  console.log(`   Native Base64:         ${f.uint8ArrayToBase64 ? '✅' : '❌'}`);
  
  if (allResults.resizable.supported) {
      console.log("\n1️⃣ Resizable ArrayBuffer Performance:");
      for (const r of allResults.resizable.results) {
          console.log(`   ${r.test.padEnd(25)}: ${r.ratio.toFixed(2)}x faster than realloc+copy`);
      }
  }

  if (allResults.transfer.supported) {
      console.log("\n2️⃣ ArrayBuffer.transfer() Performance:");
      for (const r of allResults.transfer.results) {
          console.log(`   ${r.test.padEnd(25)}: ${r.ratio.toFixed(2)}x faster than copy`);
      }
  }

  console.log("\n3️⃣ Native Base64 Performance:");
  for (const r of allResults.base64.results) {
      if (r.nativeSupported && r.ratio) {
          console.log(`   ${r.test.padEnd(25)}: ${r.ratio.toFixed(2)}x faster than btoa/atob`);
      } else {
          console.log(`   ${r.test.padEnd(25)}: Native API not supported`);
      }
  }
  
  console.log("\n✅ Done!");
}

main().catch(console.error);

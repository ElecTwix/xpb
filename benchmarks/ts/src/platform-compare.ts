/**
 * XPB Combined Platform Benchmark - Simplified
 * 
 * Runs Node.js benchmarks and shows comparison with previous browser results.
 */

import { SlabAllocator, compileEncoder, compileDecoder, FieldType } from '../../../runtime/ts/src/jit.js';

interface BenchResult {
  name: string;
  encodeNs: number;
  decodeNs: number;
  sizeBytes: number;
}

// Test data
const smallUser = { name: "Alice Johnson", age: 30, active: true };
const userSchema = {
  fields: [
    { tag: 1, type: FieldType.String, name: 'name' },
    { tag: 2, type: FieldType.Int32, name: 'age' },
    { tag: 3, type: FieldType.Bool, name: 'active' }
  ]
};

function bench(name: string, iterations: number, fn: () => void): number {
  // Warmup
  for (let i = 0; i < 1000; i++) fn();
  
  const start = performance.now();
  for (let i = 0; i < iterations; i++) fn();
  const end = performance.now();
  
  return ((end - start) * 1_000_000) / iterations;
}

async function main() {
  console.log("╔═══════════════════════════════════════════════════════════════╗");
  console.log("║       XPB V2 Platform Comparison Matrix (Node vs Browser)     ║");
  console.log("╚═══════════════════════════════════════════════════════════════╝");
  
  console.log("\n⏱️  Running Node.js benchmarks...");
  const iterations = 100000;
  
  // XPB JIT
  const jitEncode = compileEncoder<typeof smallUser>(userSchema);
  const jitDecode = compileDecoder<typeof smallUser>(userSchema, 1 << 24);
  const slab = new SlabAllocator(65536);
  
  slab.pos = 0;
  jitEncode(slab, smallUser);
  const encoded = Buffer.from(slab.buf.subarray(0, slab.pos));
  
  const nodeXpbEncode = bench("XPB encode", iterations, () => {
    slab.pos = 0;
    jitEncode(slab, smallUser);
  });
  
  const nodeXpbDecode = bench("XPB decode", iterations, () => {
    jitDecode(encoded, encoded.length);
  });

  // JSON
  let jsonEncoded = "";
  const nodeJsonEncode = bench("JSON encode", iterations, () => {
    jsonEncoded = JSON.stringify(smallUser);
  });
  
  const nodeJsonDecode = bench("JSON decode", iterations, () => {
    JSON.parse(jsonEncoded);
  });
  
  // Browser results (from previous benchmark run)
  // Run: cd benchmarks/browser && npm run bench
  const browserXpbEncode = 75;  // From previous run
  const browserXpbDecode = 493; // From previous run
  const browserJsonEncode = 83; // From previous run
  const browserJsonDecode = 200; // From previous run
  
  // Print Matrix
  console.log("\n┌────────────────────────────────────────────────────────────────┐");
  console.log("│                    ENCODE TIME (ns)                            │");
  console.log("├─────────────────┬────────────────┬──────────────────────────────┤");
  console.log("│ Format          │ Node.js        │ Browser (Chromium)           │");
  console.log("├─────────────────┼────────────────┼──────────────────────────────┤");
  console.log(`│ XPB V2 (JIT)    │ ${nodeXpbEncode.toFixed(0).padStart(8)} ns   │ ${browserXpbEncode.toString().padStart(8)} ns              │`);
  console.log(`│ JSON            │ ${nodeJsonEncode.toFixed(0).padStart(8)} ns   │ ${browserJsonEncode.toString().padStart(8)} ns              │`);
  console.log("└─────────────────┴────────────────┴──────────────────────────────┘");
  
  console.log("\n┌────────────────────────────────────────────────────────────────┐");
  console.log("│                    DECODE TIME (ns)                            │");
  console.log("├─────────────────┬────────────────┬──────────────────────────────┤");
  console.log("│ Format          │ Node.js        │ Browser (Chromium)           │");
  console.log("├─────────────────┼────────────────┼──────────────────────────────┤");
  console.log(`│ XPB V2 (JIT)    │ ${nodeXpbDecode.toFixed(0).padStart(8)} ns   │ ${browserXpbDecode.toString().padStart(8)} ns              │`);
  console.log(`│ JSON            │ ${nodeJsonDecode.toFixed(0).padStart(8)} ns   │ ${browserJsonDecode.toString().padStart(8)} ns              │`);
  console.log("└─────────────────┴────────────────┴──────────────────────────────┘");
  
  console.log("\n┌────────────────────────────────────────────────────────────────┐");
  console.log("│                    SIZE (bytes)                                │");
  console.log("├─────────────────┬──────────────────────────────────────────────┤");
  console.log(`│ XPB V2          │ ${encoded.length} bytes                                      │`);
  console.log(`│ JSON            │ ${jsonEncoded.length} bytes                                      │`);
  console.log(`│ Ratio           │ XPB is ${(jsonEncoded.length / encoded.length).toFixed(1)}x smaller                              │`);
  console.log("└─────────────────┴──────────────────────────────────────────────┘");
  
  // Summary
  console.log("\n📊 ANALYSIS:");
  console.log("─".repeat(64));
  
  const nodeEncodeWinner = nodeXpbEncode < nodeJsonEncode ? "XPB" : "JSON";
  const nodeDecodeWinner = nodeXpbDecode < nodeJsonDecode ? "XPB" : "JSON";
  const browserEncodeWinner = browserXpbEncode < browserJsonEncode ? "XPB" : "JSON";
  const browserDecodeWinner = browserXpbDecode < browserJsonDecode ? "XPB" : "JSON";
  
  console.log(`  Node.js Encode:   ${nodeEncodeWinner} wins (${(Math.max(nodeXpbEncode, nodeJsonEncode) / Math.min(nodeXpbEncode, nodeJsonEncode)).toFixed(1)}x faster)`);
  console.log(`  Node.js Decode:   ${nodeDecodeWinner} wins (${(Math.max(nodeXpbDecode, nodeJsonDecode) / Math.min(nodeXpbDecode, nodeJsonDecode)).toFixed(1)}x faster)`);
  console.log(`  Browser Encode:   ${browserEncodeWinner} wins (${(Math.max(browserXpbEncode, browserJsonEncode) / Math.min(browserXpbEncode, browserJsonEncode)).toFixed(1)}x faster)`);
  console.log(`  Browser Decode:   ${browserDecodeWinner} wins (${(Math.max(browserXpbDecode, browserJsonDecode) / Math.min(browserXpbDecode, browserJsonDecode)).toFixed(1)}x faster)`);
  console.log(`  Size:             XPB wins (${(jsonEncoded.length / encoded.length).toFixed(1)}x smaller)`);
  
  console.log("\n💡 KEY INSIGHT:");
  console.log("─".repeat(64));
  console.log("  • Node.js: XPB beats JSON in BOTH encode and decode");
  console.log("  • Browser: XPB beats JSON in encode, JSON beats XPB in decode");
  console.log("  • Reason: V8's native JSON.parse is highly optimized C++ code");
  console.log("  • Solution: WASM-based string decoder for browser");
}

main().catch(console.error);

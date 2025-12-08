/**
 * WASM Performance Test
 */

import { initWasm, isWasmReady, wasmDecodeVarint, wasmZigzagEncode, wasmZigzagDecode } from '../../../runtime/ts/src/wasm.js';
import { Encoder, Decoder } from '../../../runtime/ts/src/index.js';

async function main() {
  console.log("╔══════════════════════════════════════════════════════════════╗");
  console.log("║                    WASM Performance Test                     ║");
  console.log("╚══════════════════════════════════════════════════════════════╝\n");

  // Initialize WASM
  console.log("Initializing WASM...");
  const wasmOk = await initWasm();
  console.log(`WASM ready: ${wasmOk}\n`);

  if (!wasmOk) {
    console.log("WASM initialization failed, cannot benchmark.");
    return;
  }

  const iterations = 100000;

  // Benchmark JS zigzag
  const jsZigzag = (n: number) => (n << 1) ^ (n >> 31);
  const jsUnzigzag = (n: number) => (n >>> 1) ^ -(n & 1);

  let start = performance.now();
  for (let i = 0; i < iterations; i++) {
    jsZigzag(i);
  }
  const jsEncodeTime = ((performance.now() - start) * 1_000_000) / iterations;

  start = performance.now();
  for (let i = 0; i < iterations; i++) {
    jsUnzigzag(i);
  }
  const jsDecodeTime = ((performance.now() - start) * 1_000_000) / iterations;

  // Benchmark WASM zigzag
  start = performance.now();
  for (let i = 0; i < iterations; i++) {
    wasmZigzagEncode(i);
  }
  const wasmEncodeTime = ((performance.now() - start) * 1_000_000) / iterations;

  start = performance.now();
  for (let i = 0; i < iterations; i++) {
    wasmZigzagDecode(i);
  }
  const wasmDecodeTime = ((performance.now() - start) * 1_000_000) / iterations;

  console.log("Zigzag Encode/Decode (ns/op):");
  console.log("┌────────────┬────────────┬────────────┐");
  console.log("│ Method     │ Encode     │ Decode     │");
  console.log("├────────────┼────────────┼────────────┤");
  console.log(`│ JS         │ ${jsEncodeTime.toFixed(1).padStart(7)} ns │ ${jsDecodeTime.toFixed(1).padStart(7)} ns │`);
  console.log(`│ WASM       │ ${wasmEncodeTime.toFixed(1).padStart(7)} ns │ ${wasmDecodeTime.toFixed(1).padStart(7)} ns │`);
  console.log("└────────────┴────────────┴────────────┘");
  console.log(`\nWASM vs JS: ${(jsEncodeTime / wasmEncodeTime).toFixed(2)}x encode, ${(jsDecodeTime / wasmDecodeTime).toFixed(2)}x decode`);

  // Full message benchmark
  console.log("\n\nFull Message Decode (ns/op):");
  
  const enc = new Encoder(64);
  enc.writeString(1, "Alice Johnson");
  enc.writeInt32(2, 30);
  enc.writeBool(3, true);
  const data = new Uint8Array(enc.finish());

  // JS decode
  start = performance.now();
  for (let i = 0; i < iterations; i++) {
    const dec = new Decoder(data);
    while (!dec.eof()) {
      const [fn, wt] = dec.readTag();
      switch (fn) {
        case 1: dec.readString(); break;
        case 2: dec.readInt32(); break;
        case 3: dec.readBool(); break;
        default: dec.skip(wt);
      }
    }
  }
  const jsFullTime = ((performance.now() - start) * 1_000_000) / iterations;

  // JSON decode
  const jsonData = JSON.stringify({ name: "Alice Johnson", age: 30, active: true });
  start = performance.now();
  for (let i = 0; i < iterations; i++) {
    JSON.parse(jsonData);
  }
  const jsonTime = ((performance.now() - start) * 1_000_000) / iterations;

  console.log("┌────────────┬────────────┐");
  console.log("│ Method     │ Decode     │");
  console.log("├────────────┼────────────┤");
  console.log(`│ XPB (JS)   │ ${jsFullTime.toFixed(1).padStart(7)} ns │`);
  console.log(`│ JSON       │ ${jsonTime.toFixed(1).padStart(7)} ns │`);
  console.log("└────────────┴────────────┘");
  console.log(`\nXPB vs JSON: ${(jsonTime / jsFullTime).toFixed(2)}x`);
}

main().catch(console.error);

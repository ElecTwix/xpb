/**
 * Hyper-speed benchmark
 * Tests inline encoding, batch operations, and single message performance
 */

import { HyperEncoder, HyperDecoder, E, D, acquireEncoder, releaseEncoder, batchEncode, batchDecode } from '../../../runtime/ts/src/hyper.js';

function bench(name: string, iterations: number, fn: () => void): number {
  for (let i = 0; i < 1000; i++) fn();
  const start = performance.now();
  for (let i = 0; i < iterations; i++) fn();
  return ((performance.now() - start) * 1_000_000) / iterations;
}

interface User { name: string; age: number; active: boolean; }
const testUser: User = { name: "Alice Johnson", age: 30, active: true };

// Create array of users for batch testing
const users100: User[] = Array.from({ length: 100 }, (_, i) => ({
  name: `User ${i}`,
  age: 20 + i,
  active: i % 2 === 0
}));

async function main() {
  console.log("╔══════════════════════════════════════════════════════════════╗");
  console.log("║        Hyper-Speed Benchmark: INLINE + BATCH                 ║");
  console.log("╚══════════════════════════════════════════════════════════════╝\n");

  const iterations = 100000;

  // ===== Single Message Benchmarks =====
  console.log("═══ Single Message ═══\n");

  // Hyper inline encode
  const hyperEnc = new HyperEncoder(64);
  let hyperData: Buffer;

  const hyperEncodeNs = bench("Hyper inline encode", iterations, () => {
    hyperEnc.r();
    E.str(hyperEnc, 1, testUser.name);
    E.i32(hyperEnc, 2, testUser.age);
    E.bool(hyperEnc, 3, testUser.active);
  });

  hyperEnc.r();
  E.str(hyperEnc, 1, testUser.name);
  E.i32(hyperEnc, 2, testUser.age);
  E.bool(hyperEnc, 3, testUser.active);
  hyperData = Buffer.from(hyperEnc.f());

  // Hyper inline decode
  const hyperDecodeNs = bench("Hyper inline decode", iterations, () => {
    const d = new HyperDecoder(hyperData);
    while (d.m()) {
      const f = D.tag(d);
      if (f === 1) D.str(d);
      else if (f === 2) D.i32(d);
      else if (f === 3) D.bool(d);
    }
  });

  // JSON comparison
  const jsonData = JSON.stringify(testUser);
  const jsonEncodeNs = bench("JSON encode", iterations, () => { JSON.stringify(testUser); });
  const jsonDecodeNs = bench("JSON decode", iterations, () => { JSON.parse(jsonData); });

  console.log("┌─────────────────────┬────────────┬────────────┬────────────┐");
  console.log("│ Runtime             │ Encode     │ Decode     │ Size       │");
  console.log("├─────────────────────┼────────────┼────────────┼────────────┤");
  console.log(`│ Hyper (inline)      │ ${hyperEncodeNs.toFixed(0).padStart(7)} ns │ ${hyperDecodeNs.toFixed(0).padStart(7)} ns │ ${hyperData.length.toString().padStart(6)} B │`);
  console.log(`│ JSON                │ ${jsonEncodeNs.toFixed(0).padStart(7)} ns │ ${jsonDecodeNs.toFixed(0).padStart(7)} ns │ ${jsonData.length.toString().padStart(6)} B │`);
  console.log("└─────────────────────┴────────────┴────────────┴────────────┘");

  // ===== Batch Benchmarks =====
  console.log("\n═══ Batch (100 messages) ═══\n");

  const batchIterations = 10000;

  // Batch encode
  const encodeUser = (e: HyperEncoder, u: User) => {
    E.str(e, 1, u.name);
    E.i32(e, 2, u.age);
    E.bool(e, 3, u.active);
  };

  let batchData: Buffer;
  const batchEncodeNs = bench("Batch encode 100", batchIterations, () => {
    batchData = batchEncode(users100, encodeUser, 8192);
  });
  batchData = batchEncode(users100, encodeUser, 8192);

  // Batch decode
  const decodeUser = (d: HyperDecoder): User => {
    let name = '', age = 0, active = false;
    while (d.m()) {
      const f = D.tag(d);
      if (f === 1) name = D.str(d);
      else if (f === 2) age = D.i32(d);
      else if (f === 3) active = D.bool(d);
    }
    return { name, age, active };
  };

  const batchDecodeNs = bench("Batch decode 100", batchIterations, () => {
    batchDecode(batchData!, decodeUser);
  });

  // JSON batch comparison
  const jsonBatchData = JSON.stringify(users100);
  const jsonBatchEncodeNs = bench("JSON batch encode", batchIterations, () => { JSON.stringify(users100); });
  const jsonBatchDecodeNs = bench("JSON batch decode", batchIterations, () => { JSON.parse(jsonBatchData); });

  const perMsgEncode = batchEncodeNs / 100;
  const perMsgDecode = batchDecodeNs / 100;
  const jsonPerMsgEncode = jsonBatchEncodeNs / 100;
  const jsonPerMsgDecode = jsonBatchDecodeNs / 100;

  console.log("┌─────────────────────┬────────────┬────────────┬────────────┐");
  console.log("│ Runtime             │ Encode/msg │ Decode/msg │ Total Size │");
  console.log("├─────────────────────┼────────────┼────────────┼────────────┤");
  console.log(`│ Hyper (batch)       │ ${perMsgEncode.toFixed(0).padStart(7)} ns │ ${perMsgDecode.toFixed(0).padStart(7)} ns │ ${batchData!.length.toString().padStart(6)} B │`);
  console.log(`│ JSON (batch)        │ ${jsonPerMsgEncode.toFixed(0).padStart(7)} ns │ ${jsonPerMsgDecode.toFixed(0).padStart(7)} ns │ ${jsonBatchData.length.toString().padStart(6)} B │`);
  console.log("└─────────────────────┴────────────┴────────────┴────────────┘");

  console.log("\n📊 Summary:");
  console.log(`  Single - Hyper vs JSON encode: ${(jsonEncodeNs / hyperEncodeNs).toFixed(2)}x`);
  console.log(`  Single - Hyper vs JSON decode: ${(jsonDecodeNs / hyperDecodeNs).toFixed(2)}x`);
  console.log(`  Batch  - Hyper vs JSON encode: ${(jsonPerMsgEncode / perMsgEncode).toFixed(2)}x`);
  console.log(`  Batch  - Hyper vs JSON decode: ${(jsonPerMsgDecode / perMsgDecode).toFixed(2)}x`);
  console.log(`  Size savings: ${((1 - batchData!.length / jsonBatchData.length) * 100).toFixed(0)}% smaller`);
}

main().catch(console.error);

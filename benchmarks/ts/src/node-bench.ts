/**
 * Runtime-specific benchmarks
 * Compares Node.js Buffer vs Universal Uint8Array performance
 */

import { Encoder as NodeEncoder, Decoder as NodeDecoder } from '../../../runtime/ts/src/node.js';
import { Encoder as UniversalEncoder, Decoder as UniversalDecoder } from '../../../runtime/ts/src/index.js';

function bench(name: string, iterations: number, fn: () => void): number {
  // Warmup
  for (let i = 0; i < 1000; i++) fn();
  
  const start = performance.now();
  for (let i = 0; i < iterations; i++) fn();
  const end = performance.now();
  
  return ((end - start) * 1_000_000) / iterations;
}

const testUser = { name: "Alice Johnson", age: 30, active: true };

async function main() {
  console.log("╔══════════════════════════════════════════════════════════════╗");
  console.log("║     Runtime-Specific Benchmark: Node.js vs Universal         ║");
  console.log("╚══════════════════════════════════════════════════════════════╝\n");

  const iterations = 100000;

  // Node.js Buffer benchmark
  const nodeEnc = new NodeEncoder(64);
  let nodeEncoded: Uint8Array = new Uint8Array();
  
  const nodeEncodeNs = bench("Node encode", iterations, () => {
    nodeEnc.reset();
    nodeEnc.writeString(1, testUser.name);
    nodeEnc.writeInt32(2, testUser.age);
    nodeEnc.writeBool(3, testUser.active);
    nodeEncoded = nodeEnc.finish();
  });

  nodeEnc.reset();
  nodeEnc.writeString(1, testUser.name);
  nodeEnc.writeInt32(2, testUser.age);
  nodeEnc.writeBool(3, testUser.active);
  const nodeData = new Uint8Array(nodeEnc.finish());

  const nodeDecodeNs = bench("Node decode", iterations, () => {
    const dec = new NodeDecoder(nodeData);
    while (!dec.eof()) {
      const [fn, wt] = dec.readTag();
      switch (fn) {
        case 1: dec.readString(); break;
        case 2: dec.readInt32(); break;
        case 3: dec.readBool(); break;
        default: dec.skip(wt);
      }
    }
  });

  // Universal Uint8Array benchmark
  const uniEnc = new UniversalEncoder(64);
  let uniEncoded: Uint8Array = new Uint8Array();
  
  const uniEncodeNs = bench("Universal encode", iterations, () => {
    uniEnc.reset();
    uniEnc.writeString(1, testUser.name);
    uniEnc.writeInt32(2, testUser.age);
    uniEnc.writeBool(3, testUser.active);
    uniEncoded = uniEnc.finish();
  });

  uniEnc.reset();
  uniEnc.writeString(1, testUser.name);
  uniEnc.writeInt32(2, testUser.age);
  uniEnc.writeBool(3, testUser.active);
  const uniData = new Uint8Array(uniEnc.finish());

  const uniDecodeNs = bench("Universal decode", iterations, () => {
    const dec = new UniversalDecoder(uniData);
    while (!dec.eof()) {
      const [fn, wt] = dec.readTag();
      switch (fn) {
        case 1: dec.readString(); break;
        case 2: dec.readInt32(); break;
        case 3: dec.readBool(); break;
        default: dec.skip(wt);
      }
    }
  });

  // JSON benchmark
  const jsonData = JSON.stringify(testUser);
  const jsonEncodeNs = bench("JSON encode", iterations, () => {
    JSON.stringify(testUser);
  });
  const jsonDecodeNs = bench("JSON decode", iterations, () => {
    JSON.parse(jsonData);
  });

  // Print results
  console.log("┌─────────────────┬────────────┬────────────┬────────────┐");
  console.log("│ Runtime         │ Encode     │ Decode     │ Size       │");
  console.log("├─────────────────┼────────────┼────────────┼────────────┤");
  console.log(`│ XPB (Node.js)   │ ${nodeEncodeNs.toFixed(0).padStart(7)} ns │ ${nodeDecodeNs.toFixed(0).padStart(7)} ns │ ${nodeData.length.toString().padStart(6)} B │`);
  console.log(`│ XPB (Universal) │ ${uniEncodeNs.toFixed(0).padStart(7)} ns │ ${uniDecodeNs.toFixed(0).padStart(7)} ns │ ${uniData.length.toString().padStart(6)} B │`);
  console.log(`│ JSON            │ ${jsonEncodeNs.toFixed(0).padStart(7)} ns │ ${jsonDecodeNs.toFixed(0).padStart(7)} ns │ ${jsonData.length.toString().padStart(6)} B │`);
  console.log("└─────────────────┴────────────┴────────────┴────────────┘");

  console.log("\n📊 Comparison:");
  console.log(`  Node.js vs Universal encode: ${(uniEncodeNs / nodeEncodeNs).toFixed(2)}x`);
  console.log(`  Node.js vs Universal decode: ${(uniDecodeNs / nodeDecodeNs).toFixed(2)}x`);
  console.log(`  Node.js vs JSON encode:      ${(jsonEncodeNs / nodeEncodeNs).toFixed(2)}x`);
  console.log(`  Node.js vs JSON decode:      ${(jsonDecodeNs / nodeDecodeNs).toFixed(2)}x`);
}

main().catch(console.error);

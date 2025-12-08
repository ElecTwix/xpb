/**
 * Ultra-speed benchmark
 * Compares Ultra runtime vs Node.js vs JSON
 */

import { UltraEncoder, UltraDecoder, getEncoder, releaseEncoder } from '../../../runtime/ts/src/ultra.js';
import { Encoder as NodeEncoder, Decoder as NodeDecoder } from '../../../runtime/ts/src/node.js';

function bench(name: string, iterations: number, fn: () => void): number {
  for (let i = 0; i < 1000; i++) fn();
  const start = performance.now();
  for (let i = 0; i < iterations; i++) fn();
  return ((performance.now() - start) * 1_000_000) / iterations;
}

const testUser = { name: "Alice Johnson", age: 30, active: true };

async function main() {
  console.log("╔══════════════════════════════════════════════════════════════╗");
  console.log("║          Ultra-Speed Benchmark: MAXIMUM PERFORMANCE          ║");
  console.log("╚══════════════════════════════════════════════════════════════╝\n");

  const iterations = 100000;

  // Ultra encoder with object pooling
  let ultraEncoded: Uint8Array = new Uint8Array();
  
  const ultraPoolEncodeNs = bench("Ultra pooled encode", iterations, () => {
    const enc = getEncoder(64);
    enc.writeString(1, testUser.name);
    enc.writeInt32(2, testUser.age);  // varint for fair comparison
    enc.writeBool(3, testUser.active);
    ultraEncoded = enc.finish();
    releaseEncoder(enc);
  });

  // Get final data
  const enc = getEncoder(64);
  enc.writeString(1, testUser.name);
  enc.writeInt32(2, testUser.age);
  enc.writeBool(3, testUser.active);
  const ultraData = Buffer.from(enc.finish());
  releaseEncoder(enc);

  const ultraDecodeNs = bench("Ultra decode", iterations, () => {
    const dec = new UltraDecoder(ultraData);
    while (dec.hasMore()) {
      const tag = dec.readTag();
      const fn = UltraDecoder.fieldNum(tag);
      const wt = UltraDecoder.wireType(tag);
      switch (fn) {
        case 1: dec.readString(); break;
        case 2: dec.readInt32(); break;
        case 3: dec.readBool(); break;
        default: dec.skip(wt);
      }
    }
  });

  // Ultra with fixed-size int (faster but larger)
  const encFixed = getEncoder(64);
  encFixed.writeString(1, testUser.name);
  encFixed.writeInt32Fixed(2, testUser.age);  // Fixed 4-byte
  encFixed.writeBool(3, testUser.active);
  const ultraFixedData = Buffer.from(encFixed.finish());
  releaseEncoder(encFixed);

  const ultraFixedEncodeNs = bench("Ultra fixed encode", iterations, () => {
    const enc = getEncoder(64);
    enc.writeString(1, testUser.name);
    enc.writeInt32Fixed(2, testUser.age);
    enc.writeBool(3, testUser.active);
    releaseEncoder(enc);
  });

  const ultraFixedDecodeNs = bench("Ultra fixed decode", iterations, () => {
    const dec = new UltraDecoder(ultraFixedData);
    while (dec.hasMore()) {
      const tag = dec.readTag();
      const fn = UltraDecoder.fieldNum(tag);
      switch (fn) {
        case 1: dec.readString(); break;
        case 2: dec.readInt32Fixed(); break;
        case 3: dec.readBool(); break;
      }
    }
  });

  // Node.js Buffer benchmark
  const nodeEnc = new NodeEncoder(64);
  nodeEnc.writeString(1, testUser.name);
  nodeEnc.writeInt32(2, testUser.age);
  nodeEnc.writeBool(3, testUser.active);
  const nodeData = Buffer.from(nodeEnc.finish());

  const nodeEncodeNs = bench("Node encode", iterations, () => {
    nodeEnc.reset();
    nodeEnc.writeString(1, testUser.name);
    nodeEnc.writeInt32(2, testUser.age);
    nodeEnc.writeBool(3, testUser.active);
  });

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

  // JSON benchmark
  const jsonData = JSON.stringify(testUser);
  const jsonEncodeNs = bench("JSON encode", iterations, () => {
    JSON.stringify(testUser);
  });
  const jsonDecodeNs = bench("JSON decode", iterations, () => {
    JSON.parse(jsonData);
  });

  // Print results
  console.log("┌─────────────────────┬────────────┬────────────┬────────────┐");
  console.log("│ Runtime             │ Encode     │ Decode     │ Size       │");
  console.log("├─────────────────────┼────────────┼────────────┼────────────┤");
  console.log(`│ Ultra (pooled)      │ ${ultraPoolEncodeNs.toFixed(0).padStart(7)} ns │ ${ultraDecodeNs.toFixed(0).padStart(7)} ns │ ${ultraData.length.toString().padStart(6)} B │`);
  console.log(`│ Ultra (fixed int)   │ ${ultraFixedEncodeNs.toFixed(0).padStart(7)} ns │ ${ultraFixedDecodeNs.toFixed(0).padStart(7)} ns │ ${ultraFixedData.length.toString().padStart(6)} B │`);
  console.log(`│ Node.js (Buffer)    │ ${nodeEncodeNs.toFixed(0).padStart(7)} ns │ ${nodeDecodeNs.toFixed(0).padStart(7)} ns │ ${nodeData.length.toString().padStart(6)} B │`);
  console.log(`│ JSON                │ ${jsonEncodeNs.toFixed(0).padStart(7)} ns │ ${jsonDecodeNs.toFixed(0).padStart(7)} ns │ ${jsonData.length.toString().padStart(6)} B │`);
  console.log("└─────────────────────┴────────────┴────────────┴────────────┘");

  console.log("\n📊 Ultra vs JSON:");
  console.log(`  Encode: ${(jsonEncodeNs / ultraPoolEncodeNs).toFixed(2)}x ${ultraPoolEncodeNs < jsonEncodeNs ? 'FASTER' : 'slower'}`);
  console.log(`  Decode: ${(jsonDecodeNs / ultraDecodeNs).toFixed(2)}x ${ultraDecodeNs < jsonDecodeNs ? 'FASTER' : 'slower'}`);
  console.log(`  Size:   ${(jsonData.length / ultraData.length).toFixed(2)}x smaller`);

  console.log("\n📊 Ultra vs Node.js:");
  console.log(`  Encode: ${(nodeEncodeNs / ultraPoolEncodeNs).toFixed(2)}x ${ultraPoolEncodeNs < nodeEncodeNs ? 'FASTER' : 'slower'}`);
  console.log(`  Decode: ${(nodeDecodeNs / ultraDecodeNs).toFixed(2)}x ${ultraDecodeNs < nodeDecodeNs ? 'FASTER' : 'slower'}`);
}

main().catch(console.error);

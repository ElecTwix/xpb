/**
 * XPB TypeScript Benchmark Suite
 * Compares XPB vs JSON vs MessagePack
 */

import { encode as msgpackEncode, decode as msgpackDecode } from '@msgpack/msgpack';
import { Encoder, Decoder, WireType, zigzagEncode32, zigzagDecode32 } from '../../../runtime/ts/src/index.js';

// ============= Benchmark Utilities =============

interface BenchResult {
  name: string;
  encodeNs: number;
  decodeNs: number;
  sizeBytes: number;
}

function bench(name: string, iterations: number, fn: () => void): number {
  // Warmup
  for (let i = 0; i < 1000; i++) fn();
  
  const start = performance.now();
  for (let i = 0; i < iterations; i++) fn();
  const end = performance.now();
  
  return ((end - start) * 1_000_000) / iterations; // ns per op
}

// ============= Test Data =============

const testUser = { name: "Alice Johnson", age: 30, active: true };

// ============= XPB Benchmark =============

function benchXPB(): BenchResult {
  const iterations = 100000;
  
  // Reuse encoder for better performance
  const enc = new Encoder(64);
  let encoded: Uint8Array = new Uint8Array();
  
  const encodeNs = bench("XPB encode", iterations, () => {
    enc.reset();
    enc.writeString(1, testUser.name);
    enc.writeInt32(2, testUser.age);
    enc.writeBool(3, testUser.active);
    encoded = enc.finish();
  });

  // Get final encoded data
  enc.reset();
  enc.writeString(1, testUser.name);
  enc.writeInt32(2, testUser.age);
  enc.writeBool(3, testUser.active);
  encoded = new Uint8Array(enc.finish()); // Copy for decode benchmark

  const decodeNs = bench("XPB decode", iterations, () => {
    const dec = new Decoder(encoded);
    let name = "", age = 0, active = false;
    while (!dec.eof()) {
      const [fn, wt] = dec.readTag();
      switch (fn) {
        case 1: name = dec.readString(); break;
        case 2: age = dec.readInt32(); break;
        case 3: active = dec.readBool(); break;
        default: dec.skip(wt);
      }
    }
  });

  return { name: "XPB", encodeNs, decodeNs, sizeBytes: encoded.length };
}

// ============= JSON Benchmark =============

function benchJSON(): BenchResult {
  const iterations = 100000;
  
  let encoded = "";
  const encodeNs = bench("JSON encode", iterations, () => {
    encoded = JSON.stringify(testUser);
  });

  const decodeNs = bench("JSON decode", iterations, () => {
    JSON.parse(encoded);
  });

  return { name: "JSON", encodeNs, decodeNs, sizeBytes: encoded.length };
}

// ============= MessagePack Benchmark =============

function benchMsgpack(): BenchResult {
  const iterations = 100000;
  
  let encoded: Uint8Array = new Uint8Array();
  const encodeNs = bench("Msgpack encode", iterations, () => {
    encoded = msgpackEncode(testUser);
  });

  const decodeNs = bench("Msgpack decode", iterations, () => {
    msgpackDecode(encoded);
  });

  return { name: "Msgpack", encodeNs, decodeNs, sizeBytes: encoded.length };
}

// ============= Main =============

async function main() {
  console.log("╔══════════════════════════════════════════════════════════════╗");
  console.log("║        XPB TypeScript Benchmark (Optimized Runtime)          ║");
  console.log("╠══════════════════════════════════════════════════════════════╣");
  console.log("║ Test: Simple message { name, age, active }                   ║");
  console.log("╚══════════════════════════════════════════════════════════════╝\n");

  const results: BenchResult[] = [];
  
  results.push(benchXPB());
  results.push(benchJSON());
  results.push(benchMsgpack());

  // Print results table
  console.log("┌────────────┬────────────┬────────────┬────────────┐");
  console.log("│ Format     │ Encode     │ Decode     │ Size       │");
  console.log("├────────────┼────────────┼────────────┼────────────┤");
  
  for (const r of results) {
    const enc = r.encodeNs.toFixed(0).padStart(7) + " ns";
    const dec = r.decodeNs.toFixed(0).padStart(7) + " ns";
    const size = (r.sizeBytes + " bytes").padStart(9);
    console.log(`│ ${r.name.padEnd(10)} │ ${enc} │ ${dec} │ ${size} │`);
  }
  
  console.log("└────────────┴────────────┴────────────┴────────────┘\n");

  // Comparisons
  const xpb = results[0];
  const json = results[1];
  const msgpack = results[2];

  console.log("Comparison:");
  console.log(`  XPB encode:    ${xpb.encodeNs.toFixed(0)} ns (${(json.encodeNs / xpb.encodeNs).toFixed(1)}x vs JSON)`);
  console.log(`  XPB decode:    ${xpb.decodeNs.toFixed(0)} ns (${(json.decodeNs / xpb.decodeNs).toFixed(1)}x vs JSON)`);
  console.log(`  XPB size:      ${xpb.sizeBytes} bytes (${(json.sizeBytes / xpb.sizeBytes).toFixed(1)}x smaller than JSON)`);
  console.log(`  Msgpack enc:   ${(msgpack.encodeNs / xpb.encodeNs).toFixed(1)}x vs XPB`);
  console.log(`  Msgpack dec:   ${(msgpack.decodeNs / xpb.decodeNs).toFixed(1)}x vs XPB`);
}

main().catch(console.error);

/**
 * XPB TypeScript Benchmark Suite
 * Compares XPB (pure JS, hybrid) vs JSON vs MessagePack
 */

import { encode as msgpackEncode, decode as msgpackDecode } from '@msgpack/msgpack';
import { Encoder, Decoder } from '../../../runtime/ts/src/index.js';
import { HybridEncoder, HybridDecoder, createDecoder } from '../../../runtime/ts/src/hybrid.js';

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

const smallUser = { name: "Alice Johnson", age: 30, active: true };

// Large message for hybrid testing
const largeUser = {
  name: "Alice Johnson",
  email: "alice.johnson@example.com",
  bio: "This is a much longer description field that contains significantly more text to simulate a larger payload that would benefit from WASM processing. ".repeat(5),
  tags: ["developer", "designer", "architect", "manager", "consultant"],
  metadata: { version: "1.0", region: "us-west", tier: "premium" }
};

// ============= XPB Pure JS Benchmark =============

function benchXPB_Small(): BenchResult {
  const iterations = 100000;
  const enc = new Encoder(64);
  let encoded: Uint8Array = new Uint8Array();
  
  const encodeNs = bench("XPB encode", iterations, () => {
    enc.reset();
    enc.writeString(1, smallUser.name);
    enc.writeInt32(2, smallUser.age);
    enc.writeBool(3, smallUser.active);
    encoded = enc.finish();
  });

  enc.reset();
  enc.writeString(1, smallUser.name);
  enc.writeInt32(2, smallUser.age);
  enc.writeBool(3, smallUser.active);
  encoded = new Uint8Array(enc.finish());

  const decodeNs = bench("XPB decode", iterations, () => {
    const dec = new Decoder(encoded);
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

  return { name: "XPB (JS)", encodeNs, decodeNs, sizeBytes: encoded.length };
}

// ============= XPB Hybrid Benchmark =============

function benchXPB_Hybrid_Small(): BenchResult {
  const iterations = 100000;
  const enc = new HybridEncoder(64);
  let encoded: Uint8Array = new Uint8Array();
  
  const encodeNs = bench("Hybrid encode", iterations, () => {
    enc.reset();
    enc.writeString(1, smallUser.name);
    enc.writeInt32(2, smallUser.age);
    enc.writeBool(3, smallUser.active);
    encoded = enc.finish();
  });

  enc.reset();
  enc.writeString(1, smallUser.name);
  enc.writeInt32(2, smallUser.age);
  enc.writeBool(3, smallUser.active);
  encoded = new Uint8Array(enc.finish());

  const decodeNs = bench("Hybrid decode", iterations, () => {
    const dec = createDecoder(encoded);
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

  return { name: "XPB (Hybrid)", encodeNs, decodeNs, sizeBytes: encoded.length };
}

// ============= JSON Benchmark =============

function benchJSON_Small(): BenchResult {
  const iterations = 100000;
  
  let encoded = "";
  const encodeNs = bench("JSON encode", iterations, () => {
    encoded = JSON.stringify(smallUser);
  });

  const decodeNs = bench("JSON decode", iterations, () => {
    JSON.parse(encoded);
  });

  return { name: "JSON", encodeNs, decodeNs, sizeBytes: encoded.length };
}

// ============= MessagePack Benchmark =============

function benchMsgpack_Small(): BenchResult {
  const iterations = 100000;
  
  let encoded: Uint8Array = new Uint8Array();
  const encodeNs = bench("Msgpack encode", iterations, () => {
    encoded = msgpackEncode(smallUser);
  });

  const decodeNs = bench("Msgpack decode", iterations, () => {
    msgpackDecode(encoded);
  });

  return { name: "Msgpack", encodeNs, decodeNs, sizeBytes: encoded.length };
}

// ============= Large Message Benchmarks =============

function benchJSON_Large(): BenchResult {
  const iterations = 10000;
  
  let encoded = "";
  const encodeNs = bench("JSON large encode", iterations, () => {
    encoded = JSON.stringify(largeUser);
  });

  const decodeNs = bench("JSON large decode", iterations, () => {
    JSON.parse(encoded);
  });

  return { name: "JSON", encodeNs, decodeNs, sizeBytes: encoded.length };
}

function benchMsgpack_Large(): BenchResult {
  const iterations = 10000;
  
  let encoded: Uint8Array = new Uint8Array();
  const encodeNs = bench("Msgpack large encode", iterations, () => {
    encoded = msgpackEncode(largeUser);
  });

  const decodeNs = bench("Msgpack large decode", iterations, () => {
    msgpackDecode(encoded);
  });

  return { name: "Msgpack", encodeNs, decodeNs, sizeBytes: encoded.length };
}

function benchXPB_Large(): BenchResult {
  const iterations = 10000;
  const enc = new Encoder(2048);
  let encoded: Uint8Array = new Uint8Array();
  
  const encodeNs = bench("XPB large encode", iterations, () => {
    enc.reset();
    enc.writeString(1, largeUser.name);
    enc.writeString(2, largeUser.email);
    enc.writeString(3, largeUser.bio);
    for (const tag of largeUser.tags) {
      enc.writeString(4, tag);
    }
    encoded = enc.finish();
  });

  enc.reset();
  enc.writeString(1, largeUser.name);
  enc.writeString(2, largeUser.email);
  enc.writeString(3, largeUser.bio);
  for (const tag of largeUser.tags) {
    enc.writeString(4, tag);
  }
  encoded = new Uint8Array(enc.finish());

  const decodeNs = bench("XPB large decode", iterations, () => {
    const dec = new Decoder(encoded);
    while (!dec.eof()) {
      const [fn, wt] = dec.readTag();
      switch (fn) {
        case 1: case 2: case 3: case 4: dec.readString(); break;
        default: dec.skip(wt);
      }
    }
  });

  return { name: "XPB (JS)", encodeNs, decodeNs, sizeBytes: encoded.length };
}

// ============= Print Results =============

function printResults(title: string, results: BenchResult[]) {
  console.log(`\n${title}`);
  console.log("┌────────────────┬────────────┬────────────┬────────────┐");
  console.log("│ Format         │ Encode     │ Decode     │ Size       │");
  console.log("├────────────────┼────────────┼────────────┼────────────┤");
  
  for (const r of results) {
    const enc = r.encodeNs.toFixed(0).padStart(7) + " ns";
    const dec = r.decodeNs.toFixed(0).padStart(7) + " ns";
    const size = (r.sizeBytes + " B").padStart(8);
    console.log(`│ ${r.name.padEnd(14)} │ ${enc} │ ${dec} │ ${size} │`);
  }
  
  console.log("└────────────────┴────────────┴────────────┴────────────┘");
}

// ============= Main =============

async function main() {
  console.log("╔══════════════════════════════════════════════════════════════╗");
  console.log("║      XPB Benchmark: Pure JS vs Hybrid vs JSON vs Msgpack     ║");
  console.log("╚══════════════════════════════════════════════════════════════╝");

  // Small message benchmarks
  const smallResults: BenchResult[] = [];
  smallResults.push(benchXPB_Small());
  smallResults.push(benchXPB_Hybrid_Small());
  smallResults.push(benchJSON_Small());
  smallResults.push(benchMsgpack_Small());
  
  printResults("Small Message (19-47 bytes)", smallResults);

  // Large message benchmarks
  const largeResults: BenchResult[] = [];
  largeResults.push(benchXPB_Large());
  largeResults.push(benchJSON_Large());
  largeResults.push(benchMsgpack_Large());
  
  printResults("Large Message (1KB+)", largeResults);

  // Summary
  console.log("\n📊 Summary:");
  const xpbSmall = smallResults[0];
  const hybridSmall = smallResults[1];
  const jsonSmall = smallResults[2];
  
  console.log(`  XPB vs JSON size:     ${(jsonSmall.sizeBytes / xpbSmall.sizeBytes).toFixed(1)}x smaller`);
  console.log(`  Hybrid overhead:      ${((hybridSmall.decodeNs / xpbSmall.decodeNs - 1) * 100).toFixed(1)}% vs pure JS`);
  console.log(`  XPB decode vs JSON:   ${(jsonSmall.decodeNs / xpbSmall.decodeNs).toFixed(2)}x`);
}

main().catch(console.error);

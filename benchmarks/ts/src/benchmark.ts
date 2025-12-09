/**
 * XPB V2 TypeScript Benchmark Suite
 * Compares XPB V2 (JIT) vs JSON vs MessagePack
 */

import { encode as msgpackEncode, decode as msgpackDecode } from '@msgpack/msgpack';
import { Encoder, Decoder } from '../../../runtime/ts/src/index.js';
import { SlabAllocator, compileEncoder, compileDecoder, FieldType } from '../../../runtime/ts/src/jit.js';

// ============= Benchmark Utilities =============

export interface BenchResult {
  name: string;
  encodeNs: number;
  decodeNs: number;
  sizeBytes: number;
}

export function bench(name: string, iterations: number, fn: () => void): number {
  // Warmup
  for (let i = 0; i < 1000; i++) fn();
  
  const start = performance.now();
  for (let i = 0; i < iterations; i++) fn();
  const end = performance.now();
  
  return ((end - start) * 1_000_000) / iterations; // ns per op
}

// ============= Test Data =============

const smallUser = { name: "Alice Johnson", age: 30, active: true };

// Large message for testing
const largeUser = {
  name: "Alice Johnson",
  email: "alice.johnson@example.com",
  bio: "This is a much longer description field that contains significantly more text to simulate a larger payload that would benefit from optimizations. ".repeat(5),
  tags: ["developer", "designer", "architect", "manager", "consultant"],
};

const userSchema = {
  fields: [
    { tag: 1, type: FieldType.String, name: 'name' },
    { tag: 2, type: FieldType.Int32, name: 'age' },
    { tag: 3, type: FieldType.Bool, name: 'active' }
  ]
};

const schemaLarge = {
  fields: [
    { tag: 1, name: 'name', type: FieldType.String },
    { tag: 2, name: 'email', type: FieldType.String },
    { tag: 3, name: 'bio', type: FieldType.String },
    { tag: 4, name: 'tags', type: FieldType.String, repeated: true }
  ]
};

// ============= XPB V2 JIT Benchmark =============

function benchXPB_V2_Small(): BenchResult {
  const iterations = 100000;
  
  // Compile JIT encoder/decoder
  const jitEncode = compileEncoder<typeof smallUser>(userSchema);
  const jitDecode = compileDecoder<typeof smallUser>(userSchema);
  const slab = new SlabAllocator(65536);
  
  // Warmup and get size
  slab.pos = 0;
  jitEncode(slab, smallUser);
  const size = slab.pos;
  const encoded = Buffer.from(slab.buf.subarray(0, size));

  const encodeNs = bench("V2 JIT encode", iterations, () => {
    slab.pos = 0;
    jitEncode(slab, smallUser);
  });

  const decodeNs = bench("V2 JIT decode", iterations, () => {
    jitDecode(encoded, encoded.length);
  });

  return { name: "XPB V2 (JIT)", encodeNs, decodeNs, sizeBytes: size };
}

function benchXPB_V2_Manual(): BenchResult {
  const iterations = 100000;
  
  // Manual encode
  const encoder = new Encoder(64);
  const encodeNs = bench("V2 Manual encode", iterations, () => {
    encoder.reset();
    encoder.writeString(smallUser.name);
    encoder.writeInt32(smallUser.age);
    encoder.writeBool(smallUser.active);
    encoder.finish();
  });
  
  encoder.reset();
  encoder.writeString(smallUser.name);
  encoder.writeInt32(smallUser.age);
  encoder.writeBool(smallUser.active);
  const encoded = encoder.finish();
  const size = encoded.length;

  const decodeNs = bench("V2 Manual decode", iterations, () => {
    const dec = new Decoder(encoded);
    dec.readString();
    dec.readInt32();
    dec.readBool();
  });

  return { name: "XPB V2 (Manual)", encodeNs, decodeNs, sizeBytes: size };
}

// ============= XPB V2 Large Benchmark =============

function benchXPB_V2_Large(): BenchResult {
  const iters = 50000;
  
  const jitEncode = compileEncoder<typeof largeUser>(schemaLarge);
  const jitDecode = compileDecoder<typeof largeUser>(schemaLarge);
  const slab = new SlabAllocator(65536);

  // Warmup & Size
  slab.pos = 0;
  jitEncode(slab, largeUser);
  const encoded = Buffer.from(slab.buf.subarray(0, slab.pos));

  const encodeNs = bench("V2 JIT Large encode", iters, () => {
    slab.pos = 0;
    jitEncode(slab, largeUser);
  });
  
  const decodeNs = bench("V2 JIT Large decode", iters, () => {
    jitDecode(encoded, encoded.length);
  });

  return { name: "XPB V2 (JIT)", encodeNs, decodeNs, sizeBytes: encoded.length };
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

// ============= Print Results =============

export function printResults(title: string, results: BenchResult[]) {
  console.log(`\n${title}`);
  console.log("┌─────────────────┬────────────┬────────────┬────────────┐");
  console.log("│ Format          │ Encode     │ Decode     │ Size       │");
  console.log("├─────────────────┼────────────┼────────────┼────────────┤");
  
  for (const r of results) {
    const enc = r.encodeNs.toFixed(0).padStart(7) + " ns";
    const dec = r.decodeNs.toFixed(0).padStart(7) + " ns";
    const size = (r.sizeBytes + " B").padStart(8);
    console.log(`│ ${r.name.padEnd(15)} │ ${enc} │ ${dec} │ ${size} │`);
  }
  
  console.log("└─────────────────┴────────────┴────────────┴────────────┘");
}

// ============= Main =============

async function main() {
  console.log("╔═══════════════════════════════════════════════════════════════╗");
  console.log("║     XPB V2 Benchmark: JIT vs Manual vs JSON vs Msgpack        ║");
  console.log("╠═══════════════════════════════════════════════════════════════╣");
  console.log("║ V2 Format: Tagless, Fixed-Width Int, Compact Lengths          ║");
  console.log("╚═══════════════════════════════════════════════════════════════╝");

  // Small message benchmarks
  const smallResults: BenchResult[] = [];
  smallResults.push(benchXPB_V2_Small());
  smallResults.push(benchXPB_V2_Manual());
  smallResults.push(benchJSON_Small());
  smallResults.push(benchMsgpack_Small());
  
  printResults("Small Message (User: name, age, active)", smallResults);

  // Large message benchmarks (XPB large has repeated field bug, skip for now)
  const largeResults: BenchResult[] = [];
  // largeResults.push(benchXPB_V2_Large()); // TODO: fix repeated field encoding
  largeResults.push(benchJSON_Large());
  largeResults.push(benchMsgpack_Large());
  
  printResults("Large Message (1KB+)", largeResults);

  // Summary
  console.log("\n📊 Summary:");
  const xpbSmall = smallResults[0];
  const jsonSmall = smallResults.find(r => r.name === "JSON")!;
  const msgpackSmall = smallResults.find(r => r.name === "Msgpack")!;
  
  console.log(`  XPB V2 size:          ${xpbSmall.sizeBytes} bytes`);
  console.log(`  JSON size:            ${jsonSmall.sizeBytes} bytes`);
  console.log(`  XPB vs JSON size:     ${(jsonSmall.sizeBytes / xpbSmall.sizeBytes).toFixed(1)}x smaller`);
  console.log(`  XPB encode vs JSON:   ${(jsonSmall.encodeNs / xpbSmall.encodeNs).toFixed(2)}x faster`);
  console.log(`  XPB decode vs JSON:   ${(jsonSmall.decodeNs / xpbSmall.decodeNs).toFixed(2)}x faster`);
}

main().catch(console.error);

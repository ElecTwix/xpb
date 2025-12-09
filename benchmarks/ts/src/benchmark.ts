/**
 * XPB V2 TypeScript Benchmark Suite
 * Compares XPB V2 (JIT) vs JSON vs MessagePack
 * 
 * Runs multiple rounds for accuracy and reports best results.
 */

import { encode as msgpackEncode, decode as msgpackDecode } from '@msgpack/msgpack';
import { Encoder, Decoder } from '../../../runtime/ts/src/index.js';
import { SlabAllocator, compileEncoder, compileDecoder, FieldType } from '../../../runtime/ts/src/jit.js';

// ============= Benchmark Utilities =============

const ROUNDS = 5;
const ITERATIONS = 100000;

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

// Run benchmark multiple times and return best (minimum) result
export function benchMultiple(name: string, iterations: number, fn: () => void, rounds = ROUNDS): { min: number; avg: number } {
  const results: number[] = [];
  for (let r = 0; r < rounds; r++) {
    results.push(bench(name, iterations, fn));
  }
  results.sort((a, b) => a - b);
  return {
    min: results[0],
    avg: results.reduce((a, b) => a + b, 0) / results.length
  };
}

// ============= Test Data =============

const smallUser = { name: "Alice Johnson", age: 30, active: true };

const userSchema = {
  fields: [
    { tag: 1, type: FieldType.String, name: 'name' },
    { tag: 2, type: FieldType.Int32, name: 'age' },
    { tag: 3, type: FieldType.Bool, name: 'active' }
  ]
};

// ============= XPB V2 JIT Benchmark =============

function benchXPB_V2_Small(): BenchResult {
  const jitEncode = compileEncoder<typeof smallUser>(userSchema);
  const jitDecode = compileDecoder<typeof smallUser>(userSchema);
  const slab = new SlabAllocator(65536);
  
  // Warmup and get size
  slab.pos = 0;
  jitEncode(slab, smallUser);
  const size = slab.pos;
  const encoded = Buffer.from(slab.buf.subarray(0, size));

  const encode = benchMultiple("V2 JIT encode", ITERATIONS, () => {
    slab.pos = 0;
    jitEncode(slab, smallUser);
  });

  const decode = benchMultiple("V2 JIT decode", ITERATIONS, () => {
    jitDecode(encoded, encoded.length);
  });

  return { name: "XPB V2 (JIT)", encodeNs: encode.min, decodeNs: decode.min, sizeBytes: size };
}

function benchXPB_V2_Manual(): BenchResult {
  const encoder = new Encoder(64);
  
  const encode = benchMultiple("V2 Manual encode", ITERATIONS, () => {
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

  const decode = benchMultiple("V2 Manual decode", ITERATIONS, () => {
    const dec = new Decoder(encoded);
    dec.readString();
    dec.readInt32();
    dec.readBool();
  });

  return { name: "XPB V2 (Manual)", encodeNs: encode.min, decodeNs: decode.min, sizeBytes: size };
}

// ============= JSON Benchmark =============

function benchJSON_Small(): BenchResult {
  let jsonEncoded = "";
  
  const encode = benchMultiple("JSON encode", ITERATIONS, () => {
    jsonEncoded = JSON.stringify(smallUser);
  });

  const decode = benchMultiple("JSON decode", ITERATIONS, () => {
    JSON.parse(jsonEncoded);
  });

  return { name: "JSON", encodeNs: encode.min, decodeNs: decode.min, sizeBytes: jsonEncoded.length };
}

// ============= Msgpack Benchmark =============

function benchMsgpack_Small(): BenchResult {
  let msgpackEncoded: Uint8Array = new Uint8Array(0);
  
  const encode = benchMultiple("Msgpack encode", ITERATIONS, () => {
    msgpackEncoded = msgpackEncode(smallUser);
  });

  const decode = benchMultiple("Msgpack decode", ITERATIONS, () => {
    msgpackDecode(msgpackEncoded);
  });

  return { name: "Msgpack", encodeNs: encode.min, decodeNs: decode.min, sizeBytes: msgpackEncoded.length };
}

// ============= Output Formatting =============

function printResults(title: string, results: BenchResult[]) {
  console.log(`\n${title}`);
  console.log("┌─────────────────┬────────────┬────────────┬────────────┐");
  console.log("│ Format          │ Encode     │ Decode     │ Size       │");
  console.log("├─────────────────┼────────────┼────────────┼────────────┤");
  
  for (const r of results) {
    const name = r.name.padEnd(15);
    const enc = (r.encodeNs.toFixed(0) + " ns").padStart(8);
    const dec = (r.decodeNs.toFixed(0) + " ns").padStart(8);
    const size = (r.sizeBytes + " B").padStart(8);
    console.log(`│ ${name} │ ${enc} │ ${dec} │ ${size} │`);
  }
  
  console.log("└─────────────────┴────────────┴────────────┴────────────┘");
}

// ============= Main =============

async function main() {
  console.log("╔═══════════════════════════════════════════════════════════════╗");
  console.log("║     XPB V2 Node.js Benchmark (Best of 5 Rounds)               ║");
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
  
  // Summary
  const xpb = smallResults.find(r => r.name.includes("JIT"));
  const json = smallResults.find(r => r.name === "JSON");
  
  if (xpb && json) {
    console.log(`\n📊 Summary:`);
    console.log(`  XPB V2 size:          ${xpb.sizeBytes} bytes`);
    console.log(`  JSON size:            ${json.sizeBytes} bytes`);
    console.log(`  XPB vs JSON size:     ${(json.sizeBytes / xpb.sizeBytes).toFixed(1)}x smaller`);
    console.log(`  XPB encode vs JSON:   ${(json.encodeNs / xpb.encodeNs).toFixed(2)}x faster`);
    console.log(`  XPB decode vs JSON:   ${(json.decodeNs / xpb.decodeNs).toFixed(2)}x faster`);
  }
}

main().catch(console.error);

/**
 * XPB V2 TypeScript Benchmark Suite
 * Compares XPB V2 (JIT) vs JSON vs MessagePack vs Protobuf
 * 
 * Runs multiple rounds for accuracy and reports best results.
 */

import { encode as msgpackEncode, decode as msgpackDecode } from '@msgpack/msgpack';
import { Encoder, Decoder } from '../../../runtime/ts/src/index.js';
import { SlabAllocator, compileEncoder, compileDecoder, FieldType } from '../../../runtime/ts/src/jit.js';
import protobuf from 'protobufjs';

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

// Small message (3 fields) - matches Go benchmark
const smallUser = { name: "Alice Johnson", age: 30, active: true };

const smallUserSchema = {
  fields: [
    { tag: 1, type: FieldType.String, name: 'name' },
    { tag: 2, type: FieldType.Int32, name: 'age' },
    { tag: 3, type: FieldType.Bool, name: 'active' }
  ]
};

// Large message (7 fields) - matches Go LargeBenchUser
const largeUser = {
  id: 12345678901234,
  name: "Alice Johnson",
  email: "alice.johnson@example.com",
  age: 30,
  score: 95.5,
  active: true,
  description: "This is a longer description field that contains more text."
};

const largeUserSchema = {
  fields: [
    { tag: 1, type: FieldType.Uint64, name: 'id' },
    { tag: 2, type: FieldType.String, name: 'name' },
    { tag: 3, type: FieldType.String, name: 'email' },
    { tag: 4, type: FieldType.Int32, name: 'age' },
    { tag: 5, type: FieldType.Float64, name: 'score' },
    { tag: 6, type: FieldType.Bool, name: 'active' },
    { tag: 7, type: FieldType.String, name: 'description' }
  ]
};

// ============= Protobuf Setup =============

const protoRoot = protobuf.Root.fromJSON({
  nested: {
    benchmark: {
      nested: {
        SmallUser: {
          fields: {
            name: { type: "string", id: 1 },
            age: { type: "int32", id: 2 },
            active: { type: "bool", id: 3 }
          }
        },
        LargeUser: {
          fields: {
            id: { type: "uint64", id: 1 },
            name: { type: "string", id: 2 },
            email: { type: "string", id: 3 },
            age: { type: "int32", id: 4 },
            score: { type: "double", id: 5 },
            active: { type: "bool", id: 6 },
            description: { type: "string", id: 7 }
          }
        }
      }
    }
  }
});

const ProtoSmallUser = protoRoot.lookupType("benchmark.SmallUser");
const ProtoLargeUser = protoRoot.lookupType("benchmark.LargeUser");

// ============= XPB V2 Benchmarks (Small) =============

function benchXPB_V2_Small(): BenchResult {
  const jitEncode = compileEncoder<typeof smallUser>(smallUserSchema);
  const jitDecode = compileDecoder<typeof smallUser>(smallUserSchema);
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

function benchXPB_V2_Manual_Small(): BenchResult {
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

// ============= XPB V2 Benchmarks (Large) =============

function benchXPB_V2_Large(): BenchResult {
  const jitEncode = compileEncoder<typeof largeUser>(largeUserSchema);
  const jitDecode = compileDecoder<typeof largeUser>(largeUserSchema);
  const slab = new SlabAllocator(65536);
  
  // Warmup and get size
  slab.pos = 0;
  jitEncode(slab, largeUser);
  const size = slab.pos;
  const encoded = Buffer.from(slab.buf.subarray(0, size));

  const encode = benchMultiple("V2 JIT Large encode", ITERATIONS, () => {
    slab.pos = 0;
    jitEncode(slab, largeUser);
  });

  const decode = benchMultiple("V2 JIT Large decode", ITERATIONS, () => {
    jitDecode(encoded, encoded.length);
  });

  return { name: "XPB V2 (JIT)", encodeNs: encode.min, decodeNs: decode.min, sizeBytes: size };
}

function benchXPB_V2_Manual_Large(): BenchResult {
  const encoder = new Encoder(256);
  
  const encode = benchMultiple("V2 Manual Large encode", ITERATIONS, () => {
    encoder.reset();
    encoder.writeUint64(BigInt(largeUser.id));
    encoder.writeString(largeUser.name);
    encoder.writeString(largeUser.email);
    encoder.writeInt32(largeUser.age);
    encoder.writeFloat64(largeUser.score);
    encoder.writeBool(largeUser.active);
    encoder.writeString(largeUser.description);
    encoder.finish();
  });
  
  encoder.reset();
  encoder.writeUint64(BigInt(largeUser.id));
  encoder.writeString(largeUser.name);
  encoder.writeString(largeUser.email);
  encoder.writeInt32(largeUser.age);
  encoder.writeFloat64(largeUser.score);
  encoder.writeBool(largeUser.active);
  encoder.writeString(largeUser.description);
  const encoded = encoder.finish();
  const size = encoded.length;

  const decode = benchMultiple("V2 Manual Large decode", ITERATIONS, () => {
    const dec = new Decoder(encoded);
    dec.readUint64();
    dec.readString();
    dec.readString();
    dec.readInt32();
    dec.readFloat64();
    dec.readBool();
    dec.readString();
  });

  return { name: "XPB V2 (Manual)", encodeNs: encode.min, decodeNs: decode.min, sizeBytes: size };
}

// ============= JSON Benchmarks =============

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

function benchJSON_Large(): BenchResult {
  let jsonEncoded = "";
  
  const encode = benchMultiple("JSON Large encode", ITERATIONS, () => {
    jsonEncoded = JSON.stringify(largeUser);
  });

  const decode = benchMultiple("JSON Large decode", ITERATIONS, () => {
    JSON.parse(jsonEncoded);
  });

  return { name: "JSON", encodeNs: encode.min, decodeNs: decode.min, sizeBytes: jsonEncoded.length };
}

// ============= Msgpack Benchmarks =============

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

function benchMsgpack_Large(): BenchResult {
  let msgpackEncoded: Uint8Array = new Uint8Array(0);
  
  const encode = benchMultiple("Msgpack Large encode", ITERATIONS, () => {
    msgpackEncoded = msgpackEncode(largeUser);
  });

  const decode = benchMultiple("Msgpack Large decode", ITERATIONS, () => {
    msgpackDecode(msgpackEncoded);
  });

  return { name: "Msgpack", encodeNs: encode.min, decodeNs: decode.min, sizeBytes: msgpackEncoded.length };
}

// ============= Protobuf Benchmarks =============

function benchProtobuf_Small(): BenchResult {
  const message = ProtoSmallUser.create(smallUser);
  let protoEncoded: Uint8Array = new Uint8Array(0);
  
  const encode = benchMultiple("Protobuf encode", ITERATIONS, () => {
    protoEncoded = ProtoSmallUser.encode(message).finish();
  });

  const decode = benchMultiple("Protobuf decode", ITERATIONS, () => {
    ProtoSmallUser.decode(protoEncoded);
  });

  protoEncoded = ProtoSmallUser.encode(message).finish();
  return { name: "Protobuf", encodeNs: encode.min, decodeNs: decode.min, sizeBytes: protoEncoded.length };
}

function benchProtobuf_Large(): BenchResult {
  const message = ProtoLargeUser.create(largeUser);
  let protoEncoded: Uint8Array = new Uint8Array(0);
  
  const encode = benchMultiple("Protobuf Large encode", ITERATIONS, () => {
    protoEncoded = ProtoLargeUser.encode(message).finish();
  });

  const decode = benchMultiple("Protobuf Large decode", ITERATIONS, () => {
    ProtoLargeUser.decode(protoEncoded);
  });

  protoEncoded = ProtoLargeUser.encode(message).finish();
  return { name: "Protobuf", encodeNs: encode.min, decodeNs: decode.min, sizeBytes: protoEncoded.length };
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

function printSummary(label: string, xpb: BenchResult | undefined, baseline: BenchResult | undefined, baselineName: string) {
  if (xpb && baseline) {
    console.log(`\n📊 ${label} - XPB vs ${baselineName}:`);
    console.log(`  Size:   ${xpb.sizeBytes} B vs ${baseline.sizeBytes} B (${(baseline.sizeBytes / xpb.sizeBytes).toFixed(1)}x smaller)`);
    console.log(`  Encode: ${(baseline.encodeNs / xpb.encodeNs).toFixed(2)}x faster`);
    console.log(`  Decode: ${(baseline.decodeNs / xpb.decodeNs).toFixed(2)}x faster`);
  }
}

// ============= Main =============

async function main() {
  console.log("╔═══════════════════════════════════════════════════════════════╗");
  console.log("║     XPB V2 Node.js Benchmark (Best of 5 Rounds)               ║");
  console.log("╠═══════════════════════════════════════════════════════════════╣");
  console.log("║ V2 Format: Tagless, Fixed-Width Int, Compact Lengths          ║");
  console.log("║ Comparisons: JSON, MessagePack, Protobuf                      ║");
  console.log("╚═══════════════════════════════════════════════════════════════╝");

  // ============= Small Message Benchmarks =============
  console.log("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  console.log("  📦 Small Message (3 fields: name, age, active)");
  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  
  const smallResults: BenchResult[] = [];
  smallResults.push(benchXPB_V2_Small());
  smallResults.push(benchXPB_V2_Manual_Small());
  smallResults.push(benchJSON_Small());
  smallResults.push(benchMsgpack_Small());
  smallResults.push(benchProtobuf_Small());
  
  printResults("Small Message Results", smallResults);
  
  const xpbSmall = smallResults.find(r => r.name.includes("JIT"));
  const jsonSmall = smallResults.find(r => r.name === "JSON");
  const protoSmall = smallResults.find(r => r.name === "Protobuf");
  
  printSummary("Small", xpbSmall, jsonSmall, "JSON");
  printSummary("Small", xpbSmall, protoSmall, "Protobuf");

  // ============= Large Message Benchmarks =============
  console.log("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  console.log("  📦 Large Message (7 fields: id, name, email, age, score, active, description)");
  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  
  const largeResults: BenchResult[] = [];
  largeResults.push(benchXPB_V2_Large());
  largeResults.push(benchXPB_V2_Manual_Large());
  largeResults.push(benchJSON_Large());
  largeResults.push(benchMsgpack_Large());
  largeResults.push(benchProtobuf_Large());
  
  printResults("Large Message Results", largeResults);
  
  const xpbLarge = largeResults.find(r => r.name.includes("JIT"));
  const jsonLarge = largeResults.find(r => r.name === "JSON");
  const protoLarge = largeResults.find(r => r.name === "Protobuf");
  
  printSummary("Large", xpbLarge, jsonLarge, "JSON");
  printSummary("Large", xpbLarge, protoLarge, "Protobuf");

  console.log("\n✅ Benchmark complete!");
}

main().catch(console.error);

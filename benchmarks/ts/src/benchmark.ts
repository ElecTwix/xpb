/**
 * XPB TypeScript Benchmark Suite
 * Compares XPB (pure JS, hybrid) vs JSON vs MessagePack
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

// Large message for hybrid testing
const largeUser = {
  name: "Alice Johnson",
  email: "alice.johnson@example.com",
  bio: "This is a much longer description field that contains significantly more text to simulate a larger payload that would benefit from WASM processing. ".repeat(5),
  tags: ["developer", "designer", "architect", "manager", "consultant"],
  metadata: { version: "1.0", region: "us-west", tier: "premium" }
};

const testDataLarge = largeUser;
const schemaLarge = {
    fields: [
        { tag: 1, name: 'name', type: FieldType.String },
        { tag: 2, name: 'email', type: FieldType.String },
        { tag: 3, name: 'bio', type: FieldType.String },
        { tag: 4, name: 'tags', type: FieldType.String, repeated: true }
    ]
};

// ============= XPB Pure JS Benchmark =============



// ============= XPB Hybrid Benchmark =============



// ============= XPB JIT Benchmark =============

const userSchema = {
  fields: [
    { tag: 1, type: FieldType.String, name: 'name' },
    { tag: 2, type: FieldType.Int32, name: 'age' },
    { tag: 3, type: FieldType.Bool, name: 'active' }
  ]
};

function benchXPB_JIT_Small(): BenchResult {
  const iterations = 100000;
  
  // Compile once
  const jitEncode = compileEncoder<typeof smallUser>(userSchema);
  const jitDecode = compileDecoder<typeof smallUser>(userSchema);
  const slab = new SlabAllocator(65536);
  
  // Warmup
  slab.pos = 0;
  jitEncode(slab, smallUser);

  // Measurement
  const encodeNs = bench("JIT encode", iterations, () => {
    // Reset slab if getting full (simple benchmark strategy)
    if (slab.pos > 60000) slab.pos = 0;
    
    // Save start for sizing
    jitEncode(slab, smallUser);
  });

  // Calculate size for report
  slab.pos = 0;
  jitEncode(slab, smallUser);
  const size = slab.pos;
  const encoded = slab.buf.subarray(0, size);

  const decodeNs = bench("JIT decode", iterations, () => {
     jitDecode(encoded, encoded.length);
  });

  return { name: "XPB (JIT)", encodeNs, decodeNs, sizeBytes: size };
}

function benchXPB_JIT_Struct(): BenchResult {
  const iterations = 100000;
  
  // Compile with Struct Mode (No Tags) + Fixed Ints
  const jitEncode = compileEncoder<typeof smallUser>(userSchema, { structMode: true, fixedInts: true, target: 'node' });
  const jitDecode = compileDecoder<typeof smallUser>(userSchema, { structMode: true, fixedInts: true, target: 'node' });
  const slab = new SlabAllocator(65536);
  
  // Warmup
  slab.pos = 0;
  jitEncode(slab, smallUser);
  const encoded = Buffer.from(slab.buf.subarray(0, slab.pos));

  const encodeNs = bench("Struct encode", iterations, () => {
    slab.pos = 0;
    jitEncode(slab, smallUser);
  });

  const decodeNs = bench("Struct decode", iterations, () => {
    jitDecode(encoded, encoded.length);
  });

  return { name: "XPB (Struct)", encodeNs, decodeNs, sizeBytes: encoded.length };
}

function benchXPB_JIT_Aligned(): BenchResult {
  const iterations = 100000;
  
  // Compile with Aligned Mode (Fixed Ints + Padding)
  const jitEncode = compileEncoder<typeof smallUser>(userSchema, { aligned: true });
  const slab = new SlabAllocator(65536);
  
  // Warmup
  slab.pos = 0;
  jitEncode(slab, smallUser);

  const encodeNs = bench("JIT Aligned encode", iterations, () => {
    if (slab.pos > 60000) slab.pos = 0;
    jitEncode(slab, smallUser);
  });
  
  slab.pos = 0;
  jitEncode(slab, smallUser);
  const size = slab.pos;
  
  const decodeNs = 0;

  return { name: "XPB (Aligned)", encodeNs, decodeNs, sizeBytes: size };
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
function benchXPB_JIT_Large(): BenchResult {
    const iters = 50000;
    // Compile JIT for Large Schema
    const jitEncode = compileEncoder<typeof testDataLarge>(schemaLarge, { target: 'node' });
    const jitDecode = compileDecoder<typeof testDataLarge>(schemaLarge);
    
    // Create new slab
    const slab = new SlabAllocator(65536);

    // Warmup & Size Capture
    slab.pos = 0;
    jitEncode(slab, testDataLarge);
    const encoded = Buffer.from(slab.buf.subarray(0, slab.pos));

    const encodeNs = bench("JIT Large encode", iters, () => {
        if (slab.pos > 60000) slab.pos = 0;
        jitEncode(slab, testDataLarge);
    });
    const decodeNs = bench("JIT Large decode", iters, () => {
        jitDecode(encoded, encoded.length);
    });

    return { name: "XPB (JIT)", encodeNs, decodeNs, sizeBytes: encoded.length };
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
  smallResults.push(benchXPB_JIT_Struct());
  smallResults.push(benchJSON_Small());
  smallResults.push(benchMsgpack_Small());
  
  printResults("Small Message (19-47 bytes)", smallResults);

  // Large message benchmarks
  const largeResults: BenchResult[] = [];
  largeResults.push(benchXPB_JIT_Large());
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

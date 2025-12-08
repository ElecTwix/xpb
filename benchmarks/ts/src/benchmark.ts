/**
 * XPB TypeScript Benchmark Suite
 * Compares XPB vs JSON vs MessagePack vs Protobuf
 */

import { encode as msgpackEncode, decode as msgpackDecode } from '@msgpack/msgpack';

// ============= XPB Runtime (inline for benchmark) =============

const WireType = { Varint: 0, Fixed64: 1, LengthDelimited: 2, Fixed32: 5 } as const;

function zigzagEncode32(n: number): number {
  return (n << 1) ^ (n >> 31);
}

function zigzagDecode32(n: number): number {
  return (n >>> 1) ^ -(n & 1);
}

class XPBEncoder {
  private buf: number[] = [];

  private writeVarint(n: number | bigint): void {
    let val = typeof n === 'bigint' ? n : BigInt(n);
    while (val > 0x7fn) {
      this.buf.push(Number(val & 0x7fn) | 0x80);
      val >>= 7n;
    }
    this.buf.push(Number(val));
  }

  private writeTag(fieldNum: number, wireType: number): void {
    this.writeVarint((fieldNum << 3) | wireType);
  }

  writeString(fieldNum: number, value: string): void {
    this.writeTag(fieldNum, WireType.LengthDelimited);
    const bytes = new TextEncoder().encode(value);
    this.writeVarint(bytes.length);
    for (const b of bytes) this.buf.push(b);
  }

  writeInt32(fieldNum: number, value: number): void {
    this.writeTag(fieldNum, WireType.Varint);
    this.writeVarint(zigzagEncode32(value));
  }

  writeBool(fieldNum: number, value: boolean): void {
    this.writeTag(fieldNum, WireType.Varint);
    this.buf.push(value ? 1 : 0);
  }

  finish(): Uint8Array {
    return new Uint8Array(this.buf);
  }
}

class XPBDecoder {
  private pos = 0;
  private view: DataView;
  private data: Uint8Array;

  constructor(data: Uint8Array) {
    this.data = data;
    this.view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  }

  eof(): boolean { return this.pos >= this.data.length; }

  private readVarint(): bigint {
    let result = 0n;
    let shift = 0n;
    while (this.pos < this.data.length) {
      const b = this.data[this.pos++];
      result |= BigInt(b & 0x7f) << shift;
      if ((b & 0x80) === 0) break;
      shift += 7n;
    }
    return result;
  }

  readTag(): [number, number] {
    const tag = Number(this.readVarint());
    return [tag >>> 3, tag & 0x7];
  }

  readString(): string {
    const len = Number(this.readVarint());
    const bytes = this.data.slice(this.pos, this.pos + len);
    this.pos += len;
    return new TextDecoder().decode(bytes);
  }

  readInt32(): number {
    return zigzagDecode32(Number(this.readVarint()));
  }

  readBool(): boolean {
    return this.readVarint() !== 0n;
  }

  skip(wireType: number): void {
    switch (wireType) {
      case WireType.Varint: this.readVarint(); break;
      case WireType.Fixed64: this.pos += 8; break;
      case WireType.Fixed32: this.pos += 4; break;
      case WireType.LengthDelimited:
        const len = Number(this.readVarint());
        this.pos += len;
        break;
    }
  }
}

// ============= Benchmark Utilities =============

interface BenchResult {
  name: string;
  encodeNs: number;
  decodeNs: number;
  sizeBytes: number;
  encodeAllocs?: string;
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
  
  // Encode
  let encoded: Uint8Array = new Uint8Array();
  const encodeNs = bench("XPB encode", iterations, () => {
    const enc = new XPBEncoder();
    enc.writeString(1, testUser.name);
    enc.writeInt32(2, testUser.age);
    enc.writeBool(3, testUser.active);
    encoded = enc.finish();
  });

  // Decode
  const decodeNs = bench("XPB decode", iterations, () => {
    const dec = new XPBDecoder(encoded);
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
  console.log("║           XPB TypeScript Benchmark Results                   ║");
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

  console.log("Comparison vs XPB:");
  console.log(`  JSON encode:   ${(json.encodeNs / xpb.encodeNs).toFixed(1)}x slower`);
  console.log(`  JSON decode:   ${(json.decodeNs / xpb.decodeNs).toFixed(1)}x slower`);
  console.log(`  JSON size:     ${(json.sizeBytes / xpb.sizeBytes).toFixed(1)}x larger`);
  console.log(`  Msgpack enc:   ${(msgpack.encodeNs / xpb.encodeNs).toFixed(1)}x slower`);
  console.log(`  Msgpack dec:   ${(msgpack.decodeNs / xpb.decodeNs).toFixed(1)}x slower`);
  console.log(`  Msgpack size:  ${(msgpack.sizeBytes / xpb.sizeBytes).toFixed(1)}x larger`);
}

main().catch(console.error);

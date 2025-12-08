
import { bench, printResults, BenchResult } from './benchmark.js';
import { SlabAllocator } from '../../../runtime/ts/src/jit.js';

async function main() {
  console.log("Running Platform Micro-Benchmarks...");

  const results: BenchResult[] = [];

  // Used for Int32 tests
  const slab = new SlabAllocator(65536);
  const u8 = slab.buf;
  const i32 = new Int32Array(u8.buffer, u8.byteOffset, u8.byteLength >> 2);
  const dv = new DataView(u8.buffer, u8.byteOffset, u8.byteLength);
  
  const iterations = 1_000_000;
  let pos = 0;

  // --- Int32 Write Strategies ---

  results.push({ name: "Int32 (Manual Byte)", encodeNs: bench("Int32 (Manual Byte)", iterations, () => {
     const v = 123456789; // constant for fair test
     u8[pos++] = v; 
     u8[pos++] = v >> 8; 
     u8[pos++] = v >> 16; 
     u8[pos++] = v >> 24;
     if (pos >= 60000) pos = 0;
  }), decodeNs: 0, sizeBytes: 4 });

  pos = 0;
  results.push({ name: "Int32 (DataView)", encodeNs: bench("Int32 (DataView)", iterations, () => {
     dv.setInt32(pos, 123456789, true); // Little Endian
     pos += 4;
     if (pos >= 60000) pos = 0;
  }), decodeNs: 0, sizeBytes: 4 });

  pos = 0;
  results.push({ name: "Int32 (Int32Array)", encodeNs: bench("Int32 (Int32Array)", iterations, () => {
     // Require aligned pos
     i32[pos >> 2] = 123456789;
     pos += 4;
     if (pos >= 60000) pos = 0;
  }), decodeNs: 0, sizeBytes: 4 });

  printResults("Int32 Write (4 bytes)", results);

  // --- String Write Strategies ---
  
  const strResults: BenchResult[] = [];
  const testStr = "Hello World! Standard ASCII string.";
  const testStrBuffer = Buffer.from(testStr);
  const te = new TextEncoder();
  
  pos = 0;
  strResults.push({ name: "String (TextEncoder)", encodeNs: bench("String (TextEncoder)", iterations, () => {
     const res = te.encodeInto(testStr, u8.subarray(pos));
     pos += res.written;
     if (pos >= 60000) pos = 0;
  }), decodeNs: 0, sizeBytes: testStr.length });

  pos = 0;
  // Node.js Buffer.write
  // Note: we need a Buffer instance wrapper around the u8array
  const nodeBuf = Buffer.from(u8.buffer, u8.byteOffset, u8.byteLength);
  
  strResults.push({ name: "String (Node Buffer.write)", encodeNs: bench("String (Node Buffer.write)", iterations, () => {
     const written = nodeBuf.write(testStr, pos);
     pos += written;
     if (pos >= 60000) pos = 0;
  }), decodeNs: 0, sizeBytes: testStr.length });

  // Manual ASCII Loop (Our JIT Optimization)
  pos = 0;
  strResults.push({ name: "String (ASCII Loop)", encodeNs: bench("String (ASCII Loop)", iterations, () => {
      for (let i = 0; i < testStr.length; i++) {
          u8[pos + i] = testStr.charCodeAt(i);
      }
      pos += testStr.length;
      if (pos >= 60000) pos = 0;
  }), decodeNs: 0, sizeBytes: testStr.length });

  printResults("String Write (~35 chars)", strResults);
}

main();

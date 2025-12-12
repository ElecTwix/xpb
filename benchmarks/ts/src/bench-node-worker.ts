import { Worker } from 'worker_threads';
import path from 'path';
import { fileURLToPath } from 'url';
import { Buffer } from 'buffer';
import { Encoder, Decoder } from '../../../runtime/ts/src/node'; // Import directly from source

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const WORKER_PATH = path.join(__dirname, '../dist/node-worker.cjs');

// --- Test Data Generation ---

function generateLargeStringArray(count: number): string[] {
  const arr = [];
  for (let i = 0; i < count; i++) {
    arr.push(`item_${String.fromCharCode(65 + (i % 26))}_value_${i}_padding`);
  }
  return arr;
}

function generateLargeIntArray(count: number): number[] {
  const arr = [];
  for (let i = 0; i < count; i++) {
    arr.push(i * 17);
  }
  return arr;
}

function encodeStringArray(arr: string[]): Buffer {
  const encoder = new Encoder(arr.length * 30);
  encoder.writeInt32(arr.length);
  for (let i = 0; i < arr.length; i++) {
    const s = arr[i];
    if (s === undefined) {
      console.error(`Array contains undefined at index ${i}`);
      process.exit(1);
    }
    encoder.writeString(s);
  }
  return Buffer.from(encoder.finish());
}

function encodeInt32Array(arr: number[]): Buffer {
  const encoder = new Encoder(arr.length * 4 + 4);
  encoder.writeInt32(arr.length);
  for (const v of arr) {
    encoder.writeInt32(v);
  }
  return Buffer.from(encoder.finish());
}

// --- Main Thread Decoding ---

function decodeStringArraySync(buf: Buffer): string[] {
  const decoder = new Decoder(buf);
  const count = decoder.readInt32();
  const res = new Array(count);
  for (let i = 0; i < count; i++) {
    res[i] = decoder.readString();
  }
  return res;
}

function decodeInt32ArraySync(buf: Buffer): number[] {
  const decoder = new Decoder(buf);
  const count = decoder.readInt32();
  const res = new Array(count);
  for (let i = 0; i < count; i++) {
    res[i] = decoder.readInt32();
  }
  return res;
}

// --- Worker Decoding ---

class WorkerPool {
  private worker: Worker;
  private nextId = 0;
  private pending = new Map<number, (v: any) => void>();

  constructor() {
    this.worker = new Worker(WORKER_PATH);
    
    this.worker.on('message', (msg) => {
      const { id, result, error } = msg;
      const resolve = this.pending.get(id);
      if (resolve) {
        this.pending.delete(id);
        if (error) console.error("Worker Error:", error);
        resolve(result);
      }
    });

    this.worker.on('error', (err) => {
      console.error("Worker Thread Error:", err);
    });

    this.worker.on('exit', (code) => {
      if (code !== 0) console.error(`Worker stopped with exit code ${code}`);
    });
  }

  decode(type: string, buffer: Buffer): Promise<any> {
    const id = this.nextId++;
    return new Promise(resolve => {
      this.pending.set(id, resolve);
      const ab = buffer.buffer.slice(buffer.byteOffset, buffer.byteOffset + buffer.length);
      this.worker.postMessage({ id, type, buffer: ab }, [ab]);
    });
  }
  
  terminate() {
    this.worker.terminate();
  }
}

async function bench(name: string, fn: () => any, iter = 5) {
  const start = process.hrtime.bigint();
  for (let i = 0; i < iter; i++) {
    // if (i % 5 === 0) process.stdout.write('.');
    await fn();
  }
  // console.log("");
  const end = process.hrtime.bigint();
  const ns = Number(end - start) / iter;
  return ns / 1e6; // ms
}

async function run() {
  const pool = new WorkerPool();
  
  // Warmup
  await new Promise(r => setTimeout(r, 1000));

  console.log("╔═══════════════════════════════════════════════════════════════╗");
  console.log("║           XPB V2 Node.js Worker Benchmark                     ║");
  console.log("╚═══════════════════════════════════════════════════════════════╝");

  const sizes = [
    { name: "Small (100)", count: 100 },
    { name: "Medium (1K)", count: 1000 },
    { name: "Large (10K)", count: 10000 },
    // { name: "XLarge (100K)", count: 100000 },
    // { name: "Huge (1M)", count: 1000000 },
  ];

  console.log("\n📦 String Array Benchmark:");
  console.log("┌───────────────┬──────────┬──────────┬──────────┬─────────┐");
  console.log("│ Size          │ Payload  │ Main     │ Worker   │ Speedup │");
  console.log("├───────────────┼──────────┼──────────┼──────────┼─────────┤");

  for (const s of sizes) {
    const data = generateLargeStringArray(s.count);
    const buf = encodeStringArray(data);
    const size = (buf.length / 1024).toFixed(1) + " KB";
    
    // Main
    const tMain = await bench("Main", () => decodeStringArraySync(buf), 5);
    
    // Worker
    const tWorker = await bench("Worker", async () => {
      // Need a fresh buffer copy for each iter because it gets transferred
      const freshBuf = Buffer.from(buf);
      const res = await pool.decode('stringArray', freshBuf);
      return res;
    }, 5);
    
    const speedup = (tMain / tWorker).toFixed(2) + "x";
    const hl = tWorker < tMain ? "\x1b[32m" : "";
    const rst = "\x1b[0m";
    
    console.log(`│ ${s.name.padEnd(13)} │ ${size.padEnd(8)} │ ${tMain.toFixed(2).padStart(6)} ms │ ${hl}${tWorker.toFixed(2).padStart(6)} ms${rst} │ ${hl}${speedup.padStart(7)}${rst} │`);
  }
  console.log("└───────────────┴──────────┴──────────┴──────────┴─────────┘");

  console.log("\n📦 Int32 Array Benchmark:");
  console.log("┌───────────────┬──────────┬──────────┬──────────┬─────────┐");
  console.log("│ Size          │ Payload  │ Main     │ Worker   │ Speedup │");
  console.log("├───────────────┼──────────┼──────────┼──────────┼─────────┤");

  for (const s of sizes) {
    const data = generateLargeIntArray(s.count);
    const buf = encodeInt32Array(data);
    const size = (buf.length / 1024).toFixed(1) + " KB";
    
    // Main
    const tMain = await bench("Main", () => decodeInt32ArraySync(buf), 5);
    
    // Worker
    const tWorker = await bench("Worker", async () => {
      const freshBuf = Buffer.from(buf);
      return await pool.decode('int32Array', freshBuf);
    }, 5);
    
    const speedup = (tMain / tWorker).toFixed(2) + "x";
    const hl = tWorker < tMain ? "\x1b[32m" : "";
    const rst = "\x1b[0m";
    
    console.log(`│ ${s.name.padEnd(13)} │ ${size.padEnd(8)} │ ${tMain.toFixed(2).padStart(6)} ms │ ${hl}${tWorker.toFixed(2).padStart(6)} ms${rst} │ ${hl}${speedup.padStart(7)}${rst} │`);
  }
  console.log("└───────────────┴──────────┴──────────┴──────────┴─────────┘");

  pool.terminate();
}

run().catch(console.error);

/**
 * XPB V2 Node.js Worker Pool
 * 
 * High-performance parallel decoding using worker_threads.
 */

import { Worker } from 'worker_threads';
import { cpus } from 'os';
import { Decoder } from './node';
import { Buffer } from 'node:buffer';

interface PendingRequest {
  resolve: (value: any) => void;
  reject: (error: Error) => void;
}

interface StringArrayResult {
  offsets: Int32Array;
  data: Uint8Array;
}

// Performance thresholds for switching to worker (bytes)
// Derived from benchmarks. Node main thread is fast (C++), so thresholds are higher.
const THRESHOLD_STRINGS = 250 * 1024; // 250KB
const THRESHOLD_INTS = 500 * 1024;    // 500KB

export class XPBNodeWorkerPool {
  private workers: Worker[] = [];
  private workerQueue: Worker[] = [];
  private pendingRequests: Map<number, PendingRequest> = new Map();
  private nextRequestId = 0;
  private initialized = false;

  constructor(private poolSize = cpus().length || 4) {}

  /**
   * Initialize the pool with the path to the worker script.
   * @param workerScriptPath Path to the 'node-worker.js' script
   */
  async init(workerScriptPath: string): Promise<void> {
    if (this.initialized) return;

    for (let i = 0; i < this.poolSize; i++) {
      const worker = new Worker(workerScriptPath);
      
      worker.on('message', (msg) => {
        const { id, result, error } = msg;
        const pending = this.pendingRequests.get(id);
        
        if (pending) {
          this.pendingRequests.delete(id);
          this.workerQueue.push(worker);
          
          if (error) {
            pending.reject(new Error(error));
          } else {
            pending.resolve(result);
          }
        }
      });

      worker.on('error', (err) => console.error('Worker error:', err));
      worker.on('exit', (code) => {
        if (code !== 0) console.error(`Worker stopped with exit code ${code}`);
      });

      this.workers.push(worker);
      this.workerQueue.push(worker);
    }

    this.initialized = true;
  }

  private async getWorker(): Promise<Worker> {
    if (this.workerQueue.length > 0) return this.workerQueue.pop()!;
    
    return new Promise(resolve => {
      const interval = setInterval(() => {
        if (this.workerQueue.length > 0) {
          clearInterval(interval);
          resolve(this.workerQueue.pop()!);
        }
      }, 5);
    });
  }

  /**
   * Decodes an Int32Array using a worker.
   * - < 500KB: Main Thread (Sync, fast)
   * - > 500KB: Worker (Async, non-blocking)
   */
  async decodeInt32Array(buffer: Uint8Array): Promise<Int32Array> {
    if (buffer.byteLength < THRESHOLD_INTS) {
      const decoder = new Decoder(buffer);
      const count = decoder.readInt32();
      const result = new Int32Array(count);
      for (let i = 0; i < count; i++) {
        result[i] = decoder.readInt32();
      }
      return result;
    }

    const worker = await this.getWorker();
    const id = this.nextRequestId++;
    
    return new Promise<Int32Array>((resolve, reject) => {
      this.pendingRequests.set(id, { resolve, reject });
      
      // Zero-copy transfer if possible, or copy.
      // In Node, passing a TypedArray view of a shared buffer might clone.
      // Best to pass the underlying ArrayBuffer if it's unique.
      const ab = buffer.buffer.slice(buffer.byteOffset, buffer.byteOffset + buffer.length);
      worker.postMessage({ id, type: 'int32Array', buffer: ab }, [ab as any]);
    });
  }

  /**
   * Decodes a string array using a worker.
   * - < 250KB: Main Thread (Sync, fast)
   * - > 250KB: Worker (Async, non-blocking)
   */
  async decodeStringArray(buffer: Uint8Array): Promise<string[]> {
    if (buffer.byteLength < THRESHOLD_STRINGS) {
      const decoder = new Decoder(buffer);
      const count = decoder.readInt32();
      const result = new Array(count);
      for (let i = 0; i < count; i++) {
        result[i] = decoder.readString();
      }
      return result;
    }

    const worker = await this.getWorker();
    const id = this.nextRequestId++;
    
    const rawResult = await new Promise<StringArrayResult>((resolve, reject) => {
      this.pendingRequests.set(id, { resolve, reject });
      const ab = buffer.buffer.slice(buffer.byteOffset, buffer.byteOffset + buffer.length);
      worker.postMessage({ id, type: 'stringArray', buffer: ab }, [ab as any]);
    });

    const { offsets, data } = rawResult;
    const count = offsets.length - 1;
    const result = new Array<string>(count);
    
    // Node.js Buffer.toString is fast
    const buf = Buffer.from(data.buffer, data.byteOffset, data.length);
    
    for (let i = 0; i < count; i++) {
      const start = offsets[i];
      const end = offsets[i+1];
      result[i] = buf.toString('utf8', start, end);
    }
    
    return result;
  }

  terminate() {
    this.workers.forEach(w => w.terminate());
    this.workers = [];
    this.workerQueue = [];
    this.initialized = false;
  }
}

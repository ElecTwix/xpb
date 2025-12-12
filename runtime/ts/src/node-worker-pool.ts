/**
 * XPB V2 Node.js Worker Pool
 * 
 * High-performance parallel decoding using worker_threads.
 */

import { Worker } from 'worker_threads';
import { cpus } from 'os';

interface PendingRequest {
  resolve: (value: any) => void;
  reject: (error: Error) => void;
}

interface StringArrayResult {
  offsets: Int32Array;
  data: Uint8Array;
}

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
   * Recommendation: Use for arrays > 250KB (approx 60k items).
   */
  async decodeInt32Array(buffer: Uint8Array): Promise<Int32Array> {
    const worker = await this.getWorker();
    const id = this.nextRequestId++;
    
    return new Promise<Int32Array>((resolve, reject) => {
      this.pendingRequests.set(id, { resolve, reject });
      
      // Zero-copy transfer if possible, or copy.
      // In Node, passing a TypedArray view of a shared buffer might clone.
      // Best to pass the underlying ArrayBuffer if it's unique.
      const ab = buffer.buffer.slice(buffer.byteOffset, buffer.byteOffset + buffer.length);
      worker.postMessage({ id, type: 'int32Array', buffer: ab }, [ab]);
    });
  }

  /**
   * Decodes a string array using a worker.
   * Recommendation: Use for arrays > 250KB (approx 10k items).
   */
  async decodeStringArray(buffer: Uint8Array): Promise<string[]> {
    const worker = await this.getWorker();
    const id = this.nextRequestId++;
    
    const rawResult = await new Promise<StringArrayResult>((resolve, reject) => {
      this.pendingRequests.set(id, { resolve, reject });
      const ab = buffer.buffer.slice(buffer.byteOffset, buffer.byteOffset + buffer.length);
      worker.postMessage({ id, type: 'stringArray', buffer: ab }, [ab]);
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

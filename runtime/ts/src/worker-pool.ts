import { Decoder } from './browser';

/**
 * XPB V2 Worker Pool
 * 
 * High-performance parallel decoding using Transferable Objects.
 */

interface PendingRequest {
  resolve: (value: any) => void;
  reject: (error: Error) => void;
}

interface StringArrayResult {
  offsets: Int32Array;
  data: Uint8Array;
}

// Performance thresholds for switching to worker (bytes)
// Derived from extensive benchmarks:
// - Strings: Worker is faster > 5KB. Safe bet: 10KB.
// - Int32: Main thread is super fast. Worker only wins > 200KB.
const THRESHOLD_STRINGS = 10 * 1024;
const THRESHOLD_INTS = 200 * 1024;

export class XPBWorkerPool {
  private workers: Worker[] = [];
  private workerQueue: Worker[] = [];
  private pendingRequests: Map<number, PendingRequest> = new Map();
  private nextRequestId = 0;
  private initialized = false;
  private textDecoder = new TextDecoder();

  constructor(private poolSize = navigator.hardwareConcurrency || 4) {}

  /**
   * Initialize the pool with the path to the worker script.
   * @param workerScriptUrl URL to the bundled 'worker.js'
   */
  async init(workerScriptUrl: string): Promise<void> {
    if (this.initialized) return;

    const readyPromises: Promise<void>[] = [];

    for (let i = 0; i < this.poolSize; i++) {
      const worker = new Worker(workerScriptUrl, { type: 'module' });
      
      const readyPromise = new Promise<void>((resolve) => {
        const onReady = (event: MessageEvent) => {
          if (event.data.type === 'ready') {
            worker.removeEventListener('message', onReady);
            resolve();
          }
        };
        worker.addEventListener('message', onReady);
      });
      readyPromises.push(readyPromise);

      worker.addEventListener('message', (event: MessageEvent) => {
        if (event.data.type === 'ready') return;
        
        const { id, result, error } = event.data;
        const pending = this.pendingRequests.get(id);
        
        if (pending) {
          this.pendingRequests.delete(id);
          this.workerQueue.push(worker); // Return worker to queue
          
          if (error) {
            pending.reject(new Error(error));
          } else {
            pending.resolve(result);
          }
        }
      });

      this.workers.push(worker);
      this.workerQueue.push(worker);
    }

    await Promise.all(readyPromises);
    this.initialized = true;
  }

  private async getWorker(): Promise<Worker> {
    if (this.workerQueue.length > 0) return this.workerQueue.pop()!;
    
    // Simple busy-wait for now, can be optimized with a queue
    return new Promise(resolve => {
      const interval = setInterval(() => {
        if (this.workerQueue.length > 0) {
          clearInterval(interval);
          resolve(this.workerQueue.pop()!);
        }
      }, 5);
    });
  }

  // --- Public API ---

  /**
   * Decodes an Int32Array using a worker or main thread based on size.
   * - < 200KB: Main Thread (Sync, fast)
   * - > 200KB: Worker (Async, non-blocking)
   */
  async decodeInt32Array(buffer: ArrayBuffer): Promise<Int32Array> {
    if (buffer.byteLength < THRESHOLD_INTS) {
      const decoder = new Decoder(new Uint8Array(buffer));
      const count = decoder.readArrayCount(4);
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
      worker.postMessage({ id, type: 'int32Array', buffer }, [buffer]);
    });
  }

  /**
   * Decodes a string array using a worker or main thread based on size.
   * - < 10KB: Main Thread (Sync, fast)
   * - > 10KB: Worker (Async, non-blocking)
   */
  async decodeStringArray(buffer: ArrayBuffer): Promise<string[]> {
    if (buffer.byteLength < THRESHOLD_STRINGS) {
      const decoder = new Decoder(new Uint8Array(buffer));
      const count = decoder.readArrayCount(1);
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
      worker.postMessage({ id, type: 'stringArray', buffer }, [buffer]);
    });

    // Reconstruct strings on main thread (CPU bound but non-blocking for the decoding part)
    const { offsets, data } = rawResult;
    const count = offsets.length - 1;
    const result = new Array<string>(count);
    
    for (let i = 0; i < count; i++) {
      const start = offsets[i];
      const end = offsets[i+1];
      result[i] = this.textDecoder.decode(data.subarray(start, end));
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

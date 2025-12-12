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
          
          // Process next queued item if any (simplified: we rely on caller to await)
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

  private async request(type: string, buffer: ArrayBuffer): Promise<any> {
    if (!this.initialized) throw new Error("WorkerPool not initialized");
    const worker = await this.getWorker();
    const id = this.nextRequestId++;
    
    return new Promise((resolve, reject) => {
      this.pendingRequests.set(id, { resolve, reject });
      worker.postMessage({ id, type, buffer }, [buffer]);
    });
  }

  // --- Public API ---

  /**
   * Decodes an Int32Array using a worker.
   * Recommendation: Use only for VERY large arrays (> 200KB or 50k items).
   * For smaller arrays, the main thread is faster.
   */
  async decodeInt32Array(buffer: ArrayBuffer): Promise<Int32Array> {
    const worker = await this.getWorker();
    const id = this.nextRequestId++;
    
    return new Promise<Int32Array>((resolve, reject) => {
      this.pendingRequests.set(id, { resolve, reject });
      worker.postMessage({ id, type: 'int32Array', buffer }, [buffer]);
    });
  }

  /**
   * Decodes a string array using a worker.
   * Recommendation: Use for arrays > 20KB or 1k items.
   * Returns a standard string[] but reconstructed efficiently from shared buffers.
   */
  async decodeStringArray(buffer: ArrayBuffer): Promise<string[]> {
    const worker = await this.getWorker();
    const id = this.nextRequestId++;
    
    const rawResult = await new Promise<StringArrayResult>((resolve, reject) => {
      this.pendingRequests.set(id, { resolve, reject });
      worker.postMessage({ id, type: 'stringArray', buffer }, [buffer]);
    });

    // Reconstruct strings on main thread (CPU bound but non-blocking for the decoding part)
    // For maximum performance, users should use a LazyView, but standard API expects string[]
    const { offsets, data } = rawResult;
    const count = offsets.length - 1;
    const result = new Array<string>(count);
    
    for (let i = 0; i < count; i++) {
      const start = offsets[i];
      const end = offsets[i+1];
      // Short string optimization in V8?
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

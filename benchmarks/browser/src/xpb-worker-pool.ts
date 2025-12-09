/**
 * XPB V2 Worker Pool for Async Decoding
 * 
 * Provides an async API for decoding large XPB payloads using Web Workers.
 * This avoids blocking the main thread for large payloads (>10KB recommended).
 * 
 * Usage:
 *   const pool = new XPBWorkerPool();
 *   await pool.init();
 *   
 *   // Decode with schema
 *   const result = await pool.decodeAsync(buffer, schema);
 *   
 *   // Decode collections
 *   const strings = await pool.decodeStringArrayAsync(buffer);
 *   const ints = await pool.decodeInt32ArrayAsync(buffer);
 *   const map = await pool.decodeStringMapAsync(buffer);
 */

export interface FieldDef {
  tag: number;
  type: number;
  name: string;
}

export interface SchemaDef {
  fields: FieldDef[];
}

interface PendingRequest {
  resolve: (value: any) => void;
  reject: (error: Error) => void;
}

/**
 * Worker pool for parallel XPB decoding.
 * Uses multiple workers for concurrent decode operations.
 */
export class XPBWorkerPool {
  private workers: Worker[] = [];
  private workerQueue: Worker[] = [];
  private pendingRequests: Map<number, PendingRequest> = new Map();
  private nextRequestId = 0;
  private workerUrl: string | null = null;
  private initialized = false;
  
  /**
   * Create a worker pool.
   * @param poolSize Number of workers to create (default: navigator.hardwareConcurrency or 4)
   */
  constructor(private poolSize = navigator.hardwareConcurrency || 4) {}
  
  /**
   * Initialize the worker pool. Must be called before any decode operations.
   * @param workerUrl URL to the worker script (defaults to inline blob URL)
   */
  async init(workerUrl?: string): Promise<void> {
    if (this.initialized) return;
    
    // Create worker URL from inline code if not provided
    if (!workerUrl) {
      this.workerUrl = await this.createInlineWorkerUrl();
    } else {
      this.workerUrl = workerUrl;
    }
    
    // Create workers
    const readyPromises: Promise<void>[] = [];
    
    for (let i = 0; i < this.poolSize; i++) {
      const worker = new Worker(this.workerUrl, { type: 'module' });
      
      // Wait for worker to be ready
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
      
      // Handle responses
      worker.addEventListener('message', (event: MessageEvent) => {
        if (event.data.type === 'ready') return;
        
        const { id, result, error } = event.data;
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
      
      this.workers.push(worker);
      this.workerQueue.push(worker);
    }
    
    await Promise.all(readyPromises);
    this.initialized = true;
  }
  
  /**
   * Create an inline worker URL from embedded code.
   * This avoids needing a separate worker file.
   */
  private async createInlineWorkerUrl(): Promise<string> {
    // Fetch the worker script
    const response = await fetch('./dist/xpb-worker.js');
    const workerCode = await response.text();
    const blob = new Blob([workerCode], { type: 'application/javascript' });
    return URL.createObjectURL(blob);
  }
  
  /**
   * Get an available worker, waiting if all are busy.
   */
  private async getWorker(): Promise<Worker> {
    if (this.workerQueue.length > 0) {
      return this.workerQueue.pop()!;
    }
    
    // Wait for a worker to become available
    return new Promise((resolve) => {
      const interval = setInterval(() => {
        if (this.workerQueue.length > 0) {
          clearInterval(interval);
          resolve(this.workerQueue.pop()!);
        }
      }, 1);
    });
  }
  
  /**
   * Send a decode request to a worker.
   */
  private async sendRequest(type: string, buffer: ArrayBuffer, schema?: SchemaDef): Promise<any> {
    if (!this.initialized) {
      throw new Error('XPBWorkerPool not initialized. Call init() first.');
    }
    
    const worker = await this.getWorker();
    const id = this.nextRequestId++;
    
    return new Promise((resolve, reject) => {
      this.pendingRequests.set(id, { resolve, reject });
      
      // Transfer the buffer to avoid copy
      worker.postMessage(
        { id, type, buffer, schema },
        [buffer] // Transfer ownership
      );
    });
  }
  
  /**
   * Decode an object using a schema.
   * The buffer is transferred to the worker (zero-copy).
   * 
   * @param buffer XPB encoded data (will be transferred, cannot be reused)
   * @param schema Field definitions
   * @returns Decoded object
   */
  async decodeAsync<T>(buffer: ArrayBuffer, schema: SchemaDef): Promise<T> {
    return this.sendRequest('schema', buffer, schema);
  }
  
  /**
   * Decode a string array.
   */
  async decodeStringArrayAsync(buffer: ArrayBuffer): Promise<string[]> {
    return this.sendRequest('stringArray', buffer);
  }
  
  /**
   * Decode an int32 array.
   */
  async decodeInt32ArrayAsync(buffer: ArrayBuffer): Promise<number[]> {
    return this.sendRequest('int32Array', buffer);
  }
  
  /**
   * Decode a string map.
   */
  async decodeStringMapAsync(buffer: ArrayBuffer): Promise<Record<string, string>> {
    return this.sendRequest('stringMap', buffer);
  }
  
  /**
   * Terminate all workers and clean up resources.
   */
  terminate(): void {
    for (const worker of this.workers) {
      worker.terminate();
    }
    this.workers = [];
    this.workerQueue = [];
    this.pendingRequests.clear();
    
    if (this.workerUrl) {
      URL.revokeObjectURL(this.workerUrl);
      this.workerUrl = null;
    }
    
    this.initialized = false;
  }
}

/**
 * Threshold for using worker-based decoding (bytes).
 * Below this size, main-thread decode is faster due to worker overhead.
 */
export const WORKER_THRESHOLD = 10 * 1024; // 10KB

/**
 * Singleton worker pool for convenience.
 * Initialize once with `initWorkerPool()` before using `decodeWithWorker()`.
 */
let globalPool: XPBWorkerPool | null = null;

/**
 * Initialize the global worker pool.
 * @param poolSize Number of workers (default: CPU cores)
 */
export async function initWorkerPool(poolSize?: number): Promise<void> {
  if (globalPool) return;
  globalPool = new XPBWorkerPool(poolSize);
  await globalPool.init();
}

/**
 * Get the global worker pool, or null if not initialized.
 */
export function getWorkerPool(): XPBWorkerPool | null {
  return globalPool;
}

/**
 * Terminate the global worker pool.
 */
export function terminateWorkerPool(): void {
  if (globalPool) {
    globalPool.terminate();
    globalPool = null;
  }
}

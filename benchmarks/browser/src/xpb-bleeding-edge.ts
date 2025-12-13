/**
 * xpb-bleeding-edge.ts
 * 
 * Implementations of Next-Gen Browser Performance Techniques
 */

// ==========================================
// 1. Native Base64 (Strict 2025+)
// ==========================================

export const NativeBase64 = {
  isSupported: () => typeof Uint8Array.prototype.toBase64 === 'function',
  
  encode: (data: Uint8Array): string => {
    // @ts-ignore
    return data.toBase64();
  },
  
  decode: (base64: string): Uint8Array => {
    // @ts-ignore
    return Uint8Array.fromBase64(base64);
  }
};

// ==========================================
// 2. Accessor Pattern (Zero-Copy)
// ==========================================

// Schema: User { id: int32, name: string, active: bool, score: float64 }
// Offset Map (manual layout for max speed):
// 0: id (4 bytes)
// 4: active (1 byte)
// 5: score (8 bytes)
// 13: nameLen (1 byte - using simple encoding for demo)
// 14: nameBytes...

export class ZeroCopyUser {
  private view: DataView;
  private u8: Uint8Array;
  private baseOffset: number;
  private _name: string | null = null; // Lazy cache

  constructor(buffer: Uint8Array, offset: number = 0) {
    this.u8 = buffer;
    this.view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
    this.baseOffset = offset;
  }

  get id(): number {
    return this.view.getInt32(this.baseOffset, true);
  }

  get active(): boolean {
    return this.u8[this.baseOffset + 4] !== 0;
  }

  get score(): number {
    return this.view.getFloat64(this.baseOffset + 5, true);
  }

  get name(): string {
    if (this._name !== null) return this._name;
    
    // Read length (simple 1 byte for demo)
    const len = this.u8[this.baseOffset + 13];
    // Fast ASCII path check could go here
    const start = this.baseOffset + 14;
    const bytes = this.u8.subarray(start, start + len);
    this._name = new TextDecoder().decode(bytes);
    return this._name;
  }
}

// Standard Object for Comparison
export class StandardUser {
  id: number;
  name: string;
  active: boolean;
  score: number;

  constructor(id: number, name: string, active: boolean, score: number) {
    this.id = id;
    this.name = name;
    this.active = active;
    this.score = score;
  }

  // Simulate standard "decode" that reads bytes and creates object
  static decode(buffer: Uint8Array, offset: number = 0): StandardUser {
    const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
    const id = view.getInt32(offset, true);
    const active = buffer[offset + 4] !== 0;
    const score = view.getFloat64(offset + 5, true);
    
    const len = buffer[offset + 13];
    const bytes = buffer.subarray(offset + 14, offset + 14 + len);
    const name = new TextDecoder().decode(bytes);
    
    return new StandardUser(id, name, active, score);
  }
}

// ==========================================
// 2.5. Zero-Copy String Array (Lazy View)
// ==========================================

// Format: [Count (4B)] [Len (1B)][Str] ...
// Demo limitation: Len is 1 byte (max 255 chars)

export class ZeroCopyStringArray {
  private u8: Uint8Array;
  private offsets: Int32Array;

  constructor(buffer: Uint8Array) {
    this.u8 = buffer;
    const count = new DataView(buffer.buffer, buffer.byteOffset).getInt32(0, true);
    this.offsets = new Int32Array(count);
    
    let offset = 4;
    for (let i = 0; i < count; i++) {
        this.offsets[i] = offset;
        const len = buffer[offset];
        offset += 1 + len;
    }
  }

  get(index: number): string {
      const offset = this.offsets[index];
      const len = this.u8[offset];
      const start = offset + 1;
      // In a real implementation, we might cache this string
      return new TextDecoder().decode(this.u8.subarray(start, start + len));
  }
  
  get length(): number {
      return this.offsets.length;
  }
}

export class StandardStringArray {
  static decode(buffer: Uint8Array): string[] {
      const view = new DataView(buffer.buffer, buffer.byteOffset);
      const count = view.getInt32(0, true);
      const result = new Array(count);
      
      let offset = 4;
      for (let i = 0; i < count; i++) {
          const len = buffer[offset];
          const start = offset + 1;
          result[i] = new TextDecoder().decode(buffer.subarray(start, start + len));
          offset += 1 + len;
      }
      return result;
  }
}

// ==========================================
// 3. Shared Memory Coordinator
// ==========================================

export class SharedMemoryLink {
  private worker: Worker;
  private sab: SharedArrayBuffer;
  private sharedInt32: Int32Array;
  private sharedUint8: Uint8Array;
  
  private SIGNAL_INDEX = 0;
  private DATA_SIZE_INDEX = 1;
  private DATA_OFFSET = 16;

  constructor(workerPath: string, size: number = 1024 * 1024) {
    this.sab = new SharedArrayBuffer(size);
    this.sharedInt32 = new Int32Array(this.sab);
    this.sharedUint8 = new Uint8Array(this.sab);
    
    this.worker = new Worker(workerPath);
    this.worker.onerror = (e) => {
      console.error("Worker Error:", e.message, e.filename, e.lineno);
    };
  }

  async init(): Promise<void> {
    return new Promise((resolve, reject) => {
      const handler = (e: MessageEvent) => {
        if (e.data.type === 'ready') {
            this.worker.removeEventListener('message', handler);
            resolve();
        }
      };
      this.worker.addEventListener('message', handler);
      
      this.worker.onerror = (e) => {
          reject(new Error(`Worker failed to start: ${e.message}`));
      };

      this.worker.postMessage({ type: 'init-sab', payload: this.sab });
    });
  }

  sendZeroCopy(data: Uint8Array): Promise<number> {
    return new Promise((resolve) => {
      // 1. Write data to shared memory (Zero allocation if we mapped directly from network)
      // For bench, we mimic "network write" by copying data into SAB once.
      // In real world, fetch() would write directly here.
      this.sharedUint8.set(data, this.DATA_OFFSET);
      
      // 2. Set Size
      Atomics.store(this.sharedInt32, this.DATA_SIZE_INDEX, data.length);
      
      // 3. Set Handler
      const handler = (e: MessageEvent) => {
        if (e.data.type === 'result') {
          this.worker.removeEventListener('message', handler);
          resolve(e.data.sum);
        }
      };
      this.worker.addEventListener('message', handler);
      
      // 4. Notify Worker
      Atomics.store(this.sharedInt32, this.SIGNAL_INDEX, 1);
      Atomics.notify(this.sharedInt32, this.SIGNAL_INDEX);
    });
  }

  sendStandard(data: Uint8Array): Promise<number> {
    return new Promise((resolve) => {
      const handler = (e: MessageEvent) => {
        if (e.data.type === 'result') {
          this.worker.removeEventListener('message', handler);
          resolve(e.data.sum);
        }
      };
      this.worker.addEventListener('message', handler);
      
      // Standard postMessage (Clone)
      this.worker.postMessage({ type: 'standard-msg', payload: data });
    });
  }
  
  terminate() {
    this.worker.terminate();
  }
}

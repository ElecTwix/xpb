// bleeding-edge-worker.ts
// Worker for Zero-Copy benchmarks using SharedArrayBuffer

// State for Shared Memory
let sharedInt32: Int32Array | null = null;
let sharedUint8: Uint8Array | null = null;

// For Stream
let streamSab: SharedArrayBuffer | null = null;
let streamControl: Int32Array | null = null;

const SIGNAL_INDEX = 0;
const DATA_SIZE_INDEX = 1;

self.onmessage = (event: MessageEvent) => {
  const { type, payload } = event.data;

  if (type === 'init-sab') {
    // 1. Receive Shared Memory
    const sab = payload as SharedArrayBuffer;
    sharedInt32 = new Int32Array(sab);
    sharedUint8 = new Uint8Array(sab);
    
    // Notify main thread we are ready
    self.postMessage({ type: 'ready' });
    
    // Enter polling loop
    waitForSignal();
  } else if (type === 'standard-msg') {
    // Standard postMessage benchmark
    const data = payload as Uint8Array;
    let sum = 0;
    for(let i=0; i<data.length; i++) sum += data[i];
    self.postMessage({ type: 'result', sum });
  } else if (type === 'init-stream') {
      streamSab = payload;
      streamControl = new Int32Array(streamSab!, 0, 8);
      consumeStream();
  }
};

function waitForSignal() {
  if (!sharedInt32 || !sharedUint8) return;

  // Poll for signal
  // For bench stability, using Atomics.wait if allowed would be better,
  // but busy-wait/timeout loop works for now.
  const status = Atomics.load(sharedInt32, SIGNAL_INDEX);
    
  if (status === 1) {
      const size = Atomics.load(sharedInt32, DATA_SIZE_INDEX);
      const dataOffset = 16;
      let sum = 0;
      for(let i=0; i<size; i++) {
        sum += sharedUint8[dataOffset + i];
      }
      
      Atomics.store(sharedInt32, SIGNAL_INDEX, 0);
      self.postMessage({ type: 'result', sum });
  }
  
  setTimeout(waitForSignal, 0);
}

function consumeStream() {
    if (!streamControl) return;
    
    const IDX_WRITE = 0;
    const IDX_STATE = 2;
    // const IDX_READ = 1; // We track locally
    
    let readHead = 32; // DATA_OFFSET
    let totalBytes = 0;
    
    // Using Atomics.wait for efficient blocking
    const i32 = streamControl;
    
    while (true) {
        let writeHead = Atomics.load(i32, IDX_WRITE);
        const state = Atomics.load(i32, IDX_STATE);
        
        if (writeHead > readHead) {
            // New data available!
            const len = writeHead - readHead;
            totalBytes += len;
            readHead = writeHead;
            // Mock processing
        }
        
        if (state === 1 && writeHead === readHead) {
            // Done and caught up
            break;
        }
        
        // Wait for update if no new data
        if (writeHead === readHead && state === 0) {
             Atomics.wait(i32, IDX_WRITE, writeHead);
        }
    }
    
    self.postMessage({ type: 'stream-result', bytes: totalBytes });
}
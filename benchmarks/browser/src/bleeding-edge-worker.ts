// bleeding-edge-worker.ts
// Worker for Zero-Copy benchmarks using SharedArrayBuffer

// State for Shared Memory
let sharedInt32: Int32Array | null = null;
let sharedUint8: Uint8Array | null = null;

const SIGNAL_INDEX = 0;
const DATA_SIZE_INDEX = 1;

self.onmessage = (event) => {
  const { type, payload } = event.data;

  if (type === 'init-sab') {
    // 1. Receive Shared Memory
    const sab = payload as SharedArrayBuffer;
    sharedInt32 = new Int32Array(sab);
    sharedUint8 = new Uint8Array(sab);
    
    // Notify main thread we are ready
    self.postMessage({ type: 'ready' });
    
    // Enter polling loop
    setTimeout(waitForSignal, 0);
  } else if (type === 'standard-msg') {
    // Standard postMessage benchmark
    // Payload is the data (copied)
    const data = payload as Uint8Array;
    // Process: Sum bytes
    let sum = 0;
    for(let i=0; i<data.length; i++) sum += data[i];
    self.postMessage({ type: 'result', sum });
  }
};

function waitForSignal() {
  if (!sharedInt32 || !sharedUint8) return;

  // Poll for signal (Busy wait or yield)
  // Atomics.load is safe.
  const status = Atomics.load(sharedInt32, SIGNAL_INDEX);
    
  if (status === 1) {
      // Data is ready
      const size = Atomics.load(sharedInt32, DATA_SIZE_INDEX);
      
      // Process data (Zero Copy!)
      const dataOffset = 16;
      let sum = 0;
      for(let i=0; i<size; i++) {
        sum += sharedUint8[dataOffset + i];
      }
      
      // Reset signal
      Atomics.store(sharedInt32, SIGNAL_INDEX, 0);
      
      // Notify main thread
      self.postMessage({ type: 'result', sum });
  }
  
  // Re-schedule
  // Use setTimeout(..., 0) to yield to event loop (handle messages)
  // Ideally use RequestAnimationFrame or similar if available, but in worker setTimeout is best.
  // For high perf tight loop, we'd use a while loop with occasional yields, 
  // but for stability let's use timeout.
  setTimeout(waitForSignal, 0);
}

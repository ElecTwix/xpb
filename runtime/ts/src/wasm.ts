/**
 * XPB WASM Runtime - Minimal WASM for high-performance varint operations
 * WASM module is only 310 bytes!
 */

// Base64 encoded WASM binary (compiled from xpb.wat)
const WASM_BASE64 = "AGFzbQEAAAABDAJgAX8Bf2ACf38BfwIPAQNlbnYGbWVtb3J5AgABAwUEAAABAQdBBA16aWd6YWdfZW5jb2RlAAANemlnemFnX2RlY29kZQABDWRlY29kZV92YXJpbnQAAg1lbmNvZGVfdmFyaW50AAMKwgEEDQAgAEEBdCAAQR91cwsQACAAQQF2QQAgAEEBcWtzC1UBBH8gACEFQQAhAkEAIQMCQANAIAUtAAAhBCAFQQFqIQUgAiAEQf8AcSADdHIhAiAEQYABcUUNASADQQdqIQMgA0EjSQ0ACwsgASAFIABrNgIAIAILSwEBfyABIQICQANAIABBgAFJBEAgAiAAOgAAIAJBAWohAgwCCyACIABB/wBxQYABcjoAACACQQFqIQIgAEEHdiEADAALCyACIAFrCw==";

// Decode base64 to Uint8Array
function base64ToUint8Array(base64: string): Uint8Array {
  const binaryString = atob(base64);
  const bytes = new Uint8Array(binaryString.length);
  for (let i = 0; i < binaryString.length; i++) {
    bytes[i] = binaryString.charCodeAt(i);
  }
  return bytes;
}

let wasmInstance: WebAssembly.Instance | null = null;
let wasmMemory: WebAssembly.Memory | null = null;
let wasmBuffer: Uint8Array | null = null;

interface WasmExports {
  zigzag_encode: (n: number) => number;
  zigzag_decode: (n: number) => number;
  decode_varint: (offset: number, resultPtr: number) => number;
  encode_varint: (value: number, offset: number) => number;
}

/**
 * Initialize the WASM module
 */
export async function initWasm(): Promise<boolean> {
  if (wasmInstance) return true;
  
  try {
    wasmMemory = new WebAssembly.Memory({ initial: 1, maximum: 16 });
    wasmBuffer = new Uint8Array(wasmMemory.buffer);
    
    const wasmBytes = base64ToUint8Array(WASM_BASE64);
    const module = await WebAssembly.compile(wasmBytes as BufferSource);
    wasmInstance = await WebAssembly.instantiate(module, {
      env: { memory: wasmMemory }
    });
    return true;
  } catch (e) {
    console.warn('XPB WASM init failed, using JS fallback:', e);
    return false;
  }
}

/**
 * Check if WASM is ready
 */
export function isWasmReady(): boolean {
  return wasmInstance !== null;
}

/**
 * Get the WASM buffer for data transfer
 */
export function getWasmBuffer(): Uint8Array | null {
  return wasmBuffer;
}

/**
 * Zigzag encode using WASM
 */
export function wasmZigzagEncode(n: number): number {
  if (!wasmInstance) {
    return (n << 1) ^ (n >> 31); // Fallback
  }
  const exports = wasmInstance.exports as unknown as WasmExports;
  return exports.zigzag_encode(n);
}

/**
 * Zigzag decode using WASM
 */
export function wasmZigzagDecode(n: number): number {
  if (!wasmInstance) {
    return (n >>> 1) ^ -(n & 1); // Fallback
  }
  const exports = wasmInstance.exports as unknown as WasmExports;
  return exports.zigzag_decode(n);
}

/**
 * Decode varint using WASM
 * Returns [value, bytesRead]
 */
export function wasmDecodeVarint(data: Uint8Array, offset: number): [number, number] {
  if (!wasmInstance || !wasmBuffer) {
    throw new Error('WASM not initialized');
  }
  
  // Copy data to WASM memory (starting at offset 0)
  const copyLen = Math.min(data.length - offset, 10);
  wasmBuffer.set(data.subarray(offset, offset + copyLen), 0);
  
  // Result pointer at position 1024 (safe area)
  const resultPtr = 1024;
  
  const exports = wasmInstance.exports as unknown as WasmExports;
  const value = exports.decode_varint(0, resultPtr);
  const bytesRead = new Uint32Array(wasmBuffer.buffer, resultPtr, 1)[0];
  
  return [value, bytesRead];
}

/**
 * Encode varint using WASM
 * Returns bytes written
 */
export function wasmEncodeVarint(value: number, output: Uint8Array, offset: number): number {
  if (!wasmInstance || !wasmBuffer) {
    throw new Error('WASM not initialized');
  }
  
  const exports = wasmInstance.exports as unknown as WasmExports;
  const bytesWritten = exports.encode_varint(value, 0);
  
  // Copy result from WASM memory to output
  output.set(wasmBuffer.subarray(0, bytesWritten), offset);
  
  return bytesWritten;
}

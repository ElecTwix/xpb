/**
 * XPB V2 Worker Script
 * 
 * Handles high-performance parallel decoding using Transferable Objects.
 * Designed to be bundled or loaded as a module worker.
 */

// TextDecoder for UTF-8 string decoding
const textDecoder = new TextDecoder();

class FastDecoder {
  private data: Uint8Array;
  private pos = 0;
  private view: DataView;

  constructor(data: Uint8Array) {
    this.data = data;
    this.view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  }

  readInt32(): number {
    const v = this.view.getInt32(this.pos, true);
    this.pos += 4;
    return v;
  }
  
  readStringLength(): number {
    const len = this.data[this.pos++];
    if (len === 255) {
         const v = this.view.getUint32(this.pos, true);
         this.pos += 4;
         return v;
    }
    return len;
  }
  
  readRawStringBytes(len: number): Uint8Array {
      const bytes = this.data.subarray(this.pos, this.pos + len);
      this.pos += len;
      return bytes;
  }
}

/**
 * Decode Int32Array directly into a Transferable Int32Array (Zero-Copy)
 */
function decodeInt32Array(buffer: Uint8Array): { result: Int32Array, transfer: Transferable[] } {
  const decoder = new FastDecoder(buffer);
  const count = decoder.readInt32();
  
  const result = new Int32Array(count);
  for (let i = 0; i < count; i++) {
    result[i] = decoder.readInt32();
  }
  
  return { result, transfer: [result.buffer] };
}

/**
 * Decode Strings into Flat Arrays (Offsets + Data)
 * Returns a Transferable structure that main thread can wrap.
 */
function decodeStringArray(buffer: Uint8Array): { result: any, transfer: Transferable[] } {
  const decoder = new FastDecoder(buffer);
  const count = decoder.readInt32();
  
  const offsets = new Int32Array(count + 1);
  
  // We allocate a buffer for the data. In a perfect world we'd verify size first.
  // For now, upper bound is input size.
  const data = new Uint8Array(buffer.byteLength); 
  let dataPos = 0;
  
  for (let i = 0; i < count; i++) {
      offsets[i] = dataPos;
      const len = decoder.readStringLength();
      const bytes = decoder.readRawStringBytes(len);
      data.set(bytes, dataPos);
      dataPos += len;
  }
  offsets[count] = dataPos; // End sentinel
  
  // Slice to actual size
  const finalData = data.slice(0, dataPos);
  
  return { 
      result: { offsets, data: finalData }, 
      transfer: [offsets.buffer, finalData.buffer] 
  };
}

// Worker Message Handler
self.onmessage = (event) => {
  const { id, type, buffer } = event.data;
  const data = new Uint8Array(buffer);
  
  try {
    let response;
    
    if (type === 'int32Array') {
        response = decodeInt32Array(data);
    } else if (type === 'stringArray') {
        response = decodeStringArray(data);
    } else {
        throw new Error(`Unknown decode type: ${type}`);
    }
    
    self.postMessage({ id, result: response.result }, { transfer: response.transfer });
    
  } catch (e) {
    self.postMessage({ id, error: (e as Error).message });
  }
};

self.postMessage({ type: 'ready' });

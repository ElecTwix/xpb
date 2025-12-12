
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

// OPTIMIZED: Decode Int32Array directly into a Transferable Int32Array
function decodeInt32ArrayOptimized(buffer: Uint8Array): { result: Int32Array, transfer: Transferable[] } {
  const decoder = new FastDecoder(buffer);
  const count = decoder.readInt32();
  
  // Allocate EXACT size
  const result = new Int32Array(count);
  
  // Fill array
  // Note: DataView is slow. For bulk, we can copy bytes if alignment matches?
  // But XPB is little-endian, and typed arrays use platform endianness (usually little-endian).
  // Fastest: create a DataView on the result buffer and copy?
  // Or just loop.
  
  // Checking for direct memory copy possibility (if alignment is 4-byte and LE)
  // XPB V2 Int32 is 4 bytes.
  // If input buffer is aligned, we might be able to slice?
  // But XPB has a header (count).
  
  // Simple loop is safest for now
  for (let i = 0; i < count; i++) {
    result[i] = decoder.readInt32();
  }
  
  return { result, transfer: [result.buffer] };
}

// OPTIMIZED: Decode Strings into Flat Arrays (Offsets + Data)
// Returns: { offsets: Int32Array, data: Uint8Array }
function decodeStringArrayOptimized(buffer: Uint8Array): { result: any, transfer: Transferable[] } {
  const decoder = new FastDecoder(buffer);
  const count = decoder.readInt32();
  
  const offsets = new Int32Array(count + 1);
  let totalDataSize = 0;
  
  // Pass 1: Calculate total size and offsets
  // We have to scan the input. This is unavoidable if we want a flat buffer.
  // Wait, we can just copy the raw bytes? 
  // XPB string format: [len][bytes]...
  // We want to convert to: [bytesbytesbytes] + [0, 5, 12...]
  
  // Faster approach: Just copy the relevant section of the input buffer?
  // No, because input has Length prefixes interleaved. We need to strip them.
  
  // We will build a single Uint8Array for all string data.
  // We can estimate size or resize.
  // Input size is upper bound.
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
  
  // Slice data to actual size
  const finalData = data.slice(0, dataPos);
  
  return { 
      result: { offsets, data: finalData }, 
      transfer: [offsets.buffer, finalData.buffer] 
  };
}

self.onmessage = (event) => {
  const { id, type, buffer } = event.data;
  const data = new Uint8Array(buffer);
  
  try {
    let response;
    
    if (type === 'int32Array') {
        response = decodeInt32ArrayOptimized(data);
    } else if (type === 'stringArray') {
        response = decodeStringArrayOptimized(data);
    } else {
        throw new Error("Unknown type");
    }
    
    self.postMessage({ id, result: response.result }, response.transfer);
    
  } catch (e) {
    self.postMessage({ id, error: (e as Error).message });
  }
};

self.postMessage({ type: 'ready' });

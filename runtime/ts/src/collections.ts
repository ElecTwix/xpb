/**
 * XPB V2 Optimized Collection Encoders/Decoders
 * 
 * These are JIT-compiled for maximum performance with arrays and maps.
 * Key optimizations:
 * - Pre-allocated slab buffer to avoid reallocations
 * - ASCII fast path for strings
 * - Inlined int32 read/write (faster than DataView for small values)
 * - Buffer.write/toString for Node.js (faster than TextEncoder/TextDecoder)
 */

// Cached encoders/decoders
const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

// Detect Node.js for fast string operations
const isNode = typeof Buffer !== 'undefined';

// ============= SLAB ALLOCATOR =============

export class CollectionSlab {
  public buf: Uint8Array;
  public view: DataView;
  public pos: number = 0;

  constructor(size = 65536) {
    if (isNode) {
      this.buf = Buffer.alloc(size);
    } else {
      this.buf = new Uint8Array(size);
    }
    this.view = new DataView(this.buf.buffer, this.buf.byteOffset, this.buf.byteLength);
  }

  reset(): void {
    this.pos = 0;
  }

  getResult(): Uint8Array {
    return this.buf.subarray(0, this.pos);
  }
}

// Global slab for reuse
const globalSlab = new CollectionSlab(256 * 1024); // 256KB

// ============= STRING ARRAY (Optimized) =============

/**
 * Encode string array using optimized inline code
 */
export function encodeStringArray(arr: string[], slab?: CollectionSlab): Uint8Array {
  const s = slab || globalSlab;
  s.reset();
  
  const buf = s.buf;
  let pos = 0;
  
  // Write count as int32 (inline)
  const count = arr.length;
  buf[pos++] = count;
  buf[pos++] = count >> 8;
  buf[pos++] = count >> 16;
  buf[pos++] = count >> 24;
  
  // Write each string
  if (isNode) {
    // Node.js fast path: Buffer.write is much faster
    const nodeBuf = buf as Buffer;
    for (let i = 0; i < count; i++) {
      const str = arr[i];
      const strLen = str.length;
      
      // ASCII fast path for short strings
      if (strLen < 40) {
        let isAscii = true;
        for (let j = 0; j < strLen; j++) {
          const c = str.charCodeAt(j);
          if (c > 127) { isAscii = false; break; }
          buf[pos + 1 + j] = c;
        }
        if (isAscii) {
          buf[pos] = strLen;
          pos += 1 + strLen;
          continue;
        }
      }
      
      // Fallback to Buffer.write
      const written = nodeBuf.write(str, pos + 1, 'utf8');
      buf[pos] = written;
      pos += 1 + written;
    }
  } else {
    // Browser path: encodeInto
    for (let i = 0; i < count; i++) {
      const str = arr[i];
      const strLen = str.length;
      
      // ASCII fast path
      if (strLen < 40) {
        let isAscii = true;
        for (let j = 0; j < strLen; j++) {
          const c = str.charCodeAt(j);
          if (c > 127) { isAscii = false; break; }
          buf[pos + 1 + j] = c;
        }
        if (isAscii) {
          buf[pos] = strLen;
          pos += 1 + strLen;
          continue;
        }
      }
      
      // Fallback to encodeInto
      const result = textEncoder.encodeInto(str, buf.subarray(pos + 1));
      buf[pos] = result.written!;
      pos += 1 + result.written!;
    }
  }
  
  s.pos = pos;
  return s.getResult();
}

/**
 * Decode string array using optimized inline code
 */
export function decodeStringArray(data: Uint8Array): string[] {
  const buf = data;
  let pos = 0;
  
  // Read count (inline int32)
  const count = (buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24)) >>> 0;
  pos += 4;
  
  const arr = new Array<string>(count);
  
  // Use TextDecoder for all platforms - faster than Buffer.from() + toString()
  // because Buffer.from() creates a copy while TextDecoder works on the view
  for (let i = 0; i < count; i++) {
    const len = buf[pos++];
    arr[i] = textDecoder.decode(buf.subarray(pos, pos + len));
    pos += len;
  }
  
  return arr;
}

// ============= INT32 ARRAY (Optimized) =============

/**
 * Encode int32 array - extremely fast with TypedArray view
 */
export function encodeInt32Array(arr: number[], slab?: CollectionSlab): Uint8Array {
  const s = slab || globalSlab;
  s.reset();
  
  const buf = s.buf;
  const view = s.view;
  let pos = 0;
  
  const count = arr.length;
  
  // Write count
  buf[pos++] = count;
  buf[pos++] = count >> 8;
  buf[pos++] = count >> 16;
  buf[pos++] = count >> 24;
  
  // Write int32s using DataView (handles alignment automatically)
  for (let i = 0; i < count; i++) {
    view.setInt32(pos, arr[i], true);
    pos += 4;
  }
  
  s.pos = pos;
  return s.getResult();
}

/**
 * Decode int32 array - use typed array view for speed
 */
export function decodeInt32Array(data: Uint8Array): number[] {
  const buf = data;
  let pos = 0;
  
  // Read count
  const count = (buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24)) >>> 0;
  pos += 4;
  
  // Create Int32Array view if aligned, else fall back
  const arr = new Array<number>(count);
  const view = new DataView(buf.buffer, buf.byteOffset + pos, count * 4);
  
  for (let i = 0; i < count; i++) {
    arr[i] = view.getInt32(i * 4, true);
  }
  
  return arr;
}

// ============= STRING MAP (Optimized) =============

/**
 * Encode string map using optimized inline code
 */
export function encodeStringMap(map: Map<string, string>, slab?: CollectionSlab): Uint8Array {
  const s = slab || globalSlab;
  s.reset();
  
  const buf = s.buf;
  let pos = 0;
  
  // Write count as int32
  const count = map.size;
  buf[pos++] = count;
  buf[pos++] = count >> 8;
  buf[pos++] = count >> 16;
  buf[pos++] = count >> 24;
  
  // Write each key-value pair
  if (isNode) {
    const nodeBuf = buf as Buffer;
    for (const [key, value] of map) {
      // Encode key
      const kLen = key.length;
      if (kLen < 40) {
        let isAscii = true;
        for (let j = 0; j < kLen; j++) {
          const c = key.charCodeAt(j);
          if (c > 127) { isAscii = false; break; }
          buf[pos + 1 + j] = c;
        }
        if (isAscii) {
          buf[pos] = kLen;
          pos += 1 + kLen;
        } else {
          const written = nodeBuf.write(key, pos + 1, 'utf8');
          buf[pos] = written;
          pos += 1 + written;
        }
      } else {
        const written = nodeBuf.write(key, pos + 1, 'utf8');
        buf[pos] = written;
        pos += 1 + written;
      }
      
      // Encode value
      const vLen = value.length;
      if (vLen < 40) {
        let isAscii = true;
        for (let j = 0; j < vLen; j++) {
          const c = value.charCodeAt(j);
          if (c > 127) { isAscii = false; break; }
          buf[pos + 1 + j] = c;
        }
        if (isAscii) {
          buf[pos] = vLen;
          pos += 1 + vLen;
        } else {
          const written = nodeBuf.write(value, pos + 1, 'utf8');
          buf[pos] = written;
          pos += 1 + written;
        }
      } else {
        const written = nodeBuf.write(value, pos + 1, 'utf8');
        buf[pos] = written;
        pos += 1 + written;
      }
    }
  } else {
    // Browser path
    for (const [key, value] of map) {
      // Encode key - ASCII fast path
      const kLen = key.length;
      if (kLen < 40) {
        let isAscii = true;
        for (let j = 0; j < kLen; j++) {
          const c = key.charCodeAt(j);
          if (c > 127) { isAscii = false; break; }
          buf[pos + 1 + j] = c;
        }
        if (isAscii) {
          buf[pos] = kLen;
          pos += 1 + kLen;
        } else {
          const result = textEncoder.encodeInto(key, buf.subarray(pos + 1));
          buf[pos] = result.written!;
          pos += 1 + result.written!;
        }
      } else {
        const result = textEncoder.encodeInto(key, buf.subarray(pos + 1));
        buf[pos] = result.written!;
        pos += 1 + result.written!;
      }
      
      // Encode value
      const vLen = value.length;
      if (vLen < 40) {
        let isAscii = true;
        for (let j = 0; j < vLen; j++) {
          const c = value.charCodeAt(j);
          if (c > 127) { isAscii = false; break; }
          buf[pos + 1 + j] = c;
        }
        if (isAscii) {
          buf[pos] = vLen;
          pos += 1 + vLen;
        } else {
          const result = textEncoder.encodeInto(value, buf.subarray(pos + 1));
          buf[pos] = result.written!;
          pos += 1 + result.written!;
        }
      } else {
        const result = textEncoder.encodeInto(value, buf.subarray(pos + 1));
        buf[pos] = result.written!;
        pos += 1 + result.written!;
      }
    }
  }
  
  s.pos = pos;
  return s.getResult();
}

/**
 * Decode string map using optimized inline code
 */
export function decodeStringMap(data: Uint8Array): Map<string, string> {
  const buf = data;
  let pos = 0;
  
  // Read count
  const count = (buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24)) >>> 0;
  pos += 4;
  
  const map = new Map<string, string>();
  
  // Use TextDecoder for all platforms - faster than Buffer.from() + toString()
  for (let i = 0; i < count; i++) {
    // Decode key
    const kLen = buf[pos++];
    const key = textDecoder.decode(buf.subarray(pos, pos + kLen));
    pos += kLen;
    
    // Decode value
    const vLen = buf[pos++];
    const value = textDecoder.decode(buf.subarray(pos, pos + vLen));
    pos += vLen;
    
    map.set(key, value);
  }
  
  return map;
}

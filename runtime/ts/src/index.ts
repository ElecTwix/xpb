/**
 * XPB V2 TypeScript Runtime
 * 
 * V2 uses:
 * - Struct mode (no tags, fields in declaration order)
 * - Fixed-width integers (4/8 bytes, little-endian, two's complement)
 * - Compact length encoding (1 byte if < 255, else 0xFF + 4 bytes)
 */

// Compact length constants
export const CompactLengthThreshold = 254;
export const CompactLengthMarker = 0xFF;

/**
 * MaxDecodeDepth caps the recursion depth for nested message decoding,
 * preventing stack exhaustion from adversarial deeply-nested payloads.
 * Generated decodeAt(depth) helpers compare against this constant.
 */
export const MaxDecodeDepth = 64;

export class MaxDecodeDepthExceededError extends Error {
  constructor() {
    super('xpb: max decode depth exceeded');
    this.name = 'MaxDecodeDepthExceededError';
  }
}

// Wire types for future protocol extensions
export enum WireType {
  Varint = 0,
  Fixed64 = 1,
  LengthDelimited = 2,
  Fixed32 = 5,
}

// Zigzag encoding for signed integers (for future varint support)
export function zigzagEncode32(n: number): number {
  return (n << 1) ^ (n >> 31);
}

export function zigzagDecode32(n: number): number {
  return (n >> 1) ^ -(n & 1);
}

export function zigzagEncode64(n: bigint): bigint {
  return (n << 1n) ^ (n >> 63n);
}

export function zigzagDecode64(n: bigint): bigint {
  return (n >> 1n) ^ -(n & 1n);
}

// Fixed sizes
export const SizeBool = 1;
export const SizeInt32 = 4;
export const SizeInt64 = 8;
export const SizeUint32 = 4;
export const SizeUint64 = 8;
export const SizeFloat32 = 4;
export const SizeFloat64 = 8;

// Cached encoder/decoder for strings
const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

// ============= Base64 Utilities (Native Only) =============

export function toBase64(data: Uint8Array): string {
  // @ts-ignore - Requires 2025+ Browser / Node.js
  return data.toBase64();
}

export function fromBase64(base64: string): Uint8Array {
  // @ts-ignore - Requires 2025+ Browser / Node.js
  return Uint8Array.fromBase64(base64);
}

/**
 * V2 Encoder - tagless, fixed-width, compact lengths.
 */
export class Encoder {
  private buf: Uint8Array;
  private view: DataView;
  private pos = 0;

  constructor(initialSize = 256) {
    this.buf = new Uint8Array(initialSize);
    this.view = new DataView(this.buf.buffer);
  }

  private ensureCapacity(needed: number): void {
    if (this.pos + needed > this.buf.length) {
      const newSize = Math.max(this.buf.length * 2, this.pos + needed);
      
      // Optimization: Use transfer() if available (zero-copy resize)
      if (typeof (this.buf.buffer as any).transfer === 'function') {
        this.buf = new Uint8Array((this.buf.buffer as any).transfer(newSize));
        this.view = new DataView(this.buf.buffer);
      } else {
        const newBuf = new Uint8Array(newSize);
        newBuf.set(this.buf);
        this.buf = newBuf;
        this.view = new DataView(this.buf.buffer);
      }
    }
  }

  finish(): Uint8Array {
    return this.buf.subarray(0, this.pos);
  }

  reset(): void {
    this.pos = 0;
  }

  /** Write bool as 1 byte */
  writeBool(v: boolean): void {
    this.ensureCapacity(1);
    this.buf[this.pos++] = v ? 1 : 0;
  }

  /** Write int32 as 4 bytes (little-endian, two's complement) */
  writeInt32(v: number): void {
    this.ensureCapacity(4);
    this.view.setInt32(this.pos, v, true);
    this.pos += 4;
  }

  /** Write int64 as 8 bytes (little-endian, two's complement) */
  writeInt64(v: bigint): void {
    this.ensureCapacity(8);
    this.view.setBigInt64(this.pos, v, true);
    this.pos += 8;
  }

  /** Write uint32 as 4 bytes (little-endian) */
  writeUint32(v: number): void {
    this.ensureCapacity(4);
    this.view.setUint32(this.pos, v, true);
    this.pos += 4;
  }

  /** Write uint64 as 8 bytes (little-endian) */
  writeUint64(v: bigint): void {
    this.ensureCapacity(8);
    this.view.setBigUint64(this.pos, v, true);
    this.pos += 8;
  }

  /** Write float32 as 4 bytes */
  writeFloat32(v: number): void {
    this.ensureCapacity(4);
    this.view.setFloat32(this.pos, v, true);
    this.pos += 4;
  }

  /** Write float64 as 8 bytes */
  writeFloat64(v: number): void {
    this.ensureCapacity(8);
    this.view.setFloat64(this.pos, v, true);
    this.pos += 8;
  }

  /** Write compact length prefix */
  private writeCompactLength(length: number): void {
    if (length <= CompactLengthThreshold) {
      this.ensureCapacity(1);
      this.buf[this.pos++] = length;
    } else {
      this.ensureCapacity(5);
      this.buf[this.pos++] = CompactLengthMarker;
      this.view.setUint32(this.pos, length, true);
      this.pos += 4;
    }
  }

  /** Write string with compact length prefix */
  writeString(v: string): void {
    const len = v.length;
    // Optimization: For short strings (likely ASCII), try manual encoding
    // This avoids the heavy overhead of TextEncoder.encode()
    if (len < 64) {
      // Optimistically assume 1 byte per char + 1 byte length
      this.ensureCapacity(len + 1);
      
      let isAscii = true;
      const startPos = this.pos;
      
      // Write length (assuming < 128 for now, but space is reserved)
      // If we fail ASCII check, we rewind.
      this.buf[this.pos++] = len;

      for (let i = 0; i < len; i++) {
        const c = v.charCodeAt(i);
        if (c > 127) {
          isAscii = false;
          break;
        }
        this.buf[this.pos++] = c;
      }

      if (isAscii) {
        return;
      }
      
      // Rewind and fallback to TextEncoder
      this.pos = startPos;
    }

    const bytes = textEncoder.encode(v);
    this.writeCompactLength(bytes.length);
    this.ensureCapacity(bytes.length);
    this.buf.set(bytes, this.pos);
    this.pos += bytes.length;
  }

  /** 
   * Write Base64 string directly as bytes (Zero-Allocation).
   * Uses setFromBase64 to write directly into the buffer, handling the length prefix efficiently.
   */
  writeBase64AsBytes(v: string): void {
    // Estimate max size (Base64 is 4 chars -> 3 bytes)
    const maxLen = Math.ceil(v.length * 0.75);
    
    // Reserve space for Max Header (5 bytes) + Body
    this.ensureCapacity(5 + maxLen);

    // Write body at offset + 5 (leaving room for max header)
    // @ts-ignore - Check for new browser API (2025)
    if (this.buf.setFromBase64) {
      const dest = this.buf.subarray(this.pos + 5);
      // @ts-ignore
      const { written } = dest.setFromBase64(v);
      
      if (written <= CompactLengthThreshold) {
         // Short form: 1 byte header.
         // Shift data back 4 bytes (from +5 to +1) to close the gap
         this.buf.copyWithin(this.pos + 1, this.pos + 5, this.pos + 5 + written);
         this.buf[this.pos] = written;
         this.pos += 1 + written;
      } else {
         // Long form: 5 byte header.
         // Data is already in correct place (pos + 5)
         this.buf[this.pos] = CompactLengthMarker;
         this.view.setUint32(this.pos + 1, written, true);
         this.pos += 5 + written;
      }
    } else {
      // Fallback: Decode -> Copy
      const bytes = fromBase64(v);
      this.writeBytes(bytes);
    }
  }

  /** Write bytes with compact length prefix */
  writeBytes(v: Uint8Array): void {
    this.writeCompactLength(v.length);
    this.ensureCapacity(v.length);
    this.buf.set(v, this.pos);
    this.pos += v.length;
  }

  /** Write nested message (already encoded) with compact length prefix */
  writeMessage(data: Uint8Array): void {
    this.writeBytes(data);
  }

  /** Write array of int32 - format: count (int32) + elements */
  writeArrayInt32(arr: number[]): void {
    this.writeInt32(arr.length);
    for (const v of arr) {
      this.writeInt32(v);
    }
  }

  /** Write array of int64 - format: count (int32) + elements */
  writeArrayInt64(arr: bigint[]): void {
    this.writeInt32(arr.length);
    for (const v of arr) {
      this.writeInt64(v);
    }
  }

  /** Write array of uint32 - format: count (int32) + elements */
  writeArrayUint32(arr: number[]): void {
    this.writeInt32(arr.length);
    for (const v of arr) {
      this.writeUint32(v);
    }
  }

  /** Write array of uint64 - format: count (int32) + elements */
  writeArrayUint64(arr: bigint[]): void {
    this.writeInt32(arr.length);
    for (const v of arr) {
      this.writeUint64(v);
    }
  }

  /** Write array of float32 - format: count (int32) + elements */
  writeArrayFloat32(arr: number[]): void {
    this.writeInt32(arr.length);
    for (const v of arr) {
      this.writeFloat32(v);
    }
  }

  /** Write array of float64 - format: count (int32) + elements */
  writeArrayFloat64(arr: number[]): void {
    this.writeInt32(arr.length);
    for (const v of arr) {
      this.writeFloat64(v);
    }
  }

  /** Write array of bool - format: count (int32) + elements */
  writeArrayBool(arr: boolean[]): void {
    this.writeInt32(arr.length);
    for (const v of arr) {
      this.writeBool(v);
    }
  }

  /** Write array of string - format: count (int32) + elements */
  writeArrayString(arr: string[]): void {
    this.writeInt32(arr.length);
    for (const v of arr) {
      this.writeString(v);
    }
  }
}

/**
 * V2 Decoder - tagless, fixed-width, compact lengths.
 */
export class Decoder {
  private data: Uint8Array;
  private view: DataView;
  private pos = 0;

  constructor(data: Uint8Array) {
    this.data = data;
    this.view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  }

  eof(): boolean {
    return this.pos >= this.data.length;
  }

  remaining(): number {
    return this.data.length - this.pos;
  }

  /** Read bool from 1 byte */
  readBool(): boolean {
    if (this.pos >= this.data.length) {
      throw new Error('xpb: unexpected EOF reading bool');
    }
    return this.data[this.pos++] !== 0;
  }

  /** Read int32 from 4 bytes (little-endian, two's complement) */
  readInt32(): number {
    if (this.pos + 4 > this.data.length) {
      throw new Error('xpb: unexpected EOF reading int32');
    }
    const v = this.view.getInt32(this.pos, true);
    this.pos += 4;
    return v;
  }

  /** Read int64 from 8 bytes (little-endian, two's complement) */
  readInt64(): bigint {
    if (this.pos + 8 > this.data.length) {
      throw new Error('xpb: unexpected EOF reading int64');
    }
    const v = this.view.getBigInt64(this.pos, true);
    this.pos += 8;
    return v;
  }

  /** Read uint32 from 4 bytes (little-endian) */
  readUint32(): number {
    if (this.pos + 4 > this.data.length) {
      throw new Error('xpb: unexpected EOF reading uint32');
    }
    const v = this.view.getUint32(this.pos, true);
    this.pos += 4;
    return v;
  }

  /** Read uint64 from 8 bytes (little-endian) */
  readUint64(): bigint {
    if (this.pos + 8 > this.data.length) {
      throw new Error('xpb: unexpected EOF reading uint64');
    }
    const v = this.view.getBigUint64(this.pos, true);
    this.pos += 8;
    return v;
  }

  /** Read float32 from 4 bytes */
  readFloat32(): number {
    if (this.pos + 4 > this.data.length) {
      throw new Error('xpb: unexpected EOF reading float32');
    }
    const v = this.view.getFloat32(this.pos, true);
    this.pos += 4;
    return v;
  }

  /** Read float64 from 8 bytes */
  readFloat64(): number {
    if (this.pos + 8 > this.data.length) {
      throw new Error('xpb: unexpected EOF reading float64');
    }
    const v = this.view.getFloat64(this.pos, true);
    this.pos += 8;
    return v;
  }

  /** Read compact length prefix */
  private readCompactLength(): number {
    if (this.pos >= this.data.length) {
      throw new Error('xpb: unexpected EOF reading length');
    }
    const first = this.data[this.pos++];
    if (first !== CompactLengthMarker) {
      return first;
    }
    // Read 4-byte length
    if (this.pos + 4 > this.data.length) {
      throw new Error('xpb: unexpected EOF reading extended length');
    }
    const length = this.view.getUint32(this.pos, true);
    this.pos += 4;
    return length;
  }

  /** Read string with compact length prefix */
  readString(): string {
    const length = this.readCompactLength();
    if (this.pos + length > this.data.length) {
      throw new Error('xpb: unexpected EOF reading string');
    }
    
    // Optimization: For short strings, manual decode is faster than TextDecoder
    if (length < 20) {
      // Check for ASCII
      let isAscii = true;
      for (let i = 0; i < length; i++) {
        if (this.data[this.pos + i] > 127) {
          isAscii = false;
          break;
        }
      }
      
      if (isAscii) {
        // Use spread/apply for fast string creation
        const str = String.fromCharCode.apply(null, this.data.subarray(this.pos, this.pos + length) as any);
        this.pos += length;
        return str;
      }
    }

    const bytes = this.data.subarray(this.pos, this.pos + length);
    this.pos += length;
    return textDecoder.decode(bytes);
  }

  /** Read bytes directly as Base64 string */
  readBytesAsBase64(): string {
    const length = this.readCompactLength();
    if (this.pos + length > this.data.length) {
      throw new Error('xpb: unexpected EOF reading bytes for base64');
    }
    
    // Check for native support (Proposal 4)
    // @ts-ignore
    if (typeof Uint8Array.prototype.toBase64 === 'function') {
      const bytes = this.data.subarray(this.pos, this.pos + length);
      this.pos += length;
      // @ts-ignore
      return bytes.toBase64();
    }
    
    // Node.js Buffer optimization
    if (typeof Buffer !== 'undefined' && Buffer.isBuffer(this.data)) {
      const b64 = this.data.toString('base64', this.pos, this.pos + length);
      this.pos += length;
      return b64;
    }
    
    // Fallback
    const bytes = this.data.subarray(this.pos, this.pos + length);
    this.pos += length;
    if (typeof Buffer !== 'undefined') {
       return Buffer.from(bytes).toString('base64');
    }
    
    // Browser fallback (btoa)
    let binary = '';
    const end = bytes.length;
    const CHUNK = 0x8000;
    for (let i = 0; i < end; i += CHUNK) {
      binary += String.fromCharCode.apply(null, bytes.subarray(i, Math.min(i + CHUNK, end)) as any);
    }
    return btoa(binary);
  }

  /** Read bytes with compact length prefix */
  readBytes(): Uint8Array {
    const length = this.readCompactLength();
    if (this.pos + length > this.data.length) {
      throw new Error('xpb: unexpected EOF reading bytes');
    }
    const bytes = this.data.slice(this.pos, this.pos + length);
    this.pos += length;
    return bytes;
  }

  /** Read nested message bytes */
  readMessageBytes(): Uint8Array {
    return this.readBytes();
  }

  /** Skip n bytes */
  skip(n: number): void {
    if (this.pos + n > this.data.length) {
      throw new Error('xpb: unexpected EOF during skip');
    }
    this.pos += n;
  }

  /**
   * Read a 4-byte signed array length used by repeated and map fields,
   * validating it before the caller allocates a backing array. Rejects
   * negative counts and counts that cannot possibly fit in the remaining
   * buffer (each element occupies at least elementMinBytes on the wire).
   * Pass 1 when elements are variable-length (string, bytes, message).
   * Pass 0 to skip the upper-bound check (not recommended for untrusted
   * input).
   */
  readArrayCount(elementMinBytes: number): number {
    const n = this.readInt32();
    if (n < 0) {
      throw new Error(`xpb: negative array count: ${n}`);
    }
    if (elementMinBytes > 0) {
      const max = Math.floor((this.data.length - this.pos) / elementMinBytes);
      if (n > max) {
        throw new Error(`xpb: array count ${n} exceeds buffer-bounded max ${max}`);
      }
    }
    return n;
  }

  /** Read array of int32 - format: count (int32) + elements */
  readArrayInt32(): number[] {
    const count = this.readArrayCount(4);
    const arr: number[] = new Array(count);
    for (let i = 0; i < count; i++) {
      arr[i] = this.readInt32();
    }
    return arr;
  }

  /** Read array of int64 - format: count (int32) + elements */
  readArrayInt64(): bigint[] {
    const count = this.readArrayCount(8);
    const arr: bigint[] = new Array(count);
    for (let i = 0; i < count; i++) {
      arr[i] = this.readInt64();
    }
    return arr;
  }

  /** Read array of uint32 - format: count (int32) + elements */
  readArrayUint32(): number[] {
    const count = this.readArrayCount(4);
    const arr: number[] = new Array(count);
    for (let i = 0; i < count; i++) {
      arr[i] = this.readUint32();
    }
    return arr;
  }

  /** Read array of uint64 - format: count (int32) + elements */
  readArrayUint64(): bigint[] {
    const count = this.readArrayCount(8);
    const arr: bigint[] = new Array(count);
    for (let i = 0; i < count; i++) {
      arr[i] = this.readUint64();
    }
    return arr;
  }

  /** Read array of float32 - format: count (int32) + elements */
  readArrayFloat32(): number[] {
    const count = this.readArrayCount(4);
    const arr: number[] = new Array(count);
    for (let i = 0; i < count; i++) {
      arr[i] = this.readFloat32();
    }
    return arr;
  }

  /** Read array of float64 - format: count (int32) + elements */
  readArrayFloat64(): number[] {
    const count = this.readArrayCount(8);
    const arr: number[] = new Array(count);
    for (let i = 0; i < count; i++) {
      arr[i] = this.readFloat64();
    }
    return arr;
  }

  /** Read array of bool - format: count (int32) + elements */
  readArrayBool(): boolean[] {
    const count = this.readArrayCount(1);
    const arr: boolean[] = new Array(count);
    for (let i = 0; i < count; i++) {
      arr[i] = this.readBool();
    }
    return arr;
  }

  /** Read array of string - format: count (int32) + elements */
  readArrayString(): string[] {
    const count = this.readArrayCount(1);
    const arr: string[] = new Array(count);
    for (let i = 0; i < count; i++) {
      arr[i] = this.readString();
    }
    return arr;
  }
}

export * from './worker-pool';
export * from './view';


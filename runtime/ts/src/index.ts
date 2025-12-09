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
      const newBuf = new Uint8Array(newSize);
      newBuf.set(this.buf);
      this.buf = newBuf;
      this.view = new DataView(this.buf.buffer);
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
      let result = "";
      let isAscii = true;
      // Peek to see if we can use fast path
      for (let i = 0; i < length; i++) {
        const b = this.data[this.pos + i];
        if (b > 127) {
          isAscii = false;
          break;
        }
        result += String.fromCharCode(b);
      }
      
      if (isAscii) {
        this.pos += length;
        return result;
      }
    }

    const bytes = this.data.subarray(this.pos, this.pos + length);
    this.pos += length;
    return textDecoder.decode(bytes);
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
}

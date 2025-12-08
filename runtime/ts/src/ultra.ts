/**
 * XPB Ultra-Speed Runtime
 * 
 * TRADEOFFS:
 * - Uses fixed-size numbers (4 bytes) instead of varints
 * - Skips bounds checking for speed
 * - Slightly larger encoded size
 * - Maximum raw speed, suitable for trusted data
 * 
 * WARNING: Only use with trusted data! No bounds checking.
 */

import { Buffer } from 'node:buffer';

// Wire types (simplified for speed)
const WIRE_VARINT = 0;
const WIRE_FIXED32 = 5;
const WIRE_FIXED64 = 1;
const WIRE_BYTES = 2;

// Reusable encoder pool
const encoderPool: UltraEncoder[] = [];
const MAX_POOL_SIZE = 8;

/**
 * Get an encoder from the pool or create new one
 */
export function getEncoder(size = 256): UltraEncoder {
  const enc = encoderPool.pop();
  if (enc) {
    enc.reset();
    return enc;
  }
  return new UltraEncoder(size);
}

/**
 * Return encoder to pool for reuse
 */
export function releaseEncoder(enc: UltraEncoder): void {
  if (encoderPool.length < MAX_POOL_SIZE) {
    encoderPool.push(enc);
  }
}

/**
 * Ultra-Speed Encoder
 * - No bounds checking
 * - Fixed-size numbers
 * - Pre-allocated buffer
 */
export class UltraEncoder {
  public buf: Buffer;
  public pos = 0;

  constructor(size = 256) {
    this.buf = Buffer.allocUnsafe(size);
  }

  // Ensure capacity - call manually before large writes
  grow(needed: number): void {
    if (this.pos + needed > this.buf.length) {
      const newBuf = Buffer.allocUnsafe(Math.max(this.buf.length * 2, this.pos + needed));
      this.buf.copy(newBuf);
      this.buf = newBuf;
    }
  }

  finish(): Buffer {
    return this.buf.subarray(0, this.pos);
  }

  reset(): void {
    this.pos = 0;
  }

  // Ultra-fast tag write (inline, no function call overhead)
  writeTag(fieldNum: number, wireType: number): void {
    this.buf[this.pos++] = (fieldNum << 3) | wireType;
  }

  // Fixed 1-byte bool (fastest possible)
  writeBool(fieldNum: number, v: boolean): void {
    this.buf[this.pos++] = (fieldNum << 3) | WIRE_VARINT;
    this.buf[this.pos++] = v ? 1 : 0;
  }

  // Fixed 4-byte int32 (no varint - faster but larger)
  writeInt32Fixed(fieldNum: number, v: number): void {
    this.buf[this.pos++] = (fieldNum << 3) | WIRE_FIXED32;
    this.buf.writeInt32LE(v, this.pos);
    this.pos += 4;
  }

  // Standard varint int32 (for compatibility)
  writeInt32(fieldNum: number, v: number): void {
    this.buf[this.pos++] = (fieldNum << 3) | WIRE_VARINT;
    // Zigzag encode
    v = (v << 1) ^ (v >> 31);
    while (v >= 0x80) {
      this.buf[this.pos++] = (v & 0x7f) | 0x80;
      v >>>= 7;
    }
    this.buf[this.pos++] = v;
  }

  // Fixed 4-byte uint32
  writeUint32Fixed(fieldNum: number, v: number): void {
    this.buf[this.pos++] = (fieldNum << 3) | WIRE_FIXED32;
    this.buf.writeUInt32LE(v, this.pos);
    this.pos += 4;
  }

  // Fixed 8-byte float64
  writeFloat64(fieldNum: number, v: number): void {
    this.buf[this.pos++] = (fieldNum << 3) | WIRE_FIXED64;
    this.buf.writeDoubleLE(v, this.pos);
    this.pos += 8;
  }

  // Fixed 4-byte float32
  writeFloat32(fieldNum: number, v: number): void {
    this.buf[this.pos++] = (fieldNum << 3) | WIRE_FIXED32;
    this.buf.writeFloatLE(v, this.pos);
    this.pos += 4;
  }

  // Ultra-fast string write
  writeString(fieldNum: number, v: string): void {
    this.buf[this.pos++] = (fieldNum << 3) | WIRE_BYTES;
    const len = Buffer.byteLength(v);
    // Inline varint for length (most strings < 128 bytes)
    if (len < 128) {
      this.buf[this.pos++] = len;
    } else {
      // Rare case: longer string
      let l = len;
      while (l >= 0x80) {
        this.buf[this.pos++] = (l & 0x7f) | 0x80;
        l >>>= 7;
      }
      this.buf[this.pos++] = l;
    }
    this.buf.write(v, this.pos, len, 'utf8');
    this.pos += len;
  }

  // Raw bytes write
  writeBytes(fieldNum: number, v: Uint8Array): void {
    this.buf[this.pos++] = (fieldNum << 3) | WIRE_BYTES;
    const len = v.length;
    if (len < 128) {
      this.buf[this.pos++] = len;
    } else {
      let l = len;
      while (l >= 0x80) {
        this.buf[this.pos++] = (l & 0x7f) | 0x80;
        l >>>= 7;
      }
      this.buf[this.pos++] = l;
    }
    this.buf.set(v, this.pos);
    this.pos += len;
  }
}

/**
 * Ultra-Speed Decoder
 * - No bounds checking
 * - Direct buffer access
 * - Inline varint decoding
 */
export class UltraDecoder {
  public buf: Buffer;
  public pos = 0;
  public end: number;

  constructor(data: Uint8Array) {
    this.buf = Buffer.isBuffer(data) ? data : Buffer.from(data);
    this.end = this.buf.length;
  }

  // Check if more data available
  hasMore(): boolean {
    return this.pos < this.end;
  }

  // Read tag - returns [fieldNum, wireType]
  readTag(): number {
    return this.buf[this.pos++];
  }

  // Extract field number from tag
  static fieldNum(tag: number): number {
    return tag >>> 3;
  }

  // Extract wire type from tag
  static wireType(tag: number): number {
    return tag & 0x7;
  }

  readBool(): boolean {
    return this.buf[this.pos++] !== 0;
  }

  // Fixed 4-byte int32 read
  readInt32Fixed(): number {
    const v = this.buf.readInt32LE(this.pos);
    this.pos += 4;
    return v;
  }

  // Varint int32 read
  readInt32(): number {
    let v = 0;
    let shift = 0;
    let b: number;
    do {
      b = this.buf[this.pos++];
      v |= (b & 0x7f) << shift;
      shift += 7;
    } while (b >= 0x80);
    // Zigzag decode
    return (v >>> 1) ^ -(v & 1);
  }

  // Fixed uint32 read
  readUint32Fixed(): number {
    const v = this.buf.readUInt32LE(this.pos);
    this.pos += 4;
    return v;
  }

  // Varint read (raw)
  readVarint(): number {
    let v = 0;
    let shift = 0;
    let b: number;
    do {
      b = this.buf[this.pos++];
      v |= (b & 0x7f) << shift;
      shift += 7;
    } while (b >= 0x80);
    return v >>> 0;
  }

  readFloat64(): number {
    const v = this.buf.readDoubleLE(this.pos);
    this.pos += 8;
    return v;
  }

  readFloat32(): number {
    const v = this.buf.readFloatLE(this.pos);
    this.pos += 4;
    return v;
  }

  // Ultra-fast string read using Buffer.toString
  readString(): string {
    const len = this.readVarint();
    const s = this.buf.toString('utf8', this.pos, this.pos + len);
    this.pos += len;
    return s;
  }

  readBytes(): Buffer {
    const len = this.readVarint();
    const b = this.buf.subarray(this.pos, this.pos + len);
    this.pos += len;
    return b;
  }

  // Skip field based on wire type (for unknown fields)
  skip(wireType: number): void {
    switch (wireType) {
      case WIRE_VARINT:
        while (this.buf[this.pos++] >= 0x80);
        break;
      case WIRE_FIXED32:
        this.pos += 4;
        break;
      case WIRE_FIXED64:
        this.pos += 8;
        break;
      case WIRE_BYTES:
        this.pos += this.readVarint();
        break;
    }
  }
}

// Export wire type constants
export { WIRE_VARINT, WIRE_FIXED32, WIRE_FIXED64, WIRE_BYTES };

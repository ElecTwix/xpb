/**
 * XPB Node.js/Bun Optimized Runtime
 * Uses native Buffer for maximum performance
 */

import { Buffer } from 'node:buffer';

export const WireType = {
  Varint: 0,
  Fixed64: 1,
  LengthDelimited: 2,
  Fixed32: 5,
} as const;

export type WireType = (typeof WireType)[keyof typeof WireType];

export function zigzagEncode32(n: number): number {
  return (n << 1) ^ (n >> 31);
}

export function zigzagDecode32(n: number): number {
  return (n >>> 1) ^ -(n & 1);
}

/**
 * Node.js/Bun Optimized Encoder using native Buffer
 */
export class Encoder {
  private buf: Buffer;
  private pos = 0;

  constructor(initialSize = 256) {
    // allocUnsafe skips zero-initialization = faster
    this.buf = Buffer.allocUnsafe(initialSize);
  }

  private ensureCapacity(needed: number): void {
    if (this.pos + needed > this.buf.length) {
      const newSize = Math.max(this.buf.length * 2, this.pos + needed);
      const newBuf = Buffer.allocUnsafe(newSize);
      this.buf.copy(newBuf, 0, 0, this.pos);
      this.buf = newBuf;
    }
  }

  finish(): Uint8Array {
    // Return a view, not a copy
    return this.buf.subarray(0, this.pos);
  }

  reset(): void {
    this.pos = 0;
  }

  // Fast varint using native Buffer
  private writeVarint32(v: number): void {
    this.ensureCapacity(5);
    v = v >>> 0;
    while (v >= 0x80) {
      this.buf[this.pos++] = (v & 0x7f) | 0x80;
      v >>>= 7;
    }
    this.buf[this.pos++] = v;
  }

  writeBool(v: boolean): void {
    this.ensureCapacity(1);
    this.buf[this.pos++] = v ? 1 : 0;
  }

  writeInt32(v: number): void {
    this.ensureCapacity(4);
    this.buf.writeInt32LE(v, this.pos);
    this.pos += 4;
  }

  writeInt64(v: bigint): void {
    this.ensureCapacity(8);
    this.buf.writeBigInt64LE(v, this.pos);
    this.pos += 8;
  }

  writeUint32(v: number): void {
    this.ensureCapacity(4);
    this.buf.writeUInt32LE(v, this.pos);
    this.pos += 4;
  }

  writeUint64(v: bigint): void {
    this.ensureCapacity(8);
    this.buf.writeBigUInt64LE(v, this.pos);
    this.pos += 8;
  }

  writeFloat32(v: number): void {
    this.ensureCapacity(4);
    this.buf.writeFloatLE(v, this.pos);
    this.pos += 4;
  }

  writeFloat64(v: number): void {
    this.ensureCapacity(8);
    this.buf.writeDoubleLE(v, this.pos);
    this.pos += 8;
  }

  private writeCompactLength(length: number): void {
    if (length < 255) {
      this.ensureCapacity(1);
      this.buf[this.pos++] = length;
    } else {
      this.ensureCapacity(5);
      this.buf[this.pos++] = 0xFF;
      this.buf.writeUInt32LE(length, this.pos);
      this.pos += 4;
    }
  }

  writeString(v: string): void {
    const byteLen = Buffer.byteLength(v, 'utf8');
    this.writeCompactLength(byteLen);
    this.ensureCapacity(byteLen);
    this.buf.write(v, this.pos, byteLen, 'utf8');
    this.pos += byteLen;
  }

  writeBytes(v: Uint8Array): void {
    this.writeCompactLength(v.length);
    this.ensureCapacity(v.length);
    if (Buffer.isBuffer(v)) {
      v.copy(this.buf, this.pos);
    } else {
      this.buf.set(v, this.pos);
    }
    this.pos += v.length;
  }

  writeMessage(data: Uint8Array): void {
    this.writeBytes(data);
  }
}

/**
 * Node.js/Bun Optimized Decoder using native Buffer
 */
export class Decoder {
  private buf: Buffer;
  private pos = 0;

  constructor(data: Uint8Array) {
    this.buf = Buffer.isBuffer(data) ? data : Buffer.from(data);
  }

  eof(): boolean {
    return this.pos >= this.buf.length;
  }

  readBool(): boolean {
    return this.buf[this.pos++] !== 0;
  }

  readInt32(): number {
    const v = this.buf.readInt32LE(this.pos);
    this.pos += 4;
    return v;
  }

  readInt64(): bigint {
    const v = this.buf.readBigInt64LE(this.pos);
    this.pos += 8;
    return v;
  }

  readUint32(): number {
    const v = this.buf.readUInt32LE(this.pos);
    this.pos += 4;
    return v;
  }

  readUint64(): bigint {
    const v = this.buf.readBigUInt64LE(this.pos);
    this.pos += 8;
    return v;
  }

  readFloat32(): number {
    const v = this.buf.readFloatLE(this.pos);
    this.pos += 4;
    return v;
  }

  readFloat64(): number {
    const v = this.buf.readDoubleLE(this.pos);
    this.pos += 8;
    return v;
  }

  private readCompactLength(): number {
    const first = this.buf[this.pos++];
    if (first === 0xFF) {
      const len = this.buf.readUInt32LE(this.pos);
      this.pos += 4;
      return len;
    }
    return first;
  }

  readString(): string {
    const length = this.readCompactLength();
    const s = this.buf.toString('utf8', this.pos, this.pos + length);
    this.pos += length;
    return s;
  }

  readBytes(): Uint8Array {
    const length = this.readCompactLength();
    const bytes = Buffer.from(this.buf.buffer, this.buf.byteOffset + this.pos, length);
    this.pos += length;
    return bytes;
  }

  readMessageBytes(): Uint8Array {
    return this.readBytes();
  }

  skip(n: number): void {
    this.pos += n;
  }

  /**
   * Read and validate a 4-byte signed array length. Mirrors
   * Decoder.readArrayCount in index.ts: the caller MUST supply
   * maxElements so allocation policy is visible at every call site.
   * See index.ts for the validation order and rationale.
   */
  readArrayCount(elementMinBytes: number, maxElements: number): number {
    if (!Number.isInteger(maxElements) || maxElements < 0) {
      throw new RangeError('xpb: readArrayCount requires non-negative integer maxElements');
    }
    const n = this.readInt32();
    if (n < 0) {
      throw new Error(`xpb: negative array count: ${n}`);
    }
    if (n > maxElements) {
      throw new Error(`xpb: array count ${n} exceeds caller-supplied max ${maxElements}`);
    }
    if (elementMinBytes > 0) {
      const max = Math.floor((this.data.length - this.pos) / elementMinBytes);
      if (n > max) {
        throw new Error(`xpb: array count ${n} exceeds buffer-bounded max ${max}`);
      }
    }
    return n;
  }
}

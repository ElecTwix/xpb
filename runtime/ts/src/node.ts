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

  private writeTag(fieldNumber: number, wireType: number): void {
    this.writeVarint32((fieldNumber << 3) | wireType);
  }

  writeBool(fieldNumber: number, v: boolean): void {
    this.ensureCapacity(2);
    this.writeTag(fieldNumber, WireType.Varint);
    this.buf[this.pos++] = v ? 1 : 0;
  }

  writeInt32(fieldNumber: number, v: number): void {
    this.writeTag(fieldNumber, WireType.Varint);
    this.writeVarint32(zigzagEncode32(v));
  }

  writeInt64(fieldNumber: number, v: bigint): void {
    this.writeTag(fieldNumber, WireType.Varint);
    this.ensureCapacity(10);
    let val = v < 0n ? (v << 1n) ^ (v >> 63n) : v << 1n; // zigzag
    while (val >= 0x80n) {
      this.buf[this.pos++] = Number((val & 0x7fn) | 0x80n);
      val >>= 7n;
    }
    this.buf[this.pos++] = Number(val);
  }

  writeUint32(fieldNumber: number, v: number): void {
    this.writeTag(fieldNumber, WireType.Varint);
    this.writeVarint32(v);
  }

  writeUint64(fieldNumber: number, v: bigint): void {
    this.writeTag(fieldNumber, WireType.Varint);
    this.ensureCapacity(10);
    while (v >= 0x80n) {
      this.buf[this.pos++] = Number((v & 0x7fn) | 0x80n);
      v >>= 7n;
    }
    this.buf[this.pos++] = Number(v);
  }

  writeFloat32(fieldNumber: number, v: number): void {
    this.ensureCapacity(6);
    this.writeTag(fieldNumber, WireType.Fixed32);
    // Native Buffer method
    this.buf.writeFloatLE(v, this.pos);
    this.pos += 4;
  }

  writeFloat64(fieldNumber: number, v: number): void {
    this.ensureCapacity(10);
    this.writeTag(fieldNumber, WireType.Fixed64);
    // Native Buffer method
    this.buf.writeDoubleLE(v, this.pos);
    this.pos += 8;
  }

  writeString(fieldNumber: number, v: string): void {
    // Use Buffer.byteLength for accurate size, then write directly
    const byteLen = Buffer.byteLength(v, 'utf8');
    this.ensureCapacity(byteLen + 10);
    this.writeTag(fieldNumber, WireType.LengthDelimited);
    this.writeVarint32(byteLen);
    // Native Buffer.write is very fast
    this.buf.write(v, this.pos, byteLen, 'utf8');
    this.pos += byteLen;
  }

  writeBytes(fieldNumber: number, v: Uint8Array): void {
    this.ensureCapacity(v.length + 10);
    this.writeTag(fieldNumber, WireType.LengthDelimited);
    this.writeVarint32(v.length);
    // Fast copy
    if (Buffer.isBuffer(v)) {
      v.copy(this.buf, this.pos);
    } else {
      this.buf.set(v, this.pos);
    }
    this.pos += v.length;
  }

  writeMessage(fieldNumber: number, data: Uint8Array): void {
    this.writeBytes(fieldNumber, data);
  }
}

/**
 * Node.js/Bun Optimized Decoder using native Buffer
 */
export class Decoder {
  private buf: Buffer;
  private pos = 0;

  constructor(data: Uint8Array) {
    // Wrap in Buffer for native methods
    this.buf = Buffer.isBuffer(data) ? data : Buffer.from(data);
  }

  eof(): boolean {
    return this.pos >= this.buf.length;
  }

  private readVarint32(): number {
    let result = 0;
    let shift = 0;
    while (this.pos < this.buf.length) {
      const b = this.buf[this.pos++];
      result |= (b & 0x7f) << shift;
      if ((b & 0x80) === 0) {
        return result >>> 0;
      }
      shift += 7;
    }
    throw new Error("xpb: unexpected EOF reading varint");
  }

  private readVarint64(): bigint {
    let result = 0n;
    let shift = 0n;
    while (this.pos < this.buf.length) {
      const b = this.buf[this.pos++];
      result |= BigInt(b & 0x7f) << shift;
      if ((b & 0x80) === 0) {
        return result;
      }
      shift += 7n;
    }
    throw new Error("xpb: unexpected EOF reading varint");
  }

  readTag(): [number, WireType] {
    const tag = this.readVarint32();
    return [tag >>> 3, (tag & 0x7) as WireType];
  }

  readBool(): boolean {
    return this.readVarint32() !== 0;
  }

  readInt32(): number {
    return zigzagDecode32(this.readVarint32());
  }

  readInt64(): bigint {
    const n = this.readVarint64();
    return (n >> 1n) ^ -(n & 1n);
  }

  readUint32(): number {
    return this.readVarint32();
  }

  readUint64(): bigint {
    return this.readVarint64();
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

  readString(): string {
    const length = this.readVarint32();
    // Native Buffer.toString is very fast
    const s = this.buf.toString('utf8', this.pos, this.pos + length);
    this.pos += length;
    return s;
  }

  readBytes(): Uint8Array {
    const length = this.readVarint32();
    // Fast copy using Buffer methods
    const bytes = Buffer.from(this.buf.buffer, this.buf.byteOffset + this.pos, length);
    this.pos += length;
    return bytes;
  }

  readMessageBytes(): Uint8Array {
    return this.readBytes();
  }

  skip(wireType: WireType): void {
    switch (wireType) {
      case WireType.Varint:
        this.readVarint32();
        break;
      case WireType.Fixed32:
        this.pos += 4;
        break;
      case WireType.Fixed64:
        this.pos += 8;
        break;
      case WireType.LengthDelimited:
        const length = this.readVarint32();
        this.pos += length;
        break;
    }
  }
}

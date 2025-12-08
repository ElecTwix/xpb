/**
 * XPB Wire Types
 */
export const WireType = {
  Varint: 0,
  Fixed64: 1,
  LengthDelimited: 2,
  Fixed32: 5,
} as const;

export type WireType = (typeof WireType)[keyof typeof WireType];

// Cached encoder/decoder for strings (avoid creating new instances)
const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

/**
 * Zigzag encodes a signed 32-bit integer.
 */
export function zigzagEncode32(n: number): number {
  return (n << 1) ^ (n >> 31);
}

/**
 * Zigzag decodes a 32-bit integer.
 */
export function zigzagDecode32(n: number): number {
  return (n >>> 1) ^ -(n & 1);
}

/**
 * Zigzag encodes a signed 64-bit integer (as bigint).
 */
export function zigzagEncode64(n: bigint): bigint {
  return (n << 1n) ^ (n >> 63n);
}

/**
 * Zigzag decodes a 64-bit integer (as bigint).
 */
export function zigzagDecode64(n: bigint): bigint {
  return (n >> 1n) ^ -(n & 1n);
}

/**
 * Optimized Encoder for XPB binary format.
 * Uses preallocated buffer and avoids BigInt for 32-bit operations.
 */
export class Encoder {
  private buf: Uint8Array;
  private pos = 0;
  private view: DataView;

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

  // Fast 32-bit varint (avoids BigInt)
  private writeVarint32(v: number): void {
    this.ensureCapacity(5);
    v = v >>> 0; // Convert to unsigned
    while (v >= 0x80) {
      this.buf[this.pos++] = (v & 0x7f) | 0x80;
      v >>>= 7;
    }
    this.buf[this.pos++] = v;
  }

  // 64-bit varint (uses BigInt only when needed)
  private writeVarint64(v: bigint): void {
    this.ensureCapacity(10);
    while (v >= 0x80n) {
      this.buf[this.pos++] = Number((v & 0x7fn) | 0x80n);
      v >>= 7n;
    }
    this.buf[this.pos++] = Number(v);
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
    this.writeVarint64(zigzagEncode64(v));
  }

  writeUint32(fieldNumber: number, v: number): void {
    this.writeTag(fieldNumber, WireType.Varint);
    this.writeVarint32(v);
  }

  writeUint64(fieldNumber: number, v: bigint): void {
    this.writeTag(fieldNumber, WireType.Varint);
    this.writeVarint64(v);
  }

  writeFloat32(fieldNumber: number, v: number): void {
    this.ensureCapacity(6);
    this.writeTag(fieldNumber, WireType.Fixed32);
    this.view.setFloat32(this.pos, v, true);
    this.pos += 4;
  }

  writeFloat64(fieldNumber: number, v: number): void {
    this.ensureCapacity(10);
    this.writeTag(fieldNumber, WireType.Fixed64);
    this.view.setFloat64(this.pos, v, true);
    this.pos += 8;
  }

  writeString(fieldNumber: number, v: string): void {
    const bytes = textEncoder.encode(v);
    this.ensureCapacity(bytes.length + 10);
    this.writeTag(fieldNumber, WireType.LengthDelimited);
    this.writeVarint32(bytes.length);
    this.buf.set(bytes, this.pos);
    this.pos += bytes.length;
  }

  writeBytes(fieldNumber: number, v: Uint8Array): void {
    this.ensureCapacity(v.length + 10);
    this.writeTag(fieldNumber, WireType.LengthDelimited);
    this.writeVarint32(v.length);
    this.buf.set(v, this.pos);
    this.pos += v.length;
  }

  writeMessage(fieldNumber: number, data: Uint8Array): void {
    this.writeBytes(fieldNumber, data);
  }
}

/**
 * Optimized Decoder for XPB binary format.
 * Uses 32-bit path where possible to avoid BigInt overhead.
 */
export class Decoder {
  private data: Uint8Array;
  private pos = 0;
  private view: DataView;

  constructor(data: Uint8Array) {
    this.data = data;
    this.view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  }

  eof(): boolean {
    return this.pos >= this.data.length;
  }

  // Fast 32-bit varint (avoids BigInt)
  private readVarint32(): number {
    let result = 0;
    let shift = 0;
    while (this.pos < this.data.length) {
      const b = this.data[this.pos++];
      result |= (b & 0x7f) << shift;
      if ((b & 0x80) === 0) {
        return result >>> 0;
      }
      shift += 7;
      if (shift > 35) {
        throw new Error("xpb: varint too long for 32-bit");
      }
    }
    throw new Error("xpb: unexpected EOF reading varint");
  }

  // 64-bit varint (BigInt)
  private readVarint64(): bigint {
    let result = 0n;
    let shift = 0n;
    while (this.pos < this.data.length) {
      const b = this.data[this.pos++];
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
    return zigzagDecode64(this.readVarint64());
  }

  readUint32(): number {
    return this.readVarint32();
  }

  readUint64(): bigint {
    return this.readVarint64();
  }

  readFloat32(): number {
    if (this.pos + 4 > this.data.length) {
      throw new Error("xpb: unexpected EOF reading float32");
    }
    const v = this.view.getFloat32(this.pos, true);
    this.pos += 4;
    return v;
  }

  readFloat64(): number {
    if (this.pos + 8 > this.data.length) {
      throw new Error("xpb: unexpected EOF reading float64");
    }
    const v = this.view.getFloat64(this.pos, true);
    this.pos += 8;
    return v;
  }

  readString(): string {
    const length = this.readVarint32();
    if (this.pos + length > this.data.length) {
      throw new Error("xpb: unexpected EOF reading string");
    }
    const bytes = this.data.subarray(this.pos, this.pos + length);
    this.pos += length;
    return textDecoder.decode(bytes);
  }

  readBytes(): Uint8Array {
    const length = this.readVarint32();
    if (this.pos + length > this.data.length) {
      throw new Error("xpb: unexpected EOF reading bytes");
    }
    const bytes = this.data.slice(this.pos, this.pos + length);
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
      default:
        throw new Error(`xpb: unknown wire type ${wireType}`);
    }
  }
}

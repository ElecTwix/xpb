/**
 * XPB Browser-Optimized Runtime
 * Uses Web APIs like TextEncoder.encodeInto() for zero-copy string encoding
 */

export const WireType = {
  Varint: 0,
  Fixed64: 1,
  LengthDelimited: 2,
  Fixed32: 5,
} as const;

export type WireType = (typeof WireType)[keyof typeof WireType];

// Cached instances for reuse
const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

export function zigzagEncode32(n: number): number {
  return (n << 1) ^ (n >> 31);
}

export function zigzagDecode32(n: number): number {
  return (n >>> 1) ^ -(n & 1);
}

/**
 * Browser-Optimized Encoder using encodeInto() for strings
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
    let val = v < 0n ? (v << 1n) ^ (v >> 63n) : v << 1n;
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
    // Estimate max bytes: 3 bytes per char for UTF-8
    const maxBytes = v.length * 3;
    this.ensureCapacity(maxBytes + 10);
    
    this.writeTag(fieldNumber, WireType.LengthDelimited);
    
    // Reserve space for length (will be patched)
    const lengthPos = this.pos;
    this.pos += 1; // Assume length fits in 1 byte for now
    
    // Use encodeInto() for zero-copy encoding
    const result = textEncoder.encodeInto(v, this.buf.subarray(this.pos));
    const bytesWritten = result.written!;
    
    // Patch length
    if (bytesWritten < 128) {
      this.buf[lengthPos] = bytesWritten;
      this.pos += bytesWritten;
    } else {
      // Rare case: length needs more bytes, shift data
      const lengthBytes = this.varintSize(bytesWritten);
      if (lengthBytes > 1) {
        // Shift encoded string to make room for length
        this.buf.copyWithin(lengthPos + lengthBytes, lengthPos + 1, this.pos + bytesWritten);
        this.pos = lengthPos;
        this.writeVarint32(bytesWritten);
        this.pos += bytesWritten;
      } else {
        this.buf[lengthPos] = bytesWritten;
        this.pos += bytesWritten;
      }
    }
  }

  private varintSize(v: number): number {
    if (v < 128) return 1;
    if (v < 16384) return 2;
    if (v < 2097152) return 3;
    if (v < 268435456) return 4;
    return 5;
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
 * Browser-Optimized Decoder
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
    }
    throw new Error("xpb: unexpected EOF reading varint");
  }

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
    const v = this.view.getFloat32(this.pos, true);
    this.pos += 4;
    return v;
  }

  readFloat64(): number {
    const v = this.view.getFloat64(this.pos, true);
    this.pos += 8;
    return v;
  }

  readString(): string {
    const length = this.readVarint32();
    const bytes = this.data.subarray(this.pos, this.pos + length);
    this.pos += length;
    return textDecoder.decode(bytes);
  }

  readBytes(): Uint8Array {
    const length = this.readVarint32();
    const bytes = this.data.subarray(this.pos, this.pos + length);
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

/**
 * XPB Unsafe Runtime
 * 
 * TRADEOFFS:
 * - Direct Uint8Array access (no DataView) for maximum speed
 * - No bounds checking (caller must ensure capacity)
 * - Minimal object allocation
 * - Manual Little Endian bit manipulation
 * 
 * WARNING: NO BOUNDS CHECKING. CRASHES OR UNDEFINED BEHAVIOR IF BUFFER TOO SMALL.
 */

import { WireType, zigzagEncode32, zigzagEncode64, zigzagDecode32, zigzagDecode64 } from './index';

// Shared scratchpad for float conversion (faster than DataView for single values)
const floatScratch = new Float32Array(1);
const floatScratchU8 = new Uint8Array(floatScratch.buffer);
const doubleScratch = new Float64Array(1);
const doubleScratchU8 = new Uint8Array(doubleScratch.buffer);
const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

export class UnsafeEncoder {
  public buf: Uint8Array;
  public pos = 0;

  constructor(initialSize = 256) {
    this.buf = new Uint8Array(initialSize);
  }

  ensureCapacity(needed: number): void {
    if (this.pos + needed > this.buf.length) {
      const newSize = Math.max(this.buf.length * 2, this.pos + needed);
      const newBuf = new Uint8Array(newSize);
      newBuf.set(this.buf);
      this.buf = newBuf;
    }
  }

  finish(): Uint8Array {
    return this.buf.subarray(0, this.pos);
  }

  reset(): void {
    this.pos = 0;
  }

  // --- PRIMITIVES ---

  writeTag(fieldNumber: number, wireType: number): void {
    let v = (fieldNumber << 3) | wireType;
    while (v >= 0x80) {
      this.buf[this.pos++] = (v & 0x7f) | 0x80;
      v >>>= 7;
    }
    this.buf[this.pos++] = v;
  }

  writeBool(fieldNumber: number, v: boolean): void {
    this.writeTag(fieldNumber, WireType.Varint);
    this.buf[this.pos++] = v ? 1 : 0;
  }

  writeInt32(fieldNumber: number, v: number): void {
    this.writeTag(fieldNumber, WireType.Varint);
    let z = (v << 1) ^ (v >> 31); // zigzag
    while (z >= 0x80) {
      this.buf[this.pos++] = (z & 0x7f) | 0x80;
      z >>>= 7;
    }
    this.buf[this.pos++] = z;
  }

  writeInt64(fieldNumber: number, v: bigint): void {
    this.writeTag(fieldNumber, WireType.Varint);
    let z = (v << 1n) ^ (v >> 63n); // zigzag
    while (z >= 0x80n) {
      this.buf[this.pos++] = Number((z & 0x7fn) | 0x80n);
      z >>= 7n;
    }
    this.buf[this.pos++] = Number(z);
  }

  writeUint32(fieldNumber: number, v: number): void {
    this.writeTag(fieldNumber, WireType.Varint);
    v = v >>> 0;
    while (v >= 0x80) {
      this.buf[this.pos++] = (v & 0x7f) | 0x80;
      v >>>= 7;
    }
    this.buf[this.pos++] = v;
  }

  writeUint64(fieldNumber: number, v: bigint): void {
    this.writeTag(fieldNumber, WireType.Varint);
    while (v >= 0x80n) {
      this.buf[this.pos++] = Number((v & 0x7fn) | 0x80n);
      v >>= 7n;
    }
    this.buf[this.pos++] = Number(v);
  }

  writeFloat32(fieldNumber: number, v: number): void {
    this.writeTag(fieldNumber, WireType.Fixed32);
    floatScratch[0] = v;
    this.buf[this.pos++] = floatScratchU8[0];
    this.buf[this.pos++] = floatScratchU8[1];
    this.buf[this.pos++] = floatScratchU8[2];
    this.buf[this.pos++] = floatScratchU8[3];
  }

  writeFloat64(fieldNumber: number, v: number): void {
    this.writeTag(fieldNumber, WireType.Fixed64);
    doubleScratch[0] = v;
    this.buf.set(doubleScratchU8, this.pos);
    this.pos += 8;
  }

  writeString(fieldNumber: number, v: string): void {
    this.writeTag(fieldNumber, WireType.LengthDelimited);
    // Optimistic write: assume 1 byte per char for length calculation
    // This is safe because we can re-serialize len if needed, but 
    // for true unsafe speed we just use TextEncoder into buffer directly if possible
    // actually, let's use encodeInto if available, or just standard flow.
    
    // For unsafe, we assume caller has ensured capacity or we do a quick check
    // We'll trust TextEncoder.encodeInto which is fastest
    
    const lenPos = this.pos;
    this.pos++; // Reserve 1 byte for length (optimistic)
    
    // Check if we need to expand for the string content (rough guess)
    if (this.buf.length - this.pos < v.length * 3) {
        this.ensureCapacity(v.length * 3);
    }

    const res = textEncoder.encodeInto(v, this.buf.subarray(this.pos));
    const written = res.written;

    if (written < 128) {
      this.buf[lenPos] = written; // fits in 1 byte reserved
      this.pos += written;
    } else {
      // Length didn't fit in 1 byte. Move data.
      // This is the slow path, but rare for short strings.
      // 1. Calculate varint length size
      let sizeOfLen = 1;
      let temp = written;
        while (temp >= 128) {
            temp >>= 7;
            sizeOfLen++;
        }
      
      // 2. Shift data
      this.buf.copyWithin(lenPos + sizeOfLen, lenPos + 1, lenPos + 1 + written);
      
      // 3. Write length varint
      let l = written;
      let p = lenPos;
      while (l >= 0x80) {
        this.buf[p++] = (l & 0x7f) | 0x80;
        l >>>= 7;
      }
      this.buf[p] = l;
      
      this.pos = lenPos + sizeOfLen + written;
    }
  }

  writeBytes(fieldNumber: number, v: Uint8Array): void {
    this.writeTag(fieldNumber, WireType.LengthDelimited);
    
    // Write len varint
    let len = v.length;
    while (len >= 0x80) {
      this.buf[this.pos++] = (len & 0x7f) | 0x80;
      len >>>= 7;
    }
    this.buf[this.pos++] = len;

    this.buf.set(v, this.pos);
    this.pos += v.length;
  }
}

export class UnsafeDecoder {
  private data: Uint8Array;
  private pos = 0;

  constructor(data: Uint8Array) {
    this.data = data;
  }

  eof(): boolean {
    return this.pos >= this.data.length;
  }

  readTag(): [number, WireType] {
    let result = 0;
    let shift = 0;
    while (true) {
      const b = this.data[this.pos++];
      result |= (b & 0x7f) << shift;
      if ((b & 0x80) === 0) break;
      shift += 7;
    }
    return [result >>> 3, (result & 0x7) as WireType];
  }

  readBool(): boolean {
    // Read varint (optimized for 1 byte)
    const b = this.data[this.pos++];
    if ((b & 0x80) === 0) return b !== 0;
    
    // Slow path for multi-byte bool (unlikely but legal)
    this.pos--; 
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
    // Manual LE to float
    floatScratchU8[0] = this.data[this.pos++];
    floatScratchU8[1] = this.data[this.pos++];
    floatScratchU8[2] = this.data[this.pos++];
    floatScratchU8[3] = this.data[this.pos++];
    return floatScratch[0];
  }

  readFloat64(): number {
    doubleScratchU8.set(this.data.subarray(this.pos, this.pos + 8));
    this.pos += 8;
    return doubleScratch[0];
  }

  readString(): string {
    const len = this.readVarint32();
    const str = textDecoder.decode(this.data.subarray(this.pos, this.pos + len));
    this.pos += len;
    return str;
  }

  readBytes(): Uint8Array {
    const len = this.readVarint32();
    const bytes = this.data.slice(this.pos, this.pos + len);
    this.pos += len;
    return bytes;
  }

  skip(wireType: WireType): void {
    switch (wireType) {
      case WireType.Varint:
        while (this.data[this.pos++] >= 0x80);
        break;
      case WireType.Fixed32:
        this.pos += 4;
        break;
      case WireType.Fixed64:
        this.pos += 8;
        break;
      case WireType.LengthDelimited:
        const len = this.readVarint32();
        this.pos += len;
        break;
    }
  }

  // --- HELPERS ---

  private readVarint32(): number {
    let result = 0;
    let shift = 0;
    while (true) {
      const b = this.data[this.pos++];
      result |= (b & 0x7f) << shift;
      if ((b & 0x80) === 0) return result >>> 0;
      shift += 7;
    }
  }

  private readVarint64(): bigint {
    let result = 0n;
    let shift = 0n;
    while (true) {
      const b = this.data[this.pos++];
      result |= BigInt(b & 0x7f) << shift;
      if ((b & 0x80) === 0) return result;
      shift += 7n;
    }
  }
}

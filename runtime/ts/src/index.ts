/**
 * XPB Wire Types
 */
export const WireType = {
  Varint: 0,
  Fixed32: 1,
  Fixed64: 2,
  LengthDelimited: 3,
} as const;

export type WireType = (typeof WireType)[keyof typeof WireType];

/**
 * Extracts the field number from a tag.
 */
export function tagFieldNumber(tag: number): number {
  return tag >>> 3;
}

/**
 * Extracts the wire type from a tag.
 */
export function tagWireType(tag: number): WireType {
  return (tag & 0x7) as WireType;
}

/**
 * Creates a tag from a field number and wire type.
 */
export function makeTag(fieldNumber: number, wireType: WireType): number {
  return (fieldNumber << 3) | wireType;
}

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
 * Encoder for XPB binary format.
 */
export class Encoder {
  private buf: number[] = [];

  /**
   * Returns the encoded bytes.
   */
  finish(): Uint8Array {
    return new Uint8Array(this.buf);
  }

  /**
   * Clears the encoder for reuse.
   */
  reset(): void {
    this.buf = [];
  }

  /**
   * Writes an unsigned varint.
   */
  writeUvarint(v: number | bigint): void {
    let n = typeof v === "bigint" ? v : BigInt(v);
    while (n >= 0x80n) {
      this.buf.push(Number((n & 0x7fn) | 0x80n));
      n >>= 7n;
    }
    this.buf.push(Number(n));
  }

  /**
   * Writes a tag (field number + wire type).
   */
  writeTag(fieldNumber: number, wireType: WireType): void {
    this.writeUvarint(makeTag(fieldNumber, wireType));
  }

  /**
   * Writes a boolean field.
   */
  writeBool(fieldNumber: number, v: boolean): void {
    this.writeTag(fieldNumber, WireType.Varint);
    this.buf.push(v ? 1 : 0);
  }

  /**
   * Writes a signed 32-bit integer field.
   */
  writeInt32(fieldNumber: number, v: number): void {
    this.writeTag(fieldNumber, WireType.Varint);
    this.writeUvarint(zigzagEncode32(v));
  }

  /**
   * Writes a signed 64-bit integer field.
   */
  writeInt64(fieldNumber: number, v: bigint): void {
    this.writeTag(fieldNumber, WireType.Varint);
    this.writeUvarint(zigzagEncode64(v));
  }

  /**
   * Writes an unsigned 32-bit integer field.
   */
  writeUint32(fieldNumber: number, v: number): void {
    this.writeTag(fieldNumber, WireType.Varint);
    this.writeUvarint(v);
  }

  /**
   * Writes an unsigned 64-bit integer field.
   */
  writeUint64(fieldNumber: number, v: bigint): void {
    this.writeTag(fieldNumber, WireType.Varint);
    this.writeUvarint(v);
  }

  /**
   * Writes a 32-bit float field.
   */
  writeFloat32(fieldNumber: number, v: number): void {
    this.writeTag(fieldNumber, WireType.Fixed32);
    const buf = new ArrayBuffer(4);
    new DataView(buf).setFloat32(0, v, true);
    const bytes = new Uint8Array(buf);
    for (let i = 0; i < 4; i++) {
      this.buf.push(bytes[i]);
    }
  }

  /**
   * Writes a 64-bit float field.
   */
  writeFloat64(fieldNumber: number, v: number): void {
    this.writeTag(fieldNumber, WireType.Fixed64);
    const buf = new ArrayBuffer(8);
    new DataView(buf).setFloat64(0, v, true);
    const bytes = new Uint8Array(buf);
    for (let i = 0; i < 8; i++) {
      this.buf.push(bytes[i]);
    }
  }

  /**
   * Writes a string field.
   */
  writeString(fieldNumber: number, v: string): void {
    this.writeTag(fieldNumber, WireType.LengthDelimited);
    const bytes = new TextEncoder().encode(v);
    this.writeUvarint(bytes.length);
    for (let i = 0; i < bytes.length; i++) {
      this.buf.push(bytes[i]);
    }
  }

  /**
   * Writes a bytes field.
   */
  writeBytes(fieldNumber: number, v: Uint8Array): void {
    this.writeTag(fieldNumber, WireType.LengthDelimited);
    this.writeUvarint(v.length);
    for (let i = 0; i < v.length; i++) {
      this.buf.push(v[i]);
    }
  }

  /**
   * Writes a nested message field.
   */
  writeMessage(fieldNumber: number, data: Uint8Array): void {
    this.writeTag(fieldNumber, WireType.LengthDelimited);
    this.writeUvarint(data.length);
    for (let i = 0; i < data.length; i++) {
      this.buf.push(data[i]);
    }
  }
}

/**
 * Decoder for XPB binary format.
 */
export class Decoder {
  private data: Uint8Array;
  private pos = 0;

  constructor(data: Uint8Array) {
    this.data = data;
  }

  /**
   * Returns true if all data has been consumed.
   */
  eof(): boolean {
    return this.pos >= this.data.length;
  }

  /**
   * Reads an unsigned varint.
   */
  readUvarint(): bigint {
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

  /**
   * Reads a tag and returns [fieldNumber, wireType].
   */
  readTag(): [number, WireType] {
    const tag = Number(this.readUvarint());
    return [tagFieldNumber(tag), tagWireType(tag)];
  }

  /**
   * Reads a boolean value.
   */
  readBool(): boolean {
    return this.readUvarint() !== 0n;
  }

  /**
   * Reads a signed 32-bit integer.
   */
  readInt32(): number {
    return zigzagDecode32(Number(this.readUvarint()));
  }

  /**
   * Reads a signed 64-bit integer.
   */
  readInt64(): bigint {
    return zigzagDecode64(this.readUvarint());
  }

  /**
   * Reads an unsigned 32-bit integer.
   */
  readUint32(): number {
    return Number(this.readUvarint());
  }

  /**
   * Reads an unsigned 64-bit integer.
   */
  readUint64(): bigint {
    return this.readUvarint();
  }

  /**
   * Reads a 32-bit float.
   */
  readFloat32(): number {
    if (this.pos + 4 > this.data.length) {
      throw new Error("xpb: unexpected EOF reading float32");
    }
    const view = new DataView(this.data.buffer, this.data.byteOffset + this.pos, 4);
    this.pos += 4;
    return view.getFloat32(0, true);
  }

  /**
   * Reads a 64-bit float.
   */
  readFloat64(): number {
    if (this.pos + 8 > this.data.length) {
      throw new Error("xpb: unexpected EOF reading float64");
    }
    const view = new DataView(this.data.buffer, this.data.byteOffset + this.pos, 8);
    this.pos += 8;
    return view.getFloat64(0, true);
  }

  /**
   * Reads a string.
   */
  readString(): string {
    const length = Number(this.readUvarint());
    if (this.pos + length > this.data.length) {
      throw new Error("xpb: unexpected EOF reading string");
    }
    const bytes = this.data.subarray(this.pos, this.pos + length);
    this.pos += length;
    return new TextDecoder().decode(bytes);
  }

  /**
   * Reads bytes.
   */
  readBytes(): Uint8Array {
    const length = Number(this.readUvarint());
    if (this.pos + length > this.data.length) {
      throw new Error("xpb: unexpected EOF reading bytes");
    }
    const bytes = this.data.slice(this.pos, this.pos + length);
    this.pos += length;
    return bytes;
  }

  /**
   * Reads message bytes (for nested messages).
   */
  readMessageBytes(): Uint8Array {
    return this.readBytes();
  }

  /**
   * Skips a field based on wire type.
   */
  skip(wireType: WireType): void {
    switch (wireType) {
      case WireType.Varint:
        this.readUvarint();
        break;
      case WireType.Fixed32:
        this.pos += 4;
        break;
      case WireType.Fixed64:
        this.pos += 8;
        break;
      case WireType.LengthDelimited:
        const length = Number(this.readUvarint());
        this.pos += length;
        break;
      default:
        throw new Error(`xpb: unknown wire type ${wireType}`);
    }
  }
}

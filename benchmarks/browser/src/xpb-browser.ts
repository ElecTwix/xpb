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
    const p = this.pos;
    this.buf[p] = v;
    this.buf[p + 1] = v >> 8;
    this.buf[p + 2] = v >> 16;
    this.buf[p + 3] = v >> 24;
    this.pos += 4;
  }

  /** Write int64 as 8 bytes (little-endian, two's complement) */
  writeInt64(v: bigint): void {
    this.ensureCapacity(8);
    let lo = Number(v & 0xffffffffn);
    let hi = Number(v >> 32n);
    const p = this.pos;
    this.buf[p] = lo;
    this.buf[p + 1] = lo >> 8;
    this.buf[p + 2] = lo >> 16;
    this.buf[p + 3] = lo >> 24;
    this.buf[p + 4] = hi;
    this.buf[p + 5] = hi >> 8;
    this.buf[p + 6] = hi >> 16;
    this.buf[p + 7] = hi >> 24;
    this.pos += 8;
  }

  /** Write uint32 as 4 bytes (little-endian) */
  writeUint32(v: number): void {
    this.ensureCapacity(4);
    const p = this.pos;
    this.buf[p] = v;
    this.buf[p + 1] = v >> 8;
    this.buf[p + 2] = v >> 16;
    this.buf[p + 3] = v >> 24;
    this.pos += 4;
  }

  /** Write uint64 as 8 bytes (little-endian) */
  writeUint64(v: bigint): void {
    this.ensureCapacity(8);
    let lo = Number(v & 0xffffffffn);
    let hi = Number(v >> 32n);
    const p = this.pos;
    this.buf[p] = lo;
    this.buf[p + 1] = lo >> 8;
    this.buf[p + 2] = lo >> 16;
    this.buf[p + 3] = lo >> 24;
    this.buf[p + 4] = hi;
    this.buf[p + 5] = hi >> 8;
    this.buf[p + 6] = hi >> 16;
    this.buf[p + 7] = hi >> 24;
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
      const p = this.pos;
      this.buf[p] = length;
      this.buf[p + 1] = length >> 8;
      this.buf[p + 2] = length >> 16;
      this.buf[p + 3] = length >> 24;
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

    // Use encodeInto for zero-copy encoding into the buffer
    // Reserve space for length + max UTF8 size (3 bytes per char)
    this.ensureCapacity(len * 3 + 5);
    
    // Write placeholder for length
    const lenPos = this.pos;
    this.pos += 1; // Assume 1 byte length initially
    
    const result = textEncoder.encodeInto(v, this.buf.subarray(this.pos));
    const written = result.written!;
    
    if (written <= CompactLengthThreshold) {
      this.buf[lenPos] = written;
      this.pos += written;
    } else {
      // Rare case: length > 254. We need to shift data to make room for 5-byte length
      const endPos = this.pos + written;
      this.buf.copyWithin(lenPos + 5, lenPos + 1, endPos);
      
      this.buf[lenPos] = CompactLengthMarker;
      this.buf[lenPos + 1] = written;
      this.buf[lenPos + 2] = written >> 8;
      this.buf[lenPos + 3] = written >> 16;
      this.buf[lenPos + 4] = written >> 24;
      
      this.pos += written + 4; // +4 because we already advanced 1
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
    const p = this.pos;
    const v = this.data[p] | (this.data[p + 1] << 8) | (this.data[p + 2] << 16) | (this.data[p + 3] << 24);
    this.pos += 4;
    return v;
  }

  /** Read int64 from 8 bytes (little-endian, two's complement) */
  readInt64(): bigint {
    if (this.pos + 8 > this.data.length) {
      throw new Error('xpb: unexpected EOF reading int64');
    }
    const p = this.pos;
    const lo = this.data[p] | (this.data[p + 1] << 8) | (this.data[p + 2] << 16) | (this.data[p + 3] << 24);
    const hi = this.data[p + 4] | (this.data[p + 5] << 8) | (this.data[p + 6] << 16) | (this.data[p + 7] << 24);
    this.pos += 8;
    return BigInt(lo >>> 0) | (BigInt(hi >>> 0) << 32n);
  }

  /** Read uint32 from 4 bytes (little-endian) */
  readUint32(): number {
    if (this.pos + 4 > this.data.length) {
      throw new Error('xpb: unexpected EOF reading uint32');
    }
    const p = this.pos;
    const v = (this.data[p] | (this.data[p + 1] << 8) | (this.data[p + 2] << 16) | (this.data[p + 3] << 24)) >>> 0;
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

// ============= JIT Compiler (Browser-optimized) =============

export enum FieldType {
  Bool, Int32, Int64, Uint32, Uint64, Float32, Float64, String
}

export interface FieldDef {
  tag: number;
  type: FieldType;
  name: string;
}

export interface SchemaDef {
  fields: FieldDef[];
}

export class SlabAllocator {
  public buf: Uint8Array;
  public view: DataView;
  public pos = 0;

  constructor(size = 65536) {
    this.buf = new Uint8Array(size);
    this.view = new DataView(this.buf.buffer);
  }

  reset(): void {
    this.pos = 0;
  }
}

export function compileEncoder<T>(schema: SchemaDef): (slab: SlabAllocator, obj: T) => void {
  const lines: string[] = [`
    var buf = slab.buf;
    var view = slab.view;
    var pos = slab.pos;
    var val, str, strLen, i, c, isAscii, lenPos, lo, hi;
  `];

  for (const field of schema.fields) {
    const access = `obj.${field.name}`;
    
    switch (field.type) {
      case FieldType.Bool:
        lines.push(`buf[pos++] = ${access} ? 1 : 0;`);
        break;
      case FieldType.Int32:
      case FieldType.Uint32:
        lines.push(`
          val = ${access};
          buf[pos++] = val;
          buf[pos++] = val >> 8;
          buf[pos++] = val >> 16;
          buf[pos++] = val >> 24;
        `);
        break;
      case FieldType.Int64:
      case FieldType.Uint64:
        // For browser, handle both BigInt and Number (for large numbers)
        lines.push(`
          val = ${access};
          if (typeof val === 'bigint') {
            lo = Number(val & 0xffffffffn);
            hi = Number(val >> 32n);
          } else {
            // Convert number to lo/hi parts
            lo = val >>> 0;
            hi = Math.floor(val / 0x100000000) >>> 0;
          }
          buf[pos++] = lo;
          buf[pos++] = lo >> 8;
          buf[pos++] = lo >> 16;
          buf[pos++] = lo >> 24;
          buf[pos++] = hi;
          buf[pos++] = hi >> 8;
          buf[pos++] = hi >> 16;
          buf[pos++] = hi >> 24;
        `);
        break;
      case FieldType.Float32:
        lines.push(`
          view.setFloat32(pos, ${access}, true);
          pos += 4;
        `);
        break;
      case FieldType.Float64:
        lines.push(`
          view.setFloat64(pos, ${access}, true);
          pos += 8;
        `);
        break;
      case FieldType.String:
        // Browser-optimized: ASCII fast path + encodeInto fallback
        lines.push(`
          str = ${access} || '';
          strLen = str.length;
          lenPos = pos++;
          
          if (strLen < 40) {
            isAscii = true;
            for (i = 0; i < strLen; i++) {
              c = str.charCodeAt(i);
              if (c > 127) { isAscii = false; break; }
              buf[pos + i] = c;
            }
            if (isAscii) {
              buf[lenPos] = strLen;
              pos += strLen;
            } else {
              var enc = textEncoder.encodeInto(str, buf.subarray(pos));
              buf[lenPos] = enc.written;
              pos += enc.written;
            }
          } else {
            var enc = textEncoder.encodeInto(str, buf.subarray(pos));
            buf[lenPos] = enc.written;
            pos += enc.written;
          }
        `);
        break;
    }
  }

  lines.push(`slab.pos = pos;`);

  return new Function('textEncoder', 'slab', 'obj', lines.join('\n'))
    .bind(null, textEncoder) as any;
}

export function compileDecoder<T>(schema: SchemaDef): (buf: Uint8Array, end: number) => T {
  // V8 Optimization: Pre-initialize all properties for consistent hidden class
  const propInits = schema.fields.map(f => {
    switch (f.type) {
      case FieldType.Bool: return `${f.name}: false`;
      case FieldType.Int32:
      case FieldType.Uint32:
      case FieldType.Float32:
      case FieldType.Float64: return `${f.name}: 0`;
      case FieldType.Int64:
      case FieldType.Uint64: return `${f.name}: 0n`;
      case FieldType.String: return `${f.name}: ''`;
      default: return `${f.name}: null`;
    }
  }).join(', ');
  
  // Optimization: Only create DataView if we have float fields
  const hasFloats = schema.fields.some(f => f.type === FieldType.Float32 || f.type === FieldType.Float64);

  const lines: string[] = [`
    var pos = 0;
    var obj = { ${propInits} };
    var len, isAscii, i, lo, hi;
    ${hasFloats ? 'var view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);' : ''}
  `];

  for (const field of schema.fields) {
    switch (field.type) {
      case FieldType.Bool:
        lines.push(`obj.${field.name} = buf[pos++] !== 0;`);
        break;
      case FieldType.Int32:
        // Inline int32 read - avoid function call overhead
        lines.push(`
          obj.${field.name} = buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24);
          pos += 4;
        `);
        break;
      case FieldType.Uint32:
        lines.push(`
          obj.${field.name} = (buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24)) >>> 0;
          pos += 4;
        `);
        break;
      case FieldType.Int64:
      case FieldType.Uint64:
        lines.push(`
          lo = buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24);
          hi = buf[pos+4] | (buf[pos+5] << 8) | (buf[pos+6] << 16) | (buf[pos+7] << 24);
          obj.${field.name} = BigInt(lo >>> 0) | (BigInt(hi >>> 0) << 32n);
          pos += 8;
        `);
        break;
      case FieldType.Float32:
        lines.push(`
          obj.${field.name} = view.getFloat32(pos, true);
          pos += 4;
        `);
        break;
      case FieldType.Float64:
        lines.push(`
          obj.${field.name} = view.getFloat64(pos, true);
          pos += 8;
        `);
        break;
      case FieldType.String:
        // Highly optimized string decode:
        // 1. Unroll common lengths 0-16 with direct String.fromCharCode (no allocations, no function calls)
        // 2. For >16 bytes, use TextDecoder directly (faster than ASCII checking + building string)
        lines.push(`
          len = buf[pos++];
          if (len === 0xFF) {
            len = buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24);
            pos += 4;
          }
          
          if (len === 0) {
            obj.${field.name} = '';
          } else if (len === 1) {
            obj.${field.name} = String.fromCharCode(buf[pos]);
          } else if (len === 2) {
            obj.${field.name} = String.fromCharCode(buf[pos], buf[pos+1]);
          } else if (len === 3) {
            obj.${field.name} = String.fromCharCode(buf[pos], buf[pos+1], buf[pos+2]);
          } else if (len === 4) {
            obj.${field.name} = String.fromCharCode(buf[pos], buf[pos+1], buf[pos+2], buf[pos+3]);
          } else if (len === 5) {
            obj.${field.name} = String.fromCharCode(buf[pos], buf[pos+1], buf[pos+2], buf[pos+3], buf[pos+4]);
          } else if (len === 6) {
            obj.${field.name} = String.fromCharCode(buf[pos], buf[pos+1], buf[pos+2], buf[pos+3], buf[pos+4], buf[pos+5]);
          } else if (len === 7) {
            obj.${field.name} = String.fromCharCode(buf[pos], buf[pos+1], buf[pos+2], buf[pos+3], buf[pos+4], buf[pos+5], buf[pos+6]);
          } else if (len === 8) {
            obj.${field.name} = String.fromCharCode(buf[pos], buf[pos+1], buf[pos+2], buf[pos+3], buf[pos+4], buf[pos+5], buf[pos+6], buf[pos+7]);
          } else if (len <= 16) {
            // Build short string without intermediate allocations
            var s = String.fromCharCode(buf[pos], buf[pos+1], buf[pos+2], buf[pos+3], buf[pos+4], buf[pos+5], buf[pos+6], buf[pos+7]);
            for (i = 8; i < len; i++) s += String.fromCharCode(buf[pos + i]);
            obj.${field.name} = s;
          } else {
            // For longer strings, TextDecoder is faster than manual building
            obj.${field.name} = textDecoder.decode(buf.subarray(pos, pos + len));
          }
          pos += len;
        `);
        break;
    }
  }

  lines.push(`return obj;`);

  return new Function('textDecoder', 'buf', 'end', lines.join('\n'))
    .bind(null, textDecoder) as any;
}

// Export for browser bundle
(window as any).XPB = {
  Encoder,
  Decoder,
  SlabAllocator,
  compileEncoder,
  compileDecoder,
  FieldType
};
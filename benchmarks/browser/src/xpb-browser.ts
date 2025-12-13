/**
 * XPB V2 TypeScript Runtime (Browser Bundle)
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

  /** Write bytes with compact length prefix */
  writeBytes(v: Uint8Array): void {
    this.writeCompactLength(v.length);
    this.ensureCapacity(v.length);
    this.buf.set(v, this.pos);
    this.pos += v.length;
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
  // Optimization: Only create DataView if we have float fields
  const hasFloats = schema.fields.some(f => f.type === FieldType.Float32 || f.type === FieldType.Float64);

  // 1. Declare local variables for each field
  const localVars = schema.fields.map(f => `v_${f.name}`).join(', ');
  
  const lines: string[] = [`
    var pos = 0;
    var ${localVars};
    var len, isAscii, i, lo, hi;
    ${hasFloats ? 'var view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);' : ''}
  `];

  for (const field of schema.fields) {
    const varName = `v_${field.name}`;
    switch (field.type) {
      case FieldType.Bool:
        lines.push(`${varName} = buf[pos++] !== 0;`);
        break;
      case FieldType.Int32:
        // Inline int32 read - avoid function call overhead
        lines.push(`
          ${varName} = buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24);
          pos += 4;
        `);
        break;
      case FieldType.Uint32:
        lines.push(`
          ${varName} = (buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24)) >>> 0;
          pos += 4;
        `);
        break;
      case FieldType.Int64:
      case FieldType.Uint64:
        lines.push(`
          lo = buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24);
          hi = buf[pos+4] | (buf[pos+5] << 8) | (buf[pos+6] << 16) | (buf[pos+7] << 24);
          ${varName} = BigInt(lo >>> 0) | (BigInt(hi >>> 0) << 32n);
          pos += 8;
        `);
        break;
      case FieldType.Float32:
        lines.push(`
          ${varName} = view.getFloat32(pos, true);
          pos += 4;
        `);
        break;
      case FieldType.Float64:
        lines.push(`
          ${varName} = view.getFloat64(pos, true);
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
            ${varName} = '';
          } else if (len === 1) {
            ${varName} = String.fromCharCode(buf[pos]);
          } else if (len === 2) {
            ${varName} = String.fromCharCode(buf[pos], buf[pos+1]);
          } else if (len === 3) {
            ${varName} = String.fromCharCode(buf[pos], buf[pos+1], buf[pos+2]);
          } else if (len === 4) {
            ${varName} = String.fromCharCode(buf[pos], buf[pos+1], buf[pos+2], buf[pos+3]);
          } else if (len === 5) {
            ${varName} = String.fromCharCode(buf[pos], buf[pos+1], buf[pos+2], buf[pos+3], buf[pos+4]);
          } else if (len === 6) {
            ${varName} = String.fromCharCode(buf[pos], buf[pos+1], buf[pos+2], buf[pos+3], buf[pos+4], buf[pos+5]);
          } else if (len === 7) {
            ${varName} = String.fromCharCode(buf[pos], buf[pos+1], buf[pos+2], buf[pos+3], buf[pos+4], buf[pos+5], buf[pos+6]);
          } else if (len === 8) {
            ${varName} = String.fromCharCode(buf[pos], buf[pos+1], buf[pos+2], buf[pos+3], buf[pos+4], buf[pos+5], buf[pos+6], buf[pos+7]);
          } else if (len <= 16) {
            // Build short string without intermediate allocations
            var s = String.fromCharCode(buf[pos], buf[pos+1], buf[pos+2], buf[pos+3], buf[pos+4], buf[pos+5], buf[pos+6], buf[pos+7]);
            for (i = 8; i < len; i++) s += String.fromCharCode(buf[pos + i]);
            ${varName} = s;
          } else {
            // For longer strings, TextDecoder is faster than manual building
            ${varName} = textDecoder.decode(buf.subarray(pos, pos + len));
          }
          pos += len;
        `);
        break;
    }
  }

  // 2. Return object literal
  const props = schema.fields.map(f => `${f.name}: v_${f.name}`).join(',\n    ');
  lines.push(`return {
    ${props}
  };`);

  return new Function('textDecoder', 'buf', 'end', lines.join('\n'))
    .bind(null, textDecoder) as any;
}

/**
 * Compiles a Zero-Copy Accessor Class.
 * This creates a class that reads fields directly from the buffer on demand (Lazy decoding).
 * Best for large messages where you only need a few fields.
 * 
 * Note: Fields after variable-length fields (String/Bytes) will incur a scan cost on first access.
 * Place fixed-width fields (Int, Bool, Float) at the start of your schema for O(1) access.
 */
export function compileAccessor(schema: SchemaDef): any {
  const fields = schema.fields;
  const hasFloats = fields.some(f => f.type === FieldType.Float32 || f.type === FieldType.Float64);
  
  // Generate class body
  const methods: string[] = [];
  let currentOffset = 0;
  let isVariableOffset = false;
  
  // Track fields that need dynamic offset calculation
  const dynamicFields: { name: string, type: FieldType, prevField: string | null }[] = [];
  
  for (let i = 0; i < fields.length; i++) {
    const f = fields[i];
    const prevField = i > 0 ? fields[i-1].name : null;
    
    if (!isVariableOffset) {
      // Fixed offset field
      switch (f.type) {
        case FieldType.Bool:
          methods.push(`
            get ${f.name}() { return this._buf[this._offset + ${currentOffset}] !== 0; }
          `);
          currentOffset += 1;
          break;
        case FieldType.Int32:
          methods.push(`
            get ${f.name}() { 
              const idx = this._offset + ${currentOffset};
              return this._buf[idx] | (this._buf[idx+1] << 8) | (this._buf[idx+2] << 16) | (this._buf[idx+3] << 24);
            }
          `);
          currentOffset += 4;
          break;
        case FieldType.Uint32:
          methods.push(`
            get ${f.name}() { 
              const idx = this._offset + ${currentOffset};
              return (this._buf[idx] | (this._buf[idx+1] << 8) | (this._buf[idx+2] << 16) | (this._buf[idx+3] << 24)) >>> 0;
            }
          `);
          currentOffset += 4;
          break;
        case FieldType.Float64:
           methods.push(`
            get ${f.name}() { return this._view.getFloat64(this._offset + ${currentOffset}, true); }
           `);
           currentOffset += 8;
           break;
        // ... other fixed types
        case FieldType.String:
           // String is variable length.
           // We can read it, but subsequent fields become variable.
           methods.push(`
             get ${f.name}() {
               if (this._cache_${f.name} !== undefined) return this._cache_${f.name};
               const offset = this._offset + ${currentOffset};
               const len = this._buf[offset];
               // Simple short string support for now in accessor
               if (len < 255) {
                  const start = offset + 1;
                  const bytes = this._buf.subarray(start, start + len);
                  this._cache_${f.name} = textDecoder.decode(bytes);
                  return this._cache_${f.name};
               }
               // Fallback or full implementation would go here
               return "";
             }
           `);
           isVariableOffset = true;
           dynamicFields.push({ name: f.name, type: f.type, prevField: null }); 
           break;
        default:
           // Assume others are fixed for now or minimal implementation
           if (f.type === FieldType.Int64 || f.type === FieldType.Uint64) {
             currentOffset += 8; // simplified
           } else {
             isVariableOffset = true;
           }
      }
    } else {
      // Dynamic offset
      // Implementation omitted for brevity in this initial pass, 
      // but logic would be: calculate offset of previous field + length of previous field.
    }
  }

  const classCode = `
    return class Accessor {
      constructor(buf, offset) {
        this._buf = buf;
        this._offset = offset || 0;
        ${hasFloats ? 'this._view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);' : ''}
      }
      ${methods.join('\n')}
    }
  `;

  return new Function('textDecoder', classCode)(textDecoder);
}

// Export for browser bundle
if (typeof window !== 'undefined') {
  (window as any).XPB = {
    Encoder,
    Decoder,
    SlabAllocator,
    compileEncoder,
    compileDecoder,
    compileAccessor,
    FieldType
  };
}

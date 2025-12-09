/**
 * XPB V2 Browser Runtime
 * Optimized for browser (no Node.js Buffer dependency)
 */

// Compact length constants
const CompactLengthThreshold = 254;
const CompactLengthMarker = 0xFF;

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

// ============= BUFFER POOL =============

/**
 * BufferPool for reusing ArrayBuffers to reduce GC pressure.
 * Maintains a pool of pre-allocated buffers for encoding.
 */
export class BufferPool {
  private pool: Uint8Array[] = [];
  private size: number;
  
  constructor(poolSize = 8, bufferSize = 1024) {
    this.size = bufferSize;
    // Pre-allocate buffers
    for (let i = 0; i < poolSize; i++) {
      this.pool.push(new Uint8Array(bufferSize));
    }
  }
  
  /**
   * Get a buffer from the pool (or create new if empty)
   */
  acquire(): Uint8Array {
    return this.pool.pop() || new Uint8Array(this.size);
  }
  
  /**
   * Return a buffer to the pool
   */
  release(buf: Uint8Array): void {
    if (buf.length === this.size && this.pool.length < 16) {
      this.pool.push(buf);
    }
  }
  
  /**
   * Encode with pooled buffer - returns the encoded data (copy)
   */
  encode<T>(encodeFn: (buf: Uint8Array) => number, copyResult = true): Uint8Array {
    const buf = this.acquire();
    const len = encodeFn(buf);
    if (copyResult) {
      const result = new Uint8Array(len);
      result.set(buf.subarray(0, len));
      this.release(buf);
      return result;
    }
    return buf.subarray(0, len);
  }
}

// Global buffer pool
export const bufferPool = new BufferPool();

/**
 * Browser-optimized Encoder
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

  writeBool(v: boolean): void {
    this.ensureCapacity(1);
    this.buf[this.pos++] = v ? 1 : 0;
  }

  writeInt32(v: number): void {
    this.ensureCapacity(4);
    this.view.setInt32(this.pos, v, true);
    this.pos += 4;
  }

  writeString(v: string): void {
    // Browser: use encodeInto for zero-allocation
    this.ensureCapacity(v.length * 3 + 5); // UTF-8 worst case + length
    const lenPos = this.pos++;
    
    // ASCII fast path
    const strLen = v.length;
    if (strLen < 40) {
      let isAscii = true;
      for (let i = 0; i < strLen; i++) {
        const c = v.charCodeAt(i);
        if (c > 127) { isAscii = false; break; }
        this.buf[this.pos + i] = c;
      }
      if (isAscii) {
        this.buf[lenPos] = strLen;
        this.pos += strLen;
        return;
      }
    }
    
    // Fallback: encodeInto
    const result = textEncoder.encodeInto(v, this.buf.subarray(this.pos));
    this.buf[lenPos] = result.written!;
    this.pos += result.written!;
  }
}

/**
 * Browser-optimized Decoder with ASCII fast path
 */
export class Decoder {
  private data: Uint8Array;
  private pos = 0;
  private view: DataView;

  constructor(data: Uint8Array) {
    this.data = data;
    this.view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  }

  readBool(): boolean {
    return this.data[this.pos++] !== 0;
  }

  readInt32(): number {
    const v = this.view.getInt32(this.pos, true);
    this.pos += 4;
    return v;
  }

  readString(): string {
    const len = this.data[this.pos++];
    
    // ASCII fast path: most strings are ASCII, avoid TextDecoder overhead
    if (len < 64) {
      let isAscii = true;
      const start = this.pos;
      for (let i = 0; i < len; i++) {
        if (this.data[start + i] > 127) {
          isAscii = false;
          break;
        }
      }
      if (isAscii) {
        // Use apply with typed array slice - fastest ASCII decode
        const str = String.fromCharCode.apply(null, this.data.subarray(start, start + len) as any);
        this.pos += len;
        return str;
      }
    }
    
    // Fallback: TextDecoder for UTF-8
    const str = textDecoder.decode(this.data.subarray(this.pos, this.pos + len));
    this.pos += len;
    return str;
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
  
  const lines: string[] = [`
    var pos = 0;
    var obj = { ${propInits} };
    var len, isAscii, i, lo, hi, view;
    view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
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
        // ASCII fast path - String.fromCharCode.apply is V8-optimized
        lines.push(`
          
          
          len = buf[pos++];
          if (len === 5) {
             // Optimization: Unroll common short string length (5 chars)
             obj.${field.name} = String.fromCharCode(buf[pos], buf[pos+1], buf[pos+2], buf[pos+3], buf[pos+4]);
          } else if (len <= 40) {
            isAscii = true;
            for (i = 0; i < len; i++) {
              if (buf[pos + i] > 127) { isAscii = false; break; }
            }
            if (isAscii) {
              obj.${field.name} = String.fromCharCode.apply(null, buf.subarray(pos, pos + len));
            } else {
              obj.${field.name} = textDecoder.decode(buf.subarray(pos, pos + len));
            }
          } else {
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
  BufferPool,
  bufferPool,
  compileEncoder,
  compileDecoder,
  FieldType
};

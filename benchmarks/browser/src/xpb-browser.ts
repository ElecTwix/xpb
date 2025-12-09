/**
 * XPB V2 Browser Runtime
 * Optimized for browser (no Node.js Buffer dependency)
 */

// Compact length constants
const CompactLengthThreshold = 254;
const CompactLengthMarker = 0xFF;

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

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
 * Browser-optimized Decoder
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
    const str = textDecoder.decode(this.data.subarray(this.pos, this.pos + len));
    this.pos += len;
    return str;
  }
}

// ============= JIT Compiler (Browser-optimized) =============

export enum FieldType {
  Bool, Int32, String
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
    var val, str, strLen, i, c, isAscii, lenPos;
  `];

  for (const field of schema.fields) {
    const access = `obj.${field.name}`;
    
    switch (field.type) {
      case FieldType.Bool:
        lines.push(`buf[pos++] = ${access} ? 1 : 0;`);
        break;
      case FieldType.Int32:
        lines.push(`
          val = ${access};
          buf[pos++] = val;
          buf[pos++] = val >> 8;
          buf[pos++] = val >> 16;
          buf[pos++] = val >> 24;
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
  const lines: string[] = [`
    var pos = 0;
    var obj = {};
    var val, len;
  `];

  for (const field of schema.fields) {
    switch (field.type) {
      case FieldType.Bool:
        lines.push(`obj.${field.name} = buf[pos++] !== 0;`);
        break;
      case FieldType.Int32:
        lines.push(`
          obj.${field.name} = buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24);
          pos += 4;
        `);
        break;
      case FieldType.String:
        lines.push(`
          len = buf[pos++];
          obj.${field.name} = textDecoder.decode(buf.subarray(pos, pos + len));
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

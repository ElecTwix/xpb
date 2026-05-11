/**
 * XPB V2 JIT Compiler & Slab Allocator
 * 
 * Generates highly optimized code for V2 format:
 * - Struct mode (no tags, fields in order)
 * - Fixed-width integers (4/8 bytes)
 * - Compact length encoding
 */

import { CompactLengthThreshold, CompactLengthMarker } from './index';

// ============= SLAB ALLOCATOR =============

export class SlabAllocator {
  public buf: Uint8Array;
  public view: DataView;
  public pos: number;
  public size: number;

  constructor(size = 65536) {
    if (typeof Buffer !== 'undefined') {
      this.buf = Buffer.alloc(size);
    } else {
      this.buf = new Uint8Array(size);
    }
    this.view = new DataView(this.buf.buffer, this.buf.byteOffset, this.buf.byteLength);
    this.pos = 0;
    this.size = size;
  }

  reset(): void {
    this.pos = 0;
  }
}

// ============= SCHEMA DEFINITION =============

export enum FieldType {
  Bool,
  Int32,
  Int64, 
  Uint32,
  Uint64,
  Float32,
  Float64,
  String,
  Bytes,
  Message
}

export interface FieldDef {
  tag: number;  // Not used in V2, kept for schema compatibility
  type: FieldType;
  name: string;
  repeated?: boolean;
}

export interface SchemaDef {
  fields: FieldDef[];
}

// ============= CACHED TEXT CODECS =============

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

// Detect Node.js Buffer for fast string operations
const isNode = typeof Buffer !== 'undefined';

// ============= JIT V2 ENCODER =============

export function compileEncoder<T>(schema: SchemaDef): (slab: SlabAllocator, obj: T) => void {
  const lines: string[] = [];
  
  lines.push(`
    var buf = slab.buf;
    var view = slab.view;
    var pos = slab.pos;
    var val, str, strLen, i, c, isAscii, written, lenPos;
  `);

  for (const field of schema.fields) {
    const access = `obj.${field.name}`;
    
    if (field.repeated) {
      // V2: Write count (int32) then elements
      lines.push(`
        var arr = ${access};
        if (arr) {
          // Write count as fixed int32
          buf[pos++] = arr.length;
          buf[pos++] = arr.length >> 8;
          buf[pos++] = arr.length >> 16;
          buf[pos++] = arr.length >> 24;
          
          // Write elements
          for (var ri = 0; ri < arr.length; ri++) {
            val = arr[ri];
            ${generateFieldWrite(field, 'val')}
          }
        } else {
          // Write 0 count
          buf[pos++] = 0; buf[pos++] = 0; buf[pos++] = 0; buf[pos++] = 0;
        }
      `);
    } else {
      lines.push(`
        val = ${access};
        ${generateFieldWrite(field, 'val')}
      `);
    }
  }

  lines.push(`
    slab.pos = pos;
  `);

  // Bind dependencies
  return new Function('textEncoder', 'isNode', 'slab', 'obj', lines.join('\n'))
    .bind(null, textEncoder, isNode) as any;
}

function generateFieldWrite(field: FieldDef, valVar: string): string {
  switch (field.type) {
    case FieldType.Bool:
      return `buf[pos++] = ${valVar} ? 1 : 0;`;
    
    case FieldType.Int32:
    case FieldType.Uint32:
      // Inline fixed32 write (faster than DataView)
      return `
        var v = ${valVar};
        buf[pos++] = v;
        buf[pos++] = v >> 8;
        buf[pos++] = v >> 16;
        buf[pos++] = v >> 24;
      `;
    
    case FieldType.Int64:
    case FieldType.Uint64:
      // Handle both BigInt and Number inputs
      return `
        var v = ${valVar};
        var bv = typeof v === 'bigint' ? v : BigInt(v);
        var lo = Number(bv & 0xffffffffn);
        var hi = Number(bv >> 32n);
        buf[pos++] = lo;
        buf[pos++] = lo >> 8;
        buf[pos++] = lo >> 16;
        buf[pos++] = lo >> 24;
        buf[pos++] = hi;
        buf[pos++] = hi >> 8;
        buf[pos++] = hi >> 16;
        buf[pos++] = hi >> 24;
      `;

    case FieldType.Float32:
      return `
        view.setFloat32(pos, ${valVar}, true);
        pos += 4;
      `;

    case FieldType.Float64:
      return `
        view.setFloat64(pos, ${valVar}, true);
        pos += 8;
      `;

    case FieldType.String:
      // FAST PATH: ASCII optimization + Buffer.write() for Node.js
      return `
        str = ${valVar} || '';
        strLen = str.length;
        
        // Reserve space for compact length (1 byte for short strings)
        lenPos = pos++;
        
        // ASCII Fast Path (< 40 chars)
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
            // Fallback to Buffer.write or TextEncoder
            pos = lenPos + 1;
            if (isNode) {
              written = buf.write(str, pos);
            } else {
              var enc = textEncoder.encodeInto(str, buf.subarray(pos));
              written = enc.written;
            }
            buf[lenPos] = written;
            pos += written;
          }
        } else {
          // Long string: use Buffer.write or TextEncoder
          if (isNode) {
            written = buf.write(str, pos);
          } else {
            var enc = textEncoder.encodeInto(str, buf.subarray(pos));
            written = enc.written;
          }
          buf[lenPos] = written;
          pos += written;
        }
      `;

    default:
      return `// TODO: ${field.type}`;
  }
}

// ============= JIT V2 DECODER =============

/**
 * compileDecoder JITs a decoder for `schema`. The caller MUST supply
 * `maxElements`: it is baked into the compiled function at JIT time, so
 * every repeated-field decode runs through the same caller-supplied-max
 * / negative / buffer-bound checks the runtime `Decoder.readArrayCount`
 * applies. Without this, a wire count of -1 cast through `>>> 0` reads
 * as 4 294 967 295 and `new Array(count)` either pre-allocates a sparse
 * 4 GB slot or — once the loop runs — OOMs the JS heap.
 *
 * `maxElements` is a function-level budget (every repeated field on the
 * schema shares it). Pick the smallest budget that fits your worst-case
 * legitimate payload.
 */
export function compileDecoder<T>(
  schema: SchemaDef,
  maxElements: number,
): (buf: Uint8Array, end: number) => T {
  if (!Number.isInteger(maxElements) || maxElements < 0) {
    throw new RangeError('xpb: compileDecoder requires non-negative integer maxElements');
  }

  const lines: string[] = [];

  // Check if we need a DataView (for floats)
  const hasFloats = schema.fields.some(f => f.type === FieldType.Float32 || f.type === FieldType.Float64);

  lines.push(`
    var pos = 0;
    var obj = {};
    var val, len, first;
    ${hasFloats ? 'var view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);' : ''}
  `);

  // The budget is baked into the function body so the bounds check is a
  // constant compare. JSON.stringify guards against any unintentional
  // escapes if maxElements ever comes from less-trusted code paths.
  const maxLit = JSON.stringify(maxElements);

  for (const field of schema.fields) {
    if (field.repeated) {
      // V2 repeated layout: int32 count + elements. Read the count as
      // SIGNED so `< 0` catches the wire's negative range, then the
      // caller-max check fires before any allocation, then the
      // remaining-buffer check guarantees the per-element reads can
      // actually be satisfied.
      lines.push(`
        {
          var count = buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24);
          pos += 4;
          if (count < 0) {
            throw new Error('xpb: negative array count: ' + count);
          }
          if (count > ${maxLit}) {
            throw new Error('xpb: array count ' + count + ' exceeds caller-supplied max ' + ${maxLit});
          }
          if (count > (end - pos)) {
            throw new Error('xpb: array count ' + count + ' exceeds buffer-bounded max ' + (end - pos));
          }
          obj.${field.name} = new Array(count);
          for (var i = 0; i < count; i++) {
            ${generateFieldRead(field)}
            obj.${field.name}[i] = val;
          }
        }
      `);
    } else {
      lines.push(`
        ${generateFieldRead(field)}
        obj.${field.name} = val;
      `);
    }
  }

  lines.push(`return obj;`);

  return new Function('textDecoder', 'isNode', 'buf', 'end', lines.join('\n'))
    .bind(null, textDecoder, isNode) as any;
}

function generateFieldRead(field: FieldDef): string {
  switch (field.type) {
    case FieldType.Bool:
      return `val = buf[pos++] !== 0;`;
    
    case FieldType.Int32:
      // Inline fixed32 read (faster than DataView)
      return `
        val = buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24);
        pos += 4;
      `;
    
    case FieldType.Uint32:
      return `
        val = (buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24)) >>> 0;
        pos += 4;
      `;
    
    case FieldType.Int64:
    case FieldType.Uint64:
      return `
        var lo = buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24);
        var hi = buf[pos+4] | (buf[pos+5] << 8) | (buf[pos+6] << 16) | (buf[pos+7] << 24);
        val = BigInt(lo >>> 0) | (BigInt(hi >>> 0) << 32n);
        pos += 8;
      `;

    case FieldType.Float32:
      return `
        val = view.getFloat32(pos, true);
        pos += 4;
      `;

    case FieldType.Float64:
      return `
        val = view.getFloat64(pos, true);
        pos += 8;
      `;

    case FieldType.String:
      // V2: Compact length + string bytes. Handles both forms:
      //   short form: 1 byte length when len <= 254
      //   long form:  0xFF marker + 4-byte little-endian uint32 length
      // Missing the long-form branch silently mis-parses every string
      // >= 255 bytes (the JIT decoder reads the 0xFF marker as len=255
      // and consumes 255 bytes starting inside the 4-byte length field).
      return `
        first = buf[pos++];
        if (first === 255) {
          len = (buf[pos] | (buf[pos+1] << 8) | (buf[pos+2] << 16) | (buf[pos+3] << 24)) >>> 0;
          pos += 4;
        } else {
          len = first;
        }

        if (isNode) {
          val = buf.toString('utf8', pos, pos + len);
        } else {
          if (len < 20) {
            var isAscii = true;
            for (var i = 0; i < len; i++) {
              if (buf[pos + i] > 127) { isAscii = false; break; }
            }
            if (isAscii) {
              val = String.fromCharCode.apply(null, buf.subarray(pos, pos + len));
            } else {
              val = textDecoder.decode(buf.subarray(pos, pos + len));
            }
          } else {
            val = textDecoder.decode(buf.subarray(pos, pos + len));
          }
        }
        pos += len;
      `;

    default:
      return `val = null; // TODO: ${field.type}`;
  }
}

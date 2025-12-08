/**
 * XPB JIT Compiler & Slab Allocator
 * 
 * Generates highly optimized, unsafe code for specific schemas.
 * Uses a shared slab allocator to avoid GC.
 */

import { WireType, zigzagEncode32, zigzagDecode32 } from './index';

// ============= SLAB ALLOCATOR =============

export class SlabAllocator {
  public buf: Uint8Array;
  public pos: number;
  public size: number;

  constructor(size = 65536) {
    if (typeof Buffer !== 'undefined') {
      this.buf = Buffer.alloc(size);
    } else {
      this.buf = new Uint8Array(size);
    }
    this.pos = 0;
    this.size = size;
  }

  reset(): void {
    this.pos = 0;
  }

  // Ensure space and return current position
  // In a real system, might cycle buffers. Here we crash or reset for benchmarks.
  // We expose direct property access for JIT speed.
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
  tag: number;
  type: FieldType;
  name: string;
  repeated?: boolean;
}

export interface SchemaDef {
  fields: FieldDef[];
}

// ============= JIT COMPILER =============

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

export interface JITOptions {
  fixedInts?: boolean;
  structMode?: boolean; // No tags, implicit order (like C structs)
  aligned?: boolean;    // Force 4-byte alignment (implies fixedInts + structMode)
  target?: 'node' | 'browser'; // Optimized generation target
}

export function compileEncoder<T>(schema: SchemaDef, opts: JITOptions = {}): (slab: SlabAllocator, obj: T) => void {
  // Aligned implies structMode + fixedInts
  if (opts.aligned) {
    opts.structMode = true;
    opts.fixedInts = true;
  }
  // Auto-detect node if not specified and buffer exists
  if (!opts.target && typeof Buffer !== 'undefined') {
      opts.target = 'node';
  }

  const lines: string[] = [];
  
  lines.push(`
    var buf = slab.buf;
    var pos = slab.pos;
    var val;
  `);

  for (const field of schema.fields) {
    const propAccess = `obj.${field.name}`;
    
    if (field.repeated) {
      lines.push(`
        var arr = ${propAccess};
        if (arr && arr.length > 0) {
          // In Struct Mode, we might technically need a length prefix for the array itself!
          // For this experiment, let's assume "Packed" behavior: 
          // Write count, then elements.
          // BUT standard XPB repeated fields are just repeated tags.
          // In Struct Mode, we really need a count if it's dynamic. 
          // Let's assume for this benchmark we just write them (unsafe for decoding if variable length).
          // To be fair to "Struct Mode", usually there's a length prefix or fixed count.
          // Let's add a length prefix (Int32) for repeated fields in Struct Mode.
          
          ${opts.structMode ? 'buf[pos++] = arr.length; /* Varint count for now, optimized later */' : ''}
          ${opts.fixedInts && opts.structMode ? 
             // If fixed ints, length should be fixed too
             'buf[pos++] = arr.length; buf[pos++] = arr.length >> 8; buf[pos++] = arr.length >> 16; buf[pos++] = arr.length >> 24;'
             : ''}
          
          for (var i = 0; i < arr.length; i++) {
             val = arr[i];
             ${generateFieldWrite(field, 'val', true, opts)}
          }
        } else {
             ${opts.structMode ? '// Struct mode: write 0 length for empty array\n' + (opts.fixedInts ? 'pos += 4; buf.fill(0, pos-4, pos);' : 'buf[pos++] = 0;') : ''}
        }
      `);
    } else {
      lines.push(`
        val = ${propAccess};
        // In StructMode, we MUST write something if missing (default).
        // For benchmarks, inputs are full.
        if (val !== undefined) {
          ${generateFieldWrite(field, 'val', false, opts)}
        } ${opts.structMode ? 'else { /* Missing: Write Zero/Default */ ' + generateDefaultWrite(field, opts) + '}' : ''}
      `);
    }
  }

  lines.push(`
    slab.pos = pos;
  `);

  // Create function with dependencies
  // Arg order: WireType, textEncoder, slab, obj
  // We bind the first two so the returned function is (slab, obj)
  return new Function('WireType', 'textEncoder', 'slab', 'obj', lines.join('\n'))
    .bind(null, WireType, textEncoder) as any;
}

function generateDefaultWrite(field: FieldDef, opts: JITOptions): string {
    // Basic zeroes
    if (opts.fixedInts) {
        if (field.type === FieldType.Int64 || field.type === FieldType.Float64) return 'pos+=8;';
        return 'pos+=4;';
    }
    return 'buf[pos++] = 0;';
}

function generateFieldWrite(field: FieldDef, valVar: string, isRepeated: boolean, opts: JITOptions): string {
  let code = '';
  
  // Tag only if NOT struct mode
  if (!opts.structMode) {
      const tagVarint = (field.tag << 3) | getWireType(field.type);
      const tagBytes = encodeVarint32(tagVarint);
      // Inline tag write
      code += `
        // Tag: ${field.tag} (${field.name})
      `;
      for (const b of tagBytes) {
          code += `buf[pos++] = ${b};\n`;
      }
  }

  // Padding for Aligned mode (before value)
  // This is simplistic; real alignment requires checking pos % 4.
  // Ideally slab.pos is always aligned at start of message, and we just pad.
  if (opts.aligned) {
      code += `while ((pos & 3) !== 0) pos++;\n`;
  }

  switch (field.type) {
    case FieldType.Bool:
      if (opts.fixedInts) {
          // write 4 bytes
          return code + `buf[pos++] = ${valVar} ? 1 : 0; buf[pos++] = 0; buf[pos++] = 0; buf[pos++] = 0;`;
      }
      return code + `buf[pos++] = ${valVar} ? 1 : 0;`;
    
    case FieldType.Int32:
    case FieldType.Uint32:
      if (opts.fixedInts) {
          // Fixed 32
          return code + `
            var v = ${valVar};
            buf[pos++] = v;
            buf[pos++] = v >> 8;
            buf[pos++] = v >> 16;
            buf[pos++] = v >> 24;
          `;
      }
      
      // Inline Zigzag + Varint (Optimistic)
      return code + `
        var z = (${valVar} << 1) ^ (${valVar} >> 31);
        if (z < 128) {
          buf[pos++] = z;
        } else {
          while (z >= 0x80) {
            buf[pos++] = (z & 0x7f) | 0x80;
            z >>>= 7;
          }
          buf[pos++] = z;
        }
      `;
      
    case FieldType.Int64:
    case FieldType.Uint64:
      // BigInt
       if (opts.fixedInts) {
          // Fixed 64
          return code + `
            var v = ${valVar};
            var lo = Number(v & 0xffffffffn);
            var hi = Number(v >> 32n);
            buf[pos++] = lo;
            buf[pos++] = lo >> 8;
            buf[pos++] = lo >> 16;
            buf[pos++] = lo >> 24;
            buf[pos++] = hi;
            buf[pos++] = hi >> 8;
            buf[pos++] = hi >> 16;
            buf[pos++] = hi >> 24;
          `;
       }
       return code + `
        var z = (${valVar} << 1n) ^ (${valVar} >> 63n);
        if (z < 128n) {
          buf[pos++] = Number(z);
        } else {
          while (z >= 0x80n) {
            buf[pos++] = Number((z & 0x7fn) | 0x80n);
            z >>= 7n;
          }
          buf[pos++] = Number(z);
        }
      `;

    case FieldType.String:
       // String handling
       // Standard: Varint Length + Bytes
       // Fixed/Struct: Still need length! Strings are variable.
       // Aligned: Length (Fixed32) + Bytes + Padding.
       
       // Optimistic Length Write for String
       // 1. Write Tag (done)
       // 2. Reserve 1 byte for length (common case)
       // 3. EncodeInto
       // 4. Fixup length if needed
       
       let lenWriteCode = '';
       if (opts.fixedInts) {
           lenWriteCode = `
             var lenPos = pos;
             pos += 4; // Reserve Fixed32 length
           `;
           
           // ASCII Optimization for Fixed Ints (Struct Mode)
           return code + `
             // String: ${field.name}
             ${lenWriteCode}
             
             var str = ${valVar};
             var strLen = str.length;
             var written = 0;
             
             // ASCII Fast Path (Heuristic < 40 chars)
             if (strLen < 40) {
                 var isAscii = true;
                 for (var i = 0; i < strLen; i++) {
                     var c = str.charCodeAt(i);
                     if (c > 127) { isAscii = false; break; }
                     buf[pos + i] = c;
                 }
                 if (isAscii) {
                     written = strLen;
                     pos += strLen;
                 } else {
                     // Fallback
                     ${opts.target === 'node' ? 'written = buf.write(str, pos);' : 'var res = textEncoder.encodeInto(str, buf.subarray(pos)); written = res.written;'}
                     pos += written;
                 }
             } else {
                 ${opts.target === 'node' ? 'written = buf.write(str, pos);' : 'var res = textEncoder.encodeInto(str, buf.subarray(pos)); written = res.written;'}
                 pos += written;
             }
             
             // Write Length back
             buf[lenPos] = written;
             buf[lenPos+1] = written >> 8;
             buf[lenPos+2] = written >> 16;
             buf[lenPos+3] = written >> 24;
             
             ${opts.aligned ? 'while ((pos & 3) !== 0) buf[pos++] = 0;' : ''}
           `;
       }

       // Standard implementation (with optimistic update)
       // We can also apply ASCII path here for standard varints
       return code + `
          // String: ${field.name}
          // Note: Tag written above if needed.
          
          var str = ${valVar};
          var strLen = str.length;
          
          if (strLen < 40) {
              // Try ASCII
              var isAscii = true;
              var savePos = pos;
              // Reserve 1 byte len
              pos++; 
              
              for (var i = 0; i < strLen; i++) {
                  var c = str.charCodeAt(i);
                  if (c > 127) { isAscii = false; break; }
                  buf[pos + i] = c;
              }
              
              if (isAscii) {
                  buf[savePos] = strLen;
                  pos += strLen;
              } else {
                  // Fallback: Reset and use standard
                  pos = savePos;
                  ${opts.target === 'node' ? 
                  `var written = buf.write(str, pos + 1);` : 
                  `var res = textEncoder.encodeInto(str, buf.subarray(pos + 1));
                   var written = res.written;`
                  }
                  
                  if (written < 128) {
                     buf[pos] = written;
                     pos += written + 1;
                  } else {
                     // Fallback deep
                     var lenBytes = 0;
                     var t = written;
                     while(t >= 0x80) { t >>= 7; lenBytes++; }
                     lenBytes++; 
                     
                     buf.copyWithin(pos + lenBytes, pos + 1, pos + 1 + written);
                     
                     var l = written;
                     while (l >= 0x80) {
                        buf[pos++] = (l & 0x7f) | 0x80;
                        l >>>= 7;
                     }
                     buf[pos++] = l;
                     pos += written;
                  }
              }
          } else {
              // Long string standard path
              ${opts.target === 'node' ? 
              `var written = buf.write(str, pos + 1);` : 
              `var res = textEncoder.encodeInto(str, buf.subarray(pos + 1));
               var written = res.written;`
              }
              
              // ... same fallback logic ...
              if (written < 128) {
                 buf[pos] = written;
                 pos += written + 1;
              } else {
                 var lenBytes = 0;
                 var t = written;
                 while(t >= 0x80) { t >>= 7; lenBytes++; }
                 lenBytes++; 
                 
                 buf.copyWithin(pos + lenBytes, pos + 1, pos + 1 + written);
                 
                 var l = written;
                 while (l >= 0x80) {
                    buf[pos++] = (l & 0x7f) | 0x80;
                    l >>>= 7;
                 }
                 buf[pos++] = l;
                 pos += written;
              }
          }
       `;

    default:
        // TODO: Implement other types
        return `// TODO: ${field.type}`;
  }
}

export function compileDecoder<T>(schema: SchemaDef): (buf: Uint8Array, end: number) => T {
    // A simplified JIT decoder is extremely complex to get right with jumps/switch.
    // For now, let's optimize the Encoder first as per "make small messages faster".
    // We can just call UnsafeDecoder for decoding if needed, or implement a basic switch JIT.
    
    // We'll implement a basic one to close the loop.
    
    const lines: string[] = [];
    lines.push(`
      var pos = 0;
      var obj = {};
      var tag, wire, val;
    `);

    // We use a while loop and a switch
    lines.push(`while (pos < end) {`);
    
    // Inline Read Tag
    lines.push(`
      var b = buf[pos++];
      tag = b & 0x7f;
      if (b >= 0x80) {
        b = buf[pos++];
        tag |= (b & 0x7f) << 7;
        // Assume < 128 field IDs for speed
      }
      wire = tag & 7;
      tag = tag >>> 3;
    `);

    lines.push(`switch(tag) {`);

    for (const field of schema.fields) {
        lines.push(`case ${field.tag}:`);
        
        const assignment = field.repeated 
            ? `if (!obj.${field.name}) obj.${field.name} = []; obj.${field.name}.push(val);` 
            : `obj.${field.name} = val;`;

        switch (field.type) {
            case FieldType.String:
                lines.push(`
                   // Inline Read String
                   var len = 0, shift = 0;
                   while(true) {
                      b = buf[pos++];
                      len |= (b & 0x7f) << shift;
                      if ((b & 0x80) === 0) break;
                      shift += 7;
                   }
                   val = textDecoder.decode(buf.subarray(pos, pos + len));
                   pos += len;
                   ${assignment}
                `);
                break;
            case FieldType.Int32:
                 lines.push(`
                   // Inline Read Int32
                   var v = 0, shift = 0;
                   while(true) {
                      b = buf[pos++];
                      v |= (b & 0x7f) << shift;
                      if ((b & 0x80) === 0) break;
                      shift += 7;
                   }
                   // Zigzag
                   val = (v >>> 1) ^ -(v & 1);
                   ${assignment}
                 `);
                 break;
            case FieldType.Bool:
                 lines.push(`
                    b = buf[pos++];
                    // if (b >= 80) ... handle varint bool ...
                    val = b !== 0;
                    ${assignment}
                 `);
                 break;
            default:
                 lines.push(`// Skip unknown`);
        }
        lines.push(`break;`);
    }

    lines.push(`default: // Skip unknown
       // minimal check
       if (wire === 0) { while(buf[pos++] >= 0x80); }
       else if (wire === 2) { 
           var len = 0, shift = 0;
           while(true) {
              b = buf[pos++];
              len |= (b & 0x7f) << shift;
              if ((b & 0x80) === 0) break;
              shift += 7;
           }
           pos += len;
       }
       // ... other wires
    `);

    lines.push(`}`); // end switch
    lines.push(`}`); // end while
    lines.push(`return obj;`);

    return new Function('WireType', 'textDecoder', 'buf', 'end', lines.join('\n'))
        .bind(null, WireType, textDecoder) as any;
}


// --- Helpers ---

function getWireType(t: FieldType): number {
    switch (t) {
        case FieldType.Int32: 
        case FieldType.Bool:
        case FieldType.Int64:
        case FieldType.Uint32:
        case FieldType.Uint64:
            return WireType.Varint;
        case FieldType.String:
        case FieldType.Bytes:
        case FieldType.Message:
            return WireType.LengthDelimited;
        case FieldType.Float32:
            return WireType.Fixed32;
        case FieldType.Float64:
            return WireType.Fixed64;
    }
    return 0;
}

function encodeVarint32(v: number): number[] {
    const bytes: number[] = [];
    v = v >>> 0;
    while (v >= 0x80) {
        bytes.push((v & 0x7f) | 0x80);
        v >>>= 7;
    }
    bytes.push(v);
    return bytes;
}

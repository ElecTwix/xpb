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
    this.buf = new Uint8Array(size);
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

export function compileEncoder<T>(schema: SchemaDef): (slab: SlabAllocator, obj: T) => void {
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
          for (var i = 0; i < arr.length; i++) {
             val = arr[i];
             ${generateFieldWrite(field, 'val', true)}
          }
        }
      `);
    } else {
      lines.push(`
        val = ${propAccess};
        if (val !== undefined) {
          ${generateFieldWrite(field, 'val', false)}
        }
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

function generateFieldWrite(field: FieldDef, valVar: string, isRepeated: boolean): string {
  const tagVarint = (field.tag << 3) | getWireType(field.type);
  const tagBytes = encodeVarint32(tagVarint);
  
  // Inline tag write
  let code = `
    // Tag: ${field.tag} (${field.name})
  `;
  for (const b of tagBytes) {
      code += `buf[pos++] = ${b};\n`;
  }

  switch (field.type) {
    case FieldType.Bool:
      return code + `buf[pos++] = ${valVar} ? 1 : 0;`;
    
    case FieldType.Int32:
      // Inline Zigzag + Varint
      return code + `
        var z = (${valVar} << 1) ^ (${valVar} >> 31);
        while (z >= 0x80) {
          buf[pos++] = (z & 0x7f) | 0x80;
          z >>>= 7;
        }
        buf[pos++] = z;
      `;
      
    case FieldType.Int64:
      // BigInt Zigzag + Varint
       return code + `
        var z = (${valVar} << 1n) ^ (${valVar} >> 63n);
        while (z >= 0x80n) {
          buf[pos++] = Number((z & 0x7fn) | 0x80n);
          z >>= 7n;
        }
        buf[pos++] = Number(z);
      `;

    case FieldType.String:
       // Optimistic Length Write for String
       // 1. Write Tag (done)
       // 2. Reserve 1 byte for length (common case)
       // 3. EncodeInto
       // 4. Fixup length if needed
       return `
          // String: ${field.name}
          ${code} // Write Tag
          
          // Assume length < 128 (1 byte varint)
          // We don't check buffer size here (Unsafe!)
          var res = textEncoder.encodeInto(${valVar}, buf.subarray(pos + 1));
          var written = res.written;
          
          if (written < 128) {
             buf[pos] = written;
             pos += written + 1;
          } else {
             // Fallback for long strings: shift data and write real varint
             var lenBytes = 0;
             var t = written;
             while(t >= 0x80) { t >>= 7; lenBytes++; }
             lenBytes++; // last byte
             
             // Move data
             buf.copyWithin(pos + lenBytes, pos + 1, pos + 1 + written);
             
             // Write varint length
             var l = written;
             while (l >= 0x80) {
                buf[pos++] = (l & 0x7f) | 0x80;
                l >>>= 7;
             }
             buf[pos++] = l;
             pos += written;
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

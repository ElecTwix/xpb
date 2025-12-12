import { Decoder, SizeInt32, SizeFloat64, SizeBool } from './index';

export enum FieldType {
  Bool, Int32, Int64, Uint32, Uint64, Float32, Float64, String
}

export interface FieldDef {
  type: FieldType;
  name: string;
}

export interface SchemaDef {
  fields: FieldDef[];
}

/**
 * LazyDecoder - Decodes fields on-demand using a Proxy.
 * 
 * Performance:
 * - O(1) creation time (vs O(N) for JSON.parse)
 * - O(1) field access
 * - Ideal for: Large messages where you only need a few fields.
 * - Ideal for: Virtual scrolling (rendering only visible items).
 */
export class LazyDecoder {
  private buffer: Uint8Array;
  private schema: SchemaDef;
  private offsets: Map<string, number> | null = null;
  private startPos: number;

  constructor(buffer: Uint8Array, schema: SchemaDef, startPos = 0) {
    this.buffer = buffer;
    this.schema = schema;
    this.startPos = startPos;
  }

  /**
   * Scan the buffer to find field offsets.
   * This is much faster than decoding values.
   */
  private scan(): void {
    if (this.offsets) return;
    this.offsets = new Map();
    
    // Create a temporary decoder just for scanning
    // We manually advance pos to avoid object allocation overhead
    let pos = this.startPos;
    const view = new DataView(this.buffer.buffer, this.buffer.byteOffset, this.buffer.byteLength);

    for (const field of this.schema.fields) {
      this.offsets.set(field.name, pos);

      switch (field.type) {
        case FieldType.Bool:
          pos += 1;
          break;
        case FieldType.Int32:
        case FieldType.Uint32:
        case FieldType.Float32:
          pos += 4;
          break;
        case FieldType.Int64:
        case FieldType.Uint64:
        case FieldType.Float64:
          pos += 8;
          break;
        case FieldType.String: {
          // Read length
          const b = this.buffer[pos++];
          let len = b;
          if (b === 0xFF) {
            len = view.getUint32(pos, true);
            pos += 4;
            // Adjustment: The offset stored was for the LENGTH prefix.
            // But for string, we might want the DATA offset?
            // No, the decoder needs the length.
          }
          pos += len;
          break;
        }
      }
    }
  }

  /**
   * Create a Proxy object that looks like the decoded object.
   */
  createProxy(): any {
    // Scan first (eager scan, lazy decode)
    // Scan is fast because it just skips bytes.
    this.scan();

    const self = this;
    const decoder = new Decoder(this.buffer); // Re-use one decoder? Or create new?
    // Creating a new decoder is cheap (just a view).

    return new Proxy({}, {
      get(target, prop) {
        if (typeof prop !== 'string') return undefined;
        
        const offset = self.offsets!.get(prop);
        if (offset === undefined) return undefined;

        // Create a decoder at the specific offset
        // We cheat and just make a new decoder on the slice?
        // Or adding a setPos() method to Decoder would be better.
        // For now, slice is cheap-ish (creates view).
        const subDecoder = new Decoder(self.buffer.subarray(offset));
        
        // Find field type
        const field = self.schema.fields.find(f => f.name === prop)!;

        switch (field.type) {
          case FieldType.Bool: return subDecoder.readBool();
          case FieldType.Int32: return subDecoder.readInt32();
          case FieldType.Uint32: return subDecoder.readUint32();
          case FieldType.Int64: return subDecoder.readInt64();
          case FieldType.Uint64: return subDecoder.readUint64();
          case FieldType.Float32: return subDecoder.readFloat32();
          case FieldType.Float64: return subDecoder.readFloat64();
          case FieldType.String: return subDecoder.readString();
        }
      }
    });
  }
}

/**
 * Decode an array of objects lazily.
 * Returns a ProxyArray that decodes items on index access.
 */
export class LazyArrayDecoder {
  private buffer: Uint8Array;
  private schema: SchemaDef;
  private itemOffsets: Uint32Array; // Stores start offset of each item
  private count: number;

  constructor(buffer: Uint8Array, schema: SchemaDef) {
    this.buffer = buffer;
    this.schema = schema;
    
    // 1. Read count
    const view = new DataView(buffer.buffer, buffer.byteOffset, buffer.byteLength);
    this.count = view.getInt32(0, true);
    
    // 2. Scan all items to build index
    // This is the "cost" we pay. But it's faster than JSON.parse because we don't decode strings/create objects.
    this.itemOffsets = new Uint32Array(this.count);
    let pos = 4; // Skip count
    
    // Scan loop
    for (let i = 0; i < this.count; i++) {
      this.itemOffsets[i] = pos;
      
      // Skip over one object
      for (const field of schema.fields) {
        switch (field.type) {
          case FieldType.Bool: pos += 1; break;
          case FieldType.Int32:
          case FieldType.Uint32:
          case FieldType.Float32: pos += 4; break;
          case FieldType.Int64:
          case FieldType.Uint64:
          case FieldType.Float64: pos += 8; break;
          case FieldType.String: {
            const b = buffer[pos++];
            let len = b;
            if (b === 0xFF) {
              len = view.getUint32(pos, true);
              pos += 4;
            }
            pos += len;
            break;
          }
        }
      }
    }
  }

  get(index: number): any {
    if (index < 0 || index >= this.count) return undefined;
    const offset = this.itemOffsets[index];
    const lazy = new LazyDecoder(this.buffer, this.schema, offset);
    return lazy.createProxy();
  }

  get length(): number {
    return this.count;
  }
}

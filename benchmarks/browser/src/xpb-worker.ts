/**
 * XPB V2 Web Worker for Decoding
 * 
 * This worker handles XPB decoding in a separate thread to avoid blocking the main thread.
 * It's especially useful for large payloads (>10KB).
 */

// TextDecoder for UTF-8 string decoding
const textDecoder = new TextDecoder();

// ============= Decoder Implementation =============

class Decoder {
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

  readUint32(): number {
    const v = this.view.getUint32(this.pos, true);
    this.pos += 4;
    return v;
  }

  readInt64(): bigint {
    const lo = this.view.getUint32(this.pos, true);
    const hi = this.view.getInt32(this.pos + 4, true);
    this.pos += 8;
    return BigInt(lo) | (BigInt(hi) << 32n);
  }

  readFloat32(): number {
    const v = this.view.getFloat32(this.pos, true);
    this.pos += 4;
    return v;
  }

  readFloat64(): number {
    const v = this.view.getFloat64(this.pos, true);
    this.pos += 8;
    return v;
  }

  readString(): string {
    const len = this.data[this.pos++];
    
    // Unrolled short string handling
    if (len === 0) {
      return '';
    } else if (len === 1) {
      return String.fromCharCode(this.data[this.pos++]);
    } else if (len === 2) {
      const s = String.fromCharCode(this.data[this.pos], this.data[this.pos+1]);
      this.pos += 2;
      return s;
    } else if (len === 3) {
      const s = String.fromCharCode(this.data[this.pos], this.data[this.pos+1], this.data[this.pos+2]);
      this.pos += 3;
      return s;
    } else if (len === 4) {
      const s = String.fromCharCode(this.data[this.pos], this.data[this.pos+1], this.data[this.pos+2], this.data[this.pos+3]);
      this.pos += 4;
      return s;
    } else if (len === 5) {
      const s = String.fromCharCode(this.data[this.pos], this.data[this.pos+1], this.data[this.pos+2], this.data[this.pos+3], this.data[this.pos+4]);
      this.pos += 5;
      return s;
    } else if (len === 6) {
      const s = String.fromCharCode(this.data[this.pos], this.data[this.pos+1], this.data[this.pos+2], this.data[this.pos+3], this.data[this.pos+4], this.data[this.pos+5]);
      this.pos += 6;
      return s;
    } else if (len === 7) {
      const s = String.fromCharCode(this.data[this.pos], this.data[this.pos+1], this.data[this.pos+2], this.data[this.pos+3], this.data[this.pos+4], this.data[this.pos+5], this.data[this.pos+6]);
      this.pos += 7;
      return s;
    } else if (len === 8) {
      const s = String.fromCharCode(this.data[this.pos], this.data[this.pos+1], this.data[this.pos+2], this.data[this.pos+3], this.data[this.pos+4], this.data[this.pos+5], this.data[this.pos+6], this.data[this.pos+7]);
      this.pos += 8;
      return s;
    } else if (len <= 16) {
      let s = String.fromCharCode(this.data[this.pos], this.data[this.pos+1], this.data[this.pos+2], this.data[this.pos+3], this.data[this.pos+4], this.data[this.pos+5], this.data[this.pos+6], this.data[this.pos+7]);
      for (let i = 8; i < len; i++) s += String.fromCharCode(this.data[this.pos + i]);
      this.pos += len;
      return s;
    }
    
    // For longer strings, TextDecoder is optimized
    const str = textDecoder.decode(this.data.subarray(this.pos, this.pos + len));
    this.pos += len;
    return str;
  }
}

// ============= Field Types =============

const FieldType = {
  Bool: 0,
  Int32: 1,
  Int64: 2,
  Uint32: 3,
  Uint64: 4,
  Float32: 5,
  Float64: 6,
  String: 7,
};

// ============= Schema-based Decoding =============

interface FieldDef {
  tag: number;
  type: number;
  name: string;
}

interface SchemaDef {
  fields: FieldDef[];
}

function decodeWithSchema(buffer: Uint8Array, schema: SchemaDef): any {
  const decoder = new Decoder(buffer);
  const obj: any = {};
  
  for (const field of schema.fields) {
    switch (field.type) {
      case FieldType.Bool:
        obj[field.name] = decoder.readBool();
        break;
      case FieldType.Int32:
        obj[field.name] = decoder.readInt32();
        break;
      case FieldType.Uint32:
        obj[field.name] = decoder.readUint32();
        break;
      case FieldType.Int64:
      case FieldType.Uint64:
        obj[field.name] = decoder.readInt64();
        break;
      case FieldType.Float32:
        obj[field.name] = decoder.readFloat32();
        break;
      case FieldType.Float64:
        obj[field.name] = decoder.readFloat64();
        break;
      case FieldType.String:
        obj[field.name] = decoder.readString();
        break;
    }
  }
  
  return obj;
}

// ============= Collection Decoding =============

function decodeStringArray(buffer: Uint8Array): string[] {
  const decoder = new Decoder(buffer);
  const count = decoder.readInt32();
  const result: string[] = new Array(count);
  for (let i = 0; i < count; i++) {
    result[i] = decoder.readString();
  }
  return result;
}

function decodeInt32Array(buffer: Uint8Array): number[] {
  const decoder = new Decoder(buffer);
  const count = decoder.readInt32();
  const result: number[] = new Array(count);
  for (let i = 0; i < count; i++) {
    result[i] = decoder.readInt32();
  }
  return result;
}

function decodeStringMap(buffer: Uint8Array): Record<string, string> {
  const decoder = new Decoder(buffer);
  const count = decoder.readInt32();
  const result: Record<string, string> = {};
  for (let i = 0; i < count; i++) {
    const key = decoder.readString();
    const value = decoder.readString();
    result[key] = value;
  }
  return result;
}

// ============= Worker Message Handler =============

interface DecodeRequest {
  id: number;
  type: 'schema' | 'stringArray' | 'int32Array' | 'stringMap';
  buffer: ArrayBuffer;
  schema?: SchemaDef;
}

interface DecodeResponse {
  id: number;
  result?: any;
  error?: string;
}

self.onmessage = (event: MessageEvent<DecodeRequest>) => {
  const { id, type, buffer, schema } = event.data;
  
  try {
    const data = new Uint8Array(buffer);
    let result: any;
    
    switch (type) {
      case 'schema':
        if (!schema) throw new Error('Schema required for schema decode');
        result = decodeWithSchema(data, schema);
        break;
      case 'stringArray':
        result = decodeStringArray(data);
        break;
      case 'int32Array':
        result = decodeInt32Array(data);
        break;
      case 'stringMap':
        result = decodeStringMap(data);
        break;
      default:
        throw new Error(`Unknown decode type: ${type}`);
    }
    
    const response: DecodeResponse = { id, result };
    self.postMessage(response);
  } catch (e) {
    const response: DecodeResponse = { id, error: (e as Error).message };
    self.postMessage(response);
  }
};

// Signal worker is ready
self.postMessage({ type: 'ready' });

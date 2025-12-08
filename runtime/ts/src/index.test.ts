import { describe, it, expect } from 'vitest';
import { Encoder, Decoder, WireType, zigzagEncode32, zigzagDecode32 } from '../src/index';

describe('Encoder/Decoder', () => {
  it('should encode and decode boolean', () => {
    const enc = new Encoder();
    enc.writeBool(1, true);
    enc.writeBool(2, false);
    const data = enc.finish();

    const dec = new Decoder(data);
    const [fn1, wt1] = dec.readTag();
    expect(fn1).toBe(1);
    expect(wt1).toBe(WireType.Varint);
    expect(dec.readBool()).toBe(true);

    const [fn2] = dec.readTag();
    expect(fn2).toBe(2);
    expect(dec.readBool()).toBe(false);
  });

  it('should encode and decode int32', () => {
    const enc = new Encoder();
    enc.writeInt32(1, 42);
    enc.writeInt32(2, -42);
    const data = enc.finish();

    const dec = new Decoder(data);
    dec.readTag();
    expect(dec.readInt32()).toBe(42);
    dec.readTag();
    expect(dec.readInt32()).toBe(-42);
  });

  it('should encode and decode string', () => {
    const enc = new Encoder();
    enc.writeString(1, 'hello xpb');
    enc.writeString(2, '');
    const data = enc.finish();

    const dec = new Decoder(data);
    dec.readTag();
    expect(dec.readString()).toBe('hello xpb');
    dec.readTag();
    expect(dec.readString()).toBe('');
  });

  it('should encode and decode bytes', () => {
    const enc = new Encoder();
    const bytes = new Uint8Array([0xDE, 0xAD, 0xBE, 0xEF]);
    enc.writeBytes(1, bytes);
    const data = enc.finish();

    const dec = new Decoder(data);
    dec.readTag();
    const decoded = dec.readBytes();
    expect(decoded).toEqual(bytes);
  });

  it('should encode and decode float32', () => {
    const enc = new Encoder();
    enc.writeFloat32(1, 3.14);
    const data = enc.finish();

    const dec = new Decoder(data);
    const [fn, wt] = dec.readTag();
    expect(fn).toBe(1);
    expect(wt).toBe(WireType.Fixed32);
    const value = dec.readFloat32();
    expect(Math.abs(value - 3.14)).toBeLessThan(0.001);
  });

  it('should encode and decode float64', () => {
    const enc = new Encoder();
    enc.writeFloat64(1, 2.718281828);
    const data = enc.finish();

    const dec = new Decoder(data);
    dec.readTag();
    expect(dec.readFloat64()).toBe(2.718281828);
  });

  it('should encode and decode uint64', () => {
    const enc = new Encoder();
    enc.writeUint64(1, 18446744073709551615n);
    const data = enc.finish();

    const dec = new Decoder(data);
    dec.readTag();
    expect(dec.readUint64()).toBe(18446744073709551615n);
  });

  it('should skip unknown fields', () => {
    const enc = new Encoder();
    enc.writeString(1, 'known');
    enc.writeInt32(2, 42); // unknown
    enc.writeString(3, 'also known');
    const data = enc.finish();

    const dec = new Decoder(data);
    const results: Record<number, string> = {};
    while (!dec.eof()) {
      const [fn, wt] = dec.readTag();
      if (fn === 1 || fn === 3) {
        results[fn] = dec.readString();
      } else {
        dec.skip(wt);
      }
    }
    expect(results[1]).toBe('known');
    expect(results[3]).toBe('also known');
    expect(results[2]).toBeUndefined();
  });

  it('should handle nested messages', () => {
    // Encode inner
    const innerEnc = new Encoder();
    innerEnc.writeString(1, 'inner value');
    const innerData = innerEnc.finish();

    // Encode outer with nested
    const enc = new Encoder();
    enc.writeString(1, 'outer');
    enc.writeMessage(2, innerData);
    const data = enc.finish();

    // Decode
    const dec = new Decoder(data);
    dec.readTag();
    const outer = dec.readString();
    dec.readTag();
    const nestedData = dec.readMessageBytes();
    const nestedDec = new Decoder(nestedData);
    nestedDec.readTag();
    const inner = nestedDec.readString();

    expect(outer).toBe('outer');
    expect(inner).toBe('inner value');
  });
});

describe('Zigzag encoding', () => {
  it('should encode positive numbers', () => {
    expect(zigzagEncode32(0)).toBe(0);
    expect(zigzagEncode32(1)).toBe(2);
    expect(zigzagEncode32(2)).toBe(4);
  });

  it('should encode negative numbers', () => {
    expect(zigzagEncode32(-1)).toBe(1);
    expect(zigzagEncode32(-2)).toBe(3);
  });

  it('should round-trip', () => {
    const values = [0, 1, -1, 127, -128, 32767, -32768];
    for (const v of values) {
      const encoded = zigzagEncode32(v);
      const decoded = zigzagDecode32(encoded);
      expect(decoded).toBe(v);
    }
  });
});

describe('Benchmark-style tests', () => {
  it('should encode simple message compactly', () => {
    const enc = new Encoder();
    enc.writeString(1, 'Alice');
    enc.writeInt32(2, 30);
    enc.writeBool(3, true);
    const data = enc.finish();

    // Should be compact - around 11-15 bytes
    expect(data.length).toBeLessThan(20);
    console.log(`Simple message: ${data.length} bytes`);
  });

  it('should handle repeated fields', () => {
    const enc = new Encoder();
    const tags = ['go', 'typescript', 'xpb'];
    for (const tag of tags) {
      enc.writeString(1, tag);
    }
    const data = enc.finish();

    const dec = new Decoder(data);
    const decoded: string[] = [];
    while (!dec.eof()) {
      const [fn, wt] = dec.readTag();
      if (fn === 1) {
        decoded.push(dec.readString());
      } else {
        dec.skip(wt);
      }
    }

    expect(decoded).toEqual(tags);
    console.log(`Repeated ${tags.length} strings: ${data.length} bytes`);
  });
});

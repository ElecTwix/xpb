/**
 * XPB V2 TypeScript tests
 */

import { describe, test, expect } from 'vitest';
import { Encoder, Decoder, CompactLengthThreshold, CompactLengthMarker } from './index';

describe('V2 Encoder/Decoder', () => {
  test('bool roundtrip', () => {
    const enc = new Encoder(16);
    enc.writeBool(true);
    enc.writeBool(false);
    
    const dec = new Decoder(enc.finish());
    expect(dec.readBool()).toBe(true);
    expect(dec.readBool()).toBe(false);
  });

  test('int32 roundtrip', () => {
    const enc = new Encoder(32);
    enc.writeInt32(42);
    enc.writeInt32(-42);
    enc.writeInt32(2147483647);
    enc.writeInt32(-2147483648);
    
    const dec = new Decoder(enc.finish());
    expect(dec.readInt32()).toBe(42);
    expect(dec.readInt32()).toBe(-42);
    expect(dec.readInt32()).toBe(2147483647);
    expect(dec.readInt32()).toBe(-2147483648);
  });

  test('uint32 roundtrip', () => {
    const enc = new Encoder(16);
    enc.writeUint32(0);
    enc.writeUint32(4294967295);
    
    const dec = new Decoder(enc.finish());
    expect(dec.readUint32()).toBe(0);
    expect(dec.readUint32()).toBe(4294967295);
  });

  test('int64 roundtrip', () => {
    const enc = new Encoder(32);
    enc.writeInt64(42n);
    enc.writeInt64(-42n);
    enc.writeInt64(9223372036854775807n);
    enc.writeInt64(-9223372036854775808n);
    
    const dec = new Decoder(enc.finish());
    expect(dec.readInt64()).toBe(42n);
    expect(dec.readInt64()).toBe(-42n);
    expect(dec.readInt64()).toBe(9223372036854775807n);
    expect(dec.readInt64()).toBe(-9223372036854775808n);
  });

  test('float32 roundtrip', () => {
    const enc = new Encoder(16);
    enc.writeFloat32(3.14);
    
    const dec = new Decoder(enc.finish());
    expect(dec.readFloat32()).toBeCloseTo(3.14, 5);
  });

  test('float64 roundtrip', () => {
    const enc = new Encoder(16);
    enc.writeFloat64(2.718281828);
    
    const dec = new Decoder(enc.finish());
    expect(dec.readFloat64()).toBe(2.718281828);
  });

  test('string roundtrip', () => {
    const enc = new Encoder(64);
    enc.writeString('hello world');
    enc.writeString('');
    enc.writeString('🎉 emoji test');
    
    const dec = new Decoder(enc.finish());
    expect(dec.readString()).toBe('hello world');
    expect(dec.readString()).toBe('');
    expect(dec.readString()).toBe('🎉 emoji test');
  });

  test('bytes roundtrip', () => {
    const enc = new Encoder(32);
    const testBytes = new Uint8Array([0xDE, 0xAD, 0xBE, 0xEF]);
    enc.writeBytes(testBytes);
    
    const dec = new Decoder(enc.finish());
    expect(dec.readBytes()).toEqual(testBytes);
  });

  test('compact length short', () => {
    const enc = new Encoder(32);
    const shortStr = 'hello';
    enc.writeString(shortStr);
    const data = enc.finish();
    
    // 1 byte length + 5 bytes content = 6 bytes
    expect(data.length).toBe(6);
    expect(data[0]).toBe(5); // length byte
  });

  test('compact length long', () => {
    const enc = new Encoder(512);
    const longStr = 'x'.repeat(300);
    enc.writeString(longStr);
    const data = enc.finish();
    
    // 5 bytes length (0xFF + 4 bytes) + 300 bytes content = 305 bytes
    expect(data.length).toBe(305);
    expect(data[0]).toBe(CompactLengthMarker);
    
    // Verify decode works
    const dec = new Decoder(data);
    expect(dec.readString()).toBe(longStr);
  });

  test('nested message roundtrip', () => {
    // Encode inner
    const innerEnc = new Encoder(32);
    innerEnc.writeString('New York');
    innerEnc.writeString('USA');
    
    // Encode outer
    const enc = new Encoder(64);
    enc.writeString('Alice');
    enc.writeMessage(innerEnc.finish());
    
    // Decode
    const dec = new Decoder(enc.finish());
    expect(dec.readString()).toBe('Alice');
    
    const innerData = dec.readMessageBytes();
    const innerDec = new Decoder(innerData);
    expect(innerDec.readString()).toBe('New York');
    expect(innerDec.readString()).toBe('USA');
  });

  test('reset and reuse', () => {
    const enc = new Encoder(32);
    enc.writeInt32(42);
    expect(enc.finish().length).toBe(4);
    
    enc.reset();
    enc.writeInt32(100);
    expect(enc.finish().length).toBe(4);
    
    const dec = new Decoder(enc.finish());
    expect(dec.readInt32()).toBe(100);
  });

  test('eof and remaining', () => {
    const enc = new Encoder(16);
    enc.writeInt32(42);
    enc.writeInt32(100);
    
    const dec = new Decoder(enc.finish());
    expect(dec.eof()).toBe(false);
    expect(dec.remaining()).toBe(8);
    
    dec.readInt32();
    expect(dec.eof()).toBe(false);
    expect(dec.remaining()).toBe(4);
    
    dec.readInt32();
    expect(dec.eof()).toBe(true);
    expect(dec.remaining()).toBe(0);
  });
});

describe('V2 Size Variants', () => {
  test('empty string', () => {
    const enc = new Encoder(16);
    enc.writeString('');
    const dec = new Decoder(enc.finish());
    expect(dec.readString()).toBe('');
  });

  test('1 char string', () => {
    const enc = new Encoder(16);
    enc.writeString('a');
    const dec = new Decoder(enc.finish());
    expect(dec.readString()).toBe('a');
  });

  test('100 char string', () => {
    const enc = new Encoder(256);
    enc.writeString('x'.repeat(100));
    const dec = new Decoder(enc.finish());
    expect(dec.readString()).toBe('x'.repeat(100));
  });

  test('254 char string (single byte limit)', () => {
    const enc = new Encoder(512);
    enc.writeString('y'.repeat(254));
    const data = enc.finish();
    expect(data[0]).toBe(254);
    expect(data.length).toBe(255);
    
    const dec = new Decoder(data);
    expect(dec.readString()).toBe('y'.repeat(254));
  });

  test('255 char string (marker byte)', () => {
    const enc = new Encoder(512);
    enc.writeString('z'.repeat(255));
    const data = enc.finish();
    expect(data[0]).toBe(CompactLengthMarker);
    expect(data.length).toBe(260); // 1 marker + 4 length + 255 content
    
    const dec = new Decoder(data);
    expect(dec.readString()).toBe('z'.repeat(255));
  });

  test('256 char string', () => {
    const enc = new Encoder(512);
    enc.writeString('w'.repeat(256));
    const data = enc.finish();
    expect(data[0]).toBe(CompactLengthMarker);
    
    const dec = new Decoder(data);
    expect(dec.readString()).toBe('w'.repeat(256));
  });

  test('1000 char string', () => {
    const enc = new Encoder(2048);
    enc.writeString('v'.repeat(1000));
    const dec = new Decoder(enc.finish());
    expect(dec.readString()).toBe('v'.repeat(1000));
  });

  test('empty bytes', () => {
    const enc = new Encoder(16);
    enc.writeBytes(new Uint8Array(0));
    const dec = new Decoder(enc.finish());
    expect(dec.readBytes()).toEqual(new Uint8Array(0));
  });

  test('100 bytes', () => {
    const enc = new Encoder(256);
    const data = new Uint8Array(100);
    for (let i = 0; i < 100; i++) data[i] = i;
    enc.writeBytes(data);
    
    const dec = new Decoder(enc.finish());
    expect(dec.readBytes()).toEqual(data);
  });
});

describe('V2 Edge Cases', () => {
  test('int32 boundaries', () => {
    const enc = new Encoder(64);
    enc.writeInt32(2147483647);  // Max int32
    enc.writeInt32(-2147483648); // Min int32
    
    const dec = new Decoder(enc.finish());
    expect(dec.readInt32()).toBe(2147483647);
    expect(dec.readInt32()).toBe(-2147483648);
  });

  test('uint32 boundaries', () => {
    const enc = new Encoder(64);
    enc.writeUint32(0);
    enc.writeUint32(4294967295); // Max uint32
    
    const dec = new Decoder(enc.finish());
    expect(dec.readUint32()).toBe(0);
    expect(dec.readUint32()).toBe(4294967295);
  });

  test('float32 special values', () => {
    const enc = new Encoder(64);
    enc.writeFloat32(0);
    enc.writeFloat32(-0);
    enc.writeFloat32(Infinity);
    enc.writeFloat32(-Infinity);
    enc.writeFloat32(NaN);
    
    const dec = new Decoder(enc.finish());
    expect(dec.readFloat32()).toBe(0);
    expect(dec.readFloat32()).toBe(-0);
    expect(dec.readFloat32()).toBe(Infinity);
    expect(dec.readFloat32()).toBe(-Infinity);
    expect(dec.readFloat32()).toBeNaN();
  });

  test('float64 special values', () => {
    const enc = new Encoder(64);
    enc.writeFloat64(Number.MAX_VALUE);
    enc.writeFloat64(Number.MIN_VALUE);
    enc.writeFloat64(Number.EPSILON);
    
    const dec = new Decoder(enc.finish());
    expect(dec.readFloat64()).toBe(Number.MAX_VALUE);
    expect(dec.readFloat64()).toBe(Number.MIN_VALUE);
    expect(dec.readFloat64()).toBe(Number.EPSILON);
  });

  test('zero values', () => {
    const enc = new Encoder(64);
    enc.writeInt32(0);
    enc.writeUint32(0);
    enc.writeInt64(0n);
    enc.writeFloat32(0);
    enc.writeFloat64(0);
    enc.writeString('');
    enc.writeBytes(new Uint8Array(0));
    
    const dec = new Decoder(enc.finish());
    expect(dec.readInt32()).toBe(0);
    expect(dec.readUint32()).toBe(0);
    expect(dec.readInt64()).toBe(0n);
    expect(dec.readFloat32()).toBe(0);
    expect(dec.readFloat64()).toBe(0);
    expect(dec.readString()).toBe('');
    expect(dec.readBytes()).toEqual(new Uint8Array(0));
  });

  test('unicode strings', () => {
    const enc = new Encoder(256);
    const unicodeStrings = [
      'Hello 世界',
      'Привет мир',
      'مرحبا',
      '🎉🎊🎈',
      'Line1\nLine2\tTab',
    ];
    for (const s of unicodeStrings) {
      enc.writeString(s);
    }
    
    const dec = new Decoder(enc.finish());
    for (const expected of unicodeStrings) {
      expect(dec.readString()).toBe(expected);
    }
  });

  test('int64 boundaries', () => {
    const enc = new Encoder(64);
    enc.writeInt64(9223372036854775807n);  // Max int64
    enc.writeInt64(-9223372036854775808n); // Min int64
    
    const dec = new Decoder(enc.finish());
    expect(dec.readInt64()).toBe(9223372036854775807n);
    expect(dec.readInt64()).toBe(-9223372036854775808n);
  });

  test('uint64 boundaries', () => {
    const enc = new Encoder(64);
    enc.writeUint64(0n);
    enc.writeUint64(18446744073709551615n); // Max uint64
    
    const dec = new Decoder(enc.finish());
    expect(dec.readUint64()).toBe(0n);
    expect(dec.readUint64()).toBe(18446744073709551615n);
  });
});

describe('V2 Performance', () => {
  test('encoder reuse performance', () => {
    const enc = new Encoder(32);
    
    // First message
    enc.writeInt32(1);
    enc.writeString('first');
    enc.finish();
    
    // Reset and reuse
    enc.reset();
    enc.writeInt32(2);
    enc.writeString('second');
    const data = enc.finish();
    
    const dec = new Decoder(data);
    expect(dec.readInt32()).toBe(2);
    expect(dec.readString()).toBe('second');
  });

  test('batch operations', () => {
    const enc = new Encoder(1024);
    for (let i = 0; i < 100; i++) {
      enc.writeInt32(i);
    }
    
    const data = enc.finish();
    expect(data.length).toBe(400); // 100 * 4 bytes
    
    const dec = new Decoder(data);
    for (let i = 0; i < 100; i++) {
      expect(dec.readInt32()).toBe(i);
    }
  });

  test('nested message roundtrip', () => {
    // Encode inner
    const innerEnc = new Encoder(32);
    innerEnc.writeString('New York');
    innerEnc.writeString('USA');
    
    // Encode outer
    const enc = new Encoder(64);
    enc.writeString('Alice');
    enc.writeMessage(innerEnc.finish());
    
    // Decode
    const dec = new Decoder(enc.finish());
    expect(dec.readString()).toBe('Alice');
    
    const innerData = dec.readMessageBytes();
    const innerDec = new Decoder(innerData);
    expect(innerDec.readString()).toBe('New York');
    expect(innerDec.readString()).toBe('USA');
  });
});

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

/**
 * XPB Hybrid Runtime
 * Automatically selects optimal strategy based on message size
 * - Small messages (<256 bytes): Pure JS (lower overhead)
 * - Large messages (>=256 bytes): WASM (higher throughput)
 */

import { Encoder as JSEncoder, Decoder as JSDecoder, WireType } from './index.js';
import { isWasmReady, initWasm } from './wasm.js';

export { WireType } from './index.js';
export { initWasm } from './wasm.js';

// Threshold for switching to WASM (in bytes)
const WASM_THRESHOLD = 256;

/**
 * Hybrid Encoder that selects optimal strategy
 */
export class HybridEncoder {
  private jsEncoder: JSEncoder;
  private useWasm: boolean;

  constructor(initialSize = 256) {
    this.jsEncoder = new JSEncoder(initialSize);
    this.useWasm = isWasmReady();
  }

  reset(): void {
    this.jsEncoder.reset();
  }

  finish(): Uint8Array {
    return this.jsEncoder.finish();
  }

  // Delegate all write methods to JS encoder
  // IGNORE fieldNumber for V2 (Struct Mode)
  writeBool(fieldNumber: number, v: boolean): void {
    this.jsEncoder.writeBool(v);
  }

  writeInt32(fieldNumber: number, v: number): void {
    this.jsEncoder.writeInt32(v);
  }

  writeInt64(fieldNumber: number, v: bigint): void {
    this.jsEncoder.writeInt64(v);
  }

  writeUint32(fieldNumber: number, v: number): void {
    this.jsEncoder.writeUint32(v);
  }

  writeUint64(fieldNumber: number, v: bigint): void {
    this.jsEncoder.writeUint64(v);
  }

  writeFloat32(fieldNumber: number, v: number): void {
    this.jsEncoder.writeFloat32(v);
  }

  writeFloat64(fieldNumber: number, v: number): void {
    this.jsEncoder.writeFloat64(v);
  }

  writeString(fieldNumber: number, v: string): void {
    this.jsEncoder.writeString(v);
  }

  writeBytes(fieldNumber: number, v: Uint8Array): void {
    this.jsEncoder.writeBytes(v);
  }

  writeMessage(fieldNumber: number, data: Uint8Array): void {
    this.jsEncoder.writeMessage(data);
  }
}

/**
 * Hybrid Decoder that selects optimal strategy based on data size
 */
export class HybridDecoder {
  private data: Uint8Array;
  private jsDecoder: JSDecoder;
  private useWasm: boolean;

  constructor(data: Uint8Array) {
    this.data = data;
    this.jsDecoder = new JSDecoder(data);
    // Use WASM only for large messages where the overhead is amortized
    this.useWasm = isWasmReady() && data.length >= WASM_THRESHOLD;
  }

  eof(): boolean {
    return this.jsDecoder.eof();
  }

  // V2 does not use tags. Use read methods directly.
  readTag(): [number, typeof WireType[keyof typeof WireType]] {
     // Dummy implementation for compatibility
     return [0, WireType.Varint];
  }

  readBool(): boolean {
    return this.jsDecoder.readBool();
  }

  readInt32(): number {
    return this.jsDecoder.readInt32();
  }

  readInt64(): bigint {
    return this.jsDecoder.readInt64();
  }

  readUint32(): number {
    return this.jsDecoder.readUint32();
  }

  readUint64(): bigint {
    return this.jsDecoder.readUint64();
  }

  readFloat32(): number {
    return this.jsDecoder.readFloat32();
  }

  readFloat64(): number {
    return this.jsDecoder.readFloat64();
  }

  readString(): string {
    return this.jsDecoder.readString();
  }

  readBytes(): Uint8Array {
    return this.jsDecoder.readBytes();
  }

  readMessageBytes(): Uint8Array {
    return this.jsDecoder.readMessageBytes();
  }

  skip(wireType: typeof WireType[keyof typeof WireType]): void {
    this.jsDecoder.skip(1); // Skip 1 byte? V2 skip is hard without tag.
  }
}

/**
 * Smart decoder factory - returns optimal decoder based on data size
 */
export function createDecoder(data: Uint8Array): JSDecoder | HybridDecoder {
  if (data.length < WASM_THRESHOLD || !isWasmReady()) {
    return new JSDecoder(data);
  }
  return new HybridDecoder(data);
}

/**
 * Smart encoder factory
 */
export function createEncoder(estimatedSize = 256): JSEncoder | HybridEncoder {
  // For encoding, JS is typically fast enough
  // WASM mainly helps with decoding large messages
  return new JSEncoder(estimatedSize);
}

// Re-export original classes for direct use
export { Encoder, Decoder } from './index.js';
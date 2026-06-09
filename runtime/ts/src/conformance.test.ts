/**
 * Cross-language conformance test.
 *
 * Reads the shared `.bin` vectors and `vectors.json` manifest produced by the
 * Go reference encoder (see `tests/conformance` in the repo root), decodes each
 * with the TypeScript runtime, asserts the decoded values match the manifest,
 * then re-encodes and asserts the bytes are byte-identical to the `.bin` file.
 *
 * Value model (see manifest "format" field):
 *   - int32/uint32: JSON number
 *   - int64/uint64: decimal string -> bigint
 *   - float32/float64: hex bit-pattern string (e.g. "0x7FF0000000000000")
 *   - bytes: lowercase hex string
 *   - array: { elemType, elems: [...] } -> int32 count + elements
 *   - map: { keyType, valType, entries: [{k,v}] } -> int32 count + k/v pairs
 *   - message: { ops: [...] } -> length-prefixed nested ops
 */

import { describe, test, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { Encoder, Decoder } from './index';

const here = dirname(fileURLToPath(import.meta.url)); // runtime/ts/src
const dataDir = join(here, '..', '..', '..', 'testdata', 'conformance');

type Op = Record<string, any>;

function hexDecode(s: string): Uint8Array {
  if (s.length % 2 !== 0) throw new Error(`odd hex length: ${s.length}`);
  const out = new Uint8Array(s.length / 2);
  for (let i = 0; i < out.length; i++) {
    out[i] = parseInt(s.substr(i * 2, 2), 16);
  }
  return out;
}

function stripHexPrefix(s: string): string {
  return s.startsWith('0x') || s.startsWith('0X') ? s.slice(2) : s;
}

function f32Bits(hex: string): number {
  return parseInt(stripHexPrefix(hex), 16) >>> 0;
}

function f64Bits(hex: string): bigint {
  return BigInt('0x' + stripHexPrefix(hex));
}

// Convert a float32 value back to its 32-bit pattern for bit-exact comparison.
function float32ToBits(v: number): number {
  const dv = new DataView(new ArrayBuffer(4));
  dv.setFloat32(0, v, true);
  return dv.getUint32(0, true) >>> 0;
}

// Convert a float64 value back to its 64-bit pattern for bit-exact comparison.
function float64ToBits(v: number): bigint {
  const dv = new DataView(new ArrayBuffer(8));
  dv.setFloat64(0, v, true);
  return dv.getBigUint64(0, true);
}

function bytesEqual(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
  return true;
}

function toHex(a: Uint8Array): string {
  return Array.from(a, (b) => b.toString(16).padStart(2, '0')).join('');
}

function encodeOps(enc: Encoder, ops: Op[]): void {
  for (const op of ops) encodeOp(enc, op);
}

function encodeOp(enc: Encoder, op: Op): void {
  switch (op.type) {
    case 'bool':
      enc.writeBool(op.bool as boolean);
      break;
    case 'int32':
      enc.writeInt32(op.int32 as number);
      break;
    case 'uint32':
      enc.writeUint32(op.uint32 as number);
      break;
    case 'int64':
      enc.writeInt64(BigInt(op.int64 as string));
      break;
    case 'uint64':
      enc.writeUint64(BigInt(op.uint64 as string));
      break;
    case 'float32': {
      const dv = new DataView(new ArrayBuffer(4));
      dv.setUint32(0, f32Bits(op.floatBits), true);
      enc.writeFloat32(dv.getFloat32(0, true));
      break;
    }
    case 'float64': {
      const dv = new DataView(new ArrayBuffer(8));
      dv.setBigUint64(0, f64Bits(op.floatBits), true);
      enc.writeFloat64(dv.getFloat64(0, true));
      break;
    }
    case 'string':
      enc.writeString((op.string as string) ?? '');
      break;
    case 'bytes':
      enc.writeBytes(hexDecode((op.bytes as string) ?? ''));
      break;
    case 'array': {
      const elems: Op[] = op.elems ?? [];
      enc.writeInt32(elems.length);
      for (const el of elems) encodeOp(enc, el);
      break;
    }
    case 'map': {
      const entries: Op[] = op.entries ?? [];
      enc.writeInt32(entries.length);
      for (const ent of entries) {
        encodeOp(enc, ent.k);
        encodeOp(enc, ent.v);
      }
      break;
    }
    case 'message': {
      const innerOps: Op[] = op.ops ?? [];
      const inner = new Encoder(64);
      encodeOps(inner, innerOps);
      enc.writeMessage(inner.finish());
      break;
    }
    default:
      throw new Error(`unknown op type ${op.type}`);
  }
}

function verifyOps(dec: Decoder, ops: Op[], path: string): void {
  ops.forEach((op, i) => verifyOp(dec, op, `${path}[${i}]`));
}

function verifyOp(dec: Decoder, op: Op, path: string): void {
  switch (op.type) {
    case 'bool':
      expect(dec.readBool(), `${path} bool`).toBe(op.bool);
      break;
    case 'int32':
      expect(dec.readInt32(), `${path} int32`).toBe(op.int32);
      break;
    case 'uint32':
      expect(dec.readUint32(), `${path} uint32`).toBe(op.uint32);
      break;
    case 'int64':
      expect(dec.readInt64(), `${path} int64`).toBe(BigInt(op.int64));
      break;
    case 'uint64':
      expect(dec.readUint64(), `${path} uint64`).toBe(BigInt(op.uint64));
      break;
    case 'float32': {
      // Bit-exact comparison: NaN != NaN, -0.0 != +0.0.
      const got = dec.readFloat32();
      expect(float32ToBits(got), `${path} float32 bits`).toBe(f32Bits(op.floatBits));
      break;
    }
    case 'float64': {
      const got = dec.readFloat64();
      expect(float64ToBits(got), `${path} float64 bits`).toBe(f64Bits(op.floatBits));
      break;
    }
    case 'string':
      expect(dec.readString(), `${path} string`).toBe((op.string as string) ?? '');
      break;
    case 'bytes': {
      const got = dec.readBytes();
      const want = hexDecode((op.bytes as string) ?? '');
      expect(bytesEqual(got, want), `${path} bytes got=${toHex(got)} want=${toHex(want)}`).toBe(true);
      break;
    }
    case 'array': {
      const count = dec.readInt32();
      const elems: Op[] = op.elems ?? [];
      expect(count, `${path} array count`).toBe(elems.length);
      elems.forEach((el, i) => verifyOp(dec, el, `${path}.elem[${i}]`));
      break;
    }
    case 'map': {
      const count = dec.readInt32();
      const entries: Op[] = op.entries ?? [];
      expect(count, `${path} map count`).toBe(entries.length);
      entries.forEach((ent, i) => {
        verifyOp(dec, ent.k, `${path}.key[${i}]`);
        verifyOp(dec, ent.v, `${path}.val[${i}]`);
      });
      break;
    }
    case 'message': {
      const msg = dec.readMessageBytes();
      const innerOps: Op[] = op.ops ?? [];
      const inner = new Decoder(msg);
      verifyOps(inner, innerOps, `${path}.msg`);
      expect(inner.eof(), `${path} nested message trailing bytes`).toBe(true);
      break;
    }
    default:
      throw new Error(`unknown op type ${op.type}`);
  }
}

const manifestPath = join(dataDir, 'vectors.json');
const manifest = JSON.parse(readFileSync(manifestPath, 'utf8')) as {
  vectors: Array<{ name: string; file: string; hex: string; ops: Op[] }>;
};

describe('cross-language conformance (Go reference vectors)', () => {
  test('manifest has vectors', () => {
    expect(manifest.vectors.length).toBeGreaterThan(0);
  });

  for (const v of manifest.vectors) {
    test(v.name, () => {
      const fileBytes = new Uint8Array(readFileSync(join(dataDir, v.file)));

      // Manifest hex must match the .bin file exactly.
      const wantHex = hexDecode(v.hex);
      expect(bytesEqual(fileBytes, wantHex), `${v.name} manifest hex != .bin bytes`).toBe(true);

      // Decode the .bin and verify values bit-exactly.
      const dec = new Decoder(fileBytes);
      verifyOps(dec, v.ops, v.name);
      expect(dec.eof(), `${v.name} trailing bytes after decode`).toBe(true);

      // Re-encode from the manifest ops and assert byte-identity.
      const enc = new Encoder(256);
      encodeOps(enc, v.ops);
      const reencoded = enc.finish();
      expect(
        bytesEqual(reencoded, fileBytes),
        `${v.name} re-encode mismatch\n got:  ${toHex(reencoded)}\n want: ${toHex(fileBytes)}`
      ).toBe(true);
    });
  }
});

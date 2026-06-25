// Cross-language DIFFERENTIAL runner for the TypeScript runtime (T-9).
//
// This is the TS arm of the random cross-language differential fuzzer in
// tests/differential. It is a standalone Node script (not a vitest test) so the
// Go driver can point it at an arbitrary corpus directory, without modifying the
// committed fixed-vector conformance test (runtime/ts/src/conformance.test.ts).
//
// Usage:  node ts_diff_runner.mjs <corpus-dir> <bundle.mjs> [bytes|values]
//
// Behaviour mirrors that conformance test: read vectors.json + .bin files
// (produced by the Go reference encoder), decode each with the TS runtime and
// verify decoded values bit-exactly. In `bytes` mode (default; the map-FREE
// corpus) it then re-encodes and asserts byte-identity with the Go .bin. In
// `values` mode (the map-CONTAINING corpus) the byte-identity check is skipped,
// because map entry order is non-canonical across runtimes (T-7). Exits non-zero
// (and throws) on the first mismatch.

import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

const [, , corpusDir, distPath, modeArg] = process.argv;
if (!corpusDir || !distPath) {
  console.error('usage: node ts_diff_runner.mjs <corpus-dir> <bundle.mjs> [bytes|values]');
  process.exit(2);
}
const valuesOnly = modeArg === 'values';

const { Encoder, Decoder } = await import(pathToFileURL(distPath).href);

function hexDecode(s) {
  if (s.length % 2 !== 0) throw new Error(`odd hex length: ${s.length}`);
  const out = new Uint8Array(s.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(s.substr(i * 2, 2), 16);
  return out;
}

function stripHexPrefix(s) {
  return s.startsWith('0x') || s.startsWith('0X') ? s.slice(2) : s;
}

function f32Bits(hex) {
  return parseInt(stripHexPrefix(hex), 16) >>> 0;
}

function f64Bits(hex) {
  return BigInt('0x' + stripHexPrefix(hex));
}

function float32ToBits(v) {
  const dv = new DataView(new ArrayBuffer(4));
  dv.setFloat32(0, v, true);
  return dv.getUint32(0, true) >>> 0;
}

function float64ToBits(v) {
  const dv = new DataView(new ArrayBuffer(8));
  dv.setFloat64(0, v, true);
  return dv.getBigUint64(0, true);
}

function bytesEqual(a, b) {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
  return true;
}

function toHex(a) {
  return Array.from(a, (b) => b.toString(16).padStart(2, '0')).join('');
}

function fail(msg) {
  console.error('[FAIL] ' + msg);
  process.exit(1);
}

function assert(cond, msg) {
  if (!cond) fail(msg);
}

function encodeOps(enc, ops) {
  for (const op of ops) encodeOp(enc, op);
}

function encodeOp(enc, op) {
  switch (op.type) {
    case 'bool':
      enc.writeBool(op.bool);
      break;
    case 'int32':
      enc.writeInt32(op.int32);
      break;
    case 'uint32':
      enc.writeUint32(op.uint32);
      break;
    case 'int64':
      enc.writeInt64(BigInt(op.int64));
      break;
    case 'uint64':
      enc.writeUint64(BigInt(op.uint64));
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
      enc.writeString(op.string ?? '');
      break;
    case 'bytes':
      enc.writeBytes(hexDecode(op.bytes ?? ''));
      break;
    case 'array': {
      const elems = op.elems ?? [];
      enc.writeInt32(elems.length);
      for (const el of elems) encodeOp(enc, el);
      break;
    }
    case 'map': {
      const entries = op.entries ?? [];
      enc.writeInt32(entries.length);
      for (const ent of entries) {
        encodeOp(enc, ent.k);
        encodeOp(enc, ent.v);
      }
      break;
    }
    case 'message': {
      const inner = new Encoder(64);
      encodeOps(inner, op.ops ?? []);
      enc.writeMessage(inner.finish());
      break;
    }
    default:
      throw new Error(`unknown op type ${op.type}`);
  }
}

function verifyOps(dec, ops, path) {
  ops.forEach((op, i) => verifyOp(dec, op, `${path}[${i}]`));
}

function verifyOp(dec, op, path) {
  switch (op.type) {
    case 'bool':
      assert(dec.readBool() === op.bool, `${path} bool`);
      break;
    case 'int32':
      assert(dec.readInt32() === op.int32, `${path} int32`);
      break;
    case 'uint32':
      assert(dec.readUint32() === op.uint32, `${path} uint32`);
      break;
    case 'int64':
      assert(dec.readInt64() === BigInt(op.int64), `${path} int64`);
      break;
    case 'uint64':
      assert(dec.readUint64() === BigInt(op.uint64), `${path} uint64`);
      break;
    case 'float32':
      assert(float32ToBits(dec.readFloat32()) === f32Bits(op.floatBits), `${path} float32 bits`);
      break;
    case 'float64':
      assert(float64ToBits(dec.readFloat64()) === f64Bits(op.floatBits), `${path} float64 bits`);
      break;
    case 'string':
      assert(dec.readString() === (op.string ?? ''), `${path} string`);
      break;
    case 'bytes': {
      const got = dec.readBytes();
      const want = hexDecode(op.bytes ?? '');
      assert(bytesEqual(got, want), `${path} bytes got=${toHex(got)} want=${toHex(want)}`);
      break;
    }
    case 'array': {
      const count = dec.readInt32();
      const elems = op.elems ?? [];
      assert(count === elems.length, `${path} array count`);
      elems.forEach((el, i) => verifyOp(dec, el, `${path}.elem[${i}]`));
      break;
    }
    case 'map': {
      const count = dec.readInt32();
      const entries = op.entries ?? [];
      assert(count === entries.length, `${path} map count`);
      entries.forEach((ent, i) => {
        verifyOp(dec, ent.k, `${path}.key[${i}]`);
        verifyOp(dec, ent.v, `${path}.val[${i}]`);
      });
      break;
    }
    case 'message': {
      const msg = dec.readMessageBytes();
      const inner = new Decoder(msg);
      verifyOps(inner, op.ops ?? [], `${path}.msg`);
      assert(inner.eof(), `${path} nested message trailing bytes`);
      break;
    }
    default:
      throw new Error(`unknown op type ${op.type}`);
  }
}

const manifest = JSON.parse(readFileSync(join(corpusDir, 'vectors.json'), 'utf8'));
if (!manifest.vectors || manifest.vectors.length === 0) fail('manifest has no vectors');

let count = 0;
for (const v of manifest.vectors) {
  const fileBytes = new Uint8Array(readFileSync(join(corpusDir, v.file)));

  const dec = new Decoder(fileBytes);
  verifyOps(dec, v.ops, v.name);
  assert(dec.eof(), `${v.name} trailing bytes after decode`);

  if (!valuesOnly) {
    // Byte mode (map-free corpus): re-encode and assert byte-identity.
    const enc = new Encoder(256);
    encodeOps(enc, v.ops);
    const reencoded = enc.finish();
    assert(
      bytesEqual(reencoded, fileBytes),
      `${v.name} re-encode mismatch\n got:  ${toHex(reencoded)}\n want: ${toHex(fileBytes)}`
    );
  } else {
    // Values mode (map-containing corpus): byte order is non-canonical, so
    // exercise this runtime's ENCODER via a self round-trip -- re-encode the
    // values, decode that back, and re-verify the values.
    const enc = new Encoder(256);
    encodeOps(enc, v.ops);
    const rdec = new Decoder(enc.finish());
    verifyOps(rdec, v.ops, v.name);
    assert(rdec.eof(), `${v.name} self round-trip trailing bytes`);
  }
  count++;
}

console.log(`TypeScript differential (${valuesOnly ? 'values' : 'bytes'}): ${count} vectors verified`);

// Timed cross-runtime benchmark harness for the TypeScript runtime (xpbench / T-17).
//
// This is the TS arm of the cross-runtime benchmark TABLE driven by cmd/xpbench.
// It is the timed analogue of the proven differential runner
// (tests/diff/ts_diff_runner.mjs): it reads the shared vectors.json manifest +
// .bin corpus the Go reference encoder wrote, then for every vector times an
// encode loop (re-encode the ops with the TS Encoder) and a decode loop (decode
// the .bin bytes with the TS Decoder) over a per-vector iteration count, and
// prints a JSON array of {name, encodeNs, decodeNs, wireSize} to stdout for the
// Go driver to normalize into table rows.
//
// Usage:  node ts_bench.mjs <corpus-dir> <bundle.mjs>
//
// The bundle is produced by the project-local esbuild over runtime/ts/src/index.ts
// (the Go driver builds it); this script needs only Node + that bundle.

import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

const [, , corpusDir, distPath] = process.argv;
if (!corpusDir || !distPath) {
  console.error('usage: node ts_bench.mjs <corpus-dir> <bundle.mjs>');
  process.exit(2);
}

const { Encoder, Decoder } = await import(pathToFileURL(distPath).href);

function hexDecode(s) {
  if (!s) return new Uint8Array(0);
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

// decodeOps reads every value (decode-only, matching the Go driver's
// decode-only path and the other harnesses) and accumulates a small number so
// the read cannot be elided.
function decodeOps(dec, ops) {
  let acc = 0;
  for (const op of ops) acc += decodeOp(dec, op);
  return acc;
}

function decodeOp(dec, op) {
  switch (op.type) {
    case 'bool':
      return dec.readBool() ? 1 : 0;
    case 'int32':
      return dec.readInt32() | 0 ? 1 : 0;
    case 'uint32':
      return dec.readUint32() >>> 0 ? 1 : 0;
    case 'int64':
      return dec.readInt64() === 0n ? 0 : 1;
    case 'uint64':
      return dec.readUint64() === 0n ? 0 : 1;
    case 'float32':
      return dec.readFloat32() === 0 ? 0 : 1;
    case 'float64':
      return dec.readFloat64() === 0 ? 0 : 1;
    case 'string':
      return dec.readString().length;
    case 'bytes':
      return dec.readBytes().length;
    case 'array': {
      const elems = op.elems ?? [];
      let acc = dec.readInt32();
      for (const el of elems) acc += decodeOp(dec, el);
      return acc;
    }
    case 'map': {
      const entries = op.entries ?? [];
      let acc = dec.readInt32();
      for (const ent of entries) {
        acc += decodeOp(dec, ent.k);
        acc += decodeOp(dec, ent.v);
      }
      return acc;
    }
    case 'message': {
      const msg = dec.readMessageBytes();
      const inner = new Decoder(msg);
      return decodeOps(inner, op.ops ?? []) + msg.length;
    }
    default:
      throw new Error(`unknown op type ${op.type}`);
  }
}

const manifest = JSON.parse(readFileSync(join(corpusDir, 'vectors.json'), 'utf8'));
if (!manifest.vectors || manifest.vectors.length === 0) {
  console.error('manifest has no vectors');
  process.exit(1);
}

let sink = 0;
const results = [];
for (const v of manifest.vectors) {
  const fileBytes = new Uint8Array(readFileSync(join(corpusDir, v.file)));
  const wire = fileBytes.length;
  const iters = Math.max(1, v.iters | 0);
  const warm = Math.max(1, Math.floor(iters / 10));

  // Encode timing.
  for (let i = 0; i < warm; i++) {
    const enc = new Encoder(256);
    encodeOps(enc, v.ops);
    sink += enc.finish().length;
  }
  let t0 = process.hrtime.bigint();
  for (let i = 0; i < iters; i++) {
    const enc = new Encoder(256);
    encodeOps(enc, v.ops);
    sink += enc.finish().length;
  }
  const encNs = Number(process.hrtime.bigint() - t0) / iters;

  // Decode timing.
  for (let i = 0; i < warm; i++) {
    const dec = new Decoder(fileBytes);
    sink += decodeOps(dec, v.ops);
  }
  t0 = process.hrtime.bigint();
  for (let i = 0; i < iters; i++) {
    const dec = new Decoder(fileBytes);
    sink += decodeOps(dec, v.ops);
  }
  const decNs = Number(process.hrtime.bigint() - t0) / iters;

  results.push({ name: v.name, encodeNs: Number(encNs.toFixed(3)), decodeNs: Number(decNs.toFixed(3)), wireSize: wire });
}

process.stdout.write(JSON.stringify(results) + '\n');
// Reference the sink so the timed loops are not optimized away.
if (sink === -1) console.error('sink', sink);

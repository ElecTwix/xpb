//! Cross-language conformance test.
//!
//! Reads the shared `.bin` vectors and `vectors.json` manifest produced by the
//! Go reference encoder (see `tests/conformance` in the repo root), decodes
//! each with the Rust runtime, asserts the decoded values match the manifest,
//! then re-encodes and asserts the bytes are byte-identical to the `.bin` file.
//!
//! Value model (see manifest "format" field):
//!   - int32/uint32: JSON number
//!   - int64/uint64: decimal string
//!   - float32/float64: hex bit-pattern string (e.g. "0x7FF0000000000000")
//!   - bytes: lowercase hex string
//!   - array: { elemType, elems: [...] } -> int32 count + elements
//!   - map: { keyType, valType, entries: [{k,v}] } -> int32 count + k/v pairs
//!   - message: { ops: [...] } -> length-prefixed nested ops

use std::fs;
use std::path::PathBuf;

use serde_json::Value;
use xpb::{Decoder, Encoder};

/// Path to <repo>/testdata/conformance. CARGO_MANIFEST_DIR is <repo>/runtime/rust.
fn testdata_dir() -> PathBuf {
    let manifest = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    manifest
        .join("..") // runtime
        .join("..") // repo root
        .join("testdata")
        .join("conformance")
}

fn hex_decode(s: &str) -> Vec<u8> {
    assert!(s.len() % 2 == 0, "odd hex length: {:?}", s);
    (0..s.len())
        .step_by(2)
        .map(|i| u8::from_str_radix(&s[i..i + 2], 16).expect("valid hex"))
        .collect()
}

fn strip_hex_prefix(s: &str) -> &str {
    s.strip_prefix("0x").or_else(|| s.strip_prefix("0X")).unwrap_or(s)
}

/// Encode a sequence of ops with the Rust encoder.
fn encode_ops(enc: &mut Encoder, ops: &[Value]) {
    for op in ops {
        encode_op(enc, op);
    }
}

fn encode_op(enc: &mut Encoder, op: &Value) {
    let ty = op["type"].as_str().expect("op.type");
    match ty {
        "bool" => enc.write_bool(op["bool"].as_bool().expect("bool")),
        "int32" => enc.write_int32(op["int32"].as_i64().expect("int32") as i32),
        "uint32" => enc.write_uint32(op["uint32"].as_u64().expect("uint32") as u32),
        "int64" => {
            let v: i64 = op["int64"].as_str().expect("int64 str").parse().expect("i64");
            enc.write_int64(v);
        }
        "uint64" => {
            let v: u64 = op["uint64"].as_str().expect("uint64 str").parse().expect("u64");
            enc.write_uint64(v);
        }
        "float32" => {
            let bits =
                u32::from_str_radix(strip_hex_prefix(op["floatBits"].as_str().unwrap()), 16).unwrap();
            enc.write_float32(f32::from_bits(bits));
        }
        "float64" => {
            let bits =
                u64::from_str_radix(strip_hex_prefix(op["floatBits"].as_str().unwrap()), 16).unwrap();
            enc.write_float64(f64::from_bits(bits));
        }
        "string" => enc.write_string(op["string"].as_str().unwrap_or("")),
        "bytes" => {
            let b = hex_decode(op["bytes"].as_str().unwrap_or(""));
            enc.write_bytes(&b);
        }
        "array" => {
            let elems = op["elems"].as_array().map(|a| a.as_slice()).unwrap_or(&[]);
            enc.write_int32(elems.len() as i32);
            for el in elems {
                encode_op(enc, el);
            }
        }
        "map" => {
            let entries = op["entries"].as_array().map(|a| a.as_slice()).unwrap_or(&[]);
            enc.write_int32(entries.len() as i32);
            for ent in entries {
                encode_op(enc, &ent["k"]);
                encode_op(enc, &ent["v"]);
            }
        }
        "message" => {
            let inner_ops = op["ops"].as_array().map(|a| a.as_slice()).unwrap_or(&[]);
            let mut inner = Encoder::new(64);
            encode_ops(&mut inner, inner_ops);
            let bytes = inner.finish();
            enc.write_message(&bytes);
        }
        other => panic!("unknown op type {other:?}"),
    }
}

/// Decode + verify a sequence of ops, asserting values match.
fn verify_ops(dec: &mut Decoder, ops: &[Value], path: &str) {
    for (i, op) in ops.iter().enumerate() {
        verify_op(dec, op, &format!("{path}[{i}]"));
    }
}

fn verify_op(dec: &mut Decoder, op: &Value, path: &str) {
    let ty = op["type"].as_str().expect("op.type");
    match ty {
        "bool" => {
            let got = dec.read_bool().unwrap_or_else(|e| panic!("{path}: {e:?}"));
            assert_eq!(got, op["bool"].as_bool().unwrap(), "{path} bool");
        }
        "int32" => {
            let got = dec.read_int32().unwrap_or_else(|e| panic!("{path}: {e:?}"));
            assert_eq!(got as i64, op["int32"].as_i64().unwrap(), "{path} int32");
        }
        "uint32" => {
            let got = dec.read_uint32().unwrap_or_else(|e| panic!("{path}: {e:?}"));
            assert_eq!(got as u64, op["uint32"].as_u64().unwrap(), "{path} uint32");
        }
        "int64" => {
            let got = dec.read_int64().unwrap_or_else(|e| panic!("{path}: {e:?}"));
            let want: i64 = op["int64"].as_str().unwrap().parse().unwrap();
            assert_eq!(got, want, "{path} int64");
        }
        "uint64" => {
            let got = dec.read_uint64().unwrap_or_else(|e| panic!("{path}: {e:?}"));
            let want: u64 = op["uint64"].as_str().unwrap().parse().unwrap();
            assert_eq!(got, want, "{path} uint64");
        }
        "float32" => {
            let got = dec.read_float32().unwrap_or_else(|e| panic!("{path}: {e:?}"));
            let want = u32::from_str_radix(strip_hex_prefix(op["floatBits"].as_str().unwrap()), 16).unwrap();
            // Bit-exact comparison: NaN != NaN, -0.0 != +0.0.
            assert_eq!(
                got.to_bits(),
                want,
                "{path} float32 bits: got {:#010X} want {:#010X}",
                got.to_bits(),
                want
            );
        }
        "float64" => {
            let got = dec.read_float64().unwrap_or_else(|e| panic!("{path}: {e:?}"));
            let want = u64::from_str_radix(strip_hex_prefix(op["floatBits"].as_str().unwrap()), 16).unwrap();
            assert_eq!(
                got.to_bits(),
                want,
                "{path} float64 bits: got {:#018X} want {:#018X}",
                got.to_bits(),
                want
            );
        }
        "string" => {
            let got = dec.read_string().unwrap_or_else(|e| panic!("{path}: {e:?}"));
            assert_eq!(got, op["string"].as_str().unwrap_or(""), "{path} string");
        }
        "bytes" => {
            let got = dec.read_bytes().unwrap_or_else(|e| panic!("{path}: {e:?}"));
            let want = hex_decode(op["bytes"].as_str().unwrap_or(""));
            assert_eq!(got, want, "{path} bytes");
        }
        "array" => {
            let count = dec.read_int32().unwrap_or_else(|e| panic!("{path}: {e:?}"));
            let elems = op["elems"].as_array().map(|a| a.as_slice()).unwrap_or(&[]);
            assert_eq!(count as usize, elems.len(), "{path} array count");
            for (i, el) in elems.iter().enumerate() {
                verify_op(dec, el, &format!("{path}.elem[{i}]"));
            }
        }
        "map" => {
            let count = dec.read_int32().unwrap_or_else(|e| panic!("{path}: {e:?}"));
            let entries = op["entries"].as_array().map(|a| a.as_slice()).unwrap_or(&[]);
            assert_eq!(count as usize, entries.len(), "{path} map count");
            for (i, ent) in entries.iter().enumerate() {
                verify_op(dec, &ent["k"], &format!("{path}.key[{i}]"));
                verify_op(dec, &ent["v"], &format!("{path}.val[{i}]"));
            }
        }
        "message" => {
            let msg = dec.read_message_bytes().unwrap_or_else(|e| panic!("{path}: {e:?}"));
            let inner_ops = op["ops"].as_array().map(|a| a.as_slice()).unwrap_or(&[]);
            let mut inner = Decoder::new(&msg);
            verify_ops(&mut inner, inner_ops, &format!("{path}.msg"));
            assert!(inner.eof(), "{path} nested message trailing bytes");
        }
        other => panic!("unknown op type {other:?}"),
    }
}

#[test]
fn conformance_vectors() {
    let dir = testdata_dir();
    let manifest_path = dir.join("vectors.json");
    let raw = fs::read_to_string(&manifest_path).unwrap_or_else(|e| {
        panic!(
            "read manifest {} (run `XPB_GEN=1 go test ./tests/conformance/ -run TestGenerateVectors` first): {e}",
            manifest_path.display()
        )
    });
    let manifest: Value = serde_json::from_str(&raw).expect("parse manifest json");
    let vectors = manifest["vectors"].as_array().expect("vectors array");
    assert!(!vectors.is_empty(), "manifest has no vectors");

    let mut count = 0;
    for v in vectors {
        let name = v["name"].as_str().expect("name");
        let file = v["file"].as_str().expect("file");
        let ops = v["ops"].as_array().expect("ops");

        // Read the shared .bin file produced by Go.
        let bin_path = dir.join(file);
        let file_bytes = fs::read(&bin_path)
            .unwrap_or_else(|e| panic!("[{name}] read {}: {e}", bin_path.display()));

        // Manifest hex must match the .bin file exactly.
        let want_hex = hex_decode(v["hex"].as_str().expect("hex"));
        assert_eq!(file_bytes, want_hex, "[{name}] manifest hex != .bin bytes");

        // Decode the .bin and verify values bit-exactly.
        let mut dec = Decoder::new(&file_bytes);
        verify_ops(&mut dec, ops, name);
        assert!(dec.eof(), "[{name}] trailing bytes after decode");

        // Re-encode from the manifest ops and assert byte-identity.
        let mut enc = Encoder::new(256);
        encode_ops(&mut enc, ops);
        let reencoded = enc.finish();
        assert_eq!(
            reencoded, file_bytes,
            "[{name}] re-encode mismatch\n got:  {reencoded:02X?}\n want: {file_bytes:02X?}"
        );

        count += 1;
    }
    eprintln!("verified {count} conformance vectors (rust)");
}

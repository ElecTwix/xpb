//! Cross-language DIFFERENTIAL runner for the Rust runtime (T-9).
//!
//! This is the Rust arm of the random cross-language differential fuzzer in
//! `tests/differential`. It is deliberately a standalone *example* binary (not a
//! `#[test]`) so the Go driver can point it at an arbitrary corpus directory via
//! `cargo run --example diff_runner -- <dir>`, without touching the committed
//! fixed-vector conformance test (`tests/conformance.rs`).
//!
//! Usage:  cargo run --example diff_runner -- <corpus-dir> [bytes|values]
//!
//! Behaviour mirrors that conformance test: read the `vectors.json` manifest +
//! `.bin` files in <dir> (produced by the Go reference encoder), decode each with
//! the Rust runtime and verify the decoded values bit-exactly. In `bytes` mode
//! (default; the map-FREE corpus) it then re-encodes and asserts byte-identity
//! with the Go `.bin`. In `values` mode (the map-CONTAINING corpus) the
//! byte-identity check is skipped because map entry order is non-canonical across
//! runtimes (T-7); the decoded values remain a real cross-language oracle. Exits
//! non-zero on the first mismatch.
//!
//! `serde_json` is a dev-dependency, which Cargo makes available to examples.

use std::fs;
use std::path::PathBuf;
use std::process::exit;

use serde_json::Value;
use xpb::{Decoder, Encoder};

fn strip_hex_prefix(s: &str) -> &str {
    s.strip_prefix("0x").or_else(|| s.strip_prefix("0X")).unwrap_or(s)
}

fn hex_decode(s: &str) -> Vec<u8> {
    assert!(s.len() % 2 == 0, "odd hex length: {s:?}");
    (0..s.len())
        .step_by(2)
        .map(|i| u8::from_str_radix(&s[i..i + 2], 16).expect("valid hex"))
        .collect()
}

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
            assert_eq!(got.to_bits(), want, "{path} float32 bits");
        }
        "float64" => {
            let got = dec.read_float64().unwrap_or_else(|e| panic!("{path}: {e:?}"));
            let want = u64::from_str_radix(strip_hex_prefix(op["floatBits"].as_str().unwrap()), 16).unwrap();
            assert_eq!(got.to_bits(), want, "{path} float64 bits");
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

fn main() {
    let dir = match std::env::args().nth(1) {
        Some(d) => PathBuf::from(d),
        None => {
            eprintln!("usage: diff_runner <corpus-dir> [bytes|values]");
            exit(2);
        }
    };
    // Default to byte-identity; "values" skips the re-encode check (map corpus).
    let values_only = std::env::args().nth(2).as_deref() == Some("values");
    let manifest_path = dir.join("vectors.json");
    let raw = fs::read_to_string(&manifest_path)
        .unwrap_or_else(|e| panic!("read manifest {}: {e}", manifest_path.display()));
    let manifest: Value = serde_json::from_str(&raw).expect("parse manifest json");
    let vectors = manifest["vectors"].as_array().expect("vectors array");
    if vectors.is_empty() {
        eprintln!("manifest has no vectors");
        exit(1);
    }

    let mut count = 0usize;
    for v in vectors {
        let name = v["name"].as_str().expect("name");
        let file = v["file"].as_str().expect("file");
        let ops = v["ops"].as_array().expect("ops");

        let bin_path = dir.join(file);
        let file_bytes = fs::read(&bin_path)
            .unwrap_or_else(|e| panic!("[{name}] read {}: {e}", bin_path.display()));

        // Decode + verify values bit-exactly (always).
        let mut dec = Decoder::new(&file_bytes);
        verify_ops(&mut dec, ops, name);
        assert!(dec.eof(), "[{name}] trailing bytes after decode");

        if !values_only {
            // Byte mode (map-free corpus): re-encode and assert byte-identity
            // with the Go reference bytes.
            let mut enc = Encoder::new(256);
            encode_ops(&mut enc, ops);
            let reencoded = enc.finish();
            assert!(
                reencoded == file_bytes,
                "[{name}] re-encode mismatch\n got:  {}\n want: {}",
                hex_of(&reencoded),
                hex_of(&file_bytes)
            );
        } else {
            // Values mode (map-containing corpus): byte order is non-canonical,
            // so instead exercise this runtime's ENCODER via a self round-trip --
            // re-encode the values, decode THAT back, and re-verify the values.
            let mut enc = Encoder::new(256);
            encode_ops(&mut enc, ops);
            let reencoded = enc.finish();
            let mut rdec = Decoder::new(&reencoded);
            verify_ops(&mut rdec, ops, name);
            assert!(rdec.eof(), "[{name}] self round-trip trailing bytes");
        }
        count += 1;
    }

    let mode = if values_only { "values" } else { "bytes" };
    println!("Rust differential ({mode}): {count} vectors verified");
}

fn hex_of(b: &[u8]) -> String {
    let mut s = String::with_capacity(b.len() * 2);
    for x in b {
        s.push_str(&format!("{x:02x}"));
    }
    s
}

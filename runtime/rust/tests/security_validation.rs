//! Security-audit validation for the Rust runtime.
//!
//! Each test exercises one class of input that pre-hardening either
//! succeeded silently (returning garbage) or attempted an unbounded
//! allocation. After the runtime gained `read_array_count(min, max)`,
//! every input here is rejected with a clear `XpbError::InvalidData`.
//! A future change that removes the gate takes a test from "rejected" to
//! "succeeded with bogus value" and fails the suite.

use xpb::{Decoder, Encoder, XpbError};

/// XPB-R001 — read_array_count requires an explicit maxElements that is
/// honored before any allocation. A small payload claiming a billion-int
/// array is rejected, with no Vec preallocation.
#[test]
fn read_array_count_rejects_oversized_caller_max() {
    let mut enc = Encoder::new(8);
    enc.write_int32(1_000_000);
    let bytes = enc.finish();
    let mut dec = Decoder::new(&bytes);

    let res = dec.read_array_count(4, 64);
    match res {
        Err(XpbError::InvalidData(msg)) => {
            assert!(
                msg.contains("exceeds caller-supplied max"),
                "expected caller-max error, got: {}",
                msg
            );
        }
        other => panic!("expected InvalidData, got: {:?}", other),
    }
}

/// XPB-R002 — buffer-bound check still fires as defense in depth. A 4-byte
/// buffer carrying a count of 1_000_000 can never deliver that many
/// elements, so even with a permissive caller max the read is refused.
#[test]
fn read_array_count_rejects_unsatisfiable_buffer() {
    let mut enc = Encoder::new(8);
    enc.write_int32(1_000_000);
    let bytes = enc.finish();
    let mut dec = Decoder::new(&bytes);

    let res = dec.read_array_count(4, 1 << 30);
    match res {
        Err(XpbError::InvalidData(msg)) => {
            assert!(
                msg.contains("exceeds buffer-bounded max"),
                "expected buffer-bound error, got: {}",
                msg
            );
        }
        other => panic!("expected InvalidData, got: {:?}", other),
    }
}

/// XPB-R003 — negative counts (which the wire signed-int32 can express)
/// are rejected before the read_array_count cast to usize would otherwise
/// reinterpret them as huge positive values.
#[test]
fn read_array_count_rejects_negative() {
    let mut enc = Encoder::new(8);
    enc.write_int32(-1);
    let bytes = enc.finish();
    let mut dec = Decoder::new(&bytes);

    let res = dec.read_array_count(4, 1024);
    match res {
        Err(XpbError::InvalidData(msg)) => {
            assert!(
                msg.contains("negative array count"),
                "expected negative-count error, got: {}",
                msg
            );
        }
        other => panic!("expected InvalidData, got: {:?}", other),
    }
}

/// Regression: a legitimate count under both caps round-trips.
#[test]
fn read_array_count_accepts_honest_payload() {
    let mut enc = Encoder::new(64);
    enc.write_int32(4);
    enc.write_int32(10);
    enc.write_int32(20);
    enc.write_int32(30);
    enc.write_int32(40);
    let bytes = enc.finish();
    let mut dec = Decoder::new(&bytes);

    let n = dec.read_array_count(4, 16).expect("honest count accepted");
    assert_eq!(n, 4);
    assert_eq!(dec.read_int32().unwrap(), 10);
    assert_eq!(dec.read_int32().unwrap(), 20);
    assert_eq!(dec.read_int32().unwrap(), 30);
    assert_eq!(dec.read_int32().unwrap(), 40);
}

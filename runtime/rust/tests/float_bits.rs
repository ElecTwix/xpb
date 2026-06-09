//! Explicit float edge-case round-trips for the Rust runtime. Floats are
//! compared by raw bits because `==` is wrong for NaN and conflates -0.0 / +0.0.

use xpb::{Decoder, Encoder};

fn roundtrip_f64(v: f64) -> f64 {
    let mut enc = Encoder::new(8);
    enc.write_float64(v);
    let data = enc.finish();
    Decoder::new(&data).read_float64().unwrap()
}

fn roundtrip_f32(v: f32) -> f32 {
    let mut enc = Encoder::new(4);
    enc.write_float32(v);
    let data = enc.finish();
    Decoder::new(&data).read_float32().unwrap()
}

#[test]
fn nan_f64_bits_preserved() {
    let bits: u64 = 0x7FF8_0000_0000_0001; // a specific quiet NaN payload
    let got = roundtrip_f64(f64::from_bits(bits));
    assert_eq!(got.to_bits(), bits);
    assert!(got.is_nan());
}

#[test]
fn nan_f32_bits_preserved() {
    let bits: u32 = 0x7FC0_0001;
    let got = roundtrip_f32(f32::from_bits(bits));
    assert_eq!(got.to_bits(), bits);
    assert!(got.is_nan());
}

#[test]
fn signed_zero_f64_distinct() {
    let pos = roundtrip_f64(0.0_f64);
    let neg = roundtrip_f64(-0.0_f64);
    assert_eq!(pos.to_bits(), 0x0000_0000_0000_0000);
    assert_eq!(neg.to_bits(), 0x8000_0000_0000_0000);
    assert_ne!(pos.to_bits(), neg.to_bits(), "-0.0 and +0.0 must stay distinct");
}

#[test]
fn signed_zero_f32_distinct() {
    let pos = roundtrip_f32(0.0_f32);
    let neg = roundtrip_f32(-0.0_f32);
    assert_eq!(pos.to_bits(), 0x0000_0000);
    assert_eq!(neg.to_bits(), 0x8000_0000);
    assert_ne!(pos.to_bits(), neg.to_bits());
}

#[test]
fn infinities() {
    assert!(roundtrip_f64(f64::INFINITY).is_infinite() && roundtrip_f64(f64::INFINITY) > 0.0);
    assert!(roundtrip_f64(f64::NEG_INFINITY).is_infinite() && roundtrip_f64(f64::NEG_INFINITY) < 0.0);
    assert!(roundtrip_f32(f32::INFINITY).is_infinite() && roundtrip_f32(f32::INFINITY) > 0.0);
    assert!(roundtrip_f32(f32::NEG_INFINITY).is_infinite() && roundtrip_f32(f32::NEG_INFINITY) < 0.0);
}

#[test]
fn subnormal_and_max() {
    // Smallest positive subnormal and max finite, by bits.
    let f64_sub = f64::from_bits(0x0000_0000_0000_0001);
    assert_eq!(roundtrip_f64(f64_sub).to_bits(), 0x0000_0000_0000_0001);
    assert_eq!(roundtrip_f64(f64::MAX).to_bits(), f64::MAX.to_bits());

    let f32_sub = f32::from_bits(0x0000_0001);
    assert_eq!(roundtrip_f32(f32_sub).to_bits(), 0x0000_0001);
    assert_eq!(roundtrip_f32(f32::MAX).to_bits(), f32::MAX.to_bits());

    assert_eq!(roundtrip_f64(f64::MIN_POSITIVE).to_bits(), f64::MIN_POSITIVE.to_bits());
    assert_eq!(roundtrip_f32(f32::MIN_POSITIVE).to_bits(), f32::MIN_POSITIVE.to_bits());
}

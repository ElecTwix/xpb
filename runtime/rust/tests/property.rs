//! Property-based round-trip tests for all scalar types using proptest.
//! Floats are compared by their raw bit pattern so NaN and -0.0 round-trip
//! exactly (== would be wrong for NaN and would conflate -0.0 with +0.0).

use proptest::prelude::*;
use xpb::{Decoder, Encoder};

proptest! {
    #[test]
    fn roundtrip_bool(v in any::<bool>()) {
        let mut enc = Encoder::new(8);
        enc.write_bool(v);
        let data = enc.finish();
        let mut dec = Decoder::new(&data);
        prop_assert_eq!(dec.read_bool().unwrap(), v);
        prop_assert!(dec.eof());
    }

    #[test]
    fn roundtrip_int32(v in any::<i32>()) {
        let mut enc = Encoder::new(8);
        enc.write_int32(v);
        let data = enc.finish();
        let mut dec = Decoder::new(&data);
        prop_assert_eq!(dec.read_int32().unwrap(), v);
        prop_assert!(dec.eof());
    }

    #[test]
    fn roundtrip_int64(v in any::<i64>()) {
        let mut enc = Encoder::new(8);
        enc.write_int64(v);
        let data = enc.finish();
        let mut dec = Decoder::new(&data);
        prop_assert_eq!(dec.read_int64().unwrap(), v);
        prop_assert!(dec.eof());
    }

    #[test]
    fn roundtrip_uint32(v in any::<u32>()) {
        let mut enc = Encoder::new(8);
        enc.write_uint32(v);
        let data = enc.finish();
        let mut dec = Decoder::new(&data);
        prop_assert_eq!(dec.read_uint32().unwrap(), v);
        prop_assert!(dec.eof());
    }

    #[test]
    fn roundtrip_uint64(v in any::<u64>()) {
        let mut enc = Encoder::new(8);
        enc.write_uint64(v);
        let data = enc.finish();
        let mut dec = Decoder::new(&data);
        prop_assert_eq!(dec.read_uint64().unwrap(), v);
        prop_assert!(dec.eof());
    }

    // Generate floats from arbitrary bit patterns so NaNs / subnormals / -0.0
    // are all covered, and compare by bits.
    #[test]
    fn roundtrip_float32_bits(bits in any::<u32>()) {
        let v = f32::from_bits(bits);
        let mut enc = Encoder::new(8);
        enc.write_float32(v);
        let data = enc.finish();
        let mut dec = Decoder::new(&data);
        let got = dec.read_float32().unwrap();
        prop_assert_eq!(got.to_bits(), bits);
        prop_assert!(dec.eof());
    }

    #[test]
    fn roundtrip_float64_bits(bits in any::<u64>()) {
        let v = f64::from_bits(bits);
        let mut enc = Encoder::new(8);
        enc.write_float64(v);
        let data = enc.finish();
        let mut dec = Decoder::new(&data);
        let got = dec.read_float64().unwrap();
        prop_assert_eq!(got.to_bits(), bits);
        prop_assert!(dec.eof());
    }

    #[test]
    fn roundtrip_string(s in ".{0,512}") {
        let mut enc = Encoder::new(s.len() + 8);
        enc.write_string(&s);
        let data = enc.finish();
        let mut dec = Decoder::new(&data);
        prop_assert_eq!(dec.read_string().unwrap(), s);
        prop_assert!(dec.eof());
    }

    #[test]
    fn roundtrip_bytes(b in prop::collection::vec(any::<u8>(), 0..512)) {
        let mut enc = Encoder::new(b.len() + 8);
        enc.write_bytes(&b);
        let data = enc.finish();
        let mut dec = Decoder::new(&data);
        prop_assert_eq!(dec.read_bytes().unwrap(), b);
        prop_assert!(dec.eof());
    }

    // Mixed sequence: write a vector of tagged scalars, decode them in order.
    #[test]
    fn roundtrip_mixed_sequence(items in prop::collection::vec(scalar_strategy(), 0..32)) {
        let mut enc = Encoder::new(256);
        for item in &items {
            match item {
                Scalar::I32(v) => enc.write_int32(*v),
                Scalar::I64(v) => enc.write_int64(*v),
                Scalar::F64(bits) => enc.write_float64(f64::from_bits(*bits)),
                Scalar::Str(s) => enc.write_string(s),
            }
        }
        let data = enc.finish();
        let mut dec = Decoder::new(&data);
        for item in &items {
            match item {
                Scalar::I32(v) => prop_assert_eq!(dec.read_int32().unwrap(), *v),
                Scalar::I64(v) => prop_assert_eq!(dec.read_int64().unwrap(), *v),
                Scalar::F64(bits) => prop_assert_eq!(dec.read_float64().unwrap().to_bits(), *bits),
                Scalar::Str(s) => prop_assert_eq!(&dec.read_string().unwrap(), s),
            }
        }
        prop_assert!(dec.eof());
    }
}

#[derive(Debug, Clone)]
enum Scalar {
    I32(i32),
    I64(i64),
    F64(u64), // stored as bits for exact comparison
    Str(String),
}

fn scalar_strategy() -> impl Strategy<Value = Scalar> {
    prop_oneof![
        any::<i32>().prop_map(Scalar::I32),
        any::<i64>().prop_map(Scalar::I64),
        any::<u64>().prop_map(Scalar::F64),
        // Include lengths around the 254/255 compact-length boundary.
        ".{0,260}".prop_map(Scalar::Str),
    ]
}

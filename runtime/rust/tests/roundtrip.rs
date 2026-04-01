use xpb::{Encoder, Decoder, XpbError};

#[test]
fn roundtrip_bool() {
    let mut enc = Encoder::new(16);
    enc.write_bool(true);
    enc.write_bool(false);
    let data = enc.finish();

    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_bool().unwrap(), true);
    assert_eq!(dec.read_bool().unwrap(), false);
    assert!(dec.eof());
}

#[test]
fn roundtrip_int32() {
    let mut enc = Encoder::new(16);
    enc.write_int32(42);
    enc.write_int32(-1);
    enc.write_int32(i32::MAX);
    enc.write_int32(i32::MIN);
    let data = enc.finish();

    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_int32().unwrap(), 42);
    assert_eq!(dec.read_int32().unwrap(), -1);
    assert_eq!(dec.read_int32().unwrap(), i32::MAX);
    assert_eq!(dec.read_int32().unwrap(), i32::MIN);
    assert!(dec.eof());
}

#[test]
fn roundtrip_int64() {
    let mut enc = Encoder::new(16);
    enc.write_int64(i64::MAX);
    enc.write_int64(i64::MIN);
    enc.write_int64(0);
    let data = enc.finish();

    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_int64().unwrap(), i64::MAX);
    assert_eq!(dec.read_int64().unwrap(), i64::MIN);
    assert_eq!(dec.read_int64().unwrap(), 0);
    assert!(dec.eof());
}

#[test]
fn roundtrip_uint32() {
    let mut enc = Encoder::new(16);
    enc.write_uint32(u32::MAX);
    enc.write_uint32(0);
    let data = enc.finish();

    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_uint32().unwrap(), u32::MAX);
    assert_eq!(dec.read_uint32().unwrap(), 0);
    assert!(dec.eof());
}

#[test]
fn roundtrip_uint64() {
    let mut enc = Encoder::new(16);
    enc.write_uint64(u64::MAX);
    enc.write_uint64(0);
    let data = enc.finish();

    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_uint64().unwrap(), u64::MAX);
    assert_eq!(dec.read_uint64().unwrap(), 0);
    assert!(dec.eof());
}

#[test]
fn roundtrip_float32() {
    let mut enc = Encoder::new(16);
    enc.write_float32(3.14_f32);
    enc.write_float32(0.0_f32);
    enc.write_float32(f32::INFINITY);
    enc.write_float32(f32::NEG_INFINITY);
    let data = enc.finish();

    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_float32().unwrap(), 3.14_f32);
    assert_eq!(dec.read_float32().unwrap(), 0.0_f32);
    assert_eq!(dec.read_float32().unwrap(), f32::INFINITY);
    assert_eq!(dec.read_float32().unwrap(), f32::NEG_INFINITY);
    assert!(dec.eof());
}

#[test]
fn roundtrip_float64() {
    let mut enc = Encoder::new(16);
    enc.write_float64(3.14159);
    enc.write_float64(0.0);
    enc.write_float64(f64::NEG_INFINITY);
    enc.write_float64(f64::INFINITY);
    let data = enc.finish();

    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_float64().unwrap(), 3.14159);
    assert_eq!(dec.read_float64().unwrap(), 0.0);
    assert_eq!(dec.read_float64().unwrap(), f64::NEG_INFINITY);
    assert_eq!(dec.read_float64().unwrap(), f64::INFINITY);
    assert!(dec.eof());
}

#[test]
fn roundtrip_string() {
    let mut enc = Encoder::new(64);
    enc.write_string("hello");
    enc.write_string("");
    enc.write_string(&"a".repeat(300)); // triggers 5-byte length
    let data = enc.finish();

    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_string().unwrap(), "hello");
    assert_eq!(dec.read_string().unwrap(), "");
    assert_eq!(dec.read_string().unwrap(), "a".repeat(300));
    assert!(dec.eof());
}

#[test]
fn roundtrip_bytes() {
    let mut enc = Encoder::new(16);
    enc.write_bytes(&[1, 2, 3]);
    enc.write_bytes(&[]);
    let data = enc.finish();

    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_bytes().unwrap(), vec![1, 2, 3]);
    assert_eq!(dec.read_bytes().unwrap(), vec![]);
    assert!(dec.eof());
}

#[test]
fn roundtrip_message() {
    // Encode an inner message
    let mut inner_enc = Encoder::new(16);
    inner_enc.write_int32(99);
    let inner_data = inner_enc.finish();

    // Encode outer with nested message
    let mut enc = Encoder::new(32);
    enc.write_message(&inner_data);
    let data = enc.finish();

    let mut dec = Decoder::new(&data);
    let msg_bytes = dec.read_message_bytes().unwrap();
    assert_eq!(msg_bytes, inner_data);
    assert!(dec.eof());

    // Decode the inner message
    let mut inner_dec = Decoder::new(&msg_bytes);
    assert_eq!(inner_dec.read_int32().unwrap(), 99);
}

#[test]
fn decoder_eof() {
    let dec = Decoder::new(&[]);
    assert!(dec.eof());
    assert_eq!(dec.remaining(), 0);
}

#[test]
fn decoder_unexpected_eof() {
    let mut dec = Decoder::new(&[0x01]); // only 1 byte, need 4 for int32
    let err = dec.read_int32().unwrap_err();
    assert_eq!(err, XpbError::UnexpectedEof);
}

#[test]
fn decoder_short_buffer_for_string() {
    // Write a length of 10 but provide no data bytes
    let mut enc = Encoder::new(8);
    enc.write_uint32(0); // just some filler
    let mut data = enc.finish();
    data.clear();
    data.push(10); // compact length = 10
    data.push(0x41); // only 1 byte of payload, need 10

    let mut dec = Decoder::new(&data);
    let err = dec.read_string().unwrap_err();
    assert_eq!(err, XpbError::UnexpectedEof);
}

#[test]
fn encoder_reset() {
    let mut enc = Encoder::new(16);
    enc.write_int32(42);
    assert_eq!(enc.len(), 4);
    enc.reset();
    assert_eq!(enc.len(), 0);
    assert!(enc.is_empty());
}

#[test]
fn encoder_as_bytes() {
    let mut enc = Encoder::new(16);
    enc.write_bool(true);
    assert_eq!(enc.as_bytes(), &[1]);
}

#[test]
fn decoder_skip() {
    let mut enc = Encoder::new(16);
    enc.write_int32(1);
    enc.write_int32(2);
    let data = enc.finish();

    let mut dec = Decoder::new(&data);
    dec.skip(4).unwrap(); // skip first int32
    assert_eq!(dec.read_int32().unwrap(), 2);
}

#[test]
fn decoder_skip_too_far() {
    let mut dec = Decoder::new(&[0x00]);
    let err = dec.skip(2).unwrap_err();
    assert_eq!(err, XpbError::UnexpectedEof);
}

#[test]
fn append_raw() {
    let mut enc = Encoder::new(16);
    enc.append_raw(&[0xDE, 0xAD]);
    assert_eq!(enc.as_bytes(), &[0xDE, 0xAD]);
}

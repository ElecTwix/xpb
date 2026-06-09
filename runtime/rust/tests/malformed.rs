//! Malformed / adversarial input tests for the Rust decoder. Mirrors the Go
//! `malformed_test.go` family. Every case must return `Err(UnexpectedEof)`
//! (or another clean error) and must never panic.

use xpb::{Decoder, XpbError};

#[test]
fn truncated_length_marker() {
    // 0xFF marker says "4-byte length follows" but fewer than 4 bytes remain.
    for data in [
        vec![0xFF_u8],
        vec![0xFF, 0x01],
        vec![0xFF, 0x01, 0x02],
        vec![0xFF, 0x01, 0x02, 0x03],
    ] {
        let mut dec = Decoder::new(&data);
        assert_eq!(dec.read_string().unwrap_err(), XpbError::UnexpectedEof, "string {:x?}", data);

        let mut dec = Decoder::new(&data);
        assert_eq!(dec.read_bytes().unwrap_err(), XpbError::UnexpectedEof, "bytes {:x?}", data);
    }
}

#[test]
fn length_larger_than_buffer() {
    // Compact length 10 but only 1 payload byte present.
    let data = [0x0A_u8, 0x41];

    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_string().unwrap_err(), XpbError::UnexpectedEof);

    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_bytes().unwrap_err(), XpbError::UnexpectedEof);

    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_message_bytes().unwrap_err(), XpbError::UnexpectedEof);
}

#[test]
fn huge_length_prefix_no_oom() {
    // 0xFF marker + length ~4GB (0xFFFFFFFE) little-endian, then 1 payload byte.
    // The decoder must error on bounds BEFORE allocating ~4GB.
    let data = [0xFF_u8, 0xFE, 0xFF, 0xFF, 0xFF, 0x00];

    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_bytes().unwrap_err(), XpbError::UnexpectedEof);

    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_string().unwrap_err(), XpbError::UnexpectedEof);
}

#[test]
fn read_past_eof() {
    let data = [0x01_u8, 0x02, 0x03]; // only 3 bytes
    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_int32().unwrap_err(), XpbError::UnexpectedEof);
    assert_eq!(dec.read_int64().unwrap_err(), XpbError::UnexpectedEof);
    assert_eq!(dec.read_float32().unwrap_err(), XpbError::UnexpectedEof);
    assert_eq!(dec.read_float64().unwrap_err(), XpbError::UnexpectedEof);
}

#[test]
fn repeated_reads_after_eof() {
    let data = [0xAA_u8]; // 1 byte
    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_bool().unwrap(), true);
    // Now at EOF: hammer reads, each must keep erroring, never panic.
    for _ in 0..100 {
        assert_eq!(dec.read_bool().unwrap_err(), XpbError::UnexpectedEof);
        assert_eq!(dec.read_int32().unwrap_err(), XpbError::UnexpectedEof);
        assert_eq!(dec.read_string().unwrap_err(), XpbError::UnexpectedEof);
    }
}

#[test]
fn skip_too_far() {
    let data = [0x00_u8, 0x01];
    let mut dec = Decoder::new(&data);
    assert_eq!(dec.skip(3).unwrap_err(), XpbError::UnexpectedEof);
    // pos untouched: a valid skip still works.
    dec.skip(2).unwrap();
    assert!(dec.eof());
}

#[test]
fn skip_overflow_is_rejected() {
    // skip(usize::MAX) must not wrap `pos + n` and pass the bounds check.
    let data = [0x00_u8; 4];
    let mut dec = Decoder::new(&data);
    assert_eq!(dec.skip(usize::MAX).unwrap_err(), XpbError::UnexpectedEof);
    // Buffer intact: a normal read still works.
    assert_eq!(dec.read_int32().unwrap(), 0);
}

#[test]
fn nested_message_length_exceeds_buffer() {
    // Outer claims a nested message of length 50 but only 2 bytes follow.
    let data = [50_u8, 0xAA, 0xBB];
    let mut dec = Decoder::new(&data);
    assert_eq!(dec.read_message_bytes().unwrap_err(), XpbError::UnexpectedEof);
}

#[test]
fn invalid_utf8_string_is_clean_error() {
    // Length 2, payload is invalid UTF-8 (lone continuation bytes). Must be a
    // clean InvalidData error, not a panic.
    let data = [0x02_u8, 0xFF, 0xFE];
    let mut dec = Decoder::new(&data);
    match dec.read_string() {
        Err(XpbError::InvalidData(_)) => {}
        other => panic!("expected InvalidData, got {:?}", other),
    }
}

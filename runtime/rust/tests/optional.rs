// Round-trip tests for the optional-field wire encoding.
//
// An optional (`?`) field is encoded as a 1-byte presence flag (0x01 + value
// when present, 0x00 with no value bytes when absent), mirroring the bool
// encoding. The structs below are byte-for-byte what `xpbc --lang=rust`
// generates for:
//
//   message Profile { 1: string bio; 2: ?string avatar_url; 3: int32 followers }
//   message Pair    { 1: ?string a;  2: int32 b }
//
// They are inlined here (rather than generated at test time) so the Rust
// runtime crate's `cargo test` exercises the optional codegen contract without
// depending on the Go toolchain.

use xpb::{Decoder, Encoder, Result};

#[derive(Debug, Clone, Default)]
struct Profile {
    bio: String,
    avatar_url: Option<String>,
    followers: i32,
}

impl Profile {
    fn marshal(&self) -> Vec<u8> {
        let mut enc = Encoder::new(64);
        enc.write_string(&self.bio);
        enc.write_bool(self.avatar_url.is_some());
        if let Some(ref v) = self.avatar_url {
            enc.write_string(v);
        }
        enc.write_int32(self.followers);
        enc.finish()
    }

    fn unmarshal(data: &[u8]) -> Result<Self> {
        let mut dec = Decoder::new(data);
        let mut msg = Self::default();
        msg.bio = dec.read_string()?;
        if dec.read_bool()? {
            msg.avatar_url = Some(dec.read_string()?);
        }
        msg.followers = dec.read_int32()?;
        Ok(msg)
    }
}

#[derive(Debug, Clone, Default)]
struct Pair {
    a: Option<String>,
    b: i32,
}

impl Pair {
    fn marshal(&self) -> Vec<u8> {
        let mut enc = Encoder::new(64);
        enc.write_bool(self.a.is_some());
        if let Some(ref v) = self.a {
            enc.write_string(v);
        }
        enc.write_int32(self.b);
        enc.finish()
    }

    fn unmarshal(data: &[u8]) -> Result<Self> {
        let mut dec = Decoder::new(data);
        let mut msg = Self::default();
        if dec.read_bool()? {
            msg.a = Some(dec.read_string()?);
        }
        msg.b = dec.read_int32()?;
        Ok(msg)
    }
}

#[test]
fn optional_present_roundtrips_value() {
    let input = Profile {
        bio: "hi".to_string(),
        avatar_url: Some("http://x/y.png".to_string()),
        followers: 9,
    };
    let out = Profile::unmarshal(&input.marshal()).unwrap();
    assert_eq!(out.bio, "hi");
    assert_eq!(out.avatar_url.as_deref(), Some("http://x/y.png"));
    assert_eq!(out.followers, 9);
}

#[test]
fn optional_absent_roundtrips_as_none_and_next_field_decodes() {
    let input = Profile {
        bio: "hi".to_string(),
        avatar_url: None,
        followers: 9,
    };
    let out = Profile::unmarshal(&input.marshal()).unwrap();
    // Absent optional decodes to None ...
    assert_eq!(out.avatar_url, None);
    // ... and the field before AND after it are intact (presence byte consumed).
    assert_eq!(out.bio, "hi");
    assert_eq!(out.followers, 9, "field after absent optional corrupted");
}

#[test]
fn absent_optional_consumes_exactly_one_byte() {
    // {?string a (absent), int32 b}: 1 presence byte (0x00) + 4-byte int32.
    let input = Pair { a: None, b: 1234 };
    let data = input.marshal();
    assert_eq!(data.len(), 5, "absent-optional Pair must be 5 bytes, got {:?}", data);
    assert_eq!(data[0], 0x00, "absent presence byte must be 0x00");
    // 1234 = 0x04D2, little-endian after the presence byte.
    assert_eq!(&data[1..], &[0xD2, 0x04, 0x00, 0x00]);

    let out = Pair::unmarshal(&data).unwrap();
    assert_eq!(out.a, None);
    assert_eq!(out.b, 1234);
}

#[test]
fn present_optional_emits_presence_byte_then_value() {
    // {?string a = "ok", int32 b}: 0x01 presence, then "ok" (len 2 + bytes),
    // then the int32.
    let input = Pair {
        a: Some("ok".to_string()),
        b: 7,
    };
    let data = input.marshal();
    assert_eq!(data[0], 0x01, "present presence byte must be 0x01");
    assert_eq!(data[1], 0x02, "string length prefix");
    assert_eq!(&data[2..4], b"ok");
    assert_eq!(&data[4..], &[0x07, 0x00, 0x00, 0x00]);

    let out = Pair::unmarshal(&data).unwrap();
    assert_eq!(out.a.as_deref(), Some("ok"));
    assert_eq!(out.b, 7);
}

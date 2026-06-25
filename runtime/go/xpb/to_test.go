package xpb

import (
	"bytes"
	"testing"
)

// These tests cover the stateless cursor append helpers (the *To family) added
// in Phase 2. They assert three things for every helper, mirroring at_test.go:
//  1. round-trip: a value appended via the *To helper decodes back via the
//     matching *At read helper to the original value;
//  2. differential parity: the *To helper produces byte-identical output to its
//     stateful (*Encoder).Write* counterpart, proving the buffer rewrite keeps
//     the wire format unchanged;
//  3. the length-prefixed helpers preserve the compact-length 0xFF marker path
//     past the 254-byte boundary.

// TestAppendTo_Scalars_RoundTripAndWidth appends every fixed-width scalar via
// the *To helpers and reads them back via the *At helpers, checking value and
// cursor width.
func TestAppendTo_Scalars_RoundTripAndWidth(t *testing.T) {
	var buf []byte
	buf = AppendBoolTo(buf, true)
	buf = AppendInt32To(buf, -123456)
	buf = AppendInt64To(buf, -9000000000)
	buf = AppendUint32To(buf, 4000000000)
	buf = AppendUint64To(buf, 18000000000000000000)
	buf = AppendFloat32To(buf, 3.5)
	buf = AppendFloat64To(buf, 2.718281828)

	pos := 0
	bv, pos, err := ReadBoolAt(buf, pos)
	if err != nil || bv != true || pos != 1 {
		t.Fatalf("bool round-trip = (%v, %d, %v)", bv, pos, err)
	}
	i32, pos, err := ReadInt32At(buf, pos)
	if err != nil || i32 != -123456 || pos != 5 {
		t.Fatalf("int32 round-trip = (%v, %d, %v)", i32, pos, err)
	}
	i64, pos, err := ReadInt64At(buf, pos)
	if err != nil || i64 != -9000000000 || pos != 13 {
		t.Fatalf("int64 round-trip = (%v, %d, %v)", i64, pos, err)
	}
	u32, pos, err := ReadUint32At(buf, pos)
	if err != nil || u32 != 4000000000 || pos != 17 {
		t.Fatalf("uint32 round-trip = (%v, %d, %v)", u32, pos, err)
	}
	u64, pos, err := ReadUint64At(buf, pos)
	if err != nil || u64 != 18000000000000000000 || pos != 25 {
		t.Fatalf("uint64 round-trip = (%v, %d, %v)", u64, pos, err)
	}
	f32, pos, err := ReadFloat32At(buf, pos)
	if err != nil || f32 != 3.5 || pos != 29 {
		t.Fatalf("float32 round-trip = (%v, %d, %v)", f32, pos, err)
	}
	f64, pos, err := ReadFloat64At(buf, pos)
	if err != nil || f64 != 2.718281828 || pos != 37 {
		t.Fatalf("float64 round-trip = (%v, %d, %v)", f64, pos, err)
	}
	if pos != len(buf) {
		t.Fatalf("final cursor = %d, want %d (whole buffer consumed)", pos, len(buf))
	}
}

// TestAppendStringTo_RoundTrip round-trips a non-empty then an empty string.
func TestAppendStringTo_RoundTrip(t *testing.T) {
	var buf []byte
	buf = AppendStringTo(buf, "hello world")
	buf = AppendStringTo(buf, "")

	s, pos, err := ReadStringAt(buf, 0)
	if err != nil || s != "hello world" {
		t.Fatalf("string round-trip = (%q, %d, %v)", s, pos, err)
	}
	s, pos, err = ReadStringAt(buf, pos)
	if err != nil || s != "" {
		t.Fatalf("empty string round-trip = (%q, %d, %v)", s, pos, err)
	}
	if pos != len(buf) {
		t.Fatalf("final cursor = %d, want %d", pos, len(buf))
	}
}

// TestAppendBytesTo_RoundTrip round-trips a byte payload.
func TestAppendBytesTo_RoundTrip(t *testing.T) {
	data := []byte{0xde, 0xad, 0xbe, 0xef}
	buf := AppendBytesTo(nil, data)
	got, pos, err := ReadBytesAt(buf, 0)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("bytes round-trip = (% x, %d, %v)", got, pos, err)
	}
	if pos != len(buf) {
		t.Fatalf("final cursor = %d, want %d", pos, len(buf))
	}
}

// TestAppendMessageTo_RoundTrip round-trips a nested-message envelope.
func TestAppendMessageTo_RoundTrip(t *testing.T) {
	body := []byte{1, 2, 3, 4, 5}
	buf := AppendMessageTo(nil, body)
	got, pos, err := ReadMessageBytesAt(buf, 0)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("message round-trip = (% x, %d, %v)", got, pos, err)
	}
	if pos != len(buf) {
		t.Fatalf("final cursor = %d, want %d", pos, len(buf))
	}
}

// TestAppendCompactLengthTo_MarkerPath proves the 0xFF marker path: a length
// <= 254 is a single byte; a length past the boundary emits the marker plus a
// 4-byte little-endian length, decoding back identically via readCompactLengthAt.
func TestAppendCompactLengthTo_MarkerPath(t *testing.T) {
	for _, n := range []int{0, 1, 254, 255, 300, 70000} {
		buf := AppendCompactLengthTo(nil, n)
		got, pos, err := readCompactLengthAt(buf, 0)
		if err != nil || got != n || pos != len(buf) {
			t.Fatalf("compact length %d round-trip = (%d, %d, %v)", n, got, pos, err)
		}
		// Layout check: <=254 is 1 byte; otherwise marker + 4 bytes.
		wantLen := 1
		if n > 254 {
			wantLen = 5
			if buf[0] != 0xFF {
				t.Fatalf("compact length %d should start with 0xFF marker, got 0x%02x", n, buf[0])
			}
		}
		if len(buf) != wantLen {
			t.Fatalf("compact length %d encoded to %d bytes, want %d", n, len(buf), wantLen)
		}
	}
}

// TestAppendTo_DifferentialParity proves the *To helpers produce byte-identical
// output to the stateful (*Encoder).Write* methods over a mixed message,
// including the >254-byte compact-length path. This is the regression guard
// that the local-buffer encode rewrite preserves the exact wire format.
func TestAppendTo_DifferentialParity(t *testing.T) {
	big := bytes.Repeat([]byte{0xAB}, 512) // trips the 0xFF compact-length path

	// Stateful Encoder reference output.
	enc := NewEncoder(64)
	enc.WriteBool(true)
	enc.WriteInt32(-7)
	enc.WriteInt64(1 << 40)
	enc.WriteUint32(4242)
	enc.WriteUint64(1 << 50)
	enc.WriteFloat32(1.25)
	enc.WriteFloat64(6.022e23)
	enc.WriteString("differential")
	enc.WriteString(string(big)) // long string -> 0xFF path
	enc.WriteBytes([]byte{9, 8, 7})
	enc.WriteBytes(big) // long bytes -> 0xFF path
	enc.WriteMessage([]byte{1, 2, 3})
	want := enc.Bytes()

	// Stateless *To output.
	var buf []byte
	buf = AppendBoolTo(buf, true)
	buf = AppendInt32To(buf, -7)
	buf = AppendInt64To(buf, 1<<40)
	buf = AppendUint32To(buf, 4242)
	buf = AppendUint64To(buf, 1<<50)
	buf = AppendFloat32To(buf, 1.25)
	buf = AppendFloat64To(buf, 6.022e23)
	buf = AppendStringTo(buf, "differential")
	buf = AppendStringTo(buf, string(big))
	buf = AppendBytesTo(buf, []byte{9, 8, 7})
	buf = AppendBytesTo(buf, big)
	buf = AppendMessageTo(buf, []byte{1, 2, 3})

	if !bytes.Equal(buf, want) {
		t.Fatalf("stateless *To output differs from stateful Encoder:\n got=% x\n want=% x", buf, want)
	}
}

// TestEncoder_BufSetBufRoundTrip proves Buf()/SetBuf() let a local buffer be
// bound from the encoder, appended into, and written back once, with Bytes()
// then reflecting exactly the local content. This is the binding contract the
// generated Marshal/MarshalTo bodies rely on.
func TestEncoder_BufSetBufRoundTrip(t *testing.T) {
	enc := NewEncoder(8)
	buf := enc.Buf()
	if len(buf) != 0 {
		t.Fatalf("fresh encoder Buf() len = %d, want 0", len(buf))
	}
	buf = GrowBuf(buf, 16)
	buf = AppendInt32To(buf, 0x01020304)
	buf = AppendStringTo(buf, "x")
	enc.SetBuf(buf)
	if !bytes.Equal(enc.Bytes(), buf) {
		t.Fatalf("after SetBuf, Bytes()=% x != local buf % x", enc.Bytes(), buf)
	}
	// SetBuf preserves the exact length written into the local.
	if enc.Len() != len(buf) {
		t.Fatalf("Len()=%d, want %d", enc.Len(), len(buf))
	}
}

// TestEncoder_BufSetBuf_AppendsToExistingContent proves the append-to-existing
// contract that MarshalTo relies on: Buf() returns the encoder's FULL buffer
// (not buf[:0]), so a local bound from a non-empty encoder, grown and appended
// into, then written back via SetBuf, preserves the pre-existing prefix and
// appends after it. If Buf() were changed to return e.buf[:0], or GrowBuf/SetBuf
// mishandled a non-empty prefix, the prefix would be dropped or corrupted — this
// test fails loudly in that case. (Review finding: the prior round only ever
// bound from a fresh/empty encoder.)
func TestEncoder_BufSetBuf_AppendsToExistingContent(t *testing.T) {
	enc := NewEncoder(8)
	// Pre-load bytes via the stateful API, exactly as a manual caller might
	// before invoking a generated MarshalTo on the same encoder.
	enc.WriteString("prefix")
	prefix := append([]byte(nil), enc.Bytes()...) // snapshot before appending

	// Now run the generated-encode pattern on the same (non-empty) encoder.
	buf := enc.Buf()
	if !bytes.Equal(buf, prefix) {
		t.Fatalf("Buf() must return the full existing buffer; got % x, want % x", buf, prefix)
	}
	buf = GrowBuf(buf, 16)
	if !bytes.Equal(buf, prefix) {
		t.Fatalf("GrowBuf must preserve the existing prefix; got % x, want % x", buf, prefix)
	}
	buf = AppendInt32To(buf, 0x01020304)
	buf = AppendStringTo(buf, "x")
	enc.SetBuf(buf)

	// The encoder's bytes must be exactly: prefix ++ the appended fields.
	var want []byte
	want = append(want, prefix...)
	want = AppendInt32To(want, 0x01020304)
	want = AppendStringTo(want, "x")
	if !bytes.Equal(enc.Bytes(), want) {
		t.Fatalf("append-to-existing produced % x, want % x", enc.Bytes(), want)
	}

	// And the prefix must still decode back intact at offset 0.
	s, pos, err := ReadStringAt(enc.Bytes(), 0)
	if err != nil || s != "prefix" {
		t.Fatalf("prefix corrupted: ReadStringAt = (%q, %d, %v)", s, pos, err)
	}
	// Followed by the appended int32 and string.
	i32, pos, err := ReadInt32At(enc.Bytes(), pos)
	if err != nil || i32 != 0x01020304 {
		t.Fatalf("appended int32 = (%#x, %d, %v), want 0x01020304", i32, pos, err)
	}
	s2, pos, err := ReadStringAt(enc.Bytes(), pos)
	if err != nil || s2 != "x" || pos != len(enc.Bytes()) {
		t.Fatalf("appended string = (%q, %d, %v), want (\"x\", end, nil)", s2, pos, err)
	}
}

// TestGrowBuf_PreservesLenAndContents proves GrowBuf only grows capacity; the
// length and existing contents are unchanged, and subsequent appends fit
// without reallocating when the grow was sufficient.
func TestGrowBuf_PreservesLenAndContents(t *testing.T) {
	src := []byte{1, 2, 3}
	got := GrowBuf(src, 100)
	if len(got) != len(src) || !bytes.Equal(got, src) {
		t.Fatalf("GrowBuf changed len/contents: got % x (len %d), want % x (len %d)", got, len(got), src, len(src))
	}
	if cap(got) < len(src)+100 {
		t.Fatalf("GrowBuf cap = %d, want >= %d", cap(got), len(src)+100)
	}
}

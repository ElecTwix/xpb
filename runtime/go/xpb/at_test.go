package xpb

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/ElecTwix/xpb/pkg/wire"
)

// These tests cover the stateless cursor read helpers (the *At family). They
// assert three things for every helper:
//  1. round-trip: a value written by the Encoder decodes back via the *At
//     helper, and the returned cursor advances by exactly the wire width;
//  2. differential parity: the *At helper agrees with its stateful (*Decoder)
//     counterpart on both the decoded value and the resulting offset, proving
//     the cursor rewrite preserves identical semantics;
//  3. malformed/bounds safety: truncated buffers, oversized length prefixes,
//     and negative array counts are rejected exactly as the Decoder rejects
//     them, with the cursor left unchanged on the EOF guards.

func TestReadAt_Scalars_RoundTripAndWidth(t *testing.T) {
	enc := NewEncoder(64)
	enc.WriteBool(true)
	enc.WriteInt32(-123456)
	enc.WriteInt64(-9000000000)
	enc.WriteUint32(4000000000)
	enc.WriteUint64(18000000000000000000)
	enc.WriteFloat32(3.5)
	enc.WriteFloat64(2.718281828)
	b := enc.Bytes()

	pos := 0
	bv, pos, err := ReadBoolAt(b, pos)
	if err != nil || bv != true || pos != 1 {
		t.Fatalf("ReadBoolAt = (%v, %d, %v), want (true, 1, nil)", bv, pos, err)
	}
	i32, pos, err := ReadInt32At(b, pos)
	if err != nil || i32 != -123456 || pos != 5 {
		t.Fatalf("ReadInt32At = (%v, %d, %v), want (-123456, 5, nil)", i32, pos, err)
	}
	i64, pos, err := ReadInt64At(b, pos)
	if err != nil || i64 != -9000000000 || pos != 13 {
		t.Fatalf("ReadInt64At = (%v, %d, %v), want (-9000000000, 13, nil)", i64, pos, err)
	}
	u32, pos, err := ReadUint32At(b, pos)
	if err != nil || u32 != 4000000000 || pos != 17 {
		t.Fatalf("ReadUint32At = (%v, %d, %v), want (4000000000, 17, nil)", u32, pos, err)
	}
	u64, pos, err := ReadUint64At(b, pos)
	if err != nil || u64 != 18000000000000000000 || pos != 25 {
		t.Fatalf("ReadUint64At = (%v, %d, %v), want (18000000000000000000, 25, nil)", u64, pos, err)
	}
	f32, pos, err := ReadFloat32At(b, pos)
	if err != nil || f32 != 3.5 || pos != 29 {
		t.Fatalf("ReadFloat32At = (%v, %d, %v), want (3.5, 29, nil)", f32, pos, err)
	}
	f64, pos, err := ReadFloat64At(b, pos)
	if err != nil || f64 != 2.718281828 || pos != 37 {
		t.Fatalf("ReadFloat64At = (%v, %d, %v), want (2.718281828, 37, nil)", f64, pos, err)
	}
	if pos != len(b) {
		t.Fatalf("final cursor = %d, want %d (whole buffer consumed)", pos, len(b))
	}
}

func TestReadStringAt_RoundTrip(t *testing.T) {
	enc := NewEncoder(64)
	enc.WriteString("hello world")
	enc.WriteString("") // empty string after a non-empty one
	b := enc.Bytes()

	s, pos, err := ReadStringAt(b, 0)
	if err != nil || s != "hello world" {
		t.Fatalf("ReadStringAt = (%q, %d, %v), want (%q, _, nil)", s, pos, err, "hello world")
	}
	s, pos, err = ReadStringAt(b, pos)
	if err != nil || s != "" {
		t.Fatalf("ReadStringAt empty = (%q, %d, %v), want (\"\", _, nil)", s, pos, err)
	}
	if pos != len(b) {
		t.Fatalf("final cursor = %d, want %d", pos, len(b))
	}
}

// TestReadStringAt_ZeroCopyAliases proves ReadStringAt returns a zero-copy view
// that aliases the input buffer, matching (*Decoder).ReadString.
func TestReadStringAt_ZeroCopyAliases(t *testing.T) {
	enc := NewEncoder(64)
	enc.WriteString("alias-me")
	b := enc.Bytes()

	s, _, err := ReadStringAt(b, 0)
	if err != nil {
		t.Fatalf("ReadStringAt: %v", err)
	}
	if s != "alias-me" {
		t.Fatalf("ReadStringAt = %q, want %q", s, "alias-me")
	}
	// Mutate the underlying buffer at the string's first content byte; a
	// zero-copy alias observes the change. The content starts at offset 1
	// (1-byte compact length for an 8-byte string).
	b[1] = 'X'
	if s[0] != 'X' {
		t.Fatalf("ReadStringAt did not alias the buffer: s[0]=%q after mutation", s[0])
	}
}

func TestReadBytesAt_CopiesAndRoundTrips(t *testing.T) {
	data := []byte{0xde, 0xad, 0xbe, 0xef}
	enc := NewEncoder(64)
	enc.WriteBytes(data)
	b := enc.Bytes()

	got, pos, err := ReadBytesAt(b, 0)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("ReadBytesAt = (% x, %d, %v), want (% x, _, nil)", got, pos, err, data)
	}
	if pos != len(b) {
		t.Fatalf("final cursor = %d, want %d", pos, len(b))
	}
	// ReadBytesAt must COPY: mutating the source buffer must not change the
	// returned slice (offset 1 is the first content byte after the 1-byte len).
	b[1] = 0x00
	if got[0] != 0xde {
		t.Fatalf("ReadBytesAt aliased the buffer; expected a copy")
	}
}

func TestReadBytesUnsafeAt_Aliases(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}
	enc := NewEncoder(64)
	enc.WriteBytes(data)
	b := enc.Bytes()

	got, pos, err := ReadBytesUnsafeAt(b, 0)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("ReadBytesUnsafeAt = (% x, %d, %v), want (% x, _, nil)", got, pos, err, data)
	}
	if pos != len(b) {
		t.Fatalf("final cursor = %d, want %d", pos, len(b))
	}
	// Zero-copy: the returned slice must alias the input buffer.
	b[1] = 0xFE
	if got[0] != 0xFE {
		t.Fatalf("ReadBytesUnsafeAt did not alias the buffer")
	}
}

// TestReadCompactLengthAt_BothPaths exercises both the 1-byte length path and
// the 0xFF marker + 4-byte length path, plus the empty-buffer guard.
func TestReadCompactLengthAt_BothPaths(t *testing.T) {
	// Short path: a single byte < marker is the length itself.
	short := []byte{0x07, 'x'}
	n, pos, err := readCompactLengthAt(short, 0)
	if err != nil || n != 7 || pos != 1 {
		t.Fatalf("short readCompactLengthAt = (%d, %d, %v), want (7, 1, nil)", n, pos, err)
	}

	// Long path: a >254-byte string trips the 0xFF marker. Build it via the
	// encoder so the wire layout is authoritative.
	long := strings.Repeat("u", 300)
	enc := NewEncoder(512)
	enc.WriteString(long)
	b := enc.Bytes()
	n, pos, err = readCompactLengthAt(b, 0)
	if err != nil {
		t.Fatalf("long readCompactLengthAt: %v", err)
	}
	if n != 300 {
		t.Fatalf("long readCompactLengthAt length = %d, want 300", n)
	}
	if b[0] != wire.CompactLengthMarker {
		t.Fatalf("expected 0xFF marker at b[0], got 0x%02x", b[0])
	}
	if pos != 5 { // 1 marker + 4 length bytes
		t.Fatalf("long readCompactLengthAt cursor = %d, want 5", pos)
	}

	// Empty buffer: EOF, cursor unchanged.
	if _, p, err := readCompactLengthAt(nil, 0); !errors.Is(err, io.ErrUnexpectedEOF) || p != 0 {
		t.Fatalf("empty readCompactLengthAt = (_, %d, %v), want (0, ErrUnexpectedEOF)", p, err)
	}

	// Marker present but the 4-byte length is truncated: EOF.
	truncated := []byte{wire.CompactLengthMarker, 0x01, 0x02}
	if _, _, err := readCompactLengthAt(truncated, 0); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated 0xFF length: err = %v, want ErrUnexpectedEOF", err)
	}
}

// TestReadAt_Malformed_Truncated asserts every fixed-width / length-prefixed
// helper returns ErrUnexpectedEOF when the buffer is too small. The read
// closure returns the post-read cursor as well so the test can assert the
// cursor is left unchanged on the EOF guards (the contract documented in this
// file's header).
func TestReadAt_Malformed_Truncated(t *testing.T) {
	cases := []struct {
		name string
		buf  []byte
		read func([]byte, int) (int, error)
	}{
		{"bool empty", nil, func(b []byte, p int) (int, error) { _, np, e := ReadBoolAt(b, p); return np, e }},
		{"int32 short", []byte{0x00, 0x00, 0x00}, func(b []byte, p int) (int, error) { _, np, e := ReadInt32At(b, p); return np, e }},
		{"int64 short", []byte{0, 0, 0, 0, 0, 0, 0}, func(b []byte, p int) (int, error) { _, np, e := ReadInt64At(b, p); return np, e }},
		{"uint32 short", []byte{0x00}, func(b []byte, p int) (int, error) { _, np, e := ReadUint32At(b, p); return np, e }},
		{"uint64 short", []byte{0, 0, 0, 0}, func(b []byte, p int) (int, error) { _, np, e := ReadUint64At(b, p); return np, e }},
		{"float32 short", []byte{0x00, 0x00}, func(b []byte, p int) (int, error) { _, np, e := ReadFloat32At(b, p); return np, e }},
		{"float64 short", []byte{0, 0, 0}, func(b []byte, p int) (int, error) { _, np, e := ReadFloat64At(b, p); return np, e }},
		// length prefix says 5 bytes but only 2 follow
		{"string len exceeds buf", []byte{0x05, 'a', 'b'}, func(b []byte, p int) (int, error) { _, np, e := ReadStringAt(b, p); return np, e }},
		{"bytes len exceeds buf", []byte{0x05, 'a', 'b'}, func(b []byte, p int) (int, error) { _, np, e := ReadBytesAt(b, p); return np, e }},
		{"bytes unsafe len exceeds buf", []byte{0x05, 'a', 'b'}, func(b []byte, p int) (int, error) { _, np, e := ReadBytesUnsafeAt(b, p); return np, e }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			np, err := c.read(c.buf, 0)
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("%s: err = %v, want ErrUnexpectedEOF", c.name, err)
			}
			// The fixed-width scalar guards return the input cursor unchanged
			// on EOF (p+N > len(b) fails before any advance). The
			// length-prefixed helpers consume the length prefix before the
			// content-bounds check fails, so their cursor may legitimately have
			// advanced past the prefix; only assert the no-advance contract for
			// the fixed-width scalars, where the guard precedes any movement.
			switch c.name {
			case "bool empty", "int32 short", "int64 short", "uint32 short",
				"uint64 short", "float32 short", "float64 short":
				if np != 0 {
					t.Fatalf("%s: cursor advanced to %d on EOF, want unchanged (0)", c.name, np)
				}
			}
		})
	}
}

// TestReadStringBytesAt_LongPath round-trips a >254-byte string and bytes
// payload through ReadStringAt / ReadBytesAt, proving the 0xFF compact-length
// marker path decodes the full content and lands the cursor at end-of-buffer
// (not just the length-helper unit test).
func TestReadStringBytesAt_LongPath(t *testing.T) {
	long := strings.Repeat("z", 300) // > 254 -> 0xFF marker path
	enc := NewEncoder(512)
	enc.WriteString(long)
	sb := enc.Bytes()
	if sb[0] != wire.CompactLengthMarker {
		t.Fatalf("expected 0xFF marker for long string, got 0x%02x", sb[0])
	}
	s, pos, err := ReadStringAt(sb, 0)
	if err != nil || s != long || pos != len(sb) {
		t.Fatalf("ReadStringAt long = (len %d, %d, %v), want (300, %d, nil); match=%v",
			len(s), pos, err, len(sb), s == long)
	}

	payload := bytes.Repeat([]byte{0xAB}, 512) // > 254 -> 0xFF marker path
	enc2 := NewEncoder(1024)
	enc2.WriteBytes(payload)
	bb := enc2.Bytes()
	if bb[0] != wire.CompactLengthMarker {
		t.Fatalf("expected 0xFF marker for long bytes, got 0x%02x", bb[0])
	}
	got, pos2, err := ReadBytesAt(bb, 0)
	if err != nil || !bytes.Equal(got, payload) || pos2 != len(bb) {
		t.Fatalf("ReadBytesAt long = (len %d, %d, %v), want (512, %d, nil)", len(got), pos2, err, len(bb))
	}
}

// TestReadMessageBytesAt round-trips a length-prefixed message envelope: the
// returned body slice equals the original payload, aliases the input buffer
// (zero-copy, matching ReadBytesUnsafeAt), and the cursor lands past the
// envelope. An empty envelope decodes to an empty (len-0) body so the generated
// `len(mb) > 0` nested guard sees the nil-pointer case.
func TestReadMessageBytesAt(t *testing.T) {
	body := []byte{0x01, 0x02, 0x03, 0x04}
	enc := NewEncoder(64)
	enc.WriteMessage(body)
	b := enc.Bytes()

	got, pos, err := ReadMessageBytesAt(b, 0)
	if err != nil || !bytes.Equal(got, body) || pos != len(b) {
		t.Fatalf("ReadMessageBytesAt = (% x, %d, %v), want (% x, %d, nil)", got, pos, err, body, len(b))
	}
	// Zero-copy alias (offset 1 is the first body byte after the 1-byte len).
	b[1] = 0xFF
	if got[0] != 0xFF {
		t.Fatalf("ReadMessageBytesAt did not alias the buffer")
	}

	// Empty envelope: 0-length body, cursor advances past the length byte.
	enc2 := NewEncoder(8)
	enc2.WriteMessage(nil)
	eb := enc2.Bytes()
	emptyBody, epos, err := ReadMessageBytesAt(eb, 0)
	if err != nil || len(emptyBody) != 0 || epos != len(eb) {
		t.Fatalf("ReadMessageBytesAt empty = (len %d, %d, %v), want (0, %d, nil)", len(emptyBody), epos, err, len(eb))
	}
}

// TestReadAt_NoOOM_HugeLengthPrefix mirrors the malformed-test guard: a length
// prefix claiming a gigabyte while the buffer holds a handful of bytes must
// fail closed (EOF) rather than allocate or index out of bounds.
func TestReadAt_NoOOM_HugeLengthPrefix(t *testing.T) {
	// 0xFF marker + 4-byte length = 0x40000000 (1 GiB), then no content.
	buf := []byte{wire.CompactLengthMarker, 0x00, 0x00, 0x00, 0x40}
	if _, _, err := ReadStringAt(buf, 0); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("ReadStringAt huge length: err = %v, want ErrUnexpectedEOF", err)
	}
	if _, _, err := ReadBytesAt(buf, 0); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("ReadBytesAt huge length: err = %v, want ErrUnexpectedEOF", err)
	}
}

// TestReadArrayCountAt_Validation mirrors ReadArrayCount's fail-closed
// validation order: bad maxElements, negative count, over-max count, and
// over-buffer count are each rejected; a valid count returns and advances the
// cursor by 4.
func TestReadArrayCountAt_Validation(t *testing.T) {
	// negative maxElements is a programming error: it returns before reading
	// the count, so the cursor is left at the input position (0).
	if _, p, err := ReadArrayCountAt([]byte{0, 0, 0, 0}, 0, 1, -1); err == nil || p != 0 {
		t.Fatalf("ReadArrayCountAt maxElements<0 = (_, %d, %v), want (0, error)", p, err)
	}

	// negative count on the wire is rejected. The 4-byte count IS consumed
	// before the validation fails, so the cursor advances to 4.
	neg := NewEncoder(8)
	neg.WriteInt32(-1)
	if _, p, err := ReadArrayCountAt(neg.Bytes(), 0, 1, 1000); err == nil || p != 4 {
		t.Fatalf("ReadArrayCountAt negative count = (_, %d, %v), want (4, error)", p, err)
	}

	// count above caller max is rejected (count consumed -> cursor 4).
	over := NewEncoder(8)
	over.WriteInt32(11)
	if _, p, err := ReadArrayCountAt(over.Bytes(), 0, 1, 10); err == nil || p != 4 {
		t.Fatalf("ReadArrayCountAt over caller max = (_, %d, %v), want (4, error)", p, err)
	}

	// count that cannot fit in the remaining buffer is rejected: claims 100
	// elements of >=8 bytes each, but no bytes remain after the count (cursor 4).
	big := NewEncoder(8)
	big.WriteInt32(100)
	if _, p, err := ReadArrayCountAt(big.Bytes(), 0, 8, 1000); err == nil || p != 4 {
		t.Fatalf("ReadArrayCountAt over buffer-bound = (_, %d, %v), want (4, error)", p, err)
	}

	// valid count advances the cursor by 4 and returns the count.
	ok := NewEncoder(64)
	ok.WriteInt32(3)
	ok.WriteInt32(10)
	ok.WriteInt32(20)
	ok.WriteInt32(30)
	n, pos, err := ReadArrayCountAt(ok.Bytes(), 0, 4, 1000)
	if err != nil || n != 3 || pos != 4 {
		t.Fatalf("ReadArrayCountAt valid = (%d, %d, %v), want (3, 4, nil)", n, pos, err)
	}
	// elementMinBytes=0 skips the buffer bound (trusted input).
	if _, _, err := ReadArrayCountAt(big.Bytes(), 0, 0, 1000); err != nil {
		t.Fatalf("ReadArrayCountAt elementMinBytes=0 should skip buffer bound, got %v", err)
	}
}

// TestReadAt_DifferentialParity proves each *At helper agrees with its stateful
// (*Decoder) counterpart on BOTH the decoded value and the post-read offset,
// over a mixed buffer. This is the regression guard that the cursor rewrite
// preserves identical semantics to the long-standing Decoder methods.
func TestReadAt_DifferentialParity(t *testing.T) {
	enc := NewEncoder(256)
	enc.WriteBool(true)
	enc.WriteInt32(-7)
	enc.WriteInt64(1 << 40)
	enc.WriteUint32(4242)
	enc.WriteUint64(1 << 50)
	enc.WriteFloat32(1.25)
	enc.WriteFloat64(6.022e23)
	enc.WriteString("differential")
	enc.WriteBytes([]byte{9, 8, 7})
	b := enc.Bytes()

	d := NewDecoder(b)
	pos := 0

	// bool
	{
		want, _ := d.ReadBool()
		got, np, err := ReadBoolAt(b, pos)
		if err != nil || got != want || np != d.pos {
			t.Fatalf("bool parity: At=(%v,%d) Decoder=(%v,%d) err=%v", got, np, want, d.pos, err)
		}
		pos = np
	}
	// int32
	{
		want, _ := d.ReadInt32()
		got, np, err := ReadInt32At(b, pos)
		if err != nil || got != want || np != d.pos {
			t.Fatalf("int32 parity: At=(%v,%d) Decoder=(%v,%d) err=%v", got, np, want, d.pos, err)
		}
		pos = np
	}
	// int64
	{
		want, _ := d.ReadInt64()
		got, np, err := ReadInt64At(b, pos)
		if err != nil || got != want || np != d.pos {
			t.Fatalf("int64 parity: At=(%v,%d) Decoder=(%v,%d) err=%v", got, np, want, d.pos, err)
		}
		pos = np
	}
	// uint32
	{
		want, _ := d.ReadUint32()
		got, np, err := ReadUint32At(b, pos)
		if err != nil || got != want || np != d.pos {
			t.Fatalf("uint32 parity: At=(%v,%d) Decoder=(%v,%d) err=%v", got, np, want, d.pos, err)
		}
		pos = np
	}
	// uint64
	{
		want, _ := d.ReadUint64()
		got, np, err := ReadUint64At(b, pos)
		if err != nil || got != want || np != d.pos {
			t.Fatalf("uint64 parity: At=(%v,%d) Decoder=(%v,%d) err=%v", got, np, want, d.pos, err)
		}
		pos = np
	}
	// float32
	{
		want, _ := d.ReadFloat32()
		got, np, err := ReadFloat32At(b, pos)
		if err != nil || got != want || np != d.pos {
			t.Fatalf("float32 parity: At=(%v,%d) Decoder=(%v,%d) err=%v", got, np, want, d.pos, err)
		}
		pos = np
	}
	// float64
	{
		want, _ := d.ReadFloat64()
		got, np, err := ReadFloat64At(b, pos)
		if err != nil || got != want || np != d.pos {
			t.Fatalf("float64 parity: At=(%v,%d) Decoder=(%v,%d) err=%v", got, np, want, d.pos, err)
		}
		pos = np
	}
	// string
	{
		want, _ := d.ReadString()
		got, np, err := ReadStringAt(b, pos)
		if err != nil || got != want || np != d.pos {
			t.Fatalf("string parity: At=(%q,%d) Decoder=(%q,%d) err=%v", got, np, want, d.pos, err)
		}
		pos = np
	}
	// bytes
	{
		want, _ := d.ReadBytes()
		got, np, err := ReadBytesAt(b, pos)
		if err != nil || !bytes.Equal(got, want) || np != d.pos {
			t.Fatalf("bytes parity: At=(% x,%d) Decoder=(% x,%d) err=%v", got, np, want, d.pos, err)
		}
		pos = np
	}
	if pos != len(b) || d.pos != len(b) {
		t.Fatalf("final cursors: At=%d Decoder=%d, want %d", pos, d.pos, len(b))
	}
}

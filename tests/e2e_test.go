// Package e2e contains end-to-end tests for XPB V2 serialization.
// V2 uses struct mode (no tags), fixed-width integers, and compact lengths.
package e2e

import (
	"bytes"
	"testing"

	"github.com/anthropic/xpb/runtime/go/xpb"
)

// TestSimpleMessage_RoundTrip tests encoding and decoding a simple message with V2.
func TestSimpleMessage_RoundTrip(t *testing.T) {
	type User struct {
		Name   string
		Age    int32
		Active bool
	}

	// Encode (V2: no tags, sequential writes)
	enc := xpb.NewEncoder(64)
	enc.WriteString("Alice")
	enc.WriteInt32(30)
	enc.WriteBool(true)
	data := enc.Bytes()

	// Decode (V2: sequential reads in same order)
	dec := xpb.NewDecoder(data)
	var user User
	var err error

	user.Name, err = dec.ReadString()
	if err != nil {
		t.Fatalf("ReadString: %v", err)
	}
	user.Age, err = dec.ReadInt32()
	if err != nil {
		t.Fatalf("ReadInt32: %v", err)
	}
	user.Active, err = dec.ReadBool()
	if err != nil {
		t.Fatalf("ReadBool: %v", err)
	}

	// Verify
	if user.Name != "Alice" || user.Age != 30 || !user.Active {
		t.Errorf("got %+v, want {Alice, 30, true}", user)
	}

	t.Logf("V2 Encoded size: %d bytes (name=5 + len=1 + age=4 + active=1 = 11)", len(data))
}

// TestRepeatedField_RoundTrip tests encoding and decoding repeated fields.
// V2: Write count followed by elements.
func TestRepeatedField_RoundTrip(t *testing.T) {
	tags := []string{"go", "typescript", "xpb"}

	// Encode (V2: count + elements)
	enc := xpb.NewEncoder(64)
	enc.WriteInt32(int32(len(tags)))
	for _, tag := range tags {
		enc.WriteString(tag)
	}
	data := enc.Bytes()

	// Decode
	dec := xpb.NewDecoder(data)
	count, err := dec.ReadInt32()
	if err != nil {
		t.Fatalf("ReadInt32 count: %v", err)
	}
	decoded := make([]string, count)
	for i := int32(0); i < count; i++ {
		decoded[i], err = dec.ReadString()
		if err != nil {
			t.Fatalf("ReadString[%d]: %v", i, err)
		}
	}

	// Verify
	if len(decoded) != len(tags) {
		t.Fatalf("got %d tags, want %d", len(decoded), len(tags))
	}
	for i, tag := range tags {
		if decoded[i] != tag {
			t.Errorf("tag[%d] = %q, want %q", i, decoded[i], tag)
		}
	}

	t.Logf("V2 Encoded %d repeated strings in %d bytes", len(tags), len(data))
}

// TestMapField_RoundTrip tests encoding and decoding map fields.
// V2: count + (key, value) pairs
func TestMapField_RoundTrip(t *testing.T) {
	metadata := map[string]string{
		"env":     "production",
		"version": "1.0.0",
	}

	// Encode map (V2: count + key/value pairs)
	enc := xpb.NewEncoder(128)
	enc.WriteInt32(int32(len(metadata)))
	for k, v := range metadata {
		enc.WriteString(k)
		enc.WriteString(v)
	}
	data := enc.Bytes()

	// Decode
	dec := xpb.NewDecoder(data)
	count, err := dec.ReadInt32()
	if err != nil {
		t.Fatalf("ReadInt32 count: %v", err)
	}
	decoded := make(map[string]string)
	for i := int32(0); i < count; i++ {
		k, err := dec.ReadString()
		if err != nil {
			t.Fatalf("ReadString key: %v", err)
		}
		v, err := dec.ReadString()
		if err != nil {
			t.Fatalf("ReadString value: %v", err)
		}
		decoded[k] = v
	}

	// Verify
	for k, v := range metadata {
		if decoded[k] != v {
			t.Errorf("metadata[%q] = %q, want %q", k, decoded[k], v)
		}
	}

	t.Logf("V2 Encoded map with %d entries in %d bytes", len(metadata), len(data))
}

// TestNestedMessage_RoundTrip tests encoding and decoding nested messages.
func TestNestedMessage_RoundTrip(t *testing.T) {
	// Encode inner message
	innerEnc := xpb.NewEncoder(32)
	innerEnc.WriteString("New York")
	innerEnc.WriteString("USA")
	innerData := innerEnc.Bytes()

	// Encode outer message with nested
	enc := xpb.NewEncoder(64)
	enc.WriteString("Alice")
	enc.WriteMessage(innerData)
	data := enc.Bytes()

	// Decode
	dec := xpb.NewDecoder(data)

	name, err := dec.ReadString()
	if err != nil {
		t.Fatalf("ReadString name: %v", err)
	}

	innerBytes, err := dec.ReadMessageBytes()
	if err != nil {
		t.Fatalf("ReadMessageBytes: %v", err)
	}

	innerDec := xpb.NewDecoder(innerBytes)
	city, err := innerDec.ReadString()
	if err != nil {
		t.Fatalf("ReadString city: %v", err)
	}
	country, err := innerDec.ReadString()
	if err != nil {
		t.Fatalf("ReadString country: %v", err)
	}

	// Verify
	if name != "Alice" || city != "New York" || country != "USA" {
		t.Errorf("got name=%q city=%q country=%q, want Alice/New York/USA", name, city, country)
	}

	t.Logf("V2 Encoded nested message in %d bytes", len(data))
}

// TestAllTypes_RoundTrip tests all basic types.
func TestAllTypes_RoundTrip(t *testing.T) {
	enc := xpb.NewEncoder(128)
	enc.WriteBool(true)
	enc.WriteInt32(-42)
	enc.WriteInt64(-9223372036854775808)
	enc.WriteUint32(4294967295)
	enc.WriteUint64(18446744073709551615)
	enc.WriteFloat32(3.14)
	enc.WriteFloat64(2.718281828)
	enc.WriteString("hello xpb")
	enc.WriteBytes([]byte{0xDE, 0xAD, 0xBE, 0xEF})
	data := enc.Bytes()

	// Decode and verify
	dec := xpb.NewDecoder(data)

	b, _ := dec.ReadBool()
	if !b {
		t.Errorf("bool = %v, want true", b)
	}

	i32, _ := dec.ReadInt32()
	if i32 != -42 {
		t.Errorf("int32 = %d, want -42", i32)
	}

	i64, _ := dec.ReadInt64()
	if i64 != -9223372036854775808 {
		t.Errorf("int64 = %d, want min", i64)
	}

	u32, _ := dec.ReadUint32()
	if u32 != 4294967295 {
		t.Errorf("uint32 = %d, want max", u32)
	}

	u64, _ := dec.ReadUint64()
	if u64 != 18446744073709551615 {
		t.Errorf("uint64 = %d, want max", u64)
	}

	f32, _ := dec.ReadFloat32()
	if f32 != 3.14 {
		t.Errorf("float32 = %v, want 3.14", f32)
	}

	f64, _ := dec.ReadFloat64()
	if f64 != 2.718281828 {
		t.Errorf("float64 = %v, want 2.718281828", f64)
	}

	s, _ := dec.ReadString()
	if s != "hello xpb" {
		t.Errorf("string = %q, want 'hello xpb'", s)
	}

	bs, _ := dec.ReadBytes()
	if !bytes.Equal(bs, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Errorf("bytes = %v, want DEADBEEF", bs)
	}

	// V2: Calculate expected size
	// bool=1 + int32=4 + int64=8 + uint32=4 + uint64=8 + float32=4 + float64=8 +
	// string(len=1 + 9) + bytes(len=1 + 4) = 1+4+8+4+8+4+8+10+5 = 52
	t.Logf("V2 All types encoded in %d bytes", len(data))
}

// TestCompactLength tests the compact length encoding.
func TestCompactLength(t *testing.T) {
	// Short string (length < 255) should use 1-byte prefix
	shortStr := "hello"
	enc := xpb.NewEncoder(64)
	enc.WriteString(shortStr)
	shortData := enc.Bytes()
	// Expected: 1 byte length + 5 bytes = 6
	if len(shortData) != 6 {
		t.Errorf("short string size = %d, want 6", len(shortData))
	}

	// Long string (length >= 255) should use 5-byte prefix (0xFF + 4 bytes)
	longStr := string(make([]byte, 300))
	enc2 := xpb.NewEncoder(512)
	enc2.WriteString(longStr)
	longData := enc2.Bytes()
	// Expected: 5 bytes length + 300 bytes = 305
	if len(longData) != 305 {
		t.Errorf("long string size = %d, want 305", len(longData))
	}

	// Verify decoding
	dec := xpb.NewDecoder(longData)
	decoded, err := dec.ReadString()
	if err != nil {
		t.Fatalf("ReadString: %v", err)
	}
	if len(decoded) != 300 {
		t.Errorf("decoded length = %d, want 300", len(decoded))
	}
}

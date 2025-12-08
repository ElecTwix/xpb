// Package e2e contains end-to-end tests for XPB serialization.
package e2e

import (
	"bytes"
	"testing"

	"github.com/anthropic/xpb/pkg/wire"
	"github.com/anthropic/xpb/runtime/go/xpb"
)

var _ = wire.WireVarint

// TestSimpleMessage_RoundTrip tests encoding and decoding a simple message.
func TestSimpleMessage_RoundTrip(t *testing.T) {
	type User struct {
		Name   string
		Age    int32
		Active bool
	}

	// Encode
	enc := xpb.NewEncoder(64)
	enc.WriteString(1, "Alice")
	enc.WriteInt32(2, 30)
	enc.WriteBool(3, true)
	data := enc.Bytes()

	// Decode
	dec := xpb.NewDecoder(data)
	var user User
	for !dec.EOF() {
		fieldNum, wireType, err := dec.ReadTag()
		if err != nil {
			t.Fatalf("ReadTag: %v", err)
		}
		switch fieldNum {
		case 1:
			user.Name, err = dec.ReadString()
		case 2:
			user.Age, err = dec.ReadInt32()
		case 3:
			user.Active, err = dec.ReadBool()
		default:
			err = dec.Skip(wireType)
		}
		if err != nil {
			t.Fatalf("field %d: %v", fieldNum, err)
		}
	}

	// Verify
	if user.Name != "Alice" || user.Age != 30 || !user.Active {
		t.Errorf("got %+v, want {Alice, 30, true}", user)
	}

	t.Logf("Encoded size: %d bytes", len(data))
}

// TestRepeatedField_RoundTrip tests encoding and decoding repeated fields.
func TestRepeatedField_RoundTrip(t *testing.T) {
	tags := []string{"go", "typescript", "xpb"}

	// Encode
	enc := xpb.NewEncoder(64)
	for _, tag := range tags {
		enc.WriteString(1, tag)
	}
	data := enc.Bytes()

	// Decode
	dec := xpb.NewDecoder(data)
	var decoded []string
	for !dec.EOF() {
		fieldNum, wireType, err := dec.ReadTag()
		if err != nil {
			t.Fatalf("ReadTag: %v", err)
		}
		if fieldNum == 1 {
			s, err := dec.ReadString()
			if err != nil {
				t.Fatalf("ReadString: %v", err)
			}
			decoded = append(decoded, s)
		} else {
			if err := dec.Skip(wireType); err != nil {
				t.Fatalf("Skip: %v", err)
			}
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

	t.Logf("Encoded %d repeated strings in %d bytes", len(tags), len(data))
}

// TestMapField_RoundTrip tests encoding and decoding map fields.
func TestMapField_RoundTrip(t *testing.T) {
	metadata := map[string]string{
		"env":     "production",
		"version": "1.0.0",
	}

	// Encode map (each entry as nested message with key=1, value=2)
	enc := xpb.NewEncoder(128)
	for k, v := range metadata {
		mapEnc := xpb.NewEncoder(32)
		mapEnc.WriteString(1, k)
		mapEnc.WriteString(2, v)
		enc.WriteMessage(1, mapEnc.Bytes()) // field 1 for map
	}
	data := enc.Bytes()

	// Decode
	dec := xpb.NewDecoder(data)
	decoded := make(map[string]string)
	for !dec.EOF() {
		fieldNum, wireType, err := dec.ReadTag()
		if err != nil {
			t.Fatalf("ReadTag: %v", err)
		}
		if fieldNum == 1 {
			mapData, err := dec.ReadMessageBytes()
			if err != nil {
				t.Fatalf("ReadMessageBytes: %v", err)
			}
			mapDec := xpb.NewDecoder(mapData)
			var k, v string
			for !mapDec.EOF() {
				fn, _, _ := mapDec.ReadTag()
				switch fn {
				case 1:
					k, _ = mapDec.ReadString()
				case 2:
					v, _ = mapDec.ReadString()
				}
			}
			decoded[k] = v
		} else {
			if err := dec.Skip(wireType); err != nil {
				t.Fatalf("Skip: %v", err)
			}
		}
	}

	// Verify
	for k, v := range metadata {
		if decoded[k] != v {
			t.Errorf("metadata[%q] = %q, want %q", k, decoded[k], v)
		}
	}

	t.Logf("Encoded map with %d entries in %d bytes", len(metadata), len(data))
}

// TestNestedMessage_RoundTrip tests encoding and decoding nested messages.
func TestNestedMessage_RoundTrip(t *testing.T) {
	// Encode inner message
	innerEnc := xpb.NewEncoder(32)
	innerEnc.WriteString(1, "New York")
	innerEnc.WriteString(2, "USA")
	innerData := innerEnc.Bytes()

	// Encode outer message with nested
	enc := xpb.NewEncoder(64)
	enc.WriteString(1, "Alice")
	enc.WriteMessage(2, innerData)
	data := enc.Bytes()

	// Decode
	dec := xpb.NewDecoder(data)
	var name, city, country string
	for !dec.EOF() {
		fieldNum, wireType, err := dec.ReadTag()
		if err != nil {
			t.Fatalf("ReadTag: %v", err)
		}
		switch fieldNum {
		case 1:
			name, err = dec.ReadString()
		case 2:
			innerData, err := dec.ReadMessageBytes()
			if err == nil {
				innerDec := xpb.NewDecoder(innerData)
				for !innerDec.EOF() {
					fn, _, _ := innerDec.ReadTag()
					switch fn {
					case 1:
						city, _ = innerDec.ReadString()
					case 2:
						country, _ = innerDec.ReadString()
					}
				}
			}
		default:
			err = dec.Skip(wireType)
		}
		if err != nil {
			t.Fatalf("field %d: %v", fieldNum, err)
		}
	}

	// Verify
	if name != "Alice" || city != "New York" || country != "USA" {
		t.Errorf("got name=%q city=%q country=%q, want Alice/New York/USA", name, city, country)
	}

	t.Logf("Encoded nested message in %d bytes", len(data))
}

// TestAllTypes_RoundTrip tests all basic types.
func TestAllTypes_RoundTrip(t *testing.T) {
	enc := xpb.NewEncoder(128)
	enc.WriteBool(1, true)
	enc.WriteInt32(2, -42)
	enc.WriteInt64(3, -9223372036854775808)
	enc.WriteUint32(4, 4294967295)
	enc.WriteUint64(5, 18446744073709551615)
	enc.WriteFloat32(6, 3.14)
	enc.WriteFloat64(7, 2.718281828)
	enc.WriteString(8, "hello xpb")
	enc.WriteBytes(9, []byte{0xDE, 0xAD, 0xBE, 0xEF})
	data := enc.Bytes()

	// Decode and verify
	dec := xpb.NewDecoder(data)

	fn, _, _ := dec.ReadTag()
	if fn != 1 {
		t.Errorf("expected field 1")
	}
	b, _ := dec.ReadBool()
	if !b {
		t.Errorf("bool = %v, want true", b)
	}

	fn, _, _ = dec.ReadTag()
	if fn != 2 {
		t.Errorf("expected field 2")
	}
	i32, _ := dec.ReadInt32()
	if i32 != -42 {
		t.Errorf("int32 = %d, want -42", i32)
	}

	fn, _, _ = dec.ReadTag()
	if fn != 3 {
		t.Errorf("expected field 3")
	}
	i64, _ := dec.ReadInt64()
	if i64 != -9223372036854775808 {
		t.Errorf("int64 = %d, want min", i64)
	}

	fn, _, _ = dec.ReadTag()
	if fn != 4 {
		t.Errorf("expected field 4")
	}
	u32, _ := dec.ReadUint32()
	if u32 != 4294967295 {
		t.Errorf("uint32 = %d, want max", u32)
	}

	fn, _, _ = dec.ReadTag()
	if fn != 5 {
		t.Errorf("expected field 5")
	}
	u64, _ := dec.ReadUint64()
	if u64 != 18446744073709551615 {
		t.Errorf("uint64 = %d, want max", u64)
	}

	fn, _, _ = dec.ReadTag()
	if fn != 6 {
		t.Errorf("expected field 6")
	}
	f32, _ := dec.ReadFloat32()
	if f32 != 3.14 {
		t.Errorf("float32 = %v, want 3.14", f32)
	}

	fn, _, _ = dec.ReadTag()
	if fn != 7 {
		t.Errorf("expected field 7")
	}
	f64, _ := dec.ReadFloat64()
	if f64 != 2.718281828 {
		t.Errorf("float64 = %v, want 2.718281828", f64)
	}

	fn, _, _ = dec.ReadTag()
	if fn != 8 {
		t.Errorf("expected field 8")
	}
	s, _ := dec.ReadString()
	if s != "hello xpb" {
		t.Errorf("string = %q, want 'hello xpb'", s)
	}

	fn, _, _ = dec.ReadTag()
	if fn != 9 {
		t.Errorf("expected field 9")
	}
	bs, _ := dec.ReadBytes()
	if !bytes.Equal(bs, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Errorf("bytes = %v, want DEADBEEF", bs)
	}

	t.Logf("All types encoded in %d bytes", len(data))
}

// TestSkipUnknownFields tests that unknown fields are skipped correctly.
func TestSkipUnknownFields(t *testing.T) {
	// Encode message with fields 1, 2, 3, 4
	enc := xpb.NewEncoder(64)
	enc.WriteString(1, "known")
	enc.WriteInt32(2, 42)     // unknown to decoder
	enc.WriteFloat64(3, 3.14) // unknown to decoder
	enc.WriteString(4, "also known")
	data := enc.Bytes()

	// Decode only fields 1 and 4
	dec := xpb.NewDecoder(data)
	var field1, field4 string
	for !dec.EOF() {
		fieldNum, wireType, err := dec.ReadTag()
		if err != nil {
			t.Fatalf("ReadTag: %v", err)
		}
		switch fieldNum {
		case 1:
			field1, err = dec.ReadString()
		case 4:
			field4, err = dec.ReadString()
		default:
			err = dec.Skip(wireType)
		}
		if err != nil {
			t.Fatalf("field %d: %v", fieldNum, err)
		}
	}

	if field1 != "known" || field4 != "also known" {
		t.Errorf("got field1=%q field4=%q, want known/also known", field1, field4)
	}
}

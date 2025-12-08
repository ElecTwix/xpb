package xpb

import (
	"bytes"
	"testing"

	"github.com/anthropic/xpb/pkg/wire"
)

func TestEncoderDecoder_Bool(t *testing.T) {
	enc := NewEncoder(16)
	enc.WriteBool(1, true)
	enc.WriteBool(2, false)

	dec := NewDecoder(enc.Bytes())

	// Read first field
	fn, wt, err := dec.ReadTag()
	if err != nil {
		t.Fatalf("ReadTag: %v", err)
	}
	if fn != 1 || wt != wire.WireVarint {
		t.Errorf("Tag = (%d, %d), want (1, 0)", fn, wt)
	}
	v, err := dec.ReadBool()
	if err != nil {
		t.Fatalf("ReadBool: %v", err)
	}
	if v != true {
		t.Errorf("ReadBool = %v, want true", v)
	}

	// Read second field
	fn, wt, err = dec.ReadTag()
	if err != nil {
		t.Fatalf("ReadTag: %v", err)
	}
	if fn != 2 || wt != wire.WireVarint {
		t.Errorf("Tag = (%d, %d), want (2, 0)", fn, wt)
	}
	v, err = dec.ReadBool()
	if err != nil {
		t.Fatalf("ReadBool: %v", err)
	}
	if v != false {
		t.Errorf("ReadBool = %v, want false", v)
	}
}

func TestEncoderDecoder_Integers(t *testing.T) {
	tests := []struct {
		name    string
		encode  func(e *Encoder)
		decode  func(d *Decoder) (any, error)
		want    any
		fieldNo uint32
	}{
		{
			name:    "int32 positive",
			encode:  func(e *Encoder) { e.WriteInt32(1, 42) },
			decode:  func(d *Decoder) (any, error) { return d.ReadInt32() },
			want:    int32(42),
			fieldNo: 1,
		},
		{
			name:    "int32 negative",
			encode:  func(e *Encoder) { e.WriteInt32(1, -42) },
			decode:  func(d *Decoder) (any, error) { return d.ReadInt32() },
			want:    int32(-42),
			fieldNo: 1,
		},
		{
			name:    "int64 large",
			encode:  func(e *Encoder) { e.WriteInt64(1, 9223372036854775807) },
			decode:  func(d *Decoder) (any, error) { return d.ReadInt64() },
			want:    int64(9223372036854775807),
			fieldNo: 1,
		},
		{
			name:    "uint32",
			encode:  func(e *Encoder) { e.WriteUint32(1, 4294967295) },
			decode:  func(d *Decoder) (any, error) { return d.ReadUint32() },
			want:    uint32(4294967295),
			fieldNo: 1,
		},
		{
			name:    "uint64",
			encode:  func(e *Encoder) { e.WriteUint64(1, 18446744073709551615) },
			decode:  func(d *Decoder) (any, error) { return d.ReadUint64() },
			want:    uint64(18446744073709551615),
			fieldNo: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc := NewEncoder(16)
			tt.encode(enc)

			dec := NewDecoder(enc.Bytes())
			fn, _, err := dec.ReadTag()
			if err != nil {
				t.Fatalf("ReadTag: %v", err)
			}
			if fn != tt.fieldNo {
				t.Errorf("fieldNumber = %d, want %d", fn, tt.fieldNo)
			}

			got, err := tt.decode(dec)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEncoderDecoder_Floats(t *testing.T) {
	enc := NewEncoder(32)
	enc.WriteFloat32(1, 3.14)
	enc.WriteFloat64(2, 2.718281828)

	dec := NewDecoder(enc.Bytes())

	// Float32
	fn, wt, err := dec.ReadTag()
	if err != nil {
		t.Fatalf("ReadTag: %v", err)
	}
	if fn != 1 || wt != wire.WireFixed32 {
		t.Errorf("Tag = (%d, %d), want (1, 1)", fn, wt)
	}
	f32, err := dec.ReadFloat32()
	if err != nil {
		t.Fatalf("ReadFloat32: %v", err)
	}
	if f32 != 3.14 {
		t.Errorf("ReadFloat32 = %v, want 3.14", f32)
	}

	// Float64
	fn, wt, err = dec.ReadTag()
	if err != nil {
		t.Fatalf("ReadTag: %v", err)
	}
	if fn != 2 || wt != wire.WireFixed64 {
		t.Errorf("Tag = (%d, %d), want (2, 2)", fn, wt)
	}
	f64, err := dec.ReadFloat64()
	if err != nil {
		t.Fatalf("ReadFloat64: %v", err)
	}
	if f64 != 2.718281828 {
		t.Errorf("ReadFloat64 = %v, want 2.718281828", f64)
	}
}

func TestEncoderDecoder_String(t *testing.T) {
	enc := NewEncoder(64)
	enc.WriteString(1, "hello world")
	enc.WriteString(2, "")

	dec := NewDecoder(enc.Bytes())

	// First string
	fn, wt, err := dec.ReadTag()
	if err != nil {
		t.Fatalf("ReadTag: %v", err)
	}
	if fn != 1 || wt != wire.WireLengthDelimited {
		t.Errorf("Tag = (%d, %d), want (1, 3)", fn, wt)
	}
	s, err := dec.ReadString()
	if err != nil {
		t.Fatalf("ReadString: %v", err)
	}
	if s != "hello world" {
		t.Errorf("ReadString = %q, want %q", s, "hello world")
	}

	// Empty string
	fn, wt, err = dec.ReadTag()
	if err != nil {
		t.Fatalf("ReadTag: %v", err)
	}
	if fn != 2 || wt != wire.WireLengthDelimited {
		t.Errorf("Tag = (%d, %d), want (2, 3)", fn, wt)
	}
	s, err = dec.ReadString()
	if err != nil {
		t.Fatalf("ReadString: %v", err)
	}
	if s != "" {
		t.Errorf("ReadString = %q, want %q", s, "")
	}
}

func TestEncoderDecoder_Bytes(t *testing.T) {
	enc := NewEncoder(64)
	data := []byte{0x01, 0x02, 0x03, 0x04}
	enc.WriteBytes(1, data)

	dec := NewDecoder(enc.Bytes())

	fn, wt, err := dec.ReadTag()
	if err != nil {
		t.Fatalf("ReadTag: %v", err)
	}
	if fn != 1 || wt != wire.WireLengthDelimited {
		t.Errorf("Tag = (%d, %d), want (1, 3)", fn, wt)
	}
	got, err := dec.ReadBytes()
	if err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("ReadBytes = %v, want %v", got, data)
	}
}

func TestDecoder_Skip(t *testing.T) {
	enc := NewEncoder(64)
	enc.WriteInt32(1, 42)
	enc.WriteString(2, "skip me")
	enc.WriteFloat64(3, 3.14)
	enc.WriteInt32(4, 100)

	dec := NewDecoder(enc.Bytes())

	// Read field 1
	fn, wt, _ := dec.ReadTag()
	if fn != 1 {
		t.Fatalf("expected field 1, got %d", fn)
	}
	err := dec.Skip(wt)
	if err != nil {
		t.Fatalf("Skip varint: %v", err)
	}

	// Skip field 2 (string)
	fn, wt, _ = dec.ReadTag()
	if fn != 2 {
		t.Fatalf("expected field 2, got %d", fn)
	}
	err = dec.Skip(wt)
	if err != nil {
		t.Fatalf("Skip string: %v", err)
	}

	// Skip field 3 (float64)
	fn, wt, _ = dec.ReadTag()
	if fn != 3 {
		t.Fatalf("expected field 3, got %d", fn)
	}
	err = dec.Skip(wt)
	if err != nil {
		t.Fatalf("Skip float64: %v", err)
	}

	// Read field 4
	fn, _, _ = dec.ReadTag()
	if fn != 4 {
		t.Fatalf("expected field 4, got %d", fn)
	}
	v, err := dec.ReadInt32()
	if err != nil {
		t.Fatalf("ReadInt32: %v", err)
	}
	if v != 100 {
		t.Errorf("got %d, want 100", v)
	}
}

func TestEncoder_Reset(t *testing.T) {
	enc := NewEncoder(16)
	enc.WriteInt32(1, 42)
	if enc.Len() == 0 {
		t.Error("Len() should not be 0 after write")
	}

	enc.Reset()
	if enc.Len() != 0 {
		t.Errorf("Len() = %d after Reset, want 0", enc.Len())
	}
}

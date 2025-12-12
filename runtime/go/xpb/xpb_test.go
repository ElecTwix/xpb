package xpb

import (
	"bytes"
	"testing"
)

func TestEncoderDecoder_Bool(t *testing.T) {
	enc := NewEncoder(16)
	enc.WriteBool(true)
	enc.WriteBool(false)

	dec := NewDecoder(enc.Bytes())

	v, err := dec.ReadBool()
	if err != nil {
		t.Fatalf("ReadBool: %v", err)
	}
	if v != true {
		t.Errorf("ReadBool = %v, want true", v)
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
		name   string
		encode func(e *Encoder)
		decode func(d *Decoder) (any, error)
		want   any
	}{
		{
			name:   "int32 positive",
			encode: func(e *Encoder) { e.WriteInt32(42) },
			decode: func(d *Decoder) (any, error) { return d.ReadInt32() },
			want:   int32(42),
		},
		{
			name:   "int32 negative",
			encode: func(e *Encoder) { e.WriteInt32(-42) },
			decode: func(d *Decoder) (any, error) { return d.ReadInt32() },
			want:   int32(-42),
		},
		{
			name:   "int64 large",
			encode: func(e *Encoder) { e.WriteInt64(9223372036854775807) },
			decode: func(d *Decoder) (any, error) { return d.ReadInt64() },
			want:   int64(9223372036854775807),
		},
		{
			name:   "int64 negative",
			encode: func(e *Encoder) { e.WriteInt64(-9223372036854775807) },
			decode: func(d *Decoder) (any, error) { return d.ReadInt64() },
			want:   int64(-9223372036854775807),
		},
		{
			name:   "uint32",
			encode: func(e *Encoder) { e.WriteUint32(4294967295) },
			decode: func(d *Decoder) (any, error) { return d.ReadUint32() },
			want:   uint32(4294967295),
		},
		{
			name:   "uint64",
			encode: func(e *Encoder) { e.WriteUint64(18446744073709551615) },
			decode: func(d *Decoder) (any, error) { return d.ReadUint64() },
			want:   uint64(18446744073709551615),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc := NewEncoder(16)
			tt.encode(enc)

			dec := NewDecoder(enc.Bytes())
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
	enc.WriteFloat32(3.14)
	enc.WriteFloat64(2.718281828)

	dec := NewDecoder(enc.Bytes())

	f32, err := dec.ReadFloat32()
	if err != nil {
		t.Fatalf("ReadFloat32: %v", err)
	}
	if f32 != 3.14 {
		t.Errorf("ReadFloat32 = %v, want 3.14", f32)
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
	enc.WriteString("hello world")
	enc.WriteString("")

	dec := NewDecoder(enc.Bytes())

	s, err := dec.ReadString()
	if err != nil {
		t.Fatalf("ReadString: %v", err)
	}
	if s != "hello world" {
		t.Errorf("ReadString = %q, want %q", s, "hello world")
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
	enc.WriteBytes(data)

	dec := NewDecoder(enc.Bytes())

	got, err := dec.ReadBytes()
	if err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("ReadBytes = %v, want %v", got, data)
	}
}

func TestEncoderDecoder_CompactLength(t *testing.T) {
	// Test string with length < 255 (1-byte prefix)
	shortStr := "hello"
	enc := NewEncoder(64)
	enc.WriteString(shortStr)
	data := enc.Bytes()
	// Should be: 1 byte length + 5 bytes content = 6 bytes
	if len(data) != 6 {
		t.Errorf("short string size = %d, want 6", len(data))
	}

	// Test string with length >= 255 (5-byte prefix: 0xFF + 4 bytes)
	longStr := string(make([]byte, 300))
	enc2 := NewEncoder(512)
	enc2.WriteString(longStr)
	data2 := enc2.Bytes()
	// Should be: 5 bytes length + 300 bytes content = 305 bytes
	if len(data2) != 305 {
		t.Errorf("long string size = %d, want 305", len(data2))
	}

	// Verify decoding works for long string
	dec := NewDecoder(data2)
	got, err := dec.ReadString()
	if err != nil {
		t.Fatalf("ReadString long: %v", err)
	}
	if got != longStr {
		t.Errorf("ReadString long length = %d, want %d", len(got), len(longStr))
	}
}

func TestEncoder_Reset(t *testing.T) {
	enc := NewEncoder(16)
	enc.WriteInt32(42)
	if enc.Len() == 0 {
		t.Error("Len() should not be 0 after write")
	}

	enc.Reset()
	if enc.Len() != 0 {
		t.Errorf("Len() = %d after Reset, want 0", enc.Len())
	}
}

func TestDecoder_Skip(t *testing.T) {
	enc := NewEncoder(64)
	enc.WriteInt32(42)
	enc.WriteInt32(100)

	dec := NewDecoder(enc.Bytes())

	// Skip first int32 (4 bytes)
	err := dec.Skip(4)
	if err != nil {
		t.Fatalf("Skip: %v", err)
	}

	// Read second int32
	v, err := dec.ReadInt32()
	if err != nil {
		t.Fatalf("ReadInt32: %v", err)
	}
	if v != 100 {
		t.Errorf("got %d, want 100", v)
	}
}

func TestDecoder_EOF(t *testing.T) {
	enc := NewEncoder(16)
	enc.WriteInt32(42)

	dec := NewDecoder(enc.Bytes())
	if dec.EOF() {
		t.Error("EOF should be false before reading")
	}

	_, _ = dec.ReadInt32()
	if !dec.EOF() {
		t.Error("EOF should be true after reading all data")
	}
}

func TestDecoder_Remaining(t *testing.T) {
	enc := NewEncoder(16)
	enc.WriteInt32(42)
	enc.WriteInt32(100)

	dec := NewDecoder(enc.Bytes())
	if dec.Remaining() != 8 {
		t.Errorf("Remaining = %d, want 8", dec.Remaining())
	}

	_, _ = dec.ReadInt32()
	if dec.Remaining() != 4 {
		t.Errorf("Remaining = %d, want 4", dec.Remaining())
	}
}

func TestDecoder_ReadBytesUnsafe(t *testing.T) {
	enc := NewEncoder(1024) // Ensure large capacity
	data := []byte{0x01, 0x02, 0x03, 0x04}
	enc.WriteBytes(data)

	dec := NewDecoder(enc.Bytes())

	// ReadBytesUnsafe should alias the buffer
	unsafeBytes, err := dec.ReadBytesUnsafe()
	if err != nil {
		t.Fatalf("ReadBytesUnsafe: %v", err)
	}
	if !bytes.Equal(unsafeBytes, data) {
		t.Errorf("ReadBytesUnsafe = %v, want %v", unsafeBytes, data)
	}

	// Verify it's a slice of the original capacity (indicating no alloc copy)
	// Original buffer has cap 1024. Slice should have cap > len.
	if cap(unsafeBytes) <= len(unsafeBytes) {
		t.Errorf("ReadBytesUnsafe appears to have allocated (cap %d == len %d), expected alias of large buffer", cap(unsafeBytes), len(unsafeBytes))
	}
}

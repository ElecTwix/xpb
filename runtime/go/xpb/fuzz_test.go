package xpb

import (
	"testing"
)

// driveDecoder runs every Read* / Skip method of the Decoder against arbitrary
// bytes. It must never panic regardless of input; errors are expected and fine.
// The byte slice is also used to derive the Skip distances so the fuzzer can
// explore negative / oversized skips too.
func driveDecoder(data []byte) {
	dec := NewDecoder(data)

	// Probe-only methods.
	_ = dec.Remaining()
	_ = dec.EOF()

	// Drive every read method in sequence. We loop a bounded number of times so
	// that the decoder is exercised both before and after hitting EOF (the spec
	// requires repeated reads after EOF to keep returning errors, not panic).
	for i := 0; i < 64; i++ {
		_, _ = dec.ReadBool()
		_, _ = dec.ReadInt32()
		_, _ = dec.ReadInt64()
		_, _ = dec.ReadUint32()
		_, _ = dec.ReadUint64()
		_, _ = dec.ReadFloat32()
		_, _ = dec.ReadFloat64()
		_, _ = dec.ReadString()
		_, _ = dec.CloneString()
		_, _ = dec.ReadBytes()
		_, _ = dec.ReadBytesUnsafe()
		_, _ = dec.ReadMessageBytes()

		// Derive a Skip distance from the input, including negative values, to
		// exercise the negative-n guard and oversized skips.
		var n int
		if len(data) > 0 {
			n = int(int8(data[i%len(data)])) // range [-128, 127]
		}
		_ = dec.Skip(n)
	}
}

// FuzzDecodeAll feeds arbitrary bytes into a decoder driving every Read* method
// and asserts the decoder NEVER panics on untrusted input.
func FuzzDecodeAll(f *testing.F) {
	// Seed corpus: a few well-formed and adversarial inputs.
	f.Add([]byte{})
	f.Add([]byte{0x01})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF}) // 0xFF length marker + 4GB-ish length, no payload
	f.Add([]byte{0xFF, 0x00, 0x00})             // truncated length marker (<4 bytes follow)
	f.Add([]byte{0xFE})                         // single-byte length 254, no payload
	// A valid encoding of several scalars + a string + bytes.
	{
		enc := NewEncoder(64)
		enc.WriteBool(true)
		enc.WriteInt32(-1)
		enc.WriteInt64(1 << 40)
		enc.WriteUint32(4242)
		enc.WriteUint64(1 << 63)
		enc.WriteFloat32(3.14)
		enc.WriteFloat64(2.718281828)
		enc.WriteString("hello fuzz")
		enc.WriteBytes([]byte{1, 2, 3, 4, 5})
		f.Add(append([]byte(nil), enc.Bytes()...))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("decoder panicked on input %x: %v", data, r)
			}
		}()
		driveDecoder(data)
	})
}

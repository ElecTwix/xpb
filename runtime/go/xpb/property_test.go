package xpb

import (
	"bytes"
	"math"
	"math/rand"
	"testing"
	"testing/quick"
)

// quickConfig keeps property runs reasonably large without being slow.
var quickConfig = &quick.Config{MaxCount: 2000}

func TestProperty_Bool(t *testing.T) {
	f := func(v bool) bool {
		enc := NewEncoder(8)
		enc.WriteBool(v)
		got, err := NewDecoder(enc.Bytes()).ReadBool()
		return err == nil && got == v
	}
	if err := quick.Check(f, quickConfig); err != nil {
		t.Fatal(err)
	}
}

func TestProperty_Int32(t *testing.T) {
	f := func(v int32) bool {
		enc := NewEncoder(8)
		enc.WriteInt32(v)
		got, err := NewDecoder(enc.Bytes()).ReadInt32()
		return err == nil && got == v
	}
	if err := quick.Check(f, quickConfig); err != nil {
		t.Fatal(err)
	}
}

func TestProperty_Int64(t *testing.T) {
	f := func(v int64) bool {
		enc := NewEncoder(8)
		enc.WriteInt64(v)
		got, err := NewDecoder(enc.Bytes()).ReadInt64()
		return err == nil && got == v
	}
	if err := quick.Check(f, quickConfig); err != nil {
		t.Fatal(err)
	}
}

func TestProperty_Uint32(t *testing.T) {
	f := func(v uint32) bool {
		enc := NewEncoder(8)
		enc.WriteUint32(v)
		got, err := NewDecoder(enc.Bytes()).ReadUint32()
		return err == nil && got == v
	}
	if err := quick.Check(f, quickConfig); err != nil {
		t.Fatal(err)
	}
}

func TestProperty_Uint64(t *testing.T) {
	f := func(v uint64) bool {
		enc := NewEncoder(8)
		enc.WriteUint64(v)
		got, err := NewDecoder(enc.Bytes()).ReadUint64()
		return err == nil && got == v
	}
	if err := quick.Check(f, quickConfig); err != nil {
		t.Fatal(err)
	}
}

// Float properties use bit-pattern equality so NaN and -0.0 round-trip exactly.
func TestProperty_Float32(t *testing.T) {
	f := func(bits uint32) bool {
		v := math.Float32frombits(bits)
		enc := NewEncoder(8)
		enc.WriteFloat32(v)
		got, err := NewDecoder(enc.Bytes()).ReadFloat32()
		return err == nil && math.Float32bits(got) == bits
	}
	if err := quick.Check(f, quickConfig); err != nil {
		t.Fatal(err)
	}
}

func TestProperty_Float64(t *testing.T) {
	f := func(bits uint64) bool {
		v := math.Float64frombits(bits)
		enc := NewEncoder(8)
		enc.WriteFloat64(v)
		got, err := NewDecoder(enc.Bytes()).ReadFloat64()
		return err == nil && math.Float64bits(got) == bits
	}
	if err := quick.Check(f, quickConfig); err != nil {
		t.Fatal(err)
	}
}

func TestProperty_String(t *testing.T) {
	f := func(s string) bool {
		enc := NewEncoder(len(s) + 8)
		enc.WriteString(s)
		got, err := NewDecoder(enc.Bytes()).CloneString()
		return err == nil && got == s
	}
	if err := quick.Check(f, quickConfig); err != nil {
		t.Fatal(err)
	}
}

func TestProperty_Bytes(t *testing.T) {
	f := func(b []byte) bool {
		enc := NewEncoder(len(b) + 8)
		enc.WriteBytes(b)
		got, err := NewDecoder(enc.Bytes()).ReadBytes()
		if err != nil {
			return false
		}
		// Encode/decode of empty/nil both yield a zero-length slice.
		return bytes.Equal(got, b)
	}
	if err := quick.Check(f, quickConfig); err != nil {
		t.Fatal(err)
	}
}

// TestProperty_MixedSequence hand-rolls a property loop that writes a random
// sequence of mixed scalar types and decodes them back in order, asserting an
// exact round-trip including compact-length boundary string sizes.
func TestProperty_MixedSequence(t *testing.T) {
	rng := rand.New(rand.NewSource(0xBADC0FFEE))
	for iter := 0; iter < 1000; iter++ {
		n := rng.Intn(16)
		kinds := make([]int, n)
		i32s := make([]int32, n)
		i64s := make([]int64, n)
		f64s := make([]float64, n)
		strs := make([]string, n)

		enc := NewEncoder(64)
		for k := 0; k < n; k++ {
			kind := rng.Intn(4)
			kinds[k] = kind
			switch kind {
			case 0:
				i32s[k] = int32(rng.Uint32())
				enc.WriteInt32(i32s[k])
			case 1:
				i64s[k] = int64(rng.Uint64())
				enc.WriteInt64(i64s[k])
			case 2:
				f64s[k] = math.Float64frombits(rng.Uint64())
				enc.WriteFloat64(f64s[k])
			case 3:
				// Include lengths around the 254/255 compact-length boundary.
				ln := rng.Intn(260)
				b := make([]byte, ln)
				rng.Read(b)
				strs[k] = string(b)
				enc.WriteString(strs[k])
			}
		}

		dec := NewDecoder(enc.Bytes())
		for k := 0; k < n; k++ {
			switch kinds[k] {
			case 0:
				v, err := dec.ReadInt32()
				if err != nil || v != i32s[k] {
					t.Fatalf("iter %d idx %d int32: got %d err %v want %d", iter, k, v, err, i32s[k])
				}
			case 1:
				v, err := dec.ReadInt64()
				if err != nil || v != i64s[k] {
					t.Fatalf("iter %d idx %d int64: got %d err %v want %d", iter, k, v, err, i64s[k])
				}
			case 2:
				v, err := dec.ReadFloat64()
				if err != nil || math.Float64bits(v) != math.Float64bits(f64s[k]) {
					t.Fatalf("iter %d idx %d float64 bit mismatch err %v", iter, k, err)
				}
			case 3:
				v, err := dec.CloneString()
				if err != nil || v != strs[k] {
					t.Fatalf("iter %d idx %d string mismatch err %v", iter, k, err)
				}
			}
		}
		if !dec.EOF() {
			t.Fatalf("iter %d: decoder not at EOF, %d bytes left", iter, dec.Remaining())
		}
	}
}

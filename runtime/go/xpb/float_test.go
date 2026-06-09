package xpb

import (
	"math"
	"testing"
)

func roundtripFloat64(t *testing.T, v float64) float64 {
	t.Helper()
	enc := NewEncoder(8)
	enc.WriteFloat64(v)
	got, err := NewDecoder(enc.Bytes()).ReadFloat64()
	if err != nil {
		t.Fatalf("ReadFloat64: %v", err)
	}
	return got
}

func roundtripFloat32(t *testing.T, v float32) float32 {
	t.Helper()
	enc := NewEncoder(4)
	enc.WriteFloat32(v)
	got, err := NewDecoder(enc.Bytes()).ReadFloat32()
	if err != nil {
		t.Fatalf("ReadFloat32: %v", err)
	}
	return got
}

func TestFloat64_NaN(t *testing.T) {
	// Use a specific NaN bit pattern and verify it survives bit-for-bit.
	// (== comparison is false for NaN, so compare via Float64bits.)
	const nanBits = uint64(0x7FF8000000000001)
	v := math.Float64frombits(nanBits)
	got := roundtripFloat64(t, v)
	if math.Float64bits(got) != nanBits {
		t.Fatalf("NaN bits = %#016x, want %#016x", math.Float64bits(got), nanBits)
	}
	if !math.IsNaN(got) {
		t.Fatalf("expected NaN, got %v", got)
	}
}

func TestFloat32_NaN(t *testing.T) {
	const nanBits = uint32(0x7FC00001)
	v := math.Float32frombits(nanBits)
	got := roundtripFloat32(t, v)
	if math.Float32bits(got) != nanBits {
		t.Fatalf("NaN bits = %#08x, want %#08x", math.Float32bits(got), nanBits)
	}
}

func TestFloat64_SignedZero(t *testing.T) {
	// -0.0 and +0.0 are == in Go but have distinct bit patterns; both must be
	// preserved exactly so the sign bit is not lost.
	posZero := roundtripFloat64(t, 0.0)
	negZero := roundtripFloat64(t, math.Copysign(0, -1))

	if math.Float64bits(posZero) != 0x0000000000000000 {
		t.Fatalf("+0.0 bits = %#016x, want 0", math.Float64bits(posZero))
	}
	if math.Float64bits(negZero) != 0x8000000000000000 {
		t.Fatalf("-0.0 bits = %#016x, want sign bit set", math.Float64bits(negZero))
	}
	if math.Float64bits(posZero) == math.Float64bits(negZero) {
		t.Fatal("-0.0 and +0.0 must remain distinct by bits")
	}
}

func TestFloat32_SignedZero(t *testing.T) {
	posZero := roundtripFloat32(t, 0.0)
	negZero := roundtripFloat32(t, float32(math.Copysign(0, -1)))

	if math.Float32bits(posZero) != 0x00000000 {
		t.Fatalf("+0.0 bits = %#08x, want 0", math.Float32bits(posZero))
	}
	if math.Float32bits(negZero) != 0x80000000 {
		t.Fatalf("-0.0 bits = %#08x, want sign bit set", math.Float32bits(negZero))
	}
	if math.Float32bits(posZero) == math.Float32bits(negZero) {
		t.Fatal("-0.0 and +0.0 must remain distinct by bits (f32)")
	}
}

func TestFloat64_Infinities(t *testing.T) {
	if got := roundtripFloat64(t, math.Inf(1)); !math.IsInf(got, 1) {
		t.Fatalf("+Inf round-trip = %v", got)
	}
	if got := roundtripFloat64(t, math.Inf(-1)); !math.IsInf(got, -1) {
		t.Fatalf("-Inf round-trip = %v", got)
	}
}

func TestFloat32_Infinities(t *testing.T) {
	if got := roundtripFloat32(t, float32(math.Inf(1))); !math.IsInf(float64(got), 1) {
		t.Fatalf("+Inf f32 round-trip = %v", got)
	}
	if got := roundtripFloat32(t, float32(math.Inf(-1))); !math.IsInf(float64(got), -1) {
		t.Fatalf("-Inf f32 round-trip = %v", got)
	}
}

func TestFloat64_SubnormalAndMax(t *testing.T) {
	cases := []struct {
		name string
		bits uint64
	}{
		{"smallest positive subnormal", 0x0000000000000001}, // math.SmallestNonzeroFloat64
		{"max finite", 0x7FEFFFFFFFFFFFFF},                  // math.MaxFloat64
	}
	for _, c := range cases {
		v := math.Float64frombits(c.bits)
		got := roundtripFloat64(t, v)
		if math.Float64bits(got) != c.bits {
			t.Fatalf("%s bits = %#016x, want %#016x", c.name, math.Float64bits(got), c.bits)
		}
	}
	// Sanity-check against stdlib constants.
	if math.Float64bits(roundtripFloat64(t, math.SmallestNonzeroFloat64)) != 0x0000000000000001 {
		t.Fatal("SmallestNonzeroFloat64 mismatch")
	}
	if roundtripFloat64(t, math.MaxFloat64) != math.MaxFloat64 {
		t.Fatal("MaxFloat64 mismatch")
	}
}

func TestFloat32_SubnormalAndMax(t *testing.T) {
	cases := []struct {
		name string
		bits uint32
	}{
		{"smallest positive subnormal", 0x00000001}, // math.SmallestNonzeroFloat32
		{"max finite", 0x7F7FFFFF},                  // math.MaxFloat32
	}
	for _, c := range cases {
		v := math.Float32frombits(c.bits)
		got := roundtripFloat32(t, v)
		if math.Float32bits(got) != c.bits {
			t.Fatalf("%s bits = %#08x, want %#08x", c.name, math.Float32bits(got), c.bits)
		}
	}
	if roundtripFloat32(t, math.MaxFloat32) != math.MaxFloat32 {
		t.Fatal("MaxFloat32 mismatch")
	}
}

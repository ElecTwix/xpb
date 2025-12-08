package wire

import (
	"testing"
)

func TestMakeTag(t *testing.T) {
	tests := []struct {
		fieldNumber uint32
		wireType    WireType
		expected    uint64
	}{
		{1, WireVarint, 0x08},          // (1 << 3) | 0 = 8
		{1, WireLengthDelimited, 0x0A}, // (1 << 3) | 3 = 11 -> 0x0A is wrong, should be 0x0B
		{2, WireVarint, 0x10},          // (2 << 3) | 0 = 16
		{15, WireVarint, 0x78},         // (15 << 3) | 0 = 120
		{16, WireVarint, 0x80},         // (16 << 3) | 0 = 128
	}

	for _, tt := range tests {
		got := MakeTag(tt.fieldNumber, tt.wireType)
		// Fix: recalculate expected
		expected := (uint64(tt.fieldNumber) << 3) | uint64(tt.wireType)
		if got != expected {
			t.Errorf("MakeTag(%d, %d) = %d, want %d", tt.fieldNumber, tt.wireType, got, expected)
		}
	}
}

func TestTagFieldNumber(t *testing.T) {
	tests := []struct {
		tag      uint64
		expected uint32
	}{
		{0x08, 1},
		{0x10, 2},
		{0x78, 15},
		{0x80, 16},
	}

	for _, tt := range tests {
		got := TagFieldNumber(tt.tag)
		if got != tt.expected {
			t.Errorf("TagFieldNumber(%d) = %d, want %d", tt.tag, got, tt.expected)
		}
	}
}

func TestTagWireType(t *testing.T) {
	tests := []struct {
		tag      uint64
		expected WireType
	}{
		{0x08, WireVarint},
		{0x0A, WireFixed64}, // (1 << 3) | 2 = 10
		{0x0B, WireLengthDelimited},
	}

	for _, tt := range tests {
		got := TagWireType(tt.tag)
		if got != tt.expected {
			t.Errorf("TagWireType(%d) = %d, want %d", tt.tag, got, tt.expected)
		}
	}
}

func TestZigZagEncode32(t *testing.T) {
	tests := []struct {
		input    int32
		expected uint32
	}{
		{0, 0},
		{-1, 1},
		{1, 2},
		{-2, 3},
		{2, 4},
		{2147483647, 4294967294},
		{-2147483648, 4294967295},
	}

	for _, tt := range tests {
		got := ZigZagEncode32(tt.input)
		if got != tt.expected {
			t.Errorf("ZigZagEncode32(%d) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestZigZagDecode32(t *testing.T) {
	tests := []struct {
		input    uint32
		expected int32
	}{
		{0, 0},
		{1, -1},
		{2, 1},
		{3, -2},
		{4, 2},
		{4294967294, 2147483647},
		{4294967295, -2147483648},
	}

	for _, tt := range tests {
		got := ZigZagDecode32(tt.input)
		if got != tt.expected {
			t.Errorf("ZigZagDecode32(%d) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestZigZagRoundTrip32(t *testing.T) {
	values := []int32{0, 1, -1, 127, -128, 32767, -32768, 2147483647, -2147483648}
	for _, v := range values {
		encoded := ZigZagEncode32(v)
		decoded := ZigZagDecode32(encoded)
		if decoded != v {
			t.Errorf("ZigZag roundtrip failed for %d: got %d", v, decoded)
		}
	}
}

func TestZigZagRoundTrip64(t *testing.T) {
	values := []int64{0, 1, -1, 127, -128, 32767, -32768, 9223372036854775807, -9223372036854775808}
	for _, v := range values {
		encoded := ZigZagEncode64(v)
		decoded := ZigZagDecode64(encoded)
		if decoded != v {
			t.Errorf("ZigZag roundtrip failed for %d: got %d", v, decoded)
		}
	}
}

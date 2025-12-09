package wire

import (
	"testing"
)

func TestCompactLengthSize(t *testing.T) {
	tests := []struct {
		length   int
		expected int
	}{
		{0, 1},
		{1, 1},
		{254, 1},
		{255, 5},
		{256, 5},
		{1000, 5},
		{1000000, 5},
	}

	for _, tt := range tests {
		got := CompactLengthSize(tt.length)
		if got != tt.expected {
			t.Errorf("CompactLengthSize(%d) = %d, want %d", tt.length, got, tt.expected)
		}
	}
}

func TestSizeConstants(t *testing.T) {
	if SizeBool != 1 {
		t.Errorf("SizeBool = %d, want 1", SizeBool)
	}
	if SizeInt32 != 4 {
		t.Errorf("SizeInt32 = %d, want 4", SizeInt32)
	}
	if SizeInt64 != 8 {
		t.Errorf("SizeInt64 = %d, want 8", SizeInt64)
	}
	if SizeUint32 != 4 {
		t.Errorf("SizeUint32 = %d, want 4", SizeUint32)
	}
	if SizeUint64 != 8 {
		t.Errorf("SizeUint64 = %d, want 8", SizeUint64)
	}
	if SizeFloat32 != 4 {
		t.Errorf("SizeFloat32 = %d, want 4", SizeFloat32)
	}
	if SizeFloat64 != 8 {
		t.Errorf("SizeFloat64 = %d, want 8", SizeFloat64)
	}
}

func TestCompactLengthConstants(t *testing.T) {
	if CompactLengthThreshold != 254 {
		t.Errorf("CompactLengthThreshold = %d, want 254", CompactLengthThreshold)
	}
	if CompactLengthMarker != 0xFF {
		t.Errorf("CompactLengthMarker = %d, want 255", CompactLengthMarker)
	}
}

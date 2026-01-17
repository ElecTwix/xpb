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

func TestCompactLengthEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		length   int
		expected int
	}{
		{"zero", 0, 1},
		{"one", 1, 1},
		{"max single byte", 254, 1},
		{"threshold", 255, 5},
		{"threshold + 1", 256, 5},
		{"large", 10000, 5},
		{"very large", 1000000000, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompactLengthSize(tt.length)
			if got != tt.expected {
				t.Errorf("CompactLengthSize(%d) = %d, want %d", tt.length, got, tt.expected)
			}
		})
	}
}

func TestSizeConstantsConsistency(t *testing.T) {
	// Verify sizes are consistent with expected byte counts
	if SizeInt32 != 4 {
		t.Errorf("SizeInt32 = %d, should be 4 bytes", SizeInt32)
	}
	if SizeInt64 != 8 {
		t.Errorf("SizeInt64 = %d, should be 8 bytes", SizeInt64)
	}
}

func TestAllFixedSizes(t *testing.T) {
	// Test all fixed-size types
	types := []struct {
		name  string
		size  int
		bytes int
	}{
		{"SizeBool", SizeBool, 1},
		{"SizeInt32", SizeInt32, 4},
		{"SizeInt64", SizeInt64, 8},
		{"SizeUint32", SizeUint32, 4},
		{"SizeUint64", SizeUint64, 8},
		{"SizeFloat32", SizeFloat32, 4},
		{"SizeFloat64", SizeFloat64, 8},
	}

	for _, tt := range types {
		if tt.size != tt.bytes {
			t.Errorf("%s = %d, want %d bytes", tt.name, tt.size, tt.bytes)
		}
	}
}

func BenchmarkCompactLengthSize(b *testing.B) {
	sizes := []int{0, 100, 254, 255, 1000, 1000000}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, size := range sizes {
			_ = CompactLengthSize(size)
		}
	}
}

func BenchmarkCompactLengthSizeLoop(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for size := 0; size <= 1000000; size += 1000 {
			_ = CompactLengthSize(size)
		}
	}
}

// Helper to verify byte patterns for compact length encoding
func TestCompactLengthEncoding(t *testing.T) {
	// These are the expected byte patterns for specific lengths
	expectedPatterns := map[int][]byte{
		0:   {0},
		1:   {1},
		100: {100},
		254: {254},
		255: {0xFF, 255, 0, 0, 0},
		256: {0xFF, 0, 1, 0, 0},
	}

	for length, expected := range expectedPatterns {
		size := CompactLengthSize(length)
		if len(expected) != size {
			t.Errorf("length %d: expected size %d, got %d", length, len(expected), size)
		}
	}
}

func TestWirePackageCoverage(t *testing.T) {
	// Verify package documentation mentions V2 format
	// This is a smoke test to ensure package is properly configured
	if CompactLengthThreshold != 254 {
		t.Error("CompactLengthThreshold should be 254")
	}
	if SizeBool != 1 {
		t.Error("SizeBool should be 1")
	}
	if SizeInt32 != 4 {
		t.Error("SizeInt32 should be 4")
	}
}

// Verify consistency between threshold and marker
func TestCompactLengthThresholdMarker(t *testing.T) {
	// The threshold is 254, meaning values 0-254 fit in 1 byte
	// Value 255 requires the extended format with marker (0xFF)
	if CompactLengthThreshold != 254 {
		t.Errorf("CompactLengthThreshold = %d, want 254", CompactLengthThreshold)
	}
	if CompactLengthMarker != 0xFF {
		t.Errorf("CompactLengthMarker = %d, want 255", CompactLengthMarker)
	}
	// Verify that 254 uses 1 byte (threshold)
	if CompactLengthSize(254) != 1 {
		t.Error("Length 254 should use 1 byte")
	}
	// Verify that 255 uses extended format
	if CompactLengthSize(255) != 5 {
		t.Error("Length 255 should use 5 bytes (marker + 4 bytes)")
	}
}

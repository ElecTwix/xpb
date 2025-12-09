// Package wire defines the XPB V2 wire format constants and utilities.
// V2 uses fixed-width encoding with no tags (struct mode).
package wire

// V2 Format: Fields are written in declaration order, no tags.
// All integers use fixed-width little-endian encoding.
// Lengths use compact encoding: 1 byte if < 255, else 0xFF + 4 bytes.

const (
	// CompactLengthThreshold is the max length that fits in 1 byte.
	CompactLengthThreshold = 254

	// CompactLengthMarker indicates a 4-byte length follows.
	CompactLengthMarker = 0xFF
)

// Fixed sizes for V2 types.
const (
	SizeBool    = 1
	SizeInt32   = 4
	SizeInt64   = 8
	SizeUint32  = 4
	SizeUint64  = 8
	SizeFloat32 = 4
	SizeFloat64 = 8
)

// CompactLengthSize returns the size needed to encode a length.
func CompactLengthSize(length int) int {
	if length <= CompactLengthThreshold {
		return 1
	}
	return 5
}

// Package wire defines the XPB wire format constants and utilities.
package wire

// WireType represents the encoding type used for a field on the wire.
type WireType uint8

const (
	// WireVarint is used for int32, int64, uint32, uint64, bool.
	WireVarint WireType = 0
	// WireFixed32 is used for float32.
	WireFixed32 WireType = 1
	// WireFixed64 is used for float64.
	WireFixed64 WireType = 2
	// WireLengthDelimited is used for string, bytes, and nested messages.
	WireLengthDelimited WireType = 3
)

// MaxVarintLen is the maximum length of a varint-encoded 64-bit value.
const MaxVarintLen64 = 10

// MaxVarintLen32 is the maximum length of a varint-encoded 32-bit value.
const MaxVarintLen32 = 5

// TagFieldNumber extracts the field number from a tag.
func TagFieldNumber(tag uint64) uint32 {
	return uint32(tag >> 3)
}

// TagWireType extracts the wire type from a tag.
func TagWireType(tag uint64) WireType {
	return WireType(tag & 0x7)
}

// MakeTag creates a tag from a field number and wire type.
func MakeTag(fieldNumber uint32, wireType WireType) uint64 {
	return (uint64(fieldNumber) << 3) | uint64(wireType)
}

// ZigZagEncode32 encodes a signed int32 using zigzag encoding.
func ZigZagEncode32(n int32) uint32 {
	return uint32((n << 1) ^ (n >> 31))
}

// ZigZagDecode32 decodes a zigzag-encoded uint32 to int32.
func ZigZagDecode32(n uint32) int32 {
	return int32((n >> 1) ^ -(n & 1))
}

// ZigZagEncode64 encodes a signed int64 using zigzag encoding.
func ZigZagEncode64(n int64) uint64 {
	return uint64((n << 1) ^ (n >> 63))
}

// ZigZagDecode64 decodes a zigzag-encoded uint64 to int64.
func ZigZagDecode64(n uint64) int64 {
	return int64((n >> 1) ^ -(n & 1))
}

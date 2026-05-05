// Package xpb provides the XPB V2 runtime library for encoding and decoding.
// V2 uses struct mode (no tags), fixed-width integers, and compact lengths.
package xpb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
	"unsafe"

	"github.com/ElecTwix/xpb/pkg/wire"
)

// Common errors.
var (
	ErrBufferTooSmall    = errors.New("xpb: buffer too small")
	ErrInvalidData       = errors.New("xpb: invalid data")
	ErrMaxDepthExceeded  = errors.New("xpb: max decode depth exceeded")
)

// MaxDecodeDepth caps the recursion depth for nested message decoding,
// preventing stack exhaustion from adversarial deeply-nested payloads.
// Generated unmarshalAt(depth int) helpers compare against this constant.
const MaxDecodeDepth = 64

var encoderPool = sync.Pool{
	New: func() interface{} {
		return &Encoder{buf: make([]byte, 0, 256)}
	},
}

// GetEncoder retrieves an Encoder from the pool.
// The encoder is automatically reset and ready for use.
// Call PutEncoder when done to return it to the pool.
func GetEncoder() *Encoder {
	e := encoderPool.Get().(*Encoder)
	e.Reset()
	return e
}

// PutEncoder returns an Encoder to the pool.
func PutEncoder(e *Encoder) {
	encoderPool.Put(e)
}

// Encoder encodes values into XPB V2 binary format.
// V2 uses fixed-width encoding with no tags.
type Encoder struct {
	buf []byte
}

// NewEncoder creates a new encoder with the given initial capacity.
// For better performance, use GetEncoder() instead to utilize the object pool.
func NewEncoder(capacity int) *Encoder {
	return &Encoder{buf: make([]byte, 0, capacity)}
}

// Reset clears the encoder for reuse.
func (e *Encoder) Reset() {
	e.buf = e.buf[:0]
}

// Bytes returns the encoded bytes.
func (e *Encoder) Bytes() []byte {
	return e.buf
}

// Len returns the current length of encoded data.
func (e *Encoder) Len() int {
	return len(e.buf)
}

// WriteBool writes a boolean as 1 byte.
func (e *Encoder) WriteBool(v bool) {
	if v {
		e.buf = append(e.buf, 1)
	} else {
		e.buf = append(e.buf, 0)
	}
}

// WriteInt32 writes a signed 32-bit integer as 4 bytes (little-endian, two's complement).
func (e *Encoder) WriteInt32(v int32) {
	e.buf = binary.LittleEndian.AppendUint32(e.buf, uint32(v))
}

// WriteInt64 writes a signed 64-bit integer as 8 bytes (little-endian, two's complement).
func (e *Encoder) WriteInt64(v int64) {
	e.buf = binary.LittleEndian.AppendUint64(e.buf, uint64(v))
}

// WriteUint32 writes an unsigned 32-bit integer as 4 bytes (little-endian).
func (e *Encoder) WriteUint32(v uint32) {
	e.buf = binary.LittleEndian.AppendUint32(e.buf, v)
}

// WriteUint64 writes an unsigned 64-bit integer as 8 bytes (little-endian).
func (e *Encoder) WriteUint64(v uint64) {
	e.buf = binary.LittleEndian.AppendUint64(e.buf, v)
}

// WriteFloat32 writes a 32-bit float as 4 bytes.
func (e *Encoder) WriteFloat32(v float32) {
	e.buf = binary.LittleEndian.AppendUint32(e.buf, math.Float32bits(v))
}

// WriteFloat64 writes a 64-bit float as 8 bytes.
func (e *Encoder) WriteFloat64(v float64) {
	e.buf = binary.LittleEndian.AppendUint64(e.buf, math.Float64bits(v))
}

// writeCompactLength writes a length using compact encoding.
// If length <= 254: 1 byte
// Else: 0xFF marker + 4 bytes (little-endian)
func (e *Encoder) writeCompactLength(length int) {
	if length <= wire.CompactLengthThreshold {
		e.buf = append(e.buf, byte(length))
	} else {
		e.buf = append(e.buf, wire.CompactLengthMarker)
		e.buf = binary.LittleEndian.AppendUint32(e.buf, uint32(length))
	}
}

// WriteString writes a length-prefixed string.
func (e *Encoder) WriteString(v string) {
	e.writeCompactLength(len(v))
	e.buf = append(e.buf, v...)
}

// WriteBytes writes a length-prefixed byte slice.
func (e *Encoder) WriteBytes(v []byte) {
	e.writeCompactLength(len(v))
	e.buf = append(e.buf, v...)
}

// WriteMessage writes a nested message (already encoded).
func (e *Encoder) WriteMessage(data []byte) {
	e.writeCompactLength(len(data))
	e.buf = append(e.buf, data...)
}

// AppendRaw appends raw bytes directly.
func (e *Encoder) AppendRaw(data []byte) {
	e.buf = append(e.buf, data...)
}

// Decoder decodes XPB V2 binary format.
type Decoder struct {
	buf []byte
	pos int
}

// NewDecoder creates a new decoder for the given data.
func NewDecoder(data []byte) *Decoder {
	return &Decoder{buf: data, pos: 0}
}

// Reset resets the decoder with new data.
func (d *Decoder) Reset(data []byte) {
	d.buf = data
	d.pos = 0
}

// Remaining returns the number of bytes remaining.
func (d *Decoder) Remaining() int {
	return len(d.buf) - d.pos
}

// EOF returns true if all data has been consumed.
func (d *Decoder) EOF() bool {
	return d.pos >= len(d.buf)
}

// ReadBool reads a boolean from 1 byte.
func (d *Decoder) ReadBool() (bool, error) {
	if d.pos >= len(d.buf) {
		return false, io.ErrUnexpectedEOF
	}
	v := d.buf[d.pos] != 0
	d.pos++
	return v, nil
}

// ReadInt32 reads a signed 32-bit integer from 4 bytes.
func (d *Decoder) ReadInt32() (int32, error) {
	if d.pos+4 > len(d.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	v := int32(binary.LittleEndian.Uint32(d.buf[d.pos:]))
	d.pos += 4
	return v, nil
}

// ReadInt64 reads a signed 64-bit integer from 8 bytes.
func (d *Decoder) ReadInt64() (int64, error) {
	if d.pos+8 > len(d.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	v := int64(binary.LittleEndian.Uint64(d.buf[d.pos:]))
	d.pos += 8
	return v, nil
}

// ReadUint32 reads an unsigned 32-bit integer from 4 bytes.
func (d *Decoder) ReadUint32() (uint32, error) {
	if d.pos+4 > len(d.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.LittleEndian.Uint32(d.buf[d.pos:])
	d.pos += 4
	return v, nil
}

// ReadUint64 reads an unsigned 64-bit integer from 8 bytes.
func (d *Decoder) ReadUint64() (uint64, error) {
	if d.pos+8 > len(d.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.LittleEndian.Uint64(d.buf[d.pos:])
	d.pos += 8
	return v, nil
}

// ReadFloat32 reads a 32-bit float from 4 bytes.
func (d *Decoder) ReadFloat32() (float32, error) {
	if d.pos+4 > len(d.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	bits := binary.LittleEndian.Uint32(d.buf[d.pos:])
	d.pos += 4
	return math.Float32frombits(bits), nil
}

// ReadFloat64 reads a 64-bit float from 8 bytes.
func (d *Decoder) ReadFloat64() (float64, error) {
	if d.pos+8 > len(d.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	bits := binary.LittleEndian.Uint64(d.buf[d.pos:])
	d.pos += 8
	return math.Float64frombits(bits), nil
}

// readCompactLength reads a compact-encoded length.
func (d *Decoder) readCompactLength() (int, error) {
	if d.pos >= len(d.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	first := d.buf[d.pos]
	d.pos++
	if first != wire.CompactLengthMarker {
		return int(first), nil
	}
	// Read 4-byte length
	if d.pos+4 > len(d.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	length := binary.LittleEndian.Uint32(d.buf[d.pos:])
	d.pos += 4
	return int(length), nil
}

// ReadString reads a length-prefixed string using zero-copy (unsafe).
// The returned string aliases the decoder's buffer.
// Use CloneString() if you need a safe copy.
func (d *Decoder) ReadString() (string, error) {
	length, err := d.readCompactLength()
	if err != nil {
		return "", err
	}
	if d.pos+length > len(d.buf) {
		return "", io.ErrUnexpectedEOF
	}
	s := unsafe.String(unsafe.SliceData(d.buf[d.pos:]), length)
	d.pos += length
	return s, nil
}

// CloneString reads a length-prefixed string and returns a safe copy.
func (d *Decoder) CloneString() (string, error) {
	length, err := d.readCompactLength()
	if err != nil {
		return "", err
	}
	if d.pos+length > len(d.buf) {
		return "", io.ErrUnexpectedEOF
	}
	// Force allocation
	s := string(d.buf[d.pos : d.pos+length])
	d.pos += length
	return s, nil
}

// ReadBytes reads a length-prefixed byte slice.
func (d *Decoder) ReadBytes() ([]byte, error) {
	length, err := d.readCompactLength()
	if err != nil {
		return nil, err
	}
	if d.pos+length > len(d.buf) {
		return nil, io.ErrUnexpectedEOF
	}
	data := make([]byte, length)
	copy(data, d.buf[d.pos:d.pos+length])
	d.pos += length
	return data, nil
}

// ReadBytesUnsafe reads a length-prefixed byte slice using zero-copy.
// The returned slice aliases the decoder's buffer.
// Warning: The data remains valid only as long as the decoder buffer is valid.
func (d *Decoder) ReadBytesUnsafe() ([]byte, error) {
	length, err := d.readCompactLength()
	if err != nil {
		return nil, err
	}
	if d.pos+length > len(d.buf) {
		return nil, io.ErrUnexpectedEOF
	}
	data := d.buf[d.pos : d.pos+length]
	d.pos += length
	return data, nil
}

// ReadMessageBytes reads a length-prefixed message.
func (d *Decoder) ReadMessageBytes() ([]byte, error) {
	return d.ReadBytes()
}

// Skip skips n bytes.
func (d *Decoder) Skip(n int) error {
	if d.pos+n > len(d.buf) {
		return io.ErrUnexpectedEOF
	}
	d.pos += n
	return nil
}

// ReadArrayCount reads a 4-byte signed array length used by repeated and map
// fields, validating it before the caller allocates a backing slice. It rejects
// negative counts and counts that cannot possibly fit in the remaining buffer
// (each element must occupy at least elementMinBytes on the wire). Pass 1 when
// elements are variable-length (string, bytes, message). Pass 0 to skip the
// upper-bound check (not recommended for untrusted input).
func (d *Decoder) ReadArrayCount(elementMinBytes int) (int32, error) {
	n, err := d.ReadInt32()
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, fmt.Errorf("xpb: negative array count: %d", n)
	}
	if elementMinBytes > 0 {
		max := d.Remaining() / elementMinBytes
		if int(n) > max {
			return 0, fmt.Errorf("xpb: array count %d exceeds buffer-bounded max %d", n, max)
		}
	}
	return n, nil
}

// Package xpb provides the XPB runtime library for encoding and decoding.
package xpb

import (
	"encoding/binary"
	"errors"
	"io"
	"math"

	"github.com/anthropic/xpb/pkg/wire"
)

// Common errors.
var (
	ErrBufferTooSmall = errors.New("xpb: buffer too small")
	ErrOverflow       = errors.New("xpb: varint overflow")
	ErrInvalidData    = errors.New("xpb: invalid data")
)

// Encoder encodes values into XPB binary format.
type Encoder struct {
	buf []byte
}

// NewEncoder creates a new encoder with the given initial capacity.
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

// WriteTag writes a field tag (field number + wire type).
func (e *Encoder) WriteTag(fieldNumber uint32, wireType wire.WireType) {
	tag := wire.MakeTag(fieldNumber, wireType)
	e.WriteUvarint(tag)
}

// WriteUvarint writes an unsigned varint.
func (e *Encoder) WriteUvarint(v uint64) {
	var buf [wire.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], v)
	e.buf = append(e.buf, buf[:n]...)
}

// WriteVarint writes a signed varint using zigzag encoding.
func (e *Encoder) WriteVarint(v int64) {
	e.WriteUvarint(wire.ZigZagEncode64(v))
}

// WriteBool writes a boolean value.
func (e *Encoder) WriteBool(fieldNumber uint32, v bool) {
	e.WriteTag(fieldNumber, wire.WireVarint)
	if v {
		e.buf = append(e.buf, 1)
	} else {
		e.buf = append(e.buf, 0)
	}
}

// WriteInt32 writes a signed 32-bit integer.
func (e *Encoder) WriteInt32(fieldNumber uint32, v int32) {
	e.WriteTag(fieldNumber, wire.WireVarint)
	e.WriteUvarint(wire.ZigZagEncode64(int64(v)))
}

// WriteInt64 writes a signed 64-bit integer.
func (e *Encoder) WriteInt64(fieldNumber uint32, v int64) {
	e.WriteTag(fieldNumber, wire.WireVarint)
	e.WriteUvarint(wire.ZigZagEncode64(v))
}

// WriteUint32 writes an unsigned 32-bit integer.
func (e *Encoder) WriteUint32(fieldNumber uint32, v uint32) {
	e.WriteTag(fieldNumber, wire.WireVarint)
	e.WriteUvarint(uint64(v))
}

// WriteUint64 writes an unsigned 64-bit integer.
func (e *Encoder) WriteUint64(fieldNumber uint32, v uint64) {
	e.WriteTag(fieldNumber, wire.WireVarint)
	e.WriteUvarint(v)
}

// WriteFloat32 writes a 32-bit float.
func (e *Encoder) WriteFloat32(fieldNumber uint32, v float32) {
	e.WriteTag(fieldNumber, wire.WireFixed32)
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], math.Float32bits(v))
	e.buf = append(e.buf, buf[:]...)
}

// WriteFloat64 writes a 64-bit float.
func (e *Encoder) WriteFloat64(fieldNumber uint32, v float64) {
	e.WriteTag(fieldNumber, wire.WireFixed64)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
	e.buf = append(e.buf, buf[:]...)
}

// WriteString writes a string.
func (e *Encoder) WriteString(fieldNumber uint32, v string) {
	e.WriteTag(fieldNumber, wire.WireLengthDelimited)
	e.WriteUvarint(uint64(len(v)))
	e.buf = append(e.buf, v...)
}

// WriteBytes writes a byte slice.
func (e *Encoder) WriteBytes(fieldNumber uint32, v []byte) {
	e.WriteTag(fieldNumber, wire.WireLengthDelimited)
	e.WriteUvarint(uint64(len(v)))
	e.buf = append(e.buf, v...)
}

// WriteMessage writes a nested message.
// The message should already be encoded.
func (e *Encoder) WriteMessage(fieldNumber uint32, data []byte) {
	e.WriteTag(fieldNumber, wire.WireLengthDelimited)
	e.WriteUvarint(uint64(len(data)))
	e.buf = append(e.buf, data...)
}

// AppendRaw appends raw bytes directly (for pre-encoded data).
func (e *Encoder) AppendRaw(data []byte) {
	e.buf = append(e.buf, data...)
}

// Decoder decodes XPB binary format.
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

// ReadTag reads a field tag and returns (fieldNumber, wireType).
func (d *Decoder) ReadTag() (uint32, wire.WireType, error) {
	tag, err := d.ReadUvarint()
	if err != nil {
		return 0, 0, err
	}
	return wire.TagFieldNumber(tag), wire.TagWireType(tag), nil
}

// ReadUvarint reads an unsigned varint.
func (d *Decoder) ReadUvarint() (uint64, error) {
	if d.pos >= len(d.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	v, n := binary.Uvarint(d.buf[d.pos:])
	if n == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	if n < 0 {
		return 0, ErrOverflow
	}
	d.pos += n
	return v, nil
}

// ReadVarint reads a signed varint (zigzag encoded).
func (d *Decoder) ReadVarint() (int64, error) {
	v, err := d.ReadUvarint()
	if err != nil {
		return 0, err
	}
	return wire.ZigZagDecode64(v), nil
}

// ReadBool reads a boolean value.
func (d *Decoder) ReadBool() (bool, error) {
	v, err := d.ReadUvarint()
	if err != nil {
		return false, err
	}
	return v != 0, nil
}

// ReadInt32 reads a signed 32-bit integer.
func (d *Decoder) ReadInt32() (int32, error) {
	v, err := d.ReadVarint()
	if err != nil {
		return 0, err
	}
	return int32(v), nil
}

// ReadInt64 reads a signed 64-bit integer.
func (d *Decoder) ReadInt64() (int64, error) {
	return d.ReadVarint()
}

// ReadUint32 reads an unsigned 32-bit integer.
func (d *Decoder) ReadUint32() (uint32, error) {
	v, err := d.ReadUvarint()
	if err != nil {
		return 0, err
	}
	return uint32(v), nil
}

// ReadUint64 reads an unsigned 64-bit integer.
func (d *Decoder) ReadUint64() (uint64, error) {
	return d.ReadUvarint()
}

// ReadFloat32 reads a 32-bit float.
func (d *Decoder) ReadFloat32() (float32, error) {
	if d.pos+4 > len(d.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	bits := binary.LittleEndian.Uint32(d.buf[d.pos:])
	d.pos += 4
	return math.Float32frombits(bits), nil
}

// ReadFloat64 reads a 64-bit float.
func (d *Decoder) ReadFloat64() (float64, error) {
	if d.pos+8 > len(d.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	bits := binary.LittleEndian.Uint64(d.buf[d.pos:])
	d.pos += 8
	return math.Float64frombits(bits), nil
}

// ReadString reads a length-prefixed string.
func (d *Decoder) ReadString() (string, error) {
	length, err := d.ReadUvarint()
	if err != nil {
		return "", err
	}
	if d.pos+int(length) > len(d.buf) {
		return "", io.ErrUnexpectedEOF
	}
	s := string(d.buf[d.pos : d.pos+int(length)])
	d.pos += int(length)
	return s, nil
}

// ReadBytes reads a length-prefixed byte slice.
func (d *Decoder) ReadBytes() ([]byte, error) {
	length, err := d.ReadUvarint()
	if err != nil {
		return nil, err
	}
	if d.pos+int(length) > len(d.buf) {
		return nil, io.ErrUnexpectedEOF
	}
	// Return a copy to avoid retaining the original buffer
	data := make([]byte, length)
	copy(data, d.buf[d.pos:d.pos+int(length)])
	d.pos += int(length)
	return data, nil
}

// ReadMessageBytes reads a length-prefixed message and returns the raw bytes.
func (d *Decoder) ReadMessageBytes() ([]byte, error) {
	return d.ReadBytes()
}

// Skip skips a field based on its wire type.
func (d *Decoder) Skip(wireType wire.WireType) error {
	switch wireType {
	case wire.WireVarint:
		_, err := d.ReadUvarint()
		return err
	case wire.WireFixed32:
		if d.pos+4 > len(d.buf) {
			return io.ErrUnexpectedEOF
		}
		d.pos += 4
		return nil
	case wire.WireFixed64:
		if d.pos+8 > len(d.buf) {
			return io.ErrUnexpectedEOF
		}
		d.pos += 8
		return nil
	case wire.WireLengthDelimited:
		length, err := d.ReadUvarint()
		if err != nil {
			return err
		}
		if d.pos+int(length) > len(d.buf) {
			return io.ErrUnexpectedEOF
		}
		d.pos += int(length)
		return nil
	default:
		return ErrInvalidData
	}
}

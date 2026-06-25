// Package xpb provides the XPB V2 runtime library for encoding and decoding.
// V2 uses struct mode (no tags), fixed-width integers, and compact lengths.
package xpb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"sync"
	"unsafe"

	"github.com/ElecTwix/xpb/pkg/wire"
)

// Common errors.
var (
	ErrBufferTooSmall   = errors.New("xpb: buffer too small")
	ErrInvalidData      = errors.New("xpb: invalid data")
	ErrMaxDepthExceeded = errors.New("xpb: max decode depth exceeded")
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

// Buf returns the encoder's current backing buffer so generated Marshal/MarshalTo
// can bind it to a register-local slice, append into the local via the stateless
// Append*To helpers, and write the local back once with SetBuf. Returning the
// full buffer (not buf[:0]) preserves MarshalTo's append-to-existing semantics:
// a pooled encoder is Reset before use, so this is empty in the steady-state
// pooled path; a caller that pre-loaded bytes still has them preserved.
func (e *Encoder) Buf() []byte {
	return e.buf
}

// SetBuf writes a register-local buffer back to the encoder exactly once, at the
// end of a generated Marshal/MarshalTo body. This is the symmetric encode
// counterpart of Phase 1's stateless decode cursor: append work happens on a
// local []byte threaded through registers, and the single store back into the
// Encoder.buf struct field happens here.
func (e *Encoder) SetBuf(b []byte) {
	e.buf = b
}

// --- Stateless cursor append helpers ---
//
// The *To helpers below are the register-local-buffer counterparts of the
// stateful (*Encoder).Write* methods above, mirroring the
// binary.LittleEndian.Append* style: each takes the destination buffer b and
// the value, and returns the grown buffer. Threading a local []byte through
// registers — instead of loading/storing the in-struct (*Encoder).buf 3-word
// slice header on every field append — is what lets generated encode reach the
// hand-written local-buffer performance ceiling while keeping the wire format
// byte-identical to the stateful Encoder.
//
// The logic is intentionally identical to the matching Encoder method: same
// little-endian fixed-width layout, same compact-length 0xFF path. The stateful
// Encoder API is preserved unchanged for streaming/manual callers and the pool;
// these are added alongside it. Each helper is kept small so the Go inliner can
// inline it into generated Marshal/MarshalTo bodies. The fixed-width scalar
// helpers are thin wrappers over encoding/binary so generated code need not
// import encoding/binary or math.

// GrowBuf ensures b has spare capacity for at least n more bytes, returning a
// (possibly reallocated) slice with the same length and contents. Generated
// Marshal/MarshalTo calls it once up front with the message's fixed-size lower
// bound so the per-field Append*To helpers do not each re-check capacity for the
// fixed portion of the message. It is a thin wrapper over slices.Grow so
// generated code need not import slices.
func GrowBuf(b []byte, n int) []byte {
	return slices.Grow(b, n)
}

// AppendBoolTo appends a boolean as 1 byte.
func AppendBoolTo(b []byte, v bool) []byte {
	if v {
		return append(b, 1)
	}
	return append(b, 0)
}

// AppendInt32To appends a signed 32-bit integer as 4 bytes (little-endian).
func AppendInt32To(b []byte, v int32) []byte {
	return binary.LittleEndian.AppendUint32(b, uint32(v))
}

// AppendInt64To appends a signed 64-bit integer as 8 bytes (little-endian).
func AppendInt64To(b []byte, v int64) []byte {
	return binary.LittleEndian.AppendUint64(b, uint64(v))
}

// AppendUint32To appends an unsigned 32-bit integer as 4 bytes (little-endian).
func AppendUint32To(b []byte, v uint32) []byte {
	return binary.LittleEndian.AppendUint32(b, v)
}

// AppendUint64To appends an unsigned 64-bit integer as 8 bytes (little-endian).
func AppendUint64To(b []byte, v uint64) []byte {
	return binary.LittleEndian.AppendUint64(b, v)
}

// AppendFloat32To appends a 32-bit float as 4 bytes.
func AppendFloat32To(b []byte, v float32) []byte {
	return binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
}

// AppendFloat64To appends a 64-bit float as 8 bytes.
func AppendFloat64To(b []byte, v float64) []byte {
	return binary.LittleEndian.AppendUint64(b, math.Float64bits(v))
}

// AppendCompactLengthTo appends a length using compact encoding, preserving the
// 0xFF marker path: length <= 254 is a single byte; otherwise the 0xFF marker
// is followed by a 4-byte little-endian length. Identical to
// (*Encoder).writeCompactLength.
func AppendCompactLengthTo(b []byte, length int) []byte {
	if length <= wire.CompactLengthThreshold {
		return append(b, byte(length))
	}
	b = append(b, wire.CompactLengthMarker)
	return binary.LittleEndian.AppendUint32(b, uint32(length))
}

// AppendStringTo appends a length-prefixed string.
func AppendStringTo(b []byte, v string) []byte {
	b = AppendCompactLengthTo(b, len(v))
	return append(b, v...)
}

// AppendBytesTo appends a length-prefixed byte slice.
func AppendBytesTo(b []byte, v []byte) []byte {
	b = AppendCompactLengthTo(b, len(v))
	return append(b, v...)
}

// AppendMessageTo appends a length-prefixed nested message (already encoded).
// It is the stateless-buffer counterpart of (*Encoder).WriteMessage.
func AppendMessageTo(b []byte, data []byte) []byte {
	b = AppendCompactLengthTo(b, len(data))
	return append(b, data...)
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
//
// A negative n is rejected with io.ErrUnexpectedEOF: without this guard a
// negative n passes the upper-bound check (d.pos+n is smaller than len(d.buf)),
// then d.pos += n drives pos negative, and the next Read* indexes d.buf at a
// negative offset and panics on untrusted input.
func (d *Decoder) Skip(n int) error {
	if n < 0 || d.pos+n > len(d.buf) {
		return io.ErrUnexpectedEOF
	}
	d.pos += n
	return nil
}

// ReadArrayCount reads a 4-byte signed array length used by repeated and map
// fields, validating it before the caller allocates a backing slice. The
// caller MUST supply maxElements — the runtime does not pick a default,
// so application-level allocation policy is visible at every call site.
//
// Validation order, fail-closed: negative counts rejected first, then
// counts above the caller's maxElements, then counts that cannot fit in
// the remaining buffer (each element occupies at least elementMinBytes
// on the wire). Pass elementMinBytes=1 for variable-length elements
// (string, bytes, message). Pass elementMinBytes=0 to skip the buffer
// bound (only safe for fully trusted input).
func (d *Decoder) ReadArrayCount(elementMinBytes, maxElements int) (int32, error) {
	if maxElements < 0 {
		return 0, fmt.Errorf("xpb: ReadArrayCount maxElements must be >= 0, got %d", maxElements)
	}
	n, err := d.ReadInt32()
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, fmt.Errorf("xpb: negative array count: %d", n)
	}
	if int(n) > maxElements {
		return 0, fmt.Errorf("xpb: array count %d exceeds caller-supplied max %d", n, maxElements)
	}
	if elementMinBytes > 0 {
		max := d.Remaining() / elementMinBytes
		if int(n) > max {
			return 0, fmt.Errorf("xpb: array count %d exceeds buffer-bounded max %d", n, max)
		}
	}
	return n, nil
}

// --- Stateless cursor read helpers ---
//
// The *At helpers below are the register-local-cursor counterparts of the
// stateful (*Decoder).Read* methods above. Each takes the buffer b and the
// current read offset p, and returns the decoded value together with the new
// offset (and an error). Threading the cursor through registers — instead of
// loading and storing the in-struct (*Decoder).pos on every read — is what
// lets generated decode reach the hand-written local-cursor performance
// ceiling while keeping all bounds-check logic centralized here.
//
// The logic is intentionally identical to the matching Decoder method: same
// bounds checks, same compact-length 0xFF path, same negative-length
// rejection, same ReadArrayCount validation semantics. The stateful Decoder
// API is preserved unchanged for streaming/manual callers; these are added
// alongside it. Each helper is kept small so the Go inliner can inline it into
// generated unmarshalAt bodies.

// ReadBoolAt reads a boolean from 1 byte at offset p.
func ReadBoolAt(b []byte, p int) (bool, int, error) {
	if p >= len(b) {
		return false, p, io.ErrUnexpectedEOF
	}
	v := b[p] != 0
	return v, p + 1, nil
}

// ReadInt32At reads a signed 32-bit integer from 4 bytes at offset p.
func ReadInt32At(b []byte, p int) (int32, int, error) {
	if p+4 > len(b) {
		return 0, p, io.ErrUnexpectedEOF
	}
	v := int32(binary.LittleEndian.Uint32(b[p:]))
	return v, p + 4, nil
}

// ReadInt64At reads a signed 64-bit integer from 8 bytes at offset p.
func ReadInt64At(b []byte, p int) (int64, int, error) {
	if p+8 > len(b) {
		return 0, p, io.ErrUnexpectedEOF
	}
	v := int64(binary.LittleEndian.Uint64(b[p:]))
	return v, p + 8, nil
}

// ReadUint32At reads an unsigned 32-bit integer from 4 bytes at offset p.
func ReadUint32At(b []byte, p int) (uint32, int, error) {
	if p+4 > len(b) {
		return 0, p, io.ErrUnexpectedEOF
	}
	v := binary.LittleEndian.Uint32(b[p:])
	return v, p + 4, nil
}

// ReadUint64At reads an unsigned 64-bit integer from 8 bytes at offset p.
func ReadUint64At(b []byte, p int) (uint64, int, error) {
	if p+8 > len(b) {
		return 0, p, io.ErrUnexpectedEOF
	}
	v := binary.LittleEndian.Uint64(b[p:])
	return v, p + 8, nil
}

// ReadFloat32At reads a 32-bit float from 4 bytes at offset p.
func ReadFloat32At(b []byte, p int) (float32, int, error) {
	if p+4 > len(b) {
		return 0, p, io.ErrUnexpectedEOF
	}
	bits := binary.LittleEndian.Uint32(b[p:])
	return math.Float32frombits(bits), p + 4, nil
}

// ReadFloat64At reads a 64-bit float from 8 bytes at offset p.
func ReadFloat64At(b []byte, p int) (float64, int, error) {
	if p+8 > len(b) {
		return 0, p, io.ErrUnexpectedEOF
	}
	bits := binary.LittleEndian.Uint64(b[p:])
	return math.Float64frombits(bits), p + 8, nil
}

// readCompactLengthAt reads a compact-encoded length at offset p, preserving
// the 0xFF marker path: a leading byte != marker is the length itself; the
// marker is followed by a 4-byte little-endian length.
func readCompactLengthAt(b []byte, p int) (int, int, error) {
	if p >= len(b) {
		return 0, p, io.ErrUnexpectedEOF
	}
	first := b[p]
	p++
	if first != wire.CompactLengthMarker {
		return int(first), p, nil
	}
	// Read 4-byte length
	if p+4 > len(b) {
		return 0, p, io.ErrUnexpectedEOF
	}
	length := binary.LittleEndian.Uint32(b[p:])
	return int(length), p + 4, nil
}

// ReadStringAt reads a length-prefixed string using zero-copy (unsafe) at
// offset p. The returned string aliases b; copy it if it must outlive b.
func ReadStringAt(b []byte, p int) (string, int, error) {
	length, p, err := readCompactLengthAt(b, p)
	if err != nil {
		return "", p, err
	}
	if p+length > len(b) {
		return "", p, io.ErrUnexpectedEOF
	}
	s := unsafe.String(unsafe.SliceData(b[p:]), length)
	return s, p + length, nil
}

// ReadBytesAt reads a length-prefixed byte slice at offset p, copying it so the
// result is independent of b.
func ReadBytesAt(b []byte, p int) ([]byte, int, error) {
	length, p, err := readCompactLengthAt(b, p)
	if err != nil {
		return nil, p, err
	}
	if p+length > len(b) {
		return nil, p, io.ErrUnexpectedEOF
	}
	data := make([]byte, length)
	copy(data, b[p:p+length])
	return data, p + length, nil
}

// ReadBytesUnsafeAt reads a length-prefixed byte slice at offset p using
// zero-copy. The returned slice aliases b and stays valid only while b is.
func ReadBytesUnsafeAt(b []byte, p int) ([]byte, int, error) {
	length, p, err := readCompactLengthAt(b, p)
	if err != nil {
		return nil, p, err
	}
	if p+length > len(b) {
		return nil, p, io.ErrUnexpectedEOF
	}
	data := b[p : p+length]
	return data, p + length, nil
}

// ReadMessageBytesAt reads a length-prefixed message envelope at offset p,
// returning the (zero-copy) body slice for recursive decoding. It aliases b,
// matching (*Decoder).ReadMessageBytes semantics for the generated nested
// decode path.
func ReadMessageBytesAt(b []byte, p int) ([]byte, int, error) {
	return ReadBytesUnsafeAt(b, p)
}

// ReadArrayCountAt reads a 4-byte signed array length at offset p used by
// repeated and map fields, validating it before the caller allocates a backing
// slice. It is the stateless-cursor counterpart of (*Decoder).ReadArrayCount
// and applies the identical fail-closed validation: negative maxElements is a
// programming error; negative counts are rejected first, then counts above
// maxElements, then counts that cannot fit in the bytes remaining after p
// (each element occupies at least elementMinBytes). Pass elementMinBytes=1 for
// variable-length elements and elementMinBytes=0 to skip the buffer bound.
func ReadArrayCountAt(b []byte, p, elementMinBytes, maxElements int) (int32, int, error) {
	if maxElements < 0 {
		return 0, p, fmt.Errorf("xpb: ReadArrayCount maxElements must be >= 0, got %d", maxElements)
	}
	n, p, err := ReadInt32At(b, p)
	if err != nil {
		return 0, p, err
	}
	if n < 0 {
		return 0, p, fmt.Errorf("xpb: negative array count: %d", n)
	}
	if int(n) > maxElements {
		return 0, p, fmt.Errorf("xpb: array count %d exceeds caller-supplied max %d", n, maxElements)
	}
	if elementMinBytes > 0 {
		max := (len(b) - p) / elementMinBytes
		if int(n) > max {
			return 0, p, fmt.Errorf("xpb: array count %d exceeds buffer-bounded max %d", n, max)
		}
	}
	return n, p, nil
}

package conformance

import (
	"encoding/hex"
	"fmt"
	"math"
	"strconv"

	"github.com/ElecTwix/xpb/runtime/go/xpb"
)

// --- Op constructors -------------------------------------------------------

func OpBool(v bool) Op     { return Op{Type: TypeBool, Bool: &v} }
func OpInt32(v int32) Op   { return Op{Type: TypeInt32, Int32: &v} }
func OpUint32(v uint32) Op { return Op{Type: TypeUint32, Uint32: &v} }
func OpInt64(v int64) Op   { return Op{Type: TypeInt64, Int64: strconv.FormatInt(v, 10)} }
func OpUint64(v uint64) Op { return Op{Type: TypeUint64, Uint64: strconv.FormatUint(v, 10)} }

func OpFloat32(v float32) Op {
	return Op{Type: TypeFloat32, FloatBits: fmt.Sprintf("0x%08X", math.Float32bits(v))}
}

func OpFloat64(v float64) Op {
	return Op{Type: TypeFloat64, FloatBits: fmt.Sprintf("0x%016X", math.Float64bits(v))}
}

// OpFloat32Bits/OpFloat64Bits build a float op directly from a bit pattern,
// for NaN payloads that cannot be written as a literal.
func OpFloat32Bits(bits uint32) Op {
	return Op{Type: TypeFloat32, FloatBits: fmt.Sprintf("0x%08X", bits)}
}
func OpFloat64Bits(bits uint64) Op {
	return Op{Type: TypeFloat64, FloatBits: fmt.Sprintf("0x%016X", bits)}
}

func OpString(v string) Op { return Op{Type: TypeString, String: v} }
func OpBytes(v []byte) Op  { return Op{Type: TypeBytes, Bytes: hex.EncodeToString(v)} }

func OpArray(elemType string, elems ...Op) Op {
	return Op{Type: TypeArray, ElemType: elemType, Elems: elems}
}

func OpMap(keyType, valType string, entries ...MapEntry) Op {
	return Op{Type: TypeMap, KeyType: keyType, ValType: valType, Entries: entries}
}

func OpMessage(ops ...Op) Op { return Op{Type: TypeMessage, Ops: ops} }

// --- decoding helpers for scalar value fields ------------------------------

func (o Op) f32Bits() uint32 {
	v, err := strconv.ParseUint(stripHexPrefix(o.FloatBits), 16, 32)
	if err != nil {
		panic(fmt.Sprintf("bad float32 bits %q: %v", o.FloatBits, err))
	}
	return uint32(v)
}

func (o Op) f64Bits() uint64 {
	v, err := strconv.ParseUint(stripHexPrefix(o.FloatBits), 16, 64)
	if err != nil {
		panic(fmt.Sprintf("bad float64 bits %q: %v", o.FloatBits, err))
	}
	return v
}

func (o Op) i64() int64 {
	v, err := strconv.ParseInt(o.Int64, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("bad int64 %q: %v", o.Int64, err))
	}
	return v
}

func (o Op) u64() uint64 {
	v, err := strconv.ParseUint(o.Uint64, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("bad uint64 %q: %v", o.Uint64, err))
	}
	return v
}

func (o Op) byteVal() []byte {
	b, err := hex.DecodeString(o.Bytes)
	if err != nil {
		panic(fmt.Sprintf("bad bytes hex %q: %v", o.Bytes, err))
	}
	return b
}

func stripHexPrefix(s string) string {
	if len(s) >= 2 && (s[0:2] == "0x" || s[0:2] == "0X") {
		return s[2:]
	}
	return s
}

// --- Encode ----------------------------------------------------------------

// Encode writes a sequence of ops using the Go (reference) encoder.
func Encode(ops []Op) []byte {
	e := xpb.NewEncoder(256)
	encodeOps(e, ops)
	return e.Bytes()
}

func encodeOps(e *xpb.Encoder, ops []Op) {
	for _, o := range ops {
		encodeOp(e, o)
	}
}

func encodeOp(e *xpb.Encoder, o Op) {
	switch o.Type {
	case TypeBool:
		e.WriteBool(*o.Bool)
	case TypeInt32:
		e.WriteInt32(*o.Int32)
	case TypeUint32:
		e.WriteUint32(*o.Uint32)
	case TypeInt64:
		e.WriteInt64(o.i64())
	case TypeUint64:
		e.WriteUint64(o.u64())
	case TypeFloat32:
		e.WriteFloat32(math.Float32frombits(o.f32Bits()))
	case TypeFloat64:
		e.WriteFloat64(math.Float64frombits(o.f64Bits()))
	case TypeString:
		e.WriteString(o.String)
	case TypeBytes:
		e.WriteBytes(o.byteVal())
	case TypeArray:
		e.WriteInt32(int32(len(o.Elems)))
		for _, el := range o.Elems {
			encodeOp(e, el)
		}
	case TypeMap:
		e.WriteInt32(int32(len(o.Entries)))
		for _, ent := range o.Entries {
			encodeOp(e, ent.K)
			encodeOp(e, ent.V)
		}
	case TypeMessage:
		inner := xpb.NewEncoder(64)
		encodeOps(inner, o.Ops)
		e.WriteMessage(inner.Bytes())
	default:
		panic("unknown op type: " + o.Type)
	}
}

// --- Decode + verify -------------------------------------------------------

// DecodeAndVerify reads the given bytes with the Go decoder, asserting that the
// decoded values equal the expected ops (bit-exact for floats). It returns an
// error describing the first mismatch, or nil on success.
func DecodeAndVerify(data []byte, ops []Op) error {
	d := xpb.NewDecoder(data)
	if err := verifyOps(d, ops, ""); err != nil {
		return err
	}
	if !d.EOF() {
		return fmt.Errorf("trailing %d bytes after decode", d.Remaining())
	}
	return nil
}

func verifyOps(d *xpb.Decoder, ops []Op, path string) error {
	for i, o := range ops {
		if err := verifyOp(d, o, fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
	}
	return nil
}

func verifyOp(d *xpb.Decoder, o Op, path string) error {
	switch o.Type {
	case TypeBool:
		got, err := d.ReadBool()
		if err != nil {
			return wrap(path, err)
		}
		if got != *o.Bool {
			return fmt.Errorf("%s bool: got %v want %v", path, got, *o.Bool)
		}
	case TypeInt32:
		got, err := d.ReadInt32()
		if err != nil {
			return wrap(path, err)
		}
		if got != *o.Int32 {
			return fmt.Errorf("%s int32: got %d want %d", path, got, *o.Int32)
		}
	case TypeUint32:
		got, err := d.ReadUint32()
		if err != nil {
			return wrap(path, err)
		}
		if got != *o.Uint32 {
			return fmt.Errorf("%s uint32: got %d want %d", path, got, *o.Uint32)
		}
	case TypeInt64:
		got, err := d.ReadInt64()
		if err != nil {
			return wrap(path, err)
		}
		if got != o.i64() {
			return fmt.Errorf("%s int64: got %d want %d", path, got, o.i64())
		}
	case TypeUint64:
		got, err := d.ReadUint64()
		if err != nil {
			return wrap(path, err)
		}
		if got != o.u64() {
			return fmt.Errorf("%s uint64: got %d want %d", path, got, o.u64())
		}
	case TypeFloat32:
		got, err := d.ReadFloat32()
		if err != nil {
			return wrap(path, err)
		}
		// Compare by bit pattern: NaN != NaN, -0.0 != +0.0.
		if math.Float32bits(got) != o.f32Bits() {
			return fmt.Errorf("%s float32 bits: got 0x%08X want 0x%08X",
				path, math.Float32bits(got), o.f32Bits())
		}
	case TypeFloat64:
		got, err := d.ReadFloat64()
		if err != nil {
			return wrap(path, err)
		}
		if math.Float64bits(got) != o.f64Bits() {
			return fmt.Errorf("%s float64 bits: got 0x%016X want 0x%016X",
				path, math.Float64bits(got), o.f64Bits())
		}
	case TypeString:
		got, err := d.CloneString()
		if err != nil {
			return wrap(path, err)
		}
		if got != o.String {
			return fmt.Errorf("%s string: got %q want %q", path, got, o.String)
		}
	case TypeBytes:
		got, err := d.ReadBytes()
		if err != nil {
			return wrap(path, err)
		}
		want := o.byteVal()
		if !bytesEqual(got, want) {
			return fmt.Errorf("%s bytes: got %x want %x", path, got, want)
		}
	case TypeArray:
		count, err := d.ReadInt32()
		if err != nil {
			return wrap(path, err)
		}
		if int(count) != len(o.Elems) {
			return fmt.Errorf("%s array count: got %d want %d", path, count, len(o.Elems))
		}
		for i, el := range o.Elems {
			if err := verifyOp(d, el, fmt.Sprintf("%s.elem[%d]", path, i)); err != nil {
				return err
			}
		}
	case TypeMap:
		count, err := d.ReadInt32()
		if err != nil {
			return wrap(path, err)
		}
		if int(count) != len(o.Entries) {
			return fmt.Errorf("%s map count: got %d want %d", path, count, len(o.Entries))
		}
		for i, ent := range o.Entries {
			if err := verifyOp(d, ent.K, fmt.Sprintf("%s.key[%d]", path, i)); err != nil {
				return err
			}
			if err := verifyOp(d, ent.V, fmt.Sprintf("%s.val[%d]", path, i)); err != nil {
				return err
			}
		}
	case TypeMessage:
		msg, err := d.ReadMessageBytes()
		if err != nil {
			return wrap(path, err)
		}
		inner := xpb.NewDecoder(msg)
		if err := verifyOps(inner, o.Ops, path+".msg"); err != nil {
			return err
		}
		if !inner.EOF() {
			return fmt.Errorf("%s nested message: trailing %d bytes", path, inner.Remaining())
		}
	default:
		return fmt.Errorf("%s unknown op type %q", path, o.Type)
	}
	return nil
}

func wrap(path string, err error) error {
	return fmt.Errorf("%s: %w", path, err)
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

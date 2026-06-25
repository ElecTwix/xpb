package xpb

// Mutation-kill tests (ticket T-13).
//
// These tests exist specifically to KILL mutants that survived a gremlins
// (github.com/go-gremlins/gremlins) run over this package. Each test pins the
// EXACT boundary / arithmetic that a surviving mutant flipped, so the assertion
// fails the moment the operator is mutated. The broader behavioural tests in
// at_test.go / malformed_test.go / bomb_test.go cover the happy and clearly-bad
// paths; what was missing — and what every survivor exploited — was the precise
// value AT the boundary (off-by-one in a bounds check, or the wrong arithmetic
// operator in a buffer-bound divisor). See docs/MUTATION.md for the score delta
// and the survivor->test mapping. Run `make mutate` to reproduce.

import (
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

// TestKill_WriteCompactLength_Boundary254 pins the compact-length 0xFF marker
// boundary in (*Encoder).writeCompactLength: `length <= CompactLengthThreshold`
// (254) emits a single length byte; 255+ switches to the 0xFF marker + 4-byte
// LE length. Kills the CONDITIONALS_BOUNDARY mutant `<= 254` -> `< 254`, which
// would wrongly take the 5-byte marker path at exactly 254.
func TestKill_WriteCompactLength_Boundary254(t *testing.T) {
	// Exactly 254: must be a single length byte (value 254), no marker.
	enc := NewEncoder(0)
	enc.WriteString(strings.Repeat("a", 254))
	b := enc.Bytes()
	if b[0] != 254 {
		t.Fatalf("WriteString(len 254) first byte = %#x, want 254 (single-byte compact length)", b[0])
	}
	if len(b) != 1+254 {
		t.Fatalf("WriteString(len 254) total = %d, want %d (1 length byte + payload)", len(b), 1+254)
	}

	// Exactly 255: must trip the 0xFF marker + 4-byte LE length.
	enc.Reset()
	enc.WriteBytes(make([]byte, 255))
	b = enc.Bytes()
	if b[0] != 0xFF {
		t.Fatalf("WriteBytes(len 255) first byte = %#x, want 0xFF marker", b[0])
	}
	if got := binary.LittleEndian.Uint32(b[1:5]); got != 255 {
		t.Fatalf("WriteBytes(len 255) marker length = %d, want 255", got)
	}
	if len(b) != 5+255 {
		t.Fatalf("WriteBytes(len 255) total = %d, want %d (marker+4 + payload)", len(b), 5+255)
	}
}

// TestKill_ReadCompactLength_ExactFourByteTail pins the post-marker bounds check
// in (*Decoder).readCompactLength: `d.pos+4 > len(d.buf)`. When the 4-byte LE
// length is the LAST four bytes of the buffer (d.pos+4 == len), the read must
// succeed. Kills the CONDITIONALS_BOUNDARY mutant `>` -> `>=`, which would
// reject a length that fits exactly.
func TestKill_ReadCompactLength_ExactFourByteTail(t *testing.T) {
	// marker + LE32(0): the length field ends exactly at end-of-buffer, and the
	// decoded length (0) needs no payload, so the whole decode must succeed.
	buf := []byte{0xFF, 0x00, 0x00, 0x00, 0x00}
	dec := NewDecoder(buf)
	s, err := dec.ReadString()
	if err != nil {
		t.Fatalf("ReadString on exact 4-byte length tail = %v, want success", err)
	}
	if s != "" {
		t.Fatalf("ReadString on zero-length marker = %q, want empty", s)
	}
	if !dec.EOF() {
		t.Fatalf("decoder not at EOF after consuming exact tail, remaining=%d", dec.Remaining())
	}
}

// TestKill_Skip_ZeroAndExactEnd pins both boundaries in (*Decoder).Skip:
// `n < 0` (Skip(0) is a valid no-op) and `d.pos+n > len(d.buf)` (skipping to the
// exact end succeeds; one past fails). Kills the CONDITIONALS_BOUNDARY mutants
// `n < 0` -> `n <= 0` and `>` -> `>=`.
func TestKill_Skip_ZeroAndExactEnd(t *testing.T) {
	dec := NewDecoder([]byte{1, 2, 3, 4, 5})

	if err := dec.Skip(0); err != nil {
		t.Fatalf("Skip(0) = %v, want nil (no-op skip must be allowed)", err)
	}
	if dec.Remaining() != 5 {
		t.Fatalf("Skip(0) advanced cursor: remaining=%d, want 5", dec.Remaining())
	}

	if err := dec.Skip(5); err != nil {
		t.Fatalf("Skip(5) to exact end = %v, want nil", err)
	}
	if !dec.EOF() || dec.Remaining() != 0 {
		t.Fatalf("after Skip-to-end: EOF=%v remaining=%d, want true/0", dec.EOF(), dec.Remaining())
	}

	if err := dec.Skip(1); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Skip(1) past end = %v, want ErrUnexpectedEOF", err)
	}
}

// TestKill_ReadArrayCount_ZeroMaxAndZeroCount pins the lower boundaries in both
// (*Decoder).ReadArrayCount and ReadArrayCountAt: `maxElements < 0` (a max of 0
// is legal and admits a count of 0) and `n < 0` (a count of exactly 0 is valid).
// Kills the CONDITIONALS_BOUNDARY mutants `< 0` -> `<= 0` at both sites in both
// the stateful and stateless variants.
func TestKill_ReadArrayCount_ZeroMaxAndZeroCount(t *testing.T) {
	zero := []byte{0, 0, 0, 0} // int32 count = 0

	dec := NewDecoder(zero)
	n, err := dec.ReadArrayCount(1, 0)
	if err != nil || n != 0 {
		t.Fatalf("ReadArrayCount(elem=1,max=0) on count 0 = (%d,%v), want (0,nil)", n, err)
	}

	got, p, err := ReadArrayCountAt(zero, 0, 1, 0)
	if err != nil || got != 0 || p != 4 {
		t.Fatalf("ReadArrayCountAt(elem=1,max=0) on count 0 = (%d,%d,%v), want (0,4,nil)", got, p, err)
	}
}

// TestKill_ReadArrayCount_BufferBoundArithmetic pins the buffer-bound divisor in
// both ReadArrayCount (`max := d.Remaining() / elementMinBytes`) and
// ReadArrayCountAt (`max := (len(b) - p) / elementMinBytes`). With 8 bytes left
// after the count and elementMinBytes=4 the true bound is 2: a count of 2 must
// pass and 3 must be rejected. Any mutation of `/` -> `*`, the subtraction
// `len(b)-p` -> `len(b)+p`, or the sign of that subtraction inflates the bound
// and lets count 3 through, so it is caught here. maxElements is large so the
// buffer bound is the binding constraint.
func TestKill_ReadArrayCount_BufferBoundArithmetic(t *testing.T) {
	const maxElems = 1 << 20
	const elemMin = 4

	// 4-byte count + 8 payload bytes = 12 bytes total; 8 remain after the count.
	mk := func(count int32) []byte {
		b := make([]byte, 0, 12)
		b = binary.LittleEndian.AppendUint32(b, uint32(count))
		b = append(b, make([]byte, 8)...)
		return b
	}

	// Stateful: count 2 within bound -> ok; count 3 over bound -> rejected.
	if n, err := NewDecoder(mk(2)).ReadArrayCount(elemMin, maxElems); err != nil || n != 2 {
		t.Fatalf("ReadArrayCount count=2 (bound=2) = (%d,%v), want (2,nil)", n, err)
	}
	if _, err := NewDecoder(mk(3)).ReadArrayCount(elemMin, maxElems); err == nil {
		t.Fatalf("ReadArrayCount count=3 (bound=2) = nil error, want buffer-bound rejection")
	}

	// Stateless *At: identical contract, exercised with the cursor at 0.
	if n, _, err := ReadArrayCountAt(mk(2), 0, elemMin, maxElems); err != nil || n != 2 {
		t.Fatalf("ReadArrayCountAt count=2 (bound=2) = (%d,%v), want (2,nil)", n, err)
	}
	if _, _, err := ReadArrayCountAt(mk(3), 0, elemMin, maxElems); err == nil {
		t.Fatalf("ReadArrayCountAt count=3 (bound=2) = nil error, want buffer-bound rejection")
	}

	// Pin len(b)-p with a non-zero starting cursor: prepend one byte so the count
	// lives at offset 1. After the read the cursor is 5, leaving 8 bytes; bound
	// stays 2. A `-`->`+` mutation would compute (13+5)/4 and admit count 3.
	withPrefix := func(count int32) []byte { return append([]byte{0xAA}, mk(count)...) }
	if n, p, err := ReadArrayCountAt(withPrefix(2), 1, elemMin, maxElems); err != nil || n != 2 || p != 5 {
		t.Fatalf("ReadArrayCountAt p=1 count=2 = (%d,%d,%v), want (2,5,nil)", n, p, err)
	}
	if _, _, err := ReadArrayCountAt(withPrefix(3), 1, elemMin, maxElems); err == nil {
		t.Fatalf("ReadArrayCountAt p=1 count=3 = nil error, want buffer-bound rejection")
	}
}

// TestKill_ReadAt_ExactFitBounds pins the fixed-width bounds checks in the
// stateless cursor readers ReadInt32At/ReadInt64At/ReadUint32At/ReadUint64At:
// `p + width > len(b)`. A buffer holding EXACTLY one value (p+width == len) must
// decode successfully and advance the cursor to the end. Kills the
// CONDITIONALS_BOUNDARY mutants `>` -> `>=`, which would reject an exact fit.
func TestKill_ReadAt_ExactFitBounds(t *testing.T) {
	// int32: exactly 4 bytes. (Convert through a variable: uint32(int32(neg))
	// is a compile error as an untyped constant.)
	{
		var want32 int32 = -12345
		b := binary.LittleEndian.AppendUint32(nil, uint32(want32))
		v, p, err := ReadInt32At(b, 0)
		if err != nil || v != want32 || p != 4 {
			t.Fatalf("ReadInt32At exact fit = (%d,%d,%v), want (%d,4,nil)", v, p, err, want32)
		}
		if _, _, err := ReadInt32At(b[:3], 0); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("ReadInt32At one short = %v, want ErrUnexpectedEOF", err)
		}
	}
	// int64: exactly 8 bytes.
	{
		var want64 int64 = -987654321
		b := binary.LittleEndian.AppendUint64(nil, uint64(want64))
		v, p, err := ReadInt64At(b, 0)
		if err != nil || v != want64 || p != 8 {
			t.Fatalf("ReadInt64At exact fit = (%d,%d,%v), want (%d,8,nil)", v, p, err, want64)
		}
	}
	// uint32: exactly 4 bytes.
	{
		b := binary.LittleEndian.AppendUint32(nil, 0xDEADBEEF)
		v, p, err := ReadUint32At(b, 0)
		if err != nil || v != 0xDEADBEEF || p != 4 {
			t.Fatalf("ReadUint32At exact fit = (%#x,%d,%v), want (0xDEADBEEF,4,nil)", v, p, err)
		}
	}
	// uint64: exactly 8 bytes.
	{
		b := binary.LittleEndian.AppendUint64(nil, 0xCAFEBABEDEADBEEF)
		v, p, err := ReadUint64At(b, 0)
		if err != nil || v != 0xCAFEBABEDEADBEEF || p != 8 {
			t.Fatalf("ReadUint64At exact fit = (%#x,%d,%v), want (0xCAFEBABEDEADBEEF,8,nil)", v, p, err)
		}
	}
	// float32: exactly 4 bytes (3.5 is exactly representable).
	{
		b := AppendFloat32To(nil, 3.5)
		v, p, err := ReadFloat32At(b, 0)
		if err != nil || v != 3.5 || p != 4 {
			t.Fatalf("ReadFloat32At exact fit = (%v,%d,%v), want (3.5,4,nil)", v, p, err)
		}
	}
	// float64: exactly 8 bytes (-2.25 is exactly representable).
	{
		b := AppendFloat64To(nil, -2.25)
		v, p, err := ReadFloat64At(b, 0)
		if err != nil || v != -2.25 || p != 8 {
			t.Fatalf("ReadFloat64At exact fit = (%v,%d,%v), want (-2.25,8,nil)", v, p, err)
		}
	}
}

// TestKill_RunBoolAt_BothValues pins the truth test in RunBoolAt: `b[p] != 0`.
// A non-zero byte decodes true; a zero byte decodes false. Kills the
// CONDITIONALS_NEGATION mutant `!=` -> `==`, which inverts both.
func TestKill_RunBoolAt_BothValues(t *testing.T) {
	if !RunBoolAt([]byte{0x01}, 0) {
		t.Fatal("RunBoolAt(0x01) = false, want true")
	}
	if RunBoolAt([]byte{0x00}, 0) {
		t.Fatal("RunBoolAt(0x00) = true, want false")
	}
	// A non-1 non-zero byte is still true (any non-zero is true).
	if !RunBoolAt([]byte{0x7F}, 0) {
		t.Fatal("RunBoolAt(0x7F) = false, want true")
	}
}

// TestKill_ExtendRun_OffsetAndLength brings the coalesced-run encode helper
// ExtendRun under test (it was previously not covered) and pins its return:
// the slice length grows by exactly n and the returned offset is the old length.
// Kills the ARITHMETIC_BASE mutant on `off+n` (any other operator changes the
// returned length / panics) and proves the pre-run bytes are preserved.
func TestKill_ExtendRun_OffsetAndLength(t *testing.T) {
	base := []byte{1, 2, 3}
	b, off := ExtendRun(base, 5)
	if off != 3 {
		t.Fatalf("ExtendRun offset = %d, want 3 (old length)", off)
	}
	if len(b) != 8 {
		t.Fatalf("ExtendRun new length = %d, want 8 (3 + 5)", len(b))
	}
	// Pre-run bytes preserved; the whole run window is writable.
	for i, want := range []byte{1, 2, 3} {
		if b[i] != want {
			t.Fatalf("ExtendRun clobbered byte %d = %d, want %d", i, b[i], want)
		}
	}
	for i := off; i < off+5; i++ {
		b[i] = 0xEE // must not panic: the whole window is in-bounds
	}
}

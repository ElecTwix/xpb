package xpb

// Security audit — vulnerability validation tests for the XPB Go runtime
// and the patterns the Go codegen emits. Each test name is
// TestSecurity_{ID}_{description}.
//
// Threat model: an attacker controls the bytes passed to a Decoder and the
// element-count fields embedded in those bytes. The decoder must not
// allocate memory proportional to attacker-supplied counts unless those
// counts are first bounded against the available buffer.

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

// SecurityFinding: XPB-001
// Severity: High
// Description: Generated decoder code reads a repeated/map field's length
//   via dec.ReadInt32() and immediately calls make([]Type, count). A signed
//   int32 read from the wire can be negative, and `make([]T, -1)` panics
//   with "makeslice: len out of range" — turning any malicious payload
//   into an unrecovered runtime panic from the decoder goroutine.
//
//   Fix: ReadArrayCount validates the count is non-negative before
//   returning it, so generated code can fail fast with an error instead of
//   panicking.
// Expected: After fix — ReadArrayCount returns a non-nil error containing
//   "negative array count" when the wire bytes encode -1.
func TestSecurity_XPB001_NegativeArrayCountRejected(t *testing.T) {
	var Negative int32 = -1
	var Buf [4]byte
	binary.LittleEndian.PutUint32(Buf[:], uint32(Negative))

	Dec := NewDecoder(Buf[:])
	N, Err := Dec.ReadArrayCount(4)
	if Err == nil {
		t.Fatalf("FIX REGRESSED: negative count accepted (got %d, expected error)", N)
	}
	if !strings.Contains(Err.Error(), "negative array count") {
		t.Fatalf("unexpected error shape: %v", Err)
	}
	t.Logf("FIX VERIFIED XPB-001: negative array count rejected with %q", Err)
}

// SecurityFinding: XPB-002
// Severity: High
// Description: Generated decoder code reads a repeated-field count and
//   passes it directly to make([]T, count). A 4-byte int32 can encode up
//   to 2^31-1 elements; for an int32 element type that's an 8 GB
//   allocation triggered by a 4-byte attacker payload — a single message
//   OOMs the receiving process. The generated code never bounds the count
//   against the bytes actually available in the buffer.
//
//   Fix: ReadArrayCount(elementMinBytes) rejects any count that exceeds
//   `Remaining()/elementMinBytes`, so a count of 2^31-1 in a small buffer
//   is refused before any allocation happens.
// Expected: After fix — a count of 2^31-1 with only a few trailing bytes
//   in the buffer returns an "exceeds buffer-bounded max" error.
func TestSecurity_XPB002_OversizedArrayCountRejected(t *testing.T) {
	const Bogus int32 = 1 << 30 // a bit over 1 billion entries

	var Buf [4]byte
	binary.LittleEndian.PutUint32(Buf[:], uint32(Bogus))

	Dec := NewDecoder(Buf[:])
	N, Err := Dec.ReadArrayCount(4) // claim each element is 4 bytes (int32)
	if Err == nil {
		t.Fatalf("FIX REGRESSED: count %d accepted in %d-byte buffer (would allocate ~4 GB)", N, len(Buf))
	}
	if !strings.Contains(Err.Error(), "exceeds buffer-bounded max") {
		t.Fatalf("unexpected error shape: %v", Err)
	}
	t.Logf("FIX VERIFIED XPB-002: oversized array count rejected with %q", Err)
}

// Regression: legitimate counts that fit in the buffer must still pass.
// elementMinBytes=4 and a 16-element claim with 64+4 bytes available is honest.
func TestSecurity_XPB002_LegitimateCountAccepted(t *testing.T) {
	const N int32 = 16
	Buf := make([]byte, 4+int(N)*4)
	binary.LittleEndian.PutUint32(Buf[:4], uint32(N))

	Dec := NewDecoder(Buf)
	Got, Err := Dec.ReadArrayCount(4)
	if Err != nil {
		t.Fatalf("legitimate count rejected: %v", Err)
	}
	if Got != N {
		t.Fatalf("ReadArrayCount returned %d, want %d", Got, N)
	}
}

// elementMinBytes=0 disables the upper-bound check. Useful when the caller
// knows what it's doing (e.g., decoding a trusted payload). Negative
// counts are still rejected.
func TestSecurity_XPB002_DisabledUpperBound(t *testing.T) {
	const Bogus int32 = 1 << 30
	var Buf [4]byte
	binary.LittleEndian.PutUint32(Buf[:], uint32(Bogus))

	Dec := NewDecoder(Buf[:])
	Got, Err := Dec.ReadArrayCount(0)
	if Err != nil {
		t.Fatalf("elementMinBytes=0 should skip upper-bound check, got error %v", Err)
	}
	if Got != Bogus {
		t.Fatalf("ReadArrayCount returned %d, want %d", Got, Bogus)
	}
}

// SecurityFinding: XPB-003
// Severity: Medium
// Description: Generated Unmarshal previously called itself directly for
//   nested messages with no depth limit. A self-referential message type
//   (`message Node { 1: ?Node child }`) accepts a 16 MB payload of nested
//   1-byte length prefixes — that's ~16 M Unmarshal frames on the stack.
//   Go grows goroutine stacks up to 1 GB before crashing with "stack
//   overflow" (process-wide signal — uteka's recover() can't catch it).
//
//   Fix: the codegen now wraps the public Unmarshal as a thin shim that
//   delegates to unmarshalAt(data, 0). unmarshalAt checks
//   `depth > MaxDecodeDepth` on entry and returns ErrMaxDepthExceeded.
//   Each nested decode passes depth+1.
//
//   This test simulates the generated pattern (the lib doesn't ship a
//   recursive type itself) and asserts an attacker payload that nests
//   deeper than MaxDecodeDepth is rejected before exhausting the stack.
type recNode struct {
	Child *recNode
}

func (m *recNode) Unmarshal(data []byte) error { return m.unmarshalAt(data, 0) }
func (m *recNode) unmarshalAt(data []byte, depth int) error {
	if depth > MaxDecodeDepth {
		return ErrMaxDepthExceeded
	}
	dec := NewDecoder(data)
	if dec.EOF() {
		return nil
	}
	childData, err := dec.ReadMessageBytes()
	if err != nil {
		return err
	}
	if len(childData) == 0 {
		return nil
	}
	m.Child = &recNode{}
	return m.Child.unmarshalAt(childData, depth+1)
}

func encodeRecNode(depth int) []byte {
	enc := NewEncoder(depth + 4)
	// Innermost is empty; wrap depth times.
	inner := []byte{}
	for i := 0; i < depth; i++ {
		enc.Reset()
		enc.WriteMessage(inner)
		inner = append([]byte(nil), enc.Bytes()...)
	}
	return inner
}

func TestSecurity_XPB003_NestedMessageDepthCapped(t *testing.T) {
	Payload := encodeRecNode(MaxDecodeDepth + 5)
	var Root recNode
	Err := Root.Unmarshal(Payload)
	if Err == nil {
		t.Fatal("FIX REGRESSED: payload nested past MaxDecodeDepth was accepted")
	}
	if !errors.Is(Err, ErrMaxDepthExceeded) {
		t.Fatalf("unexpected error: %v (want ErrMaxDepthExceeded)", Err)
	}
	t.Logf("FIX VERIFIED XPB-003: depth cap %d enforced; over-deep payload rejected", MaxDecodeDepth)
}

func TestSecurity_XPB003_LegitimateNestingAccepted(t *testing.T) {
	Payload := encodeRecNode(MaxDecodeDepth)
	var Root recNode
	if Err := Root.Unmarshal(Payload); Err != nil {
		t.Fatalf("legitimate payload at exactly MaxDecodeDepth was rejected: %v", Err)
	}
}

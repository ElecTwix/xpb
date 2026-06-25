package utekabench

import (
	"bytes"
	"testing"

	"github.com/ElecTwix/xpb/benchmarks/go/uteka/ptr"
	"github.com/ElecTwix/xpb/benchmarks/go/uteka/val"
)

// These tests document and ENFORCE the 0.5.0 bytes-aliasing contract, which is
// the INVERSE of the pre-0.5.0 one: zero-copy is now the DEFAULT and copying is
// the opt-out.
//
//   - val style (generated with the DEFAULT flags) decodes bytes via
//     ReadBytesUnsafe, so the decoded Payload slice ALIASES the input buffer.
//     Mutating the input afterwards is visible through the decoded slice. Fast,
//     but the caller must treat the source buffer as immutable while the slice
//     lives.
//   - ptr style (generated with the --go-safe-bytes opt-out — the old default)
//     decodes bytes via ReadBytes, which COPIES. The decoded slice owns its
//     memory; mutating the input is NOT visible.
//
// Both decode the SAME wire bytes; the only difference is the generated decode
// strategy. The test locates the payload bytes in the buffer by scanning for a
// distinctive marker, so it does not hard-code a fragile offset.

// payloadMarker is a distinctive byte run we embed as the message Payload so we
// can find it in the encoded buffer and mutate exactly those bytes.
var payloadMarker = []byte{0xC0, 0xFF, 0xEE, 0xC0, 0xFF, 0xEE, 0xC0, 0xFF, 0xEE, 0x42}

// aliasCase is a small message whose only present optional is Payload, so the
// payload bytes appear verbatim and contiguously in the wire encoding.
func aliasCaseVal() *val.UtekaMessage {
	return &val.UtekaMessage{
		Type:    9,
		Id:      "alias-id",
		Payload: append([]byte(nil), payloadMarker...), HasPayload: true,
		Timestamp: 123,
	}
}

func indexOf(buf, sub []byte) int { return bytes.Index(buf, sub) }

// TestAliasing_DefaultValAliasesInput proves the DEFAULT (val) zero-copy decode
// aliases the input buffer: mutating the source buffer changes the decoded
// Payload.
func TestAliasing_DefaultValAliasesInput(t *testing.T) {
	wire, err := aliasCaseVal().Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Own a mutable copy of the buffer.
	buf := append([]byte(nil), wire...)

	var m val.UtekaMessage
	if err := m.Unmarshal(buf); err != nil {
		t.Fatalf("val unmarshal: %v", err)
	}
	if !m.HasPayload || !bytes.Equal(m.Payload, payloadMarker) {
		t.Fatalf("decode did not populate Payload: has=%v payload=%x", m.HasPayload, m.Payload)
	}

	// Locate and mutate the payload bytes inside the source buffer.
	off := indexOf(buf, payloadMarker)
	if off < 0 {
		t.Fatalf("payload marker not found in buffer % x", buf)
	}
	buf[off] = 0x00

	// Because the default val decode is zero-copy, the mutation must be visible
	// through the decoded slice. If this fails, the default bytes decode stopped
	// aliasing.
	if m.Payload[0] != 0x00 {
		t.Fatalf("zero-copy decode did NOT alias input: Payload[0]=%#x, want 0x00 (mutation must be visible)", m.Payload[0])
	}
}

// TestAliasing_SafeBytesPtrDoesNotAliasInput proves the --go-safe-bytes opt-out
// (ptr) decode does NOT alias: it copies the payload, so mutating the source
// buffer leaves the decoded slice untouched.
func TestAliasing_SafeBytesPtrDoesNotAliasInput(t *testing.T) {
	// Encode via the value style and decode via the pointer style: both share the
	// same wire format, so this isolates the decode strategy difference.
	wire, err := aliasCaseVal().Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	buf := append([]byte(nil), wire...)

	var m ptr.UtekaMessage
	if err := m.Unmarshal(buf); err != nil {
		t.Fatalf("ptr unmarshal: %v", err)
	}
	if m.Payload == nil || !bytes.Equal(*m.Payload, payloadMarker) {
		t.Fatalf("decode did not populate Payload: %v", m.Payload)
	}

	off := indexOf(buf, payloadMarker)
	if off < 0 {
		t.Fatalf("payload marker not found in buffer % x", buf)
	}
	buf[off] = 0x00

	// The safe-bytes opt-out decode copies; the mutation must NOT be visible.
	if (*m.Payload)[0] != payloadMarker[0] {
		t.Fatalf("safe-bytes decode aliased input: Payload[0]=%#x, want %#x (must be an independent copy)", (*m.Payload)[0], payloadMarker[0])
	}
}

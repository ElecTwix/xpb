package utekabench

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/ElecTwix/xpb/benchmarks/go/uteka/ptr"
	"github.com/ElecTwix/xpb/benchmarks/go/uteka/val"
)

// logical is a style-independent description of a UtekaMessage value. It carries
// the required scalars/strings plus an optional value AND a presence flag for
// every optional field, so a single case fully specifies both the pointer-style
// (*T / nil) and value-style (value + Has<Field>) representations. This lets the
// differential tests prove that the two generated styles agree on the wire and
// on the decoded logical value.
type logical struct {
	name string

	Type      int32
	Id        string
	Timestamp int64
	Seq       int64
	Flags     int32

	Method    string
	HasMethod bool

	Payload    []byte
	HasPayload bool

	Error    string
	HasError bool

	StreamId    string
	HasStreamId bool

	SessionId    string
	HasSessionId bool
}

// toPtr builds the pointer-style struct. Present optionals get a fresh pointer;
// absent optionals stay nil.
func (l logical) toPtr() *ptr.UtekaMessage {
	m := &ptr.UtekaMessage{
		Type:      l.Type,
		Id:        l.Id,
		Timestamp: l.Timestamp,
		Seq:       l.Seq,
		Flags:     l.Flags,
	}
	if l.HasMethod {
		s := l.Method
		m.Method = &s
	}
	if l.HasPayload {
		// Copy so the struct never shares the case's backing array.
		b := append([]byte(nil), l.Payload...)
		m.Payload = &b
	}
	if l.HasError {
		s := l.Error
		m.Error = &s
	}
	if l.HasStreamId {
		s := l.StreamId
		m.StreamId = &s
	}
	if l.HasSessionId {
		s := l.SessionId
		m.SessionId = &s
	}
	return m
}

// toVal builds the value-style struct.
func (l logical) toVal() *val.UtekaMessage {
	m := &val.UtekaMessage{
		Type:      l.Type,
		Id:        l.Id,
		Timestamp: l.Timestamp,
		Seq:       l.Seq,
		Flags:     l.Flags,
	}
	if l.HasMethod {
		m.Method, m.HasMethod = l.Method, true
	}
	if l.HasPayload {
		m.Payload, m.HasPayload = append([]byte(nil), l.Payload...), true
	}
	if l.HasError {
		m.Error, m.HasError = l.Error, true
	}
	if l.HasStreamId {
		m.StreamId, m.HasStreamId = l.StreamId, true
	}
	if l.HasSessionId {
		m.SessionId, m.HasSessionId = l.SessionId, true
	}
	return m
}

// bigStr is 300 bytes long, tripping the 0xFF compact-length path (>254) on the
// wire: a 1-byte length cannot represent it, so the encoder emits the 0xFF
// marker plus a 4-byte little-endian length.
var bigStr = strings.Repeat("u", 300)

// bigPayload is a 512-byte byte payload, also past the 254-byte boundary.
var bigPayload = bytes.Repeat([]byte{0xAB}, 512)

// diffCases generalizes well beyond the single TestRoundtrip sample: it exercises
// the realistic RPC message, the all-optionals-absent shape, the >254-byte
// compact-length path for both string and bytes, present-but-empty strings/bytes,
// and all-zero scalars.
func diffCases() []logical {
	return []logical{
		{
			name:      "realistic_rpc",
			Type:      2,
			Id:        msgID,
			Timestamp: timestamp,
			Method:    method, HasMethod: true,
			Payload: []byte(payload), HasPayload: true,
			SessionId: sessionID, HasSessionId: true,
		},
		{
			name:      "all_optionals_absent",
			Type:      7,
			Id:        "id-only",
			Timestamp: 42,
			Seq:       9,
			Flags:     3,
		},
		{
			name:      "all_optionals_present",
			Type:      -1,
			Id:        "everything",
			Timestamp: -99,
			Seq:       1 << 40,
			Flags:     -2147483648,
			Method:    "m", HasMethod: true,
			Payload: []byte{0, 1, 2, 3, 4, 5}, HasPayload: true,
			Error: "boom", HasError: true,
			StreamId: "stream-1", HasStreamId: true,
			SessionId: "sess", HasSessionId: true,
		},
		{
			name:      "large_string_and_payload_0xFF_path",
			Type:      1,
			Id:        bigStr, // required string also exceeds 254
			Timestamp: 1,
			Method:    bigStr, HasMethod: true,
			Payload: bigPayload, HasPayload: true,
			StreamId: bigStr, HasStreamId: true,
		},
		{
			name:      "present_empty_string_and_bytes",
			Type:      0,
			Id:        "", // required, empty
			Timestamp: 0,
			Method:    "", HasMethod: true, // present but empty
			Payload: []byte{}, HasPayload: true, // present but empty
			Error: "", HasError: true,
		},
		{
			name: "all_zero_scalars",
			// Every field at its zero value; Id is the empty string. All
			// optionals absent. This is the degenerate all-zero wire encoding.
		},
		{
			name:      "boundary_254",
			Type:      5,
			Id:        strings.Repeat("x", 254), // exactly the 1-byte max
			Timestamp: 5,
			Method:    strings.Repeat("y", 255), // exactly one past -> 0xFF path
			HasMethod: true,
		},
	}
}

// TestDifferential_WireBytesIdentical proves equal logical values encode to
// byte-identical output across the pointer and value styles.
func TestDifferential_WireBytesIdentical(t *testing.T) {
	for _, c := range diffCases() {
		t.Run(c.name, func(t *testing.T) {
			pb, err := c.toPtr().Marshal()
			if err != nil {
				t.Fatalf("ptr marshal: %v", err)
			}
			vb, err := c.toVal().Marshal()
			if err != nil {
				t.Fatalf("val marshal: %v", err)
			}
			if !bytes.Equal(pb, vb) {
				t.Fatalf("ptr and val wire bytes differ:\n ptr=%x\n val=%x", pb, vb)
			}
		})
	}
}

// TestDifferential_WireLayoutAnchor pins the EXACT leading wire bytes for a
// known case, so the suite anchors the absolute wire layout — not just
// ptr==val cross-style equality (which a shared codegen-template framing bug
// would pass). The "all_optionals_absent" case is fully determined:
//
//	Type=7    -> int32 LE: 07 00 00 00
//	Id="id-only" (7 bytes) -> compact len 07, then "id-only"
//	Method absent          -> presence byte 00
//
// We assert this exact prefix on both styles' output.
func TestDifferential_WireLayoutAnchor(t *testing.T) {
	var c logical
	for _, cc := range diffCases() {
		if cc.name == "all_optionals_absent" {
			c = cc
		}
	}
	if c.name == "" {
		t.Fatal("all_optionals_absent case not found")
	}

	wantPrefix := []byte{
		0x07, 0x00, 0x00, 0x00, // Type=7 int32 LE
		0x07, // Id compact length = 7
		'i', 'd', '-', 'o', 'n', 'l', 'y',
		0x00, // Method absent: presence flag 0x00, no value bytes
	}
	for _, style := range []struct {
		name string
		buf  []byte
	}{
		{"ptr", mustMarshalPtr(t, c.toPtr())},
		{"val", mustMarshalVal(t, c.toVal())},
	} {
		if len(style.buf) < len(wantPrefix) || !bytes.Equal(style.buf[:len(wantPrefix)], wantPrefix) {
			t.Errorf("%s wire layout prefix mismatch:\n got =% x\n want=% x", style.name, style.buf, wantPrefix)
		}
	}
}

func mustMarshalPtr(t *testing.T, m *ptr.UtekaMessage) []byte {
	t.Helper()
	b, err := m.Marshal()
	if err != nil {
		t.Fatalf("ptr marshal: %v", err)
	}
	return b
}

func mustMarshalVal(t *testing.T, m *val.UtekaMessage) []byte {
	t.Helper()
	b, err := m.Marshal()
	if err != nil {
		t.Fatalf("val marshal: %v", err)
	}
	return b
}

// TestDifferential_DecodeAgree proves the same wire bytes decode to
// logically-equal values across the two styles. The wire bytes are produced by
// the value style and fed to both decoders.
func TestDifferential_DecodeAgree(t *testing.T) {
	for _, c := range diffCases() {
		t.Run(c.name, func(t *testing.T) {
			wire, err := c.toVal().Marshal()
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var pd ptr.UtekaMessage
			if err := pd.Unmarshal(append([]byte(nil), wire...)); err != nil {
				t.Fatalf("ptr unmarshal: %v", err)
			}
			var vd val.UtekaMessage
			if err := vd.Unmarshal(append([]byte(nil), wire...)); err != nil {
				t.Fatalf("val unmarshal: %v", err)
			}

			if err := agree(&pd, &vd); err != nil {
				t.Fatalf("decoded values disagree across styles: %v", err)
			}

			// And both must match the original logical case.
			if err := agreeLogical(c, &pd, &vd); err != nil {
				t.Fatalf("decoded values do not match the logical case: %v", err)
			}
		})
	}
}

// agree asserts that a pointer-style and value-style decode carry the same
// presence + values for every field.
func agree(p *ptr.UtekaMessage, v *val.UtekaMessage) error {
	if p.Type != v.Type {
		return fmt.Errorf("Type: ptr=%d val=%d", p.Type, v.Type)
	}
	if p.Id != v.Id {
		return fmt.Errorf("Id: ptr=%q val=%q", p.Id, v.Id)
	}
	if p.Timestamp != v.Timestamp {
		return fmt.Errorf("Timestamp: ptr=%d val=%d", p.Timestamp, v.Timestamp)
	}
	if p.Seq != v.Seq {
		return fmt.Errorf("Seq: ptr=%d val=%d", p.Seq, v.Seq)
	}
	if p.Flags != v.Flags {
		return fmt.Errorf("Flags: ptr=%d val=%d", p.Flags, v.Flags)
	}
	if err := agreeOptStr("Method", p.Method, v.Method, v.HasMethod); err != nil {
		return err
	}
	if err := agreeOptBytes("Payload", p.Payload, v.Payload, v.HasPayload); err != nil {
		return err
	}
	if err := agreeOptStr("Error", p.Error, v.Error, v.HasError); err != nil {
		return err
	}
	if err := agreeOptStr("StreamId", p.StreamId, v.StreamId, v.HasStreamId); err != nil {
		return err
	}
	if err := agreeOptStr("SessionId", p.SessionId, v.SessionId, v.HasSessionId); err != nil {
		return err
	}
	return nil
}

func agreeOptStr(name string, p *string, v string, has bool) error {
	if (p != nil) != has {
		return fmt.Errorf("%s presence: ptr=%v val=%v", name, p != nil, has)
	}
	if has && *p != v {
		return fmt.Errorf("%s value: ptr=%q val=%q", name, *p, v)
	}
	return nil
}

func agreeOptBytes(name string, p *[]byte, v []byte, has bool) error {
	if (p != nil) != has {
		return fmt.Errorf("%s presence: ptr=%v val=%v", name, p != nil, has)
	}
	if has && !bytes.Equal(*p, v) {
		return fmt.Errorf("%s value: ptr=%x val=%x", name, *p, v)
	}
	return nil
}

// agreeLogical asserts both decoded structs match the original logical case for
// EVERY field. This anchors the differential decode to the known source value,
// so a codegen bug that corrupted a field identically in both styles (which
// agree(p, v) alone would miss, since it only compares the two styles to each
// other) is still caught here.
func agreeLogical(c logical, p *ptr.UtekaMessage, v *val.UtekaMessage) error {
	// Required scalars/strings, checked against the case in both styles.
	for _, chk := range []struct {
		name             string
		want, gotP, gotV int64
	}{
		{"Type", int64(c.Type), int64(p.Type), int64(v.Type)},
		{"Timestamp", c.Timestamp, p.Timestamp, v.Timestamp},
		{"Seq", c.Seq, p.Seq, v.Seq},
		{"Flags", int64(c.Flags), int64(p.Flags), int64(v.Flags)},
	} {
		if chk.gotP != chk.want || chk.gotV != chk.want {
			return fmt.Errorf("%s want %d ptr=%d val=%d", chk.name, chk.want, chk.gotP, chk.gotV)
		}
	}
	if p.Id != c.Id || v.Id != c.Id {
		return fmt.Errorf("Id want %q ptr=%q val=%q", c.Id, p.Id, v.Id)
	}

	// Optional strings: presence + value against the case, both styles.
	for _, chk := range []struct {
		name string
		has  bool
		val  string
		gotP *string
		hasV bool
		gotV string
	}{
		{"Method", c.HasMethod, c.Method, p.Method, v.HasMethod, v.Method},
		{"Error", c.HasError, c.Error, p.Error, v.HasError, v.Error},
		{"StreamId", c.HasStreamId, c.StreamId, p.StreamId, v.HasStreamId, v.StreamId},
		{"SessionId", c.HasSessionId, c.SessionId, p.SessionId, v.HasSessionId, v.SessionId},
	} {
		if (chk.gotP != nil) != chk.has || chk.hasV != chk.has {
			return fmt.Errorf("%s presence want %v ptr=%v val=%v", chk.name, chk.has, chk.gotP != nil, chk.hasV)
		}
		if chk.has {
			if *chk.gotP != chk.val || chk.gotV != chk.val {
				return fmt.Errorf("%s value want %q ptr=%q val=%q", chk.name, chk.val, *chk.gotP, chk.gotV)
			}
		}
	}

	// Optional bytes (Payload): presence + value against the case, both styles.
	if (p.Payload != nil) != c.HasPayload || v.HasPayload != c.HasPayload {
		return fmt.Errorf("Payload presence want %v ptr=%v val=%v", c.HasPayload, p.Payload != nil, v.HasPayload)
	}
	if c.HasPayload {
		if !bytes.Equal(*p.Payload, c.Payload) || !bytes.Equal(v.Payload, c.Payload) {
			return fmt.Errorf("Payload value mismatch against case")
		}
	}
	return nil
}

// Package utekabench compares XPB codegen styles against JSON and msgpack for a
// realistic control-plane RPC message (UTEKA_MESSAGE). Two XPB variants are
// generated from identical schemas:
//
//	val/ — DEFAULT (0.5.0): value + Has<Field> bool optionals, bytes decoded by
//	       aliasing the input buffer (zero-copy)
//	ptr/ — opt-out (--go-optional-style=pointer --go-safe-bytes; the old default):
//	       optional fields are *T (one heap box per present field on decode),
//	       bytes copied on decode
//
// Run: go test ./benchmarks/go/uteka/ -bench=. -benchmem -run=Roundtrip
package utekabench

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/ElecTwix/xpb/benchmarks/go/uteka/ptr"
	"github.com/ElecTwix/xpb/benchmarks/go/uteka/val"
	"github.com/ElecTwix/xpb/runtime/go/xpb"
	"github.com/vmihailenco/msgpack/v5"
)

// UTEKA_MESSAGE is the original application struct (JSON/msgpack reference).
type UTEKA_MESSAGE struct {
	Type      int             `json:"Type" msgpack:"Type"`
	Id        string          `json:"Id" msgpack:"Id"`
	Method    string          `json:"Method,omitempty" msgpack:"Method,omitempty"`
	Payload   json.RawMessage `json:"Payload,omitempty" msgpack:"Payload,omitempty"`
	Timestamp int64           `json:"Timestamp" msgpack:"Timestamp"`
	Error     string          `json:"Error,omitempty" msgpack:"Error,omitempty"`
	StreamId  string          `json:"StreamId,omitempty" msgpack:"StreamId,omitempty"`
	Seq       int64           `json:"Seq,omitempty" msgpack:"Seq,omitempty"`
	Flags     int             `json:"Flags,omitempty" msgpack:"Flags,omitempty"`
	SessionId string          `json:"SessionId,omitempty" msgpack:"SessionId,omitempty"`
}

// Realistic RPC request: Type, Id, Method, Payload, Timestamp, SessionId set;
// Error/StreamId/Seq/Flags absent.
var (
	payload   = json.RawMessage(`{"user":"alice","action":"subscribe","topic":"prices","limit":100}`)
	method    = "market.subscribe"
	sessionID = "sess_7f3a9c2e1b4d6857"
	msgID     = "01HQ9Z3K7M2X8N4P6R0T"
	timestamp = int64(1735128000000)
)

func sampleOrig() UTEKA_MESSAGE {
	return UTEKA_MESSAGE{
		Type: 2, Id: msgID, Method: method, Payload: payload,
		Timestamp: timestamp, SessionId: sessionID,
	}
}

func samplePtr() *ptr.UtekaMessage {
	m, s := method, sessionID
	p := []byte(payload)
	return &ptr.UtekaMessage{
		Type: 2, Id: msgID, Method: &m, Payload: &p,
		Timestamp: timestamp, SessionId: &s,
	}
}

func sampleVal() *val.UtekaMessage {
	return &val.UtekaMessage{
		Type: 2, Id: msgID, Method: method, HasMethod: true,
		Payload: []byte(payload), HasPayload: true,
		Timestamp: timestamp, SessionId: sessionID, HasSessionId: true,
	}
}

// ---------- correctness ----------

func TestRoundtrip(t *testing.T) {
	// ptr style
	pb, err := samplePtr().Marshal()
	if err != nil {
		t.Fatalf("ptr marshal: %v", err)
	}
	var pd ptr.UtekaMessage
	if err := pd.Unmarshal(pb); err != nil {
		t.Fatalf("ptr unmarshal: %v", err)
	}
	if pd.Method == nil || *pd.Method != method {
		t.Errorf("ptr Method = %v, want %q", pd.Method, method)
	}
	if pd.Payload == nil || !bytes.Equal(*pd.Payload, payload) {
		t.Errorf("ptr Payload mismatch")
	}
	if pd.Error != nil {
		t.Errorf("ptr Error should be absent (nil), got %v", *pd.Error)
	}

	// val style (value optionals + zero-copy bytes)
	vb, err := sampleVal().Marshal()
	if err != nil {
		t.Fatalf("val marshal: %v", err)
	}
	var vd val.UtekaMessage
	if err := vd.Unmarshal(vb); err != nil {
		t.Fatalf("val unmarshal: %v", err)
	}
	if !vd.HasMethod || vd.Method != method {
		t.Errorf("val Method = %q has=%v, want %q", vd.Method, vd.HasMethod, method)
	}
	if !vd.HasPayload || !bytes.Equal(vd.Payload, payload) {
		t.Errorf("val Payload mismatch")
	}
	if vd.HasError {
		t.Errorf("val Error should be absent (HasError=false)")
	}

	// both styles must produce identical wire bytes
	if !bytes.Equal(pb, vb) {
		t.Errorf("ptr and val wire bytes differ:\n ptr=%x\n val=%x", pb, vb)
	}
}

func TestSizes(t *testing.T) {
	o := sampleOrig()
	jb, _ := json.Marshal(o)
	mb, _ := msgpack.Marshal(o)
	xb, _ := samplePtr().Marshal()
	t.Logf("wire bytes: json=%d msgpack=%d xpb=%d", len(jb), len(mb), len(xb))
}

// ---------- encode ----------

func BenchmarkEncode_JSON(b *testing.B) {
	o := sampleOrig()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(o)
	}
}

func BenchmarkEncode_Msgpack(b *testing.B) {
	o := sampleOrig()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = msgpack.Marshal(o)
	}
}

func BenchmarkEncode_XPB_Ptr(b *testing.B) {
	m := samplePtr()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = m.Marshal()
	}
}

func BenchmarkEncode_XPB_Val_Pooled(b *testing.B) {
	m := sampleVal()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		enc := xpb.GetEncoder()
		m.MarshalTo(enc)
		_ = enc.Bytes()
		xpb.PutEncoder(enc)
	}
}

// ---------- decode ----------

func BenchmarkDecode_JSON(b *testing.B) {
	data, _ := json.Marshal(sampleOrig())
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var o UTEKA_MESSAGE
		_ = json.Unmarshal(data, &o)
	}
}

func BenchmarkDecode_Msgpack(b *testing.B) {
	data, _ := msgpack.Marshal(sampleOrig())
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var o UTEKA_MESSAGE
		_ = msgpack.Unmarshal(data, &o)
	}
}

func BenchmarkDecode_XPB_Ptr(b *testing.B) {
	data, _ := samplePtr().Marshal()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var m ptr.UtekaMessage
		_ = m.Unmarshal(data)
	}
}

func BenchmarkDecode_XPB_Val(b *testing.B) {
	data, _ := sampleVal().Marshal()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var m val.UtekaMessage
		_ = m.Unmarshal(data)
	}
}

// Package benchmark compares XPB V2 to other serialization formats.
package benchmark

import (
	"encoding/json"
	"testing"

	pb "github.com/anthropic/xpb/benchmarks/go/proto"
	"github.com/anthropic/xpb/runtime/go/xpb"
	"github.com/vmihailenco/msgpack/v5"
	"google.golang.org/protobuf/proto"
)

// Common test data structure
type BenchUser struct {
	Name   string `json:"name" msgpack:"name"`
	Age    int32  `json:"age" msgpack:"age"`
	Active bool   `json:"active" msgpack:"active"`
}

// ============= XPB V2 Benchmarks =============

func BenchmarkXPB_Encode_Simple(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		enc := xpb.NewEncoder(64)
		enc.WriteString("Alice Johnson")
		enc.WriteInt32(30)
		enc.WriteBool(true)
		_ = enc.Bytes()
	}
}

func BenchmarkXPB_Decode_Simple(b *testing.B) {
	enc := xpb.NewEncoder(64)
	enc.WriteString("Alice Johnson")
	enc.WriteInt32(30)
	enc.WriteBool(true)
	data := enc.Bytes()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dec := xpb.NewDecoder(data)
		name, _ := dec.ReadString()
		age, _ := dec.ReadInt32()
		active, _ := dec.ReadBool()
		_, _, _ = name, age, active
	}
}

// ============= Protobuf Benchmarks =============

func BenchmarkProtobuf_Encode_Simple(b *testing.B) {
	user := &pb.BenchUser{Name: "Alice Johnson", Age: 30, Active: true}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = proto.Marshal(user)
	}
}

func BenchmarkProtobuf_Decode_Simple(b *testing.B) {
	user := &pb.BenchUser{Name: "Alice Johnson", Age: 30, Active: true}
	data, _ := proto.Marshal(user)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		u := &pb.BenchUser{}
		_ = proto.Unmarshal(data, u)
	}
}

// ============= JSON Benchmarks =============

func BenchmarkJSON_Encode_Simple(b *testing.B) {
	user := BenchUser{Name: "Alice Johnson", Age: 30, Active: true}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(&user)
	}
}

func BenchmarkJSON_Decode_Simple(b *testing.B) {
	user := BenchUser{Name: "Alice Johnson", Age: 30, Active: true}
	data, _ := json.Marshal(&user)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var u BenchUser
		_ = json.Unmarshal(data, &u)
	}
}

// ============= MessagePack Benchmarks =============

func BenchmarkMsgpack_Encode_Simple(b *testing.B) {
	user := BenchUser{Name: "Alice Johnson", Age: 30, Active: true}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = msgpack.Marshal(&user)
	}
}

func BenchmarkMsgpack_Decode_Simple(b *testing.B) {
	user := BenchUser{Name: "Alice Johnson", Age: 30, Active: true}
	data, _ := msgpack.Marshal(&user)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var u BenchUser
		_ = msgpack.Unmarshal(data, &u)
	}
}

// ============= Size Comparison Test =============

func TestEncodedSizes(t *testing.T) {
	// XPB V2 (no tags, fixed-width ints)
	enc := xpb.NewEncoder(64)
	enc.WriteString("Alice Johnson")
	enc.WriteInt32(30)
	enc.WriteBool(true)
	xpbData := enc.Bytes()

	// Protobuf
	protoUser := &pb.BenchUser{Name: "Alice Johnson", Age: 30, Active: true}
	protoData, _ := proto.Marshal(protoUser)

	// JSON
	user := BenchUser{Name: "Alice Johnson", Age: 30, Active: true}
	jsonData, _ := json.Marshal(&user)

	// Msgpack
	msgpackData, _ := msgpack.Marshal(&user)

	t.Logf("=== Encoded Sizes (Simple Message) ===")
	t.Logf("XPB V2:   %d bytes (tagless, fixed-width)", len(xpbData))
	t.Logf("Protobuf: %d bytes", len(protoData))
	t.Logf("JSON:     %d bytes", len(jsonData))
	t.Logf("Msgpack:  %d bytes", len(msgpackData))
	t.Logf("")
	t.Logf("XPB vs JSON:     %.2fx smaller", float64(len(jsonData))/float64(len(xpbData)))
	t.Logf("XPB vs Msgpack:  %.2fx smaller", float64(len(msgpackData))/float64(len(xpbData)))
}

// ============= Large Message Benchmarks =============

type LargeBenchUser struct {
	ID          uint64  `json:"id" msgpack:"id"`
	Name        string  `json:"name" msgpack:"name"`
	Email       string  `json:"email" msgpack:"email"`
	Age         int32   `json:"age" msgpack:"age"`
	Score       float64 `json:"score" msgpack:"score"`
	Active      bool    `json:"active" msgpack:"active"`
	Description string  `json:"description" msgpack:"description"`
}

func BenchmarkXPB_Encode_Large(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		enc := xpb.NewEncoder(256)
		enc.WriteUint64(12345678901234)
		enc.WriteString("Alice Johnson")
		enc.WriteString("alice.johnson@example.com")
		enc.WriteInt32(30)
		enc.WriteFloat64(95.5)
		enc.WriteBool(true)
		enc.WriteString("This is a longer description field that contains more text.")
		_ = enc.Bytes()
	}
}

func BenchmarkProtobuf_Encode_Large(b *testing.B) {
	user := &pb.LargeBenchUser{
		Id:          12345678901234,
		Name:        "Alice Johnson",
		Email:       "alice.johnson@example.com",
		Age:         30,
		Score:       95.5,
		Active:      true,
		Description: "This is a longer description field that contains more text.",
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = proto.Marshal(user)
	}
}

func BenchmarkJSON_Encode_Large(b *testing.B) {
	user := LargeBenchUser{
		ID:          12345678901234,
		Name:        "Alice Johnson",
		Email:       "alice.johnson@example.com",
		Age:         30,
		Score:       95.5,
		Active:      true,
		Description: "This is a longer description field that contains more text.",
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(&user)
	}
}

func BenchmarkMsgpack_Encode_Large(b *testing.B) {
	user := LargeBenchUser{
		ID:          12345678901234,
		Name:        "Alice Johnson",
		Email:       "alice.johnson@example.com",
		Age:         30,
		Score:       95.5,
		Active:      true,
		Description: "This is a longer description field that contains more text.",
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = msgpack.Marshal(&user)
	}
}

func TestLargeEncodedSizes(t *testing.T) {
	// XPB V2
	enc := xpb.NewEncoder(256)
	enc.WriteUint64(12345678901234)
	enc.WriteString("Alice Johnson")
	enc.WriteString("alice.johnson@example.com")
	enc.WriteInt32(30)
	enc.WriteFloat64(95.5)
	enc.WriteBool(true)
	enc.WriteString("This is a longer description field that contains more text.")
	xpbData := enc.Bytes()

	// Protobuf
	protoUser := &pb.LargeBenchUser{
		Id:          12345678901234,
		Name:        "Alice Johnson",
		Email:       "alice.johnson@example.com",
		Age:         30,
		Score:       95.5,
		Active:      true,
		Description: "This is a longer description field that contains more text.",
	}
	protoData, _ := proto.Marshal(protoUser)

	// JSON
	user := LargeBenchUser{
		ID:          12345678901234,
		Name:        "Alice Johnson",
		Email:       "alice.johnson@example.com",
		Age:         30,
		Score:       95.5,
		Active:      true,
		Description: "This is a longer description field that contains more text.",
	}
	jsonData, _ := json.Marshal(&user)

	// Msgpack
	msgpackData, _ := msgpack.Marshal(&user)

	t.Logf("=== Encoded Sizes (Large Message) ===")
	t.Logf("XPB V2:   %d bytes (tagless, fixed-width)", len(xpbData))
	t.Logf("Protobuf: %d bytes", len(protoData))
	t.Logf("JSON:     %d bytes", len(jsonData))
	t.Logf("Msgpack:  %d bytes", len(msgpackData))
}

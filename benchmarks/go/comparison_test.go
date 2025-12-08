// Package benchmark compares XPB to other serialization formats.
package benchmark

import (
	"encoding/json"
	"testing"

	"github.com/anthropic/xpb/pkg/wire"
	"github.com/anthropic/xpb/runtime/go/xpb"
	"github.com/vmihailenco/msgpack/v5"
	"google.golang.org/protobuf/proto"
)

var _ = wire.WireVarint

// Common test data structure
type BenchUser struct {
	Name   string `json:"name" msgpack:"name"`
	Age    int32  `json:"age" msgpack:"age"`
	Active bool   `json:"active" msgpack:"active"`
}

// For size comparison output
var sizeResults = make(map[string]int)

// ============= XPB Benchmarks =============

func BenchmarkXPB_Encode_Simple(b *testing.B) {
	user := BenchUser{Name: "Alice Johnson", Age: 30, Active: true}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		enc := xpb.NewEncoder(64)
		enc.WriteString(1, user.Name)
		enc.WriteInt32(2, user.Age)
		enc.WriteBool(3, user.Active)
		_ = enc.Bytes()
	}
}

func BenchmarkXPB_Decode_Simple(b *testing.B) {
	enc := xpb.NewEncoder(64)
	enc.WriteString(1, "Alice Johnson")
	enc.WriteInt32(2, 30)
	enc.WriteBool(3, true)
	data := enc.Bytes()
	sizeResults["XPB"] = len(data)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dec := xpb.NewDecoder(data)
		var user BenchUser
		for !dec.EOF() {
			fn, wt, _ := dec.ReadTag()
			switch fn {
			case 1:
				user.Name, _ = dec.ReadString()
			case 2:
				user.Age, _ = dec.ReadInt32()
			case 3:
				user.Active, _ = dec.ReadBool()
			default:
				dec.Skip(wt)
			}
		}
		_ = user
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
	sizeResults["JSON"] = len(data)

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
	sizeResults["Msgpack"] = len(data)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var u BenchUser
		_ = msgpack.Unmarshal(data, &u)
	}
}

// ============= Size Comparison Test =============

func TestEncodedSizes(t *testing.T) {
	user := BenchUser{Name: "Alice Johnson", Age: 30, Active: true}

	// XPB
	enc := xpb.NewEncoder(64)
	enc.WriteString(1, user.Name)
	enc.WriteInt32(2, user.Age)
	enc.WriteBool(3, user.Active)
	xpbData := enc.Bytes()

	// JSON
	jsonData, _ := json.Marshal(&user)

	// Msgpack
	msgpackData, _ := msgpack.Marshal(&user)

	t.Logf("=== Encoded Sizes ===")
	t.Logf("XPB:      %d bytes", len(xpbData))
	t.Logf("JSON:     %d bytes", len(jsonData))
	t.Logf("Msgpack:  %d bytes", len(msgpackData))
	t.Logf("")
	t.Logf("XPB is %.1fx smaller than JSON", float64(len(jsonData))/float64(len(xpbData)))
	t.Logf("XPB is %.1fx smaller than Msgpack", float64(len(msgpackData))/float64(len(xpbData)))
}

// ============= Larger Message Benchmarks =============

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
		enc := xpb.NewEncoder(256)
		enc.WriteUint64(1, user.ID)
		enc.WriteString(2, user.Name)
		enc.WriteString(3, user.Email)
		enc.WriteInt32(4, user.Age)
		enc.WriteFloat64(5, user.Score)
		enc.WriteBool(6, user.Active)
		enc.WriteString(7, user.Description)
		_ = enc.Bytes()
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
	user := LargeBenchUser{
		ID:          12345678901234,
		Name:        "Alice Johnson",
		Email:       "alice.johnson@example.com",
		Age:         30,
		Score:       95.5,
		Active:      true,
		Description: "This is a longer description field that contains more text.",
	}

	// XPB
	enc := xpb.NewEncoder(256)
	enc.WriteUint64(1, user.ID)
	enc.WriteString(2, user.Name)
	enc.WriteString(3, user.Email)
	enc.WriteInt32(4, user.Age)
	enc.WriteFloat64(5, user.Score)
	enc.WriteBool(6, user.Active)
	enc.WriteString(7, user.Description)
	xpbData := enc.Bytes()

	// JSON
	jsonData, _ := json.Marshal(&user)

	// Msgpack
	msgpackData, _ := msgpack.Marshal(&user)

	t.Logf("=== Large Message Encoded Sizes ===")
	t.Logf("XPB:      %d bytes", len(xpbData))
	t.Logf("JSON:     %d bytes", len(jsonData))
	t.Logf("Msgpack:  %d bytes", len(msgpackData))
	t.Logf("")
	t.Logf("XPB is %.1fx smaller than JSON", float64(len(jsonData))/float64(len(xpbData)))
	t.Logf("XPB is %.1fx smaller than Msgpack", float64(len(msgpackData))/float64(len(xpbData)))
}

// Suppress protobuf import for now (requires proto file compilation)
var _ = proto.Marshal

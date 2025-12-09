// Package benchmark provides size scaling benchmarks for XPB V2.
// Tests performance across different payload sizes: Tiny, Small, Medium, Large, XLarge.
package benchmark

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/anthropic/xpb/runtime/go/xpb"
	"github.com/vmihailenco/msgpack/v5"
)

// ============= Size Scaling Test Data =============

// TinyMessage ~10 bytes - heartbeats, acks
type TinyMessage struct {
	OK bool `json:"ok" msgpack:"ok"`
}

// SmallMessage ~50 bytes - events, metrics
type SmallMessage struct {
	ID    int32  `json:"id" msgpack:"id"`
	Value int32  `json:"value" msgpack:"value"`
	Type  string `json:"type" msgpack:"type"`
}

// MediumMessage ~500 bytes - API responses
type MediumMessage struct {
	ID          uint64  `json:"id" msgpack:"id"`
	Name        string  `json:"name" msgpack:"name"`
	Email       string  `json:"email" msgpack:"email"`
	Description string  `json:"description" msgpack:"description"`
	Score       float64 `json:"score" msgpack:"score"`
	Active      bool    `json:"active" msgpack:"active"`
	Tags        string  `json:"tags" msgpack:"tags"`
	Metadata    string  `json:"metadata" msgpack:"metadata"`
}

// LargeMessage ~10KB - documents
type LargeMessage struct {
	ID          uint64   `json:"id" msgpack:"id"`
	Title       string   `json:"title" msgpack:"title"`
	Content     string   `json:"content" msgpack:"content"`
	Author      string   `json:"author" msgpack:"author"`
	Tags        []string `json:"tags" msgpack:"tags"`
	Metadata    string   `json:"metadata" msgpack:"metadata"`
	Description string   `json:"description" msgpack:"description"`
}

// XLargeMessage ~100KB - batch data
type XLargeMessage struct {
	ID      uint64   `json:"id" msgpack:"id"`
	Payload string   `json:"payload" msgpack:"payload"`
	Items   []string `json:"items" msgpack:"items"`
}

// ============= Test Data Generators =============

func makeTinyMessage() TinyMessage {
	return TinyMessage{OK: true}
}

func makeSmallMessage() SmallMessage {
	return SmallMessage{
		ID:    12345,
		Value: 98765,
		Type:  "metric_event",
	}
}

func makeMediumMessage() MediumMessage {
	return MediumMessage{
		ID:          12345678901234,
		Name:        "Alice Johnson",
		Email:       "alice.johnson@example.com",
		Description: strings.Repeat("This is a medium description. ", 10),
		Score:       95.5,
		Active:      true,
		Tags:        "tag1,tag2,tag3,tag4,tag5,tag6,tag7,tag8",
		Metadata:    `{"key1":"value1","key2":"value2","key3":"value3"}`,
	}
}

func makeLargeMessage() LargeMessage {
	tags := make([]string, 50)
	for i := 0; i < 50; i++ {
		tags[i] = fmt.Sprintf("tag_%d", i)
	}
	return LargeMessage{
		ID:          12345678901234,
		Title:       "Large Document Title for Testing Performance",
		Content:     strings.Repeat("This is the content of a large document. ", 200),
		Author:      "Alice Johnson <alice.johnson@example.com>",
		Tags:        tags,
		Metadata:    strings.Repeat(`{"key":"value"},`, 50),
		Description: strings.Repeat("Long description text. ", 50),
	}
}

func makeXLargeMessage() XLargeMessage {
	items := make([]string, 500)
	for i := 0; i < 500; i++ {
		items[i] = fmt.Sprintf("item_%d_with_some_additional_data_to_make_it_larger", i)
	}
	return XLargeMessage{
		ID:      12345678901234,
		Payload: strings.Repeat("Large payload data block. ", 1000),
		Items:   items,
	}
}

// ============= XPB Encode/Decode Functions =============

func encodeTinyXPB(m TinyMessage) []byte {
	enc := xpb.NewEncoder(16)
	enc.WriteBool(m.OK)
	return enc.Bytes()
}

func decodeTinyXPB(data []byte) TinyMessage {
	dec := xpb.NewDecoder(data)
	ok, _ := dec.ReadBool()
	return TinyMessage{OK: ok}
}

func encodeSmallXPB(m SmallMessage) []byte {
	enc := xpb.NewEncoder(64)
	enc.WriteInt32(m.ID)
	enc.WriteInt32(m.Value)
	enc.WriteString(m.Type)
	return enc.Bytes()
}

func decodeSmallXPB(data []byte) SmallMessage {
	dec := xpb.NewDecoder(data)
	id, _ := dec.ReadInt32()
	value, _ := dec.ReadInt32()
	typ, _ := dec.ReadString()
	return SmallMessage{ID: id, Value: value, Type: typ}
}

func encodeMediumXPB(m MediumMessage) []byte {
	enc := xpb.NewEncoder(1024)
	enc.WriteUint64(m.ID)
	enc.WriteString(m.Name)
	enc.WriteString(m.Email)
	enc.WriteString(m.Description)
	enc.WriteFloat64(m.Score)
	enc.WriteBool(m.Active)
	enc.WriteString(m.Tags)
	enc.WriteString(m.Metadata)
	return enc.Bytes()
}

func decodeMediumXPB(data []byte) MediumMessage {
	dec := xpb.NewDecoder(data)
	id, _ := dec.ReadUint64()
	name, _ := dec.ReadString()
	email, _ := dec.ReadString()
	desc, _ := dec.ReadString()
	score, _ := dec.ReadFloat64()
	active, _ := dec.ReadBool()
	tags, _ := dec.ReadString()
	meta, _ := dec.ReadString()
	return MediumMessage{
		ID: id, Name: name, Email: email, Description: desc,
		Score: score, Active: active, Tags: tags, Metadata: meta,
	}
}

func encodeLargeXPB(m LargeMessage) []byte {
	enc := xpb.NewEncoder(16384)
	enc.WriteUint64(m.ID)
	enc.WriteString(m.Title)
	enc.WriteString(m.Content)
	enc.WriteString(m.Author)
	enc.WriteInt32(int32(len(m.Tags)))
	for _, t := range m.Tags {
		enc.WriteString(t)
	}
	enc.WriteString(m.Metadata)
	enc.WriteString(m.Description)
	return enc.Bytes()
}

func decodeLargeXPB(data []byte) LargeMessage {
	dec := xpb.NewDecoder(data)
	id, _ := dec.ReadUint64()
	title, _ := dec.ReadString()
	content, _ := dec.ReadString()
	author, _ := dec.ReadString()
	tagCount, _ := dec.ReadInt32()
	tags := make([]string, tagCount)
	for i := int32(0); i < tagCount; i++ {
		tags[i], _ = dec.ReadString()
	}
	meta, _ := dec.ReadString()
	desc, _ := dec.ReadString()
	return LargeMessage{
		ID: id, Title: title, Content: content, Author: author,
		Tags: tags, Metadata: meta, Description: desc,
	}
}

func encodeXLargeXPB(m XLargeMessage) []byte {
	enc := xpb.NewEncoder(131072)
	enc.WriteUint64(m.ID)
	enc.WriteString(m.Payload)
	enc.WriteInt32(int32(len(m.Items)))
	for _, item := range m.Items {
		enc.WriteString(item)
	}
	return enc.Bytes()
}

func decodeXLargeXPB(data []byte) XLargeMessage {
	dec := xpb.NewDecoder(data)
	id, _ := dec.ReadUint64()
	payload, _ := dec.ReadString()
	itemCount, _ := dec.ReadInt32()
	items := make([]string, itemCount)
	for i := int32(0); i < itemCount; i++ {
		items[i], _ = dec.ReadString()
	}
	return XLargeMessage{ID: id, Payload: payload, Items: items}
}

// ============= Tiny (~10 bytes) Benchmarks =============

func BenchmarkXPB_Encode_Tiny(b *testing.B) {
	m := makeTinyMessage()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encodeTinyXPB(m)
	}
}

func BenchmarkXPB_Decode_Tiny(b *testing.B) {
	m := makeTinyMessage()
	data := encodeTinyXPB(m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = decodeTinyXPB(data)
	}
}

func BenchmarkJSON_Encode_Tiny(b *testing.B) {
	m := makeTinyMessage()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(&m)
	}
}

func BenchmarkJSON_Decode_Tiny(b *testing.B) {
	m := makeTinyMessage()
	data, _ := json.Marshal(&m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out TinyMessage
		_ = json.Unmarshal(data, &out)
	}
}

func BenchmarkMsgpack_Encode_Tiny(b *testing.B) {
	m := makeTinyMessage()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = msgpack.Marshal(&m)
	}
}

func BenchmarkMsgpack_Decode_Tiny(b *testing.B) {
	m := makeTinyMessage()
	data, _ := msgpack.Marshal(&m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out TinyMessage
		_ = msgpack.Unmarshal(data, &out)
	}
}

// ============= Small (~50 bytes) Benchmarks =============

func BenchmarkXPB_Encode_Small(b *testing.B) {
	m := makeSmallMessage()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encodeSmallXPB(m)
	}
}

func BenchmarkXPB_Decode_Small(b *testing.B) {
	m := makeSmallMessage()
	data := encodeSmallXPB(m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = decodeSmallXPB(data)
	}
}

func BenchmarkJSON_Encode_Small(b *testing.B) {
	m := makeSmallMessage()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(&m)
	}
}

func BenchmarkJSON_Decode_Small(b *testing.B) {
	m := makeSmallMessage()
	data, _ := json.Marshal(&m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out SmallMessage
		_ = json.Unmarshal(data, &out)
	}
}

func BenchmarkMsgpack_Encode_Small(b *testing.B) {
	m := makeSmallMessage()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = msgpack.Marshal(&m)
	}
}

func BenchmarkMsgpack_Decode_Small(b *testing.B) {
	m := makeSmallMessage()
	data, _ := msgpack.Marshal(&m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out SmallMessage
		_ = msgpack.Unmarshal(data, &out)
	}
}

// ============= Medium (~500 bytes) Benchmarks =============

func BenchmarkXPB_Encode_Medium(b *testing.B) {
	m := makeMediumMessage()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encodeMediumXPB(m)
	}
}

func BenchmarkXPB_Decode_Medium(b *testing.B) {
	m := makeMediumMessage()
	data := encodeMediumXPB(m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = decodeMediumXPB(data)
	}
}

func BenchmarkJSON_Encode_Medium(b *testing.B) {
	m := makeMediumMessage()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(&m)
	}
}

func BenchmarkJSON_Decode_Medium(b *testing.B) {
	m := makeMediumMessage()
	data, _ := json.Marshal(&m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out MediumMessage
		_ = json.Unmarshal(data, &out)
	}
}

func BenchmarkMsgpack_Encode_Medium(b *testing.B) {
	m := makeMediumMessage()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = msgpack.Marshal(&m)
	}
}

func BenchmarkMsgpack_Decode_Medium(b *testing.B) {
	m := makeMediumMessage()
	data, _ := msgpack.Marshal(&m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out MediumMessage
		_ = msgpack.Unmarshal(data, &out)
	}
}

// ============= Large (~10KB) Benchmarks =============

func BenchmarkXPB_Encode_LargeDoc(b *testing.B) {
	m := makeLargeMessage()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encodeLargeXPB(m)
	}
}

func BenchmarkXPB_Decode_LargeDoc(b *testing.B) {
	m := makeLargeMessage()
	data := encodeLargeXPB(m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = decodeLargeXPB(data)
	}
}

func BenchmarkJSON_Encode_LargeDoc(b *testing.B) {
	m := makeLargeMessage()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(&m)
	}
}

func BenchmarkJSON_Decode_LargeDoc(b *testing.B) {
	m := makeLargeMessage()
	data, _ := json.Marshal(&m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out LargeMessage
		_ = json.Unmarshal(data, &out)
	}
}

func BenchmarkMsgpack_Encode_LargeDoc(b *testing.B) {
	m := makeLargeMessage()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = msgpack.Marshal(&m)
	}
}

func BenchmarkMsgpack_Decode_LargeDoc(b *testing.B) {
	m := makeLargeMessage()
	data, _ := msgpack.Marshal(&m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out LargeMessage
		_ = msgpack.Unmarshal(data, &out)
	}
}

// ============= XLarge (~100KB) Benchmarks =============

func BenchmarkXPB_Encode_XLarge(b *testing.B) {
	m := makeXLargeMessage()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encodeXLargeXPB(m)
	}
}

func BenchmarkXPB_Decode_XLarge(b *testing.B) {
	m := makeXLargeMessage()
	data := encodeXLargeXPB(m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = decodeXLargeXPB(data)
	}
}

func BenchmarkJSON_Encode_XLarge(b *testing.B) {
	m := makeXLargeMessage()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(&m)
	}
}

func BenchmarkJSON_Decode_XLarge(b *testing.B) {
	m := makeXLargeMessage()
	data, _ := json.Marshal(&m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out XLargeMessage
		_ = json.Unmarshal(data, &out)
	}
}

func BenchmarkMsgpack_Encode_XLarge(b *testing.B) {
	m := makeXLargeMessage()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = msgpack.Marshal(&m)
	}
}

func BenchmarkMsgpack_Decode_XLarge(b *testing.B) {
	m := makeXLargeMessage()
	data, _ := msgpack.Marshal(&m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out XLargeMessage
		_ = msgpack.Unmarshal(data, &out)
	}
}

// ============= Size Comparison Test =============

func TestSizeScalingEncodedSizes(t *testing.T) {
	// Tiny
	tiny := makeTinyMessage()
	xpbTiny := encodeTinyXPB(tiny)
	jsonTiny, _ := json.Marshal(&tiny)
	msgpackTiny, _ := msgpack.Marshal(&tiny)

	t.Logf("=== Tiny Message (~10 bytes) ===")
	t.Logf("XPB V2:   %d bytes", len(xpbTiny))
	t.Logf("JSON:     %d bytes", len(jsonTiny))
	t.Logf("Msgpack:  %d bytes", len(msgpackTiny))

	// Small
	small := makeSmallMessage()
	xpbSmall := encodeSmallXPB(small)
	jsonSmall, _ := json.Marshal(&small)
	msgpackSmall, _ := msgpack.Marshal(&small)

	t.Logf("\n=== Small Message (~50 bytes) ===")
	t.Logf("XPB V2:   %d bytes", len(xpbSmall))
	t.Logf("JSON:     %d bytes", len(jsonSmall))
	t.Logf("Msgpack:  %d bytes", len(msgpackSmall))

	// Medium
	medium := makeMediumMessage()
	xpbMedium := encodeMediumXPB(medium)
	jsonMedium, _ := json.Marshal(&medium)
	msgpackMedium, _ := msgpack.Marshal(&medium)

	t.Logf("\n=== Medium Message (~500 bytes) ===")
	t.Logf("XPB V2:   %d bytes", len(xpbMedium))
	t.Logf("JSON:     %d bytes", len(jsonMedium))
	t.Logf("Msgpack:  %d bytes", len(msgpackMedium))

	// Large
	large := makeLargeMessage()
	xpbLarge := encodeLargeXPB(large)
	jsonLarge, _ := json.Marshal(&large)
	msgpackLarge, _ := msgpack.Marshal(&large)

	t.Logf("\n=== Large Message (~10KB) ===")
	t.Logf("XPB V2:   %d bytes", len(xpbLarge))
	t.Logf("JSON:     %d bytes", len(jsonLarge))
	t.Logf("Msgpack:  %d bytes", len(msgpackLarge))

	// XLarge
	xlarge := makeXLargeMessage()
	xpbXLarge := encodeXLargeXPB(xlarge)
	jsonXLarge, _ := json.Marshal(&xlarge)
	msgpackXLarge, _ := msgpack.Marshal(&xlarge)

	t.Logf("\n=== XLarge Message (~100KB) ===")
	t.Logf("XPB V2:   %d bytes", len(xpbXLarge))
	t.Logf("JSON:     %d bytes", len(jsonXLarge))
	t.Logf("Msgpack:  %d bytes", len(msgpackXLarge))

	// Summary table
	t.Logf("\n=== Size Scaling Summary ===")
	t.Logf("%-10s %10s %10s %10s %10s", "Size", "XPB", "JSON", "Msgpack", "XPB Savings")
	t.Logf("%-10s %10d %10d %10d %9.1f%%", "Tiny", len(xpbTiny), len(jsonTiny), len(msgpackTiny),
		float64(len(jsonTiny)-len(xpbTiny))/float64(len(jsonTiny))*100)
	t.Logf("%-10s %10d %10d %10d %9.1f%%", "Small", len(xpbSmall), len(jsonSmall), len(msgpackSmall),
		float64(len(jsonSmall)-len(xpbSmall))/float64(len(jsonSmall))*100)
	t.Logf("%-10s %10d %10d %10d %9.1f%%", "Medium", len(xpbMedium), len(jsonMedium), len(msgpackMedium),
		float64(len(jsonMedium)-len(xpbMedium))/float64(len(jsonMedium))*100)
	t.Logf("%-10s %10d %10d %10d %9.1f%%", "Large", len(xpbLarge), len(jsonLarge), len(msgpackLarge),
		float64(len(jsonLarge)-len(xpbLarge))/float64(len(jsonLarge))*100)
	t.Logf("%-10s %10d %10d %10d %9.1f%%", "XLarge", len(xpbXLarge), len(jsonXLarge), len(msgpackXLarge),
		float64(len(jsonXLarge)-len(xpbXLarge))/float64(len(jsonXLarge))*100)
}

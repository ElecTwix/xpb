// Package benchmark provides benchmarks comparing XPB V2 collection types to other formats.
package benchmark

import (
	"encoding/json"
	"testing"

	"github.com/anthropic/xpb/runtime/go/xpb"
	"github.com/vmihailenco/msgpack/v5"
)

// ============= Test Data Generation =============

func generateStringArray(size int) []string {
	arr := make([]string, size)
	for i := 0; i < size; i++ {
		arr[i] = "item_" + string(rune('A'+i%26)) + "_value"
	}
	return arr
}

func generateInt32Array(size int) []int32 {
	arr := make([]int32, size)
	for i := 0; i < size; i++ {
		arr[i] = int32(i * 17) // Some varied values
	}
	return arr
}

func generateStringMap(size int) map[string]string {
	m := make(map[string]string, size)
	for i := 0; i < size; i++ {
		key := "key_" + string(rune('A'+i%26)) + "_" + string(rune('0'+i%10))
		m[key] = "value_for_" + key
	}
	return m
}

// ============= XPB Array Encoding =============

func encodeStringArrayXPB(arr []string) []byte {
	enc := xpb.NewEncoder(len(arr) * 20)
	enc.WriteInt32(int32(len(arr)))
	for _, s := range arr {
		enc.WriteString(s)
	}
	return enc.Bytes()
}

func decodeStringArrayXPB(data []byte) []string {
	dec := xpb.NewDecoder(data)
	count, _ := dec.ReadInt32()
	arr := make([]string, count)
	for i := int32(0); i < count; i++ {
		arr[i], _ = dec.ReadString()
	}
	return arr
}

func encodeInt32ArrayXPB(arr []int32) []byte {
	enc := xpb.NewEncoder(len(arr)*4 + 4)
	enc.WriteInt32(int32(len(arr)))
	for _, v := range arr {
		enc.WriteInt32(v)
	}
	return enc.Bytes()
}

func decodeInt32ArrayXPB(data []byte) []int32 {
	dec := xpb.NewDecoder(data)
	count, _ := dec.ReadInt32()
	arr := make([]int32, count)
	for i := int32(0); i < count; i++ {
		arr[i], _ = dec.ReadInt32()
	}
	return arr
}

func encodeStringMapXPB(m map[string]string) []byte {
	enc := xpb.NewEncoder(len(m) * 40)
	enc.WriteInt32(int32(len(m)))
	for k, v := range m {
		enc.WriteString(k)
		enc.WriteString(v)
	}
	return enc.Bytes()
}

func decodeStringMapXPB(data []byte) map[string]string {
	dec := xpb.NewDecoder(data)
	count, _ := dec.ReadInt32()
	m := make(map[string]string, count)
	for i := int32(0); i < count; i++ {
		k, _ := dec.ReadString()
		v, _ := dec.ReadString()
		m[k] = v
	}
	return m
}

// ============= String Array Benchmarks (100 elements) =============

func BenchmarkXPB_Encode_StringArray100(b *testing.B) {
	arr := generateStringArray(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encodeStringArrayXPB(arr)
	}
}

func BenchmarkXPB_Decode_StringArray100(b *testing.B) {
	arr := generateStringArray(100)
	data := encodeStringArrayXPB(arr)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = decodeStringArrayXPB(data)
	}
}

func BenchmarkJSON_Encode_StringArray100(b *testing.B) {
	arr := generateStringArray(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(arr)
	}
}

func BenchmarkJSON_Decode_StringArray100(b *testing.B) {
	arr := generateStringArray(100)
	data, _ := json.Marshal(arr)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out []string
		_ = json.Unmarshal(data, &out)
	}
}

func BenchmarkMsgpack_Encode_StringArray100(b *testing.B) {
	arr := generateStringArray(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = msgpack.Marshal(arr)
	}
}

func BenchmarkMsgpack_Decode_StringArray100(b *testing.B) {
	arr := generateStringArray(100)
	data, _ := msgpack.Marshal(arr)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out []string
		_ = msgpack.Unmarshal(data, &out)
	}
}

// ============= Int32 Array Benchmarks (100 elements) =============

func BenchmarkXPB_Encode_Int32Array100(b *testing.B) {
	arr := generateInt32Array(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encodeInt32ArrayXPB(arr)
	}
}

func BenchmarkXPB_Decode_Int32Array100(b *testing.B) {
	arr := generateInt32Array(100)
	data := encodeInt32ArrayXPB(arr)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = decodeInt32ArrayXPB(data)
	}
}

func BenchmarkJSON_Encode_Int32Array100(b *testing.B) {
	arr := generateInt32Array(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(arr)
	}
}

func BenchmarkJSON_Decode_Int32Array100(b *testing.B) {
	arr := generateInt32Array(100)
	data, _ := json.Marshal(arr)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out []int32
		_ = json.Unmarshal(data, &out)
	}
}

func BenchmarkMsgpack_Encode_Int32Array100(b *testing.B) {
	arr := generateInt32Array(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = msgpack.Marshal(arr)
	}
}

func BenchmarkMsgpack_Decode_Int32Array100(b *testing.B) {
	arr := generateInt32Array(100)
	data, _ := msgpack.Marshal(arr)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out []int32
		_ = msgpack.Unmarshal(data, &out)
	}
}

// ============= String Map Benchmarks (100 entries) =============

func BenchmarkXPB_Encode_StringMap100(b *testing.B) {
	m := generateStringMap(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = encodeStringMapXPB(m)
	}
}

func BenchmarkXPB_Decode_StringMap100(b *testing.B) {
	m := generateStringMap(100)
	data := encodeStringMapXPB(m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = decodeStringMapXPB(data)
	}
}

func BenchmarkJSON_Encode_StringMap100(b *testing.B) {
	m := generateStringMap(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(m)
	}
}

func BenchmarkJSON_Decode_StringMap100(b *testing.B) {
	m := generateStringMap(100)
	data, _ := json.Marshal(m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out map[string]string
		_ = json.Unmarshal(data, &out)
	}
}

func BenchmarkMsgpack_Encode_StringMap100(b *testing.B) {
	m := generateStringMap(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = msgpack.Marshal(m)
	}
}

func BenchmarkMsgpack_Decode_StringMap100(b *testing.B) {
	m := generateStringMap(100)
	data, _ := msgpack.Marshal(m)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out map[string]string
		_ = msgpack.Unmarshal(data, &out)
	}
}

// ============= Size Comparison Test =============

func TestCollectionEncodedSizes(t *testing.T) {
	// String Array (100 elements)
	strArr := generateStringArray(100)
	xpbStrArr := encodeStringArrayXPB(strArr)
	jsonStrArr, _ := json.Marshal(strArr)
	msgpackStrArr, _ := msgpack.Marshal(strArr)

	t.Logf("=== String Array (100 elements) ===")
	t.Logf("XPB V2:   %d bytes", len(xpbStrArr))
	t.Logf("JSON:     %d bytes", len(jsonStrArr))
	t.Logf("Msgpack:  %d bytes", len(msgpackStrArr))

	// Int32 Array (100 elements)
	intArr := generateInt32Array(100)
	xpbIntArr := encodeInt32ArrayXPB(intArr)
	jsonIntArr, _ := json.Marshal(intArr)
	msgpackIntArr, _ := msgpack.Marshal(intArr)

	t.Logf("\n=== Int32 Array (100 elements) ===")
	t.Logf("XPB V2:   %d bytes", len(xpbIntArr))
	t.Logf("JSON:     %d bytes", len(jsonIntArr))
	t.Logf("Msgpack:  %d bytes", len(msgpackIntArr))

	// String Map (100 entries)
	strMap := generateStringMap(100)
	xpbStrMap := encodeStringMapXPB(strMap)
	jsonStrMap, _ := json.Marshal(strMap)
	msgpackStrMap, _ := msgpack.Marshal(strMap)

	t.Logf("\n=== String Map (100 entries) ===")
	t.Logf("XPB V2:   %d bytes", len(xpbStrMap))
	t.Logf("JSON:     %d bytes", len(jsonStrMap))
	t.Logf("Msgpack:  %d bytes", len(msgpackStrMap))
}

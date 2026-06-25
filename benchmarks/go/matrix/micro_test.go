package matrixbench

import (
	"testing"

	"github.com/ElecTwix/xpb/runtime/go/xpb"
)

// Microbenchmarks isolate the individual hot runtime primitives so a regression
// in any single helper (inlining loss, an added bounds check, an allocation) is
// attributable on its own rather than only visible through a whole-message
// benchmark. Each writes its result into a package-level sink so the compiler
// cannot eliminate the call.
var (
	sinkI32 int32
	sinkStr string
	sinkBuf []byte
	sinkPos int
)

func BenchmarkMicro_ReadInt32At(b *testing.B) {
	buf := xpb.AppendInt32To(nil, -123456)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkI32, sinkPos, _ = xpb.ReadInt32At(buf, 0)
	}
}

func BenchmarkMicro_ReadStringAt(b *testing.B) {
	buf := xpb.AppendStringTo(nil, "market.subscribe")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkStr, sinkPos, _ = xpb.ReadStringAt(buf, 0)
	}
}

func BenchmarkMicro_AppendStringTo(b *testing.B) {
	const s = "market.subscribe"
	buf := make([]byte, 0, 64)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkBuf = xpb.AppendStringTo(buf[:0], s)
	}
}

func BenchmarkMicro_EnsureRunAt(b *testing.B) {
	buf := make([]byte, 64)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkPos, _ = xpb.EnsureRunAt(buf, 0, 50)
	}
}

func BenchmarkMicro_RunInt32At(b *testing.B) {
	buf := make([]byte, 64)
	xpb.PutInt32At(buf, 0, -123456)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkI32 = xpb.RunInt32At(buf, 0)
	}
}

// Package matrixbench is the Go codegen shape x size benchmark + deterministic
// perf-gate matrix for ticket T-16. It exercises every structural shape the Go
// emitter produces, across a payload-size axis, with three layers of coverage:
//
//   - Benchmarks (this file): BenchmarkMatrix runs encode+decode sub-benchmarks
//     via b.Run over {shape} x {size}; BenchmarkMatrixParallel adds
//     b.RunParallel pooled-encode / decode variants. Every sub-benchmark sets
//     b.SetBytes(len(wire)) (so `go test -bench` reports MB/s) and calls
//     b.ReportAllocs().
//   - Microbenchmarks (micro_test.go): the hot runtime primitives in isolation
//     (ReadInt32At / ReadStringAt / AppendStringTo / EnsureRunAt / RunInt32At).
//   - Deterministic gates (gate_test.go): normal Test funcs run by `make verify`
//     that fail CI on an allocation, wire-size, or determinism regression.
//
// Shapes (one generated package each; val = 0.5.0 default, ptr = the
// --go-optional-style=pointer --go-safe-bytes opt-out, generated only where it
// differs):
//
//	scalar  — all-scalar, coalesced fixed-width run (one ExtendRun/EnsureRunAt)
//	strval  — string/bytes-heavy, val  (zero-copy bytes)
//	strptr  — string/bytes-heavy, ptr  (copying bytes)
//	optval  — optional-heavy, val      (value + Has<Field> bool)
//	optptr  — optional-heavy, ptr      (*T)
//	arr     — array/repeated-heavy
//	mapd    — map-heavy (non-canonical wire; compare decoded VALUES)
//	nest    — deeply-nested (five-level message chain)
//
// Size axes: string/bytes content length {8B, 64B, 1KB, 64KB}; array length
// {0, 16, 1024, 65536}. Fixed-shape messages use a single representative size.
//
// Run: go test -bench=. -benchmem ./benchmarks/go/matrix/
package matrixbench

import (
	"fmt"
	"testing"

	"github.com/ElecTwix/xpb/benchmarks/go/matrix/arr"
	"github.com/ElecTwix/xpb/benchmarks/go/matrix/mapd"
	"github.com/ElecTwix/xpb/benchmarks/go/matrix/nest"
	"github.com/ElecTwix/xpb/benchmarks/go/matrix/optptr"
	"github.com/ElecTwix/xpb/benchmarks/go/matrix/optval"
	"github.com/ElecTwix/xpb/benchmarks/go/matrix/scalar"
	"github.com/ElecTwix/xpb/benchmarks/go/matrix/strptr"
	"github.com/ElecTwix/xpb/benchmarks/go/matrix/strval"
	"github.com/ElecTwix/xpb/runtime/go/xpb"
)

// Size axes from the ticket.
var (
	strSizes = []int{8, 64, 1024, 65536} // string/bytes content length in bytes
	arrSizes = []int{0, 16, 1024, 65536} // repeated-field element count
)

// byteLabel renders a byte-length size axis value as 8B / 64B / 1KB / 64KB.
func byteLabel(n int) string {
	if n >= 1024 {
		return fmt.Sprintf("%dKB", n/1024)
	}
	return fmt.Sprintf("%dB", n)
}

// ---------- sample builders ----------

func sampleScalar() *scalar.Scalar {
	return &scalar.Scalar{
		A: -123456,
		B: -9_000_000_000,
		C: 4_000_000_000,
		D: 18_000_000_000_000_000_000,
		E: 3.5,
		F: 2.718281828,
		G: true,
		H: 777,
		I: 1_735_128_000_000,
		J: false,
	}
}

// makeStr returns a deterministic n-byte ASCII string.
func makeStr(n int) string { return string(makeBytes(n)) }

// makeBytes returns a deterministic n-byte slice.
func makeBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('a' + (i % 26))
	}
	return b
}

func sampleStrVal(n int) *strval.StrBytes {
	s, b := makeStr(n), makeBytes(n)
	return &strval.StrBytes{S1: s, B1: b, S2: s, B2: b}
}

func sampleStrPtr(n int) *strptr.StrBytes {
	s, b := makeStr(n), makeBytes(n)
	return &strptr.StrBytes{S1: s, B1: b, S2: s, B2: b}
}

func sampleOptVal() *optval.Optional {
	return &optval.Optional{
		A: -5, HasA: true,
		B: 1 << 40, HasB: true,
		C: "alpha", HasC: true,
		D: true, HasD: true,
		E: 6.022e23, HasE: true,
		F: 4242, HasF: true,
		G: "bravo", HasG: true,
		H: 99, HasH: true,
	}
}

func sampleOptPtr() *optptr.Optional {
	a, b, h := int32(-5), int64(1<<40), int32(99)
	c, g := "alpha", "bravo"
	d := true
	e := 6.022e23
	f := uint32(4242)
	return &optptr.Optional{A: &a, B: &b, C: &c, D: &d, E: &e, F: &f, G: &g, H: &h}
}

func sampleArr(n int) *arr.Arrays {
	ints := make([]int32, n)
	strs := make([]string, n)
	for i := 0; i < n; i++ {
		ints[i] = int32(i)
		strs[i] = "ab"
	}
	return &arr.Arrays{Ints: ints, Strs: strs}
}

func sampleMap() *mapd.Maps {
	return &mapd.Maps{
		M1: map[string]int32{"alpha": 1, "bravo": 2, "charlie": 3, "delta": 4},
		M2: map[int32]string{1: "one", 2: "two", 3: "three", 4: "four"},
	}
}

func sampleNest() *nest.Level1 {
	return &nest.Level1{V: 1, Child: &nest.Level2{V: 2, Child: &nest.Level3{
		V: 3, Child: &nest.Level4{V: 4, Child: &nest.Level5{V: 5, S: "leaf"}}}}}
}

// ---------- benchmark plumbing ----------

// benchEncodeDecode emits the encode (pooled) and decode sub-benchmarks for one
// (shape, size) cell. encode performs one pooled MarshalTo; decode unmarshals
// wire into a fresh value. Both report MB/s (b.SetBytes) and allocs.
func benchEncodeDecode(b *testing.B, wire []byte, encode func(*xpb.Encoder), decode func([]byte) error) {
	b.Run("op=encode", func(b *testing.B) {
		b.SetBytes(int64(len(wire)))
		b.ReportAllocs()
		warm := xpb.GetEncoder() // grow the pooled buffer once before timing
		encode(warm)
		xpb.PutEncoder(warm)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			e := xpb.GetEncoder()
			encode(e)
			_ = e.Bytes()
			xpb.PutEncoder(e)
		}
	})
	b.Run("op=decode", func(b *testing.B) {
		b.SetBytes(int64(len(wire)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := decode(wire); err != nil {
				b.Fatalf("decode: %v", err)
			}
		}
	})
}

func marshalB(b *testing.B, marshal func() ([]byte, error)) []byte {
	b.Helper()
	data, err := marshal()
	if err != nil {
		b.Fatalf("marshal: %v", err)
	}
	return data
}

// BenchmarkMatrix is the {shape} x {size} encode+decode benchmark grid.
func BenchmarkMatrix(b *testing.B) {
	b.Run("shape=allscalar", func(b *testing.B) {
		m := sampleScalar()
		wire := marshalB(b, m.Marshal)
		benchEncodeDecode(b, wire, m.MarshalTo, func(d []byte) error {
			var x scalar.Scalar
			return x.Unmarshal(d)
		})
	})

	b.Run("shape=strbytes/style=val", func(b *testing.B) {
		for _, n := range strSizes {
			m := sampleStrVal(n)
			wire := marshalB(b, m.Marshal)
			b.Run("size="+byteLabel(n), func(b *testing.B) {
				benchEncodeDecode(b, wire, m.MarshalTo, func(d []byte) error {
					var x strval.StrBytes
					return x.Unmarshal(d)
				})
			})
		}
	})

	b.Run("shape=strbytes/style=ptr", func(b *testing.B) {
		for _, n := range strSizes {
			m := sampleStrPtr(n)
			wire := marshalB(b, m.Marshal)
			b.Run("size="+byteLabel(n), func(b *testing.B) {
				benchEncodeDecode(b, wire, m.MarshalTo, func(d []byte) error {
					var x strptr.StrBytes
					return x.Unmarshal(d)
				})
			})
		}
	})

	b.Run("shape=optional/style=val", func(b *testing.B) {
		m := sampleOptVal()
		wire := marshalB(b, m.Marshal)
		benchEncodeDecode(b, wire, m.MarshalTo, func(d []byte) error {
			var x optval.Optional
			return x.Unmarshal(d)
		})
	})

	b.Run("shape=optional/style=ptr", func(b *testing.B) {
		m := sampleOptPtr()
		wire := marshalB(b, m.Marshal)
		benchEncodeDecode(b, wire, m.MarshalTo, func(d []byte) error {
			var x optptr.Optional
			return x.Unmarshal(d)
		})
	})

	b.Run("shape=array", func(b *testing.B) {
		for _, n := range arrSizes {
			m := sampleArr(n)
			wire := marshalB(b, m.Marshal)
			b.Run(fmt.Sprintf("size=%d", n), func(b *testing.B) {
				benchEncodeDecode(b, wire, m.MarshalTo, func(d []byte) error {
					var x arr.Arrays
					return x.Unmarshal(d)
				})
			})
		}
	})

	b.Run("shape=map", func(b *testing.B) {
		m := sampleMap()
		wire := marshalB(b, m.Marshal)
		benchEncodeDecode(b, wire, m.MarshalTo, func(d []byte) error {
			var x mapd.Maps
			return x.Unmarshal(d)
		})
	})

	b.Run("shape=nested", func(b *testing.B) {
		m := sampleNest()
		wire := marshalB(b, m.Marshal)
		benchEncodeDecode(b, wire, m.MarshalTo, func(d []byte) error {
			var x nest.Level1
			return x.Unmarshal(d)
		})
	})
}

// BenchmarkMatrixParallel runs pooled-encode and decode under b.RunParallel for
// a few representative shapes/sizes. The encoder pool is concurrency-safe and
// zero-copy decode only reads the shared wire, so both are safe to fan out.
func BenchmarkMatrixParallel(b *testing.B) {
	b.Run("shape=allscalar", func(b *testing.B) {
		m := sampleScalar()
		wire := marshalB(b, m.Marshal)
		benchParallel(b, wire, m.MarshalTo, func() func([]byte) error {
			var x scalar.Scalar
			return func(d []byte) error { return x.Unmarshal(d) }
		})
	})
	b.Run("shape=strbytes/style=val/size=1KB", func(b *testing.B) {
		m := sampleStrVal(1024)
		wire := marshalB(b, m.Marshal)
		benchParallel(b, wire, m.MarshalTo, func() func([]byte) error {
			var x strval.StrBytes
			return func(d []byte) error { return x.Unmarshal(d) }
		})
	})
	b.Run("shape=array/size=1024", func(b *testing.B) {
		m := sampleArr(1024)
		wire := marshalB(b, m.Marshal)
		benchParallel(b, wire, m.MarshalTo, func() func([]byte) error {
			var x arr.Arrays
			return func(d []byte) error { return x.Unmarshal(d) }
		})
	})
}

// benchParallel emits parallel encode/decode sub-benchmarks. newDecoder returns
// a fresh decode closure (with its own goroutine-local target) per goroutine.
func benchParallel(b *testing.B, wire []byte, encode func(*xpb.Encoder), newDecoder func() func([]byte) error) {
	b.Run("op=encode", func(b *testing.B) {
		b.SetBytes(int64(len(wire)))
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				e := xpb.GetEncoder()
				encode(e)
				_ = e.Bytes()
				xpb.PutEncoder(e)
			}
		})
	})
	b.Run("op=decode", func(b *testing.B) {
		b.SetBytes(int64(len(wire)))
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			decode := newDecoder()
			for pb.Next() {
				if err := decode(wire); err != nil {
					// b.Errorf (not Fatalf) is the correct failure call from a
					// RunParallel worker goroutine: FailNow/Fatal must come from
					// the benchmark's own goroutine. wire is always valid, so
					// this branch is latent, but kept correct.
					b.Errorf("decode: %v", err)
					return
				}
			}
		})
	})
}

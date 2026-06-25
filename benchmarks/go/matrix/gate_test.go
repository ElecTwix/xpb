package matrixbench

import (
	"bytes"
	"fmt"
	"maps"
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

// These deterministic gates run under plain `go test` (hence `make verify`) and
// fail CI the moment a behavioural promise of the Go codegen regresses, without
// depending on any wall-clock benchmark number:
//
//   - zero-alloc value decode for the allocation-free shapes;
//   - exact pinned decode-alloc count for the shapes that MUST allocate backing
//     storage (a true property of the wire format, not a regression -- the gate
//     pins it so a NEW allocation is still caught);
//   - zero-alloc pooled encode for every shape;
//   - pinned encoded wire SIZE per non-map shape;
//   - byte-identical re-encode (encoder determinism) per non-map shape;
//   - decoded-VALUE equality for the map shape (map wire is non-canonical, so we
//     never byte-compare it -- see CLAUDE.md / T-7).

// ---------- pinned wire sizes ----------

const (
	wantSizeScalar = 50 // one coalesced fixed-width run: 4+8+4+8+4+8+1+4+8+1
	wantSizeOptVal = 49 // 8 present optionals: presence byte + value (+len) each
	wantSizeNest   = 29 // five-level chain, top body not length-prefixed
)

// strval/strptr wire size for content length n (4 fields, each len n):
// 4*(prefix(n)+n) where prefix is 1 byte for n<=254 else 5 (0xFF + 4-byte len).
var wantSizeStr = map[int]int{8: 36, 64: 260, 1024: 4116, 65536: 262164}

// arr wire size for element count n: 8 + 7n
// (two int32 count prefixes = 8; each int = 4, each "ab" string = 1+2 = 3).
var wantSizeArr = map[int]int{0: 8, 16: 120, 1024: 7176, 65536: 458760}

// ---------- round-trip correctness ----------

func TestRoundtrip_AllShapes(t *testing.T) {
	t.Run("scalar", func(t *testing.T) {
		w, err := sampleScalar().Marshal()
		if err != nil {
			t.Fatal(err)
		}
		var m scalar.Scalar
		if err := m.Unmarshal(w); err != nil {
			t.Fatal(err)
		}
		if *sampleScalar() != m {
			t.Fatalf("scalar mismatch: got %+v want %+v", m, *sampleScalar())
		}
	})

	t.Run("strbytes/val", func(t *testing.T) {
		for _, n := range strSizes {
			w, err := sampleStrVal(n).Marshal()
			if err != nil {
				t.Fatal(err)
			}
			var m strval.StrBytes
			if err := m.Unmarshal(w); err != nil {
				t.Fatal(err)
			}
			s, b := makeStr(n), makeBytes(n)
			if m.S1 != s || m.S2 != s || !bytes.Equal(m.B1, b) || !bytes.Equal(m.B2, b) {
				t.Fatalf("strval n=%d round-trip mismatch", n)
			}
		}
	})

	t.Run("strbytes/ptr", func(t *testing.T) {
		for _, n := range strSizes {
			w, err := sampleStrPtr(n).Marshal()
			if err != nil {
				t.Fatal(err)
			}
			var m strptr.StrBytes
			if err := m.Unmarshal(w); err != nil {
				t.Fatal(err)
			}
			s, b := makeStr(n), makeBytes(n)
			if m.S1 != s || m.S2 != s || !bytes.Equal(m.B1, b) || !bytes.Equal(m.B2, b) {
				t.Fatalf("strptr n=%d round-trip mismatch", n)
			}
		}
	})

	t.Run("optional/val", func(t *testing.T) {
		w, err := sampleOptVal().Marshal()
		if err != nil {
			t.Fatal(err)
		}
		var m optval.Optional
		if err := m.Unmarshal(w); err != nil {
			t.Fatal(err)
		}
		want := sampleOptVal()
		if m != *want {
			t.Fatalf("optval mismatch: got %+v want %+v", m, *want)
		}
	})

	t.Run("optional/ptr", func(t *testing.T) {
		w, err := sampleOptPtr().Marshal()
		if err != nil {
			t.Fatal(err)
		}
		var m optptr.Optional
		if err := m.Unmarshal(w); err != nil {
			t.Fatal(err)
		}
		if m.A == nil || *m.A != -5 || m.B == nil || *m.B != 1<<40 ||
			m.C == nil || *m.C != "alpha" || m.D == nil || !*m.D ||
			m.E == nil || *m.E != 6.022e23 || m.F == nil || *m.F != 4242 ||
			m.G == nil || *m.G != "bravo" || m.H == nil || *m.H != 99 {
			t.Fatalf("optptr round-trip mismatch: %+v", m)
		}
	})

	t.Run("array", func(t *testing.T) {
		for _, n := range arrSizes {
			w, err := sampleArr(n).Marshal()
			if err != nil {
				t.Fatal(err)
			}
			var m arr.Arrays
			if err := m.Unmarshal(w); err != nil {
				t.Fatal(err)
			}
			if len(m.Ints) != n || len(m.Strs) != n {
				t.Fatalf("array n=%d: got len ints=%d strs=%d", n, len(m.Ints), len(m.Strs))
			}
			for i := 0; i < n; i++ {
				if m.Ints[i] != int32(i) || m.Strs[i] != "ab" {
					t.Fatalf("array n=%d element %d mismatch", n, i)
				}
			}
		}
	})

	t.Run("nested", func(t *testing.T) {
		w, err := sampleNest().Marshal()
		if err != nil {
			t.Fatal(err)
		}
		var m nest.Level1
		if err := m.Unmarshal(w); err != nil {
			t.Fatal(err)
		}
		if m.V != 1 || m.Child == nil || m.Child.V != 2 || m.Child.Child == nil ||
			m.Child.Child.V != 3 || m.Child.Child.Child == nil || m.Child.Child.Child.V != 4 ||
			m.Child.Child.Child.Child == nil ||
			m.Child.Child.Child.Child.V != 5 || m.Child.Child.Child.Child.S != "leaf" {
			t.Fatalf("nested round-trip mismatch: %+v", m)
		}
	})
}

// TestMapRoundTripValues exercises the map shape by decoded VALUE, never bytes:
// map fields encode in Go map-iteration order, so the wire is non-canonical
// (CLAUDE.md / T-7). A round trip must preserve the key/value pairs.
func TestMapRoundTripValues(t *testing.T) {
	src := sampleMap()
	w, err := src.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var m mapd.Maps
	if err := m.Unmarshal(w); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(src.M1, m.M1) {
		t.Fatalf("M1 values mismatch: got %v want %v", m.M1, src.M1)
	}
	if !maps.Equal(src.M2, m.M2) {
		t.Fatalf("M2 values mismatch: got %v want %v", m.M2, src.M2)
	}
}

// TestDecodeTruncated feeds each shape a wire buffer with its final byte removed
// and asserts Unmarshal surfaces a decode error rather than silently returning
// nil -- proving the generated short-buffer error checks are actually wired.
func TestDecodeTruncated(t *testing.T) {
	mk := func(marshal func() ([]byte, error)) []byte {
		w, err := marshal()
		if err != nil {
			t.Fatal(err)
		}
		return w
	}
	cases := []struct {
		name   string
		wire   []byte
		decode func([]byte) error
	}{
		{"scalar", mk(sampleScalar().Marshal), func(d []byte) error { var m scalar.Scalar; return m.Unmarshal(d) }},
		{"strval", mk(sampleStrVal(64).Marshal), func(d []byte) error { var m strval.StrBytes; return m.Unmarshal(d) }},
		{"strptr", mk(sampleStrPtr(64).Marshal), func(d []byte) error { var m strptr.StrBytes; return m.Unmarshal(d) }},
		{"optval", mk(sampleOptVal().Marshal), func(d []byte) error { var m optval.Optional; return m.Unmarshal(d) }},
		{"optptr", mk(sampleOptPtr().Marshal), func(d []byte) error { var m optptr.Optional; return m.Unmarshal(d) }},
		{"array", mk(sampleArr(16).Marshal), func(d []byte) error { var m arr.Arrays; return m.Unmarshal(d) }},
		{"map", mk(sampleMap().Marshal), func(d []byte) error { var m mapd.Maps; return m.Unmarshal(d) }},
		{"nested", mk(sampleNest().Marshal), func(d []byte) error { var m nest.Level1; return m.Unmarshal(d) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if len(c.wire) < 2 {
				t.Skipf("wire too short to truncate: %d bytes", len(c.wire))
			}
			if err := c.decode(c.wire[:len(c.wire)-1]); err == nil {
				t.Fatalf("%s: truncated wire decoded without error", c.name)
			}
		})
	}
}

// ---------- pinned wire size ----------

func TestGateWireSize(t *testing.T) {
	check := func(name string, got, want int) {
		if got != want {
			t.Errorf("%s wire size = %d, want pinned %d", name, got, want)
		}
	}
	w, _ := sampleScalar().Marshal()
	check("scalar", len(w), wantSizeScalar)
	w, _ = sampleOptVal().Marshal()
	check("optval", len(w), wantSizeOptVal)
	w, _ = sampleNest().Marshal()
	check("nested", len(w), wantSizeNest)
	for _, n := range strSizes {
		w, _ = sampleStrVal(n).Marshal()
		check("strval/"+byteLabel(n), len(w), wantSizeStr[n])
		w, _ = sampleStrPtr(n).Marshal()
		check("strptr/"+byteLabel(n), len(w), wantSizeStr[n])
	}
	for _, n := range arrSizes {
		w, _ = sampleArr(n).Marshal()
		check("array", len(w), wantSizeArr[n])
	}
	// Compact-length boundary: a string of len <=254 carries a 1-byte length
	// prefix; len >=255 carries 0xFF + a 4-byte prefix. Pin both sides of the
	// transition (4 fields each: 4*(prefix+n)).
	for _, bc := range []struct{ n, want int }{{254, 1020}, {255, 1040}, {256, 1044}} {
		w, _ = sampleStrVal(bc.n).Marshal()
		check(fmt.Sprintf("strval/boundary-%d", bc.n), len(w), bc.want)
	}
}

// ---------- encoder determinism (non-map shapes) ----------

func TestGateEncoderDeterministic(t *testing.T) {
	twice := func(name string, marshal func() ([]byte, error)) {
		a, err := marshal()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b, err := marshal()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s: re-encode not byte-identical", name)
		}
	}
	twice("scalar", sampleScalar().Marshal)
	twice("strval", sampleStrVal(64).Marshal)
	twice("strptr", sampleStrPtr(64).Marshal)
	twice("optval", sampleOptVal().Marshal)
	twice("optptr", sampleOptPtr().Marshal)
	twice("array", sampleArr(16).Marshal)
	twice("nested", sampleNest().Marshal)
}

// ---------- zero-alloc value decode (allocation-free shapes) ----------

func TestGateZeroAllocValueDecode(t *testing.T) {
	t.Run("scalar", func(t *testing.T) {
		w, _ := sampleScalar().Marshal()
		var m scalar.Scalar
		assertZeroAllocs(t, "scalar decode", func() {
			m = scalar.Scalar{}
			if err := m.Unmarshal(w); err != nil {
				t.Fatal(err)
			}
		})
	})
	t.Run("strval", func(t *testing.T) {
		for _, n := range strSizes {
			w, _ := sampleStrVal(n).Marshal()
			var m strval.StrBytes
			assertZeroAllocs(t, "strval decode n="+byteLabel(n), func() {
				m = strval.StrBytes{}
				if err := m.Unmarshal(w); err != nil {
					t.Fatal(err)
				}
			})
		}
	})
	t.Run("optval", func(t *testing.T) {
		w, _ := sampleOptVal().Marshal()
		var m optval.Optional
		assertZeroAllocs(t, "optval decode", func() {
			m = optval.Optional{}
			if err := m.Unmarshal(w); err != nil {
				t.Fatal(err)
			}
		})
	})
}

// ---------- pinned decode-alloc count (shapes that must allocate) ----------

func TestGateDecodeAllocBounded(t *testing.T) {
	// array: backing slices only -- 0 for empty (zerobase), exactly 2 (one per
	// slice) for non-empty; NO per-element boxing (strings alias the input).
	t.Run("array", func(t *testing.T) {
		cases := map[int]float64{0: 0, 16: 2}
		for n, want := range cases {
			w, _ := sampleArr(n).Marshal()
			var m arr.Arrays
			assertAllocs(t, "array decode n=", n, want, func() {
				m = arr.Arrays{}
				if err := m.Unmarshal(w); err != nil {
					t.Fatal(err)
				}
			})
		}
	})
	// nested: exactly one *T heap box per level below the root (4 for a 5-level
	// chain); strings alias, so nothing else allocates.
	t.Run("nested", func(t *testing.T) {
		w, _ := sampleNest().Marshal()
		var m nest.Level1
		assertAllocs(t, "nested decode depth=", 5, 4, func() {
			m = nest.Level1{}
			if err := m.Unmarshal(w); err != nil {
				t.Fatal(err)
			}
		})
	})
	// strptr (--go-safe-bytes): the two bytes fields are decoded by COPYING
	// (ReadBytesAt), so decode allocates exactly two backing slices; the two
	// string fields still alias, adding nothing. Pins the copying-bytes opt-out
	// path against an unexpected extra copy.
	t.Run("strbytes/ptr", func(t *testing.T) {
		w, _ := sampleStrPtr(64).Marshal()
		var m strptr.StrBytes
		assertAllocs(t, "strptr decode n=", 64, 2, func() {
			m = strptr.StrBytes{}
			if err := m.Unmarshal(w); err != nil {
				t.Fatal(err)
			}
		})
	})
	// NOTE: the map shape's decode allocation count is intentionally NOT pinned.
	// It depends on Go's map runtime (header + bucket growth, which changed with
	// the 1.24 Swiss-table maps), so an exact pin would be fragile across Go
	// versions for no correctness benefit. Map decode correctness is gated by
	// value equality (TestMapRoundTripValues); map ENCODE is alloc-gated in
	// TestGateZeroAllocPooledEncode.
}

// ---------- zero-alloc pooled encode (every shape) ----------

func TestGateZeroAllocPooledEncode(t *testing.T) {
	encScalar := sampleScalar()
	encStr := sampleStrVal(64)
	encStrPtr := sampleStrPtr(64)
	encOpt := sampleOptVal()
	encOptPtr := sampleOptPtr()
	encArr := sampleArr(16)
	encMap := sampleMap()
	encNest := sampleNest()

	cases := []struct {
		name   string
		encode func(*xpb.Encoder)
	}{
		{"scalar", encScalar.MarshalTo},
		{"strval", encStr.MarshalTo},
		{"strptr", encStrPtr.MarshalTo},
		{"optval", encOpt.MarshalTo},
		{"optptr", encOptPtr.MarshalTo},
		{"array", encArr.MarshalTo},
		{"map", encMap.MarshalTo},
		{"nested", encNest.MarshalTo},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Warm the pool so its backing buffer (and, for nested, the spare
			// child encoders) are already allocated before measuring.
			warm := xpb.GetEncoder()
			c.encode(warm)
			xpb.PutEncoder(warm)
			assertZeroAllocs(t, c.name+" pooled encode", func() {
				e := xpb.GetEncoder()
				c.encode(e)
				_ = e.Bytes()
				xpb.PutEncoder(e)
			})
		})
	}
}

// ---------- helpers ----------

func assertZeroAllocs(t *testing.T, what string, fn func()) {
	t.Helper()
	if got := testing.AllocsPerRun(1000, fn); got != 0 {
		t.Fatalf("%s: allocs/run = %v, want 0 (allocation regression)", what, got)
	}
}

func assertAllocs(t *testing.T, prefix string, param int, want float64, fn func()) {
	t.Helper()
	if got := testing.AllocsPerRun(1000, fn); got != want {
		t.Fatalf("%s%d: allocs/run = %v, want pinned %v", prefix, param, got, want)
	}
}

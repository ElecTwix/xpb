package xpb

// bomb_test.go — adversarial / resource-exhaustion ("bomb") payload simulation
// against the decode paths, with emphasis on the coalesced fixed-width run
// helpers (EnsureRunAt + Run*At) and the stateless *At cursor family.
//
// The class of attack here is "a tiny input that claims an enormous amount of
// data". A safe decoder must validate the claim against the bytes actually
// present BEFORE it allocates or indexes — so an attacker who sends six bytes
// claiming a 4 GiB string gets a clean io.ErrUnexpectedEOF, not an OOM or a
// panic. These tests assert both the error behaviour AND a bounded-memory
// property (the decoder does not allocate proportional to the *claimed* size).
//
// This file is additive: it reuses runNoPanic (malformed_test.go) and the
// driveDecoder fuzz harness style (fuzz_test.go) without editing those files,
// and it uses uniquely-named local helpers (bombNode / encodeBombNest) so it
// does not collide with recNode / encodeRecNode in security_validation_test.go.

import (
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"

	"github.com/ElecTwix/xpb/pkg/wire"
)

// hugeLenPrefix builds a compact-length prefix claiming ~4 GiB (0xFF marker +
// a 4-byte little-endian length) followed by `payload` trailing bytes. The
// claimed length dwarfs len(payload), so any reader that allocated up front
// would try to grab gigabytes; a correctly-ordered reader rejects it.
func hugeLenPrefix(payload ...byte) []byte {
	// 0xFFFFFFFE ≈ 4 GiB - 2. Using a value just below the uint32 max keeps it
	// unambiguously "huge" while staying a valid uint32 length on the wire.
	b := []byte{wire.CompactLengthMarker, 0xFE, 0xFF, 0xFF, 0xFF}
	return append(b, payload...)
}

// TestBomb_HugeCompactLengthBoundedMemory feeds many tiny inputs that each
// claim a ~4 GiB string/bytes payload while only a handful of real bytes
// follow. Every reader (stateful and *At, copy and zero-copy) must:
//   - return io.ErrUnexpectedEOF (fail closed), and
//   - NOT allocate memory proportional to the claimed length (no make([]byte,
//     4GB) before the bounds check).
//
// The bounded-memory property is asserted two ways for robustness against
// flakes: testing.AllocsPerRun (a small, generous per-call allocation ceiling)
// and a runtime.ReadMemStats TotalAlloc delta over many iterations (a
// generous-but-real byte ceiling that a single 4 GiB allocation would blow
// past by three orders of magnitude).
func TestBomb_HugeCompactLengthBoundedMemory(t *testing.T) {
	data := hugeLenPrefix(0x00) // one real trailing byte

	// 1. Correctness: every length-prefixed reader fails closed, never panics.
	readers := []struct {
		name string
		read func([]byte) error
	}{
		{"ReadBytes", func(b []byte) error { _, e := NewDecoder(b).ReadBytes(); return e }},
		{"ReadString", func(b []byte) error { _, e := NewDecoder(b).ReadString(); return e }},
		{"CloneString", func(b []byte) error { _, e := NewDecoder(b).CloneString(); return e }},
		{"ReadBytesUnsafe", func(b []byte) error { _, e := NewDecoder(b).ReadBytesUnsafe(); return e }},
		{"ReadMessageBytes", func(b []byte) error { _, e := NewDecoder(b).ReadMessageBytes(); return e }},
		{"ReadStringAt", func(b []byte) error { _, _, e := ReadStringAt(b, 0); return e }},
		{"ReadBytesAt", func(b []byte) error { _, _, e := ReadBytesAt(b, 0); return e }},
		{"ReadBytesUnsafeAt", func(b []byte) error { _, _, e := ReadBytesUnsafeAt(b, 0); return e }},
		{"ReadMessageBytesAt", func(b []byte) error { _, _, e := ReadMessageBytesAt(b, 0); return e }},
	}
	for _, r := range readers {
		runNoPanic(t, "huge len "+r.name, func() {
			if err := r.read(data); !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("%s(huge len) err = %v, want io.ErrUnexpectedEOF", r.name, err)
			}
		})
	}

	// 2. Bounded memory via AllocsPerRun. On the huge-len error path every
	//    reader bounds-checks (p+length > len(b)) and returns the package-level
	//    io.ErrUnexpectedEOF sentinel BEFORE any make([]byte, length), so no
	//    payload allocation happens. The ceiling is deliberately generous (and
	//    well above the handful of incidental allocs the runtime/race-detector
	//    accounting may attribute per call) so it is robust against compiler,
	//    Go-version and -race accounting variance — it is NOT meant to pin an
	//    exact alloc count. The strong proof that nothing 4 GiB-sized is
	//    allocated is the TotalAlloc-delta check below; this is a cheap
	//    fast-signal that allocations stay O(1), not O(claimed length).
	const allocCeiling = 16.0
	for _, r := range readers {
		got := testing.AllocsPerRun(200, func() {
			_ = r.read(data)
		})
		if got > allocCeiling {
			t.Errorf("%s(huge len): %.1f allocs/run exceeds ceiling %.1f (alloc-before-bounds?)",
				r.name, got, allocCeiling)
		}
	}

	// 3. Bounded memory via a TotalAlloc delta over many iterations. Each huge
	//    claim asks for ~4 GiB; if even one in N iterations actually allocated
	//    it, TotalAlloc would jump by gigabytes. We allow a generous-but-real
	//    ceiling that is orders of magnitude below a single 4 GiB allocation.
	const iters = 5000
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < iters; i++ {
		for _, r := range readers {
			_ = r.read(data)
		}
	}
	runtime.ReadMemStats(&after)

	delta := after.TotalAlloc - before.TotalAlloc
	// 64 MiB ceiling: comfortably above any incidental per-call bookkeeping for
	// iters*len(readers) calls, yet ~64x below a single claimed 4 GiB payload —
	// so any alloc-before-bounds regression trips it without false positives.
	const totalAllocCeiling = uint64(64 << 20)
	if delta > totalAllocCeiling {
		t.Errorf("huge-len decode allocated %d bytes over %d iters (ceiling %d); "+
			"decoder may be allocating before bounds-checking the claimed length",
			delta, iters, totalAllocCeiling)
	}
}

// TestBomb_ArrayCountExhaustion is the array-count bomb: a count claiming a
// huge number of elements with an empty/short body must be rejected — without
// allocating a backing slice for the claimed count — by ReadArrayCount AND
// ReadArrayCountAt. The decoders apply two independent caps in a fixed order:
// the caller-supplied maxElements, then the buffer-bounded max. This test
// proves BOTH guards reject a bomb and, crucially, pins WHICH guard fires for
// each case by asserting the error message (the prior version set the count
// above the caller cap, so the buffer-bound path it claimed to test was never
// reached).
func TestBomb_ArrayCountExhaustion(t *testing.T) {
	const maxInt32 = int32(^uint32(0) >> 1) // 2147483647, without importing math

	// --- Buffer-bound bomb. To prove the BUFFER-bounded max is the limiting
	// factor (not the caller cap), the caller cap must sit ABOVE the claimed
	// count. We claim maxInt32 elements with a generous caller cap of maxInt32
	// (so the count is <= cap and the caller-cap check passes), but with
	// elementMinBytes=8 against an empty body the buffer-bounded max is 0, so
	// the buffer bound is what rejects it. The error message identifies the
	// guard, so a regression that rejected for the wrong reason fails here.
	bufBombEnc := NewEncoder(8)
	bufBombEnc.WriteInt32(maxInt32)
	bufBomb := bufBombEnc.Bytes()
	const bufBombMax = int(maxInt32) // >= count, so the caller cap does NOT fire

	runNoPanic(t, "ReadArrayCount buffer-bound bomb", func() {
		dec := NewDecoder(bufBomb)
		n, err := dec.ReadArrayCount(8, bufBombMax)
		if err == nil {
			t.Fatalf("ReadArrayCount buffer-bound bomb: got n=%d err=nil, want rejection", n)
		}
		if !strings.Contains(err.Error(), "buffer-bounded max") {
			t.Fatalf("ReadArrayCount buffer-bound bomb: err = %v, want buffer-bounded-max rejection", err)
		}
		// The 4-byte count must have been consumed before the bound failed.
		if dec.Remaining() != 0 {
			t.Fatalf("ReadArrayCount buffer-bound bomb: remaining=%d, want 0 (count consumed)", dec.Remaining())
		}
	})
	runNoPanic(t, "ReadArrayCountAt buffer-bound bomb", func() {
		n, p, err := ReadArrayCountAt(bufBomb, 0, 8, bufBombMax)
		if err == nil {
			t.Fatalf("ReadArrayCountAt buffer-bound bomb: got n=%d err=nil, want rejection", n)
		}
		if !strings.Contains(err.Error(), "buffer-bounded max") {
			t.Fatalf("ReadArrayCountAt buffer-bound bomb: err = %v, want buffer-bounded-max rejection", err)
		}
		if p != 4 {
			t.Fatalf("ReadArrayCountAt buffer-bound bomb: cursor=%d, want 4 (count consumed before bound)", p)
		}
	})

	// Variable-length element bomb (elementMinBytes=1): claim far more elements
	// than there are remaining bytes. With only the count present, remaining is
	// 0, so the buffer-bounded max (0) rejects any positive count — again with
	// the caller cap set above the count so the buffer bound is the cause.
	varBombEnc := NewEncoder(8)
	varBombEnc.WriteInt32(1_000_000)
	varBomb := varBombEnc.Bytes()
	runNoPanic(t, "ReadArrayCount var-len bomb", func() {
		_, err := NewDecoder(varBomb).ReadArrayCount(1, 1<<30)
		if err == nil || !strings.Contains(err.Error(), "buffer-bounded max") {
			t.Fatalf("ReadArrayCount var-len bomb: err = %v, want buffer-bounded-max rejection", err)
		}
	})
	runNoPanic(t, "ReadArrayCountAt var-len bomb", func() {
		_, _, err := ReadArrayCountAt(varBomb, 0, 1, 1<<30)
		if err == nil || !strings.Contains(err.Error(), "buffer-bounded max") {
			t.Fatalf("ReadArrayCountAt var-len bomb: err = %v, want buffer-bounded-max rejection", err)
		}
	})

	// --- Caller-cap bomb. The complementary guard: a count that fits the
	// buffer but exceeds the caller's allocation policy is rejected by the
	// caller cap. Here the buffer holds the 4-byte count plus 8 element bytes
	// (room for one 8-byte element), the count claims 2 elements, but the
	// caller cap is 1 — so the caller cap fires first.
	capBombEnc := NewEncoder(16)
	capBombEnc.WriteInt32(2)
	capBombEnc.WriteInt64(0) // one element's worth of body so the buffer bound would pass
	capBomb := capBombEnc.Bytes()
	runNoPanic(t, "ReadArrayCount caller-cap bomb", func() {
		_, err := NewDecoder(capBomb).ReadArrayCount(8, 1)
		if err == nil || !strings.Contains(err.Error(), "caller-supplied max") {
			t.Fatalf("ReadArrayCount caller-cap bomb: err = %v, want caller-cap rejection", err)
		}
	})
	runNoPanic(t, "ReadArrayCountAt caller-cap bomb", func() {
		_, _, err := ReadArrayCountAt(capBomb, 0, 8, 1)
		if err == nil || !strings.Contains(err.Error(), "caller-supplied max") {
			t.Fatalf("ReadArrayCountAt caller-cap bomb: err = %v, want caller-cap rejection", err)
		}
	})

	// Bounded memory: rejecting an array-count bomb allocates a small constant
	// (no backing slice for the claimed element count). maxInt32 int32 elements
	// would be ~8 GiB if allocated; this proves nothing of the sort happens.
	// The rejection builds an error via fmt.Errorf (which allocates), so the
	// ceiling tolerates that plus runtime/-race accounting variance — the point
	// is O(1) allocation, not zero.
	const allocCeiling = 16.0
	if got := testing.AllocsPerRun(200, func() {
		_, _, _ = ReadArrayCountAt(bufBomb, 0, 8, bufBombMax)
	}); got > allocCeiling {
		t.Errorf("ReadArrayCountAt bomb: %.1f allocs/run exceeds ceiling %.1f", got, allocCeiling)
	}
}

// --- depth-bomb scaffolding (uniquely named to avoid collision with
// recNode/encodeRecNode in security_validation_test.go) ---

// bombNode mimics the generated self-referential decode pattern: a thin
// Unmarshal shim delegating to unmarshalAt(data, 0), which checks the depth
// cap on entry and recurses with depth+1 for each nested envelope. This is the
// exact shape generated code uses so the depth-bomb test exercises the real
// MaxDecodeDepth contract.
type bombNode struct {
	Child *bombNode
}

func (m *bombNode) Unmarshal(data []byte) error { return m.unmarshalAt(data, 0) }

func (m *bombNode) unmarshalAt(data []byte, depth int) error {
	if depth > MaxDecodeDepth {
		return ErrMaxDepthExceeded
	}
	dec := NewDecoder(data)
	if dec.EOF() {
		return nil
	}
	childData, err := dec.ReadMessageBytes()
	if err != nil {
		return err
	}
	if len(childData) == 0 {
		return nil
	}
	m.Child = &bombNode{}
	return m.Child.unmarshalAt(childData, depth+1)
}

// encodeBombNest wraps an empty innermost message in `depth` nested
// length-prefixed envelopes.
func encodeBombNest(depth int) []byte {
	enc := NewEncoder(depth + 4)
	inner := []byte{}
	for i := 0; i < depth; i++ {
		enc.Reset()
		enc.WriteMessage(inner)
		inner = append([]byte(nil), enc.Bytes()...)
	}
	return inner
}

// TestBomb_DepthExceeded is the depth bomb: nested message envelopes well past
// MaxDecodeDepth must be rejected with ErrMaxDepthExceeded BEFORE the recursion
// exhausts the stack (a Go stack-overflow is a process-wide fatal that recover
// cannot catch — so "no panic" here also means "no fatal stack growth"). A
// payload nested to exactly MaxDecodeDepth must still be accepted.
func TestBomb_DepthExceeded(t *testing.T) {
	// encodeBombNest(N) emits N envelopes (innermost empty), so the deepest
	// recursion entry is at depth N-1; the cap check `depth > MaxDecodeDepth`
	// fires only once depth reaches MaxDecodeDepth+1, i.e. N >= MaxDecodeDepth+2.
	// So +2 is the minimal "over the cap" payload that must be rejected.
	for _, over := range []int{2, 5, 64, 1000} {
		over := over
		runNoPanic(t, "depth bomb", func() {
			payload := encodeBombNest(MaxDecodeDepth + over)
			var root bombNode
			err := root.Unmarshal(payload)
			if !errors.Is(err, ErrMaxDepthExceeded) {
				t.Fatalf("depth %d (+%d over cap): err = %v, want ErrMaxDepthExceeded",
					MaxDecodeDepth+over, over, err)
			}
		})
	}

	// The boundary-legitimate payloads must still be accepted: encodeBombNest(N)
	// recurses to depth N-1, so both MaxDecodeDepth and MaxDecodeDepth+1
	// envelopes stay at-or-below the cap (deepest entry depth 63 and 64
	// respectively) and must decode cleanly.
	for _, n := range []int{MaxDecodeDepth, MaxDecodeDepth + 1} {
		n := n
		runNoPanic(t, "depth at cap accepted", func() {
			var root bombNode
			if err := root.Unmarshal(encodeBombNest(n)); err != nil {
				t.Fatalf("payload nested to %d envelopes (within cap) rejected: %v", n, err)
			}
		})
	}
}

// runWidth describes one coalesced-run field layout used to exercise
// EnsureRunAt + the matching Run*At reader at a known offset.
type runWidth struct {
	name string
	size int
	// read indexes a run reader at offset p inside a window already covered by
	// EnsureRunAt; it must read in-bounds (no panic) for a full-width buffer.
	read func(b []byte, p int)
}

var bombRunWidths = []runWidth{
	{"bool", 1, func(b []byte, p int) { _ = RunBoolAt(b, p) }},
	{"int32", 4, func(b []byte, p int) { _ = RunInt32At(b, p) }},
	{"uint32", 4, func(b []byte, p int) { _ = RunUint32At(b, p) }},
	{"float32", 4, func(b []byte, p int) { _ = RunFloat32At(b, p) }},
	{"int64", 8, func(b []byte, p int) { _ = RunInt64At(b, p) }},
	{"uint64", 8, func(b []byte, p int) { _ = RunUint64At(b, p) }},
	{"float64", 8, func(b []byte, p int) { _ = RunFloat64At(b, p) }},
}

// TestBomb_TruncatedCoalescedRun broadens the truncated-mid-run coverage: for a
// coalesced run of total width N, a buffer holding fewer than N bytes must be
// rejected by the single up-front EnsureRunAt check — exactly as the per-field
// ReadInt32At/ReadInt64At/ReadBoolAt path would have rejected the first short
// field. It checks every truncation offset from 0..N-1 (reject) and the
// full-width buffer (accept, then every Run*At reads in-bounds without panic).
func TestBomb_TruncatedCoalescedRun(t *testing.T) {
	// Single-field runs: parity with the per-field reader at every truncation.
	for _, w := range bombRunWidths {
		w := w
		t.Run(w.name, func(t *testing.T) {
			for trunc := 0; trunc < w.size; trunc++ {
				buf := make([]byte, trunc)
				if _, err := EnsureRunAt(buf, 0, w.size); !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("EnsureRunAt(len=%d, run=%d): err = %v, want io.ErrUnexpectedEOF",
						trunc, w.size, err)
				}
			}
			// Full-width buffer: EnsureRunAt accepts and returns the post-run
			// offset, and the Run*At reader then reads in-bounds.
			full := make([]byte, w.size)
			np, err := EnsureRunAt(full, 0, w.size)
			if err != nil || np != w.size {
				t.Fatalf("EnsureRunAt(full %s): (%d, %v), want (%d, nil)", w.name, np, err, w.size)
			}
			runNoPanic(t, "run reader in-bounds "+w.name, func() { w.read(full, 0) })
		})
	}

	// A multi-field run (bool + int32 + int64 = 13 bytes): truncating anywhere
	// inside the coalesced window is rejected once, up front, regardless of
	// which field the truncation lands in.
	const multiRun = 1 + 4 + 8
	t.Run("multi-field run", func(t *testing.T) {
		for trunc := 0; trunc < multiRun; trunc++ {
			buf := make([]byte, trunc)
			if _, err := EnsureRunAt(buf, 0, multiRun); !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("EnsureRunAt(len=%d, run=%d): err = %v, want io.ErrUnexpectedEOF",
					trunc, multiRun, err)
			}
		}
		// Full window: accepted, and every field reads in-bounds at its offset.
		full := make([]byte, multiRun)
		if np, err := EnsureRunAt(full, 0, multiRun); err != nil || np != multiRun {
			t.Fatalf("EnsureRunAt(full multi): (%d, %v), want (%d, nil)", np, err, multiRun)
		}
		runNoPanic(t, "multi-run readers in-bounds", func() {
			_ = RunBoolAt(full, 0)
			_ = RunInt32At(full, 1)
			_ = RunInt64At(full, 5)
		})
	})

	// Differential parity: at a truncation that is short for the run, EnsureRunAt
	// and the equivalent first per-field ReadAt must agree on rejection. For a
	// run starting with an int32, a 3-byte buffer is short for both.
	t.Run("ensure-run vs per-field parity", func(t *testing.T) {
		short := []byte{0x01, 0x02, 0x03} // 3 bytes: short for a 4-byte int32 lead
		_, ensureErr := EnsureRunAt(short, 0, 4)
		_, _, fieldErr := ReadInt32At(short, 0)
		if !errors.Is(ensureErr, io.ErrUnexpectedEOF) || !errors.Is(fieldErr, io.ErrUnexpectedEOF) {
			t.Fatalf("parity: EnsureRunAt err=%v, ReadInt32At err=%v, both want io.ErrUnexpectedEOF",
				ensureErr, fieldErr)
		}
	})

	// EnsureRunAt at a non-zero offset (a run that follows an earlier field):
	// truncation relative to the offset is still rejected, full window accepted.
	t.Run("non-zero offset", func(t *testing.T) {
		const lead, run = 5, 8
		short := make([]byte, lead+run-1) // one byte short for the run at offset lead
		if _, err := EnsureRunAt(short, lead, run); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("EnsureRunAt offset=%d run=%d short buf: err = %v, want io.ErrUnexpectedEOF",
				lead, run, err)
		}
		ok := make([]byte, lead+run)
		if np, err := EnsureRunAt(ok, lead, run); err != nil || np != lead+run {
			t.Fatalf("EnsureRunAt offset=%d run=%d full: (%d, %v), want (%d, nil)", lead, run, np, err, lead+run)
		}
	})
}

// bombCorpus returns a set of adversarial "bomb" payloads to drive through the
// fuzz-style harness: huge-length prefixes, array-count bombs, deeply nested
// envelopes, and truncated runs. None may cause a panic when fed to the
// decoder driver.
func bombCorpus() [][]byte {
	corpus := [][]byte{
		hugeLenPrefix(),     // 0xFF + 4GB, no payload
		hugeLenPrefix(0x00), // 0xFF + 4GB, 1 payload byte
		{wire.CompactLengthMarker, 0xFF, 0xFF, 0xFF, 0x7F}, // ~2GiB, no payload
		encodeBombNest(MaxDecodeDepth + 10),                // over-deep nest
		encodeBombNest(MaxDecodeDepth),                     // at-cap nest
	}
	// Array-count bomb: max int32 count, no body.
	enc := NewEncoder(8)
	enc.WriteInt32(int32(^uint32(0) >> 1))
	corpus = append(corpus, append([]byte(nil), enc.Bytes()...))
	// A buffer truncated mid-run after a couple of valid scalars.
	enc2 := NewEncoder(16)
	enc2.WriteInt32(7)
	enc2.WriteBool(true)
	full := enc2.Bytes()
	corpus = append(corpus, full[:len(full)-1]) // drop the last byte
	return corpus
}

// TestBomb_FuzzStyleCorpus drives the adversarial bomb corpus through the same
// driveDecoder harness FuzzDecodeAll uses, asserting the decoder NEVER panics
// on any of these resource-exhaustion payloads. This also folds the bomb
// inputs into the explicit (non-fuzz) test run so they execute under
// `make verify` without requiring a fuzzing pass.
func TestBomb_FuzzStyleCorpus(t *testing.T) {
	for i, data := range bombCorpus() {
		data := data
		runNoPanic(t, "bomb corpus drive", func() {
			driveDecoder(data)
		})
		_ = i
	}
}

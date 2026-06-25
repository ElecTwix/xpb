package utekabench

import (
	"testing"

	"github.com/ElecTwix/xpb/benchmarks/go/uteka/val"
	"github.com/ElecTwix/xpb/runtime/go/xpb"
)

// These are zero-alloc GATES expressed as NORMAL tests (not benchmarks), so they
// run under plain `go test` and deterministically fail CI the moment a 0->N
// allocation regression sneaks into the value-style decode or the pooled encode
// path. testing.AllocsPerRun averages the allocations over many iterations; a
// hot path that boxes a value or grows a buffer would push the average above 0.

// TestZeroAlloc_ValDecode asserts that decoding the realistic RPC message into a
// reused value-style struct (value optionals + zero-copy bytes) performs zero
// heap allocations. This is the core promise of --go-optional-style=value
// --go-zero-copy-bytes: no per-present-field pointer boxing and no payload copy.
func TestZeroAlloc_ValDecode(t *testing.T) {
	data, err := sampleVal().Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Decode into a buffer copy that outlives the run, since zero-copy decode
	// aliases it; reusing one target struct keeps the struct itself off the
	// per-iteration allocation budget.
	var m val.UtekaMessage
	allocs := testing.AllocsPerRun(1000, func() {
		m = val.UtekaMessage{}
		if err := m.Unmarshal(data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
	})
	if allocs != 0 {
		t.Fatalf("value-style decode allocs/run = %v, want 0 (allocation regression)", allocs)
	}
	// Sanity: the decode actually populated the fields (guards against a
	// short-circuit that would trivially be zero-alloc).
	if !m.HasMethod || m.Method != method {
		t.Fatalf("decode did not populate Method: has=%v val=%q", m.HasMethod, m.Method)
	}
}

// TestZeroAlloc_PooledEncode asserts that encoding via the encoder pool
// (GetEncoder + MarshalTo + Bytes + PutEncoder) performs zero heap allocations
// in steady state. The first few iterations may grow the pooled buffer, but
// AllocsPerRun's averaging over 1000 runs makes any per-call allocation visible
// while tolerating the one-time warmup absorbed by the pool.
func TestZeroAlloc_PooledEncode(t *testing.T) {
	m := sampleVal()
	// Warm the pool so the backing buffer is already large enough; the measured
	// runs must then allocate nothing.
	enc := xpb.GetEncoder()
	m.MarshalTo(enc)
	xpb.PutEncoder(enc)

	allocs := testing.AllocsPerRun(1000, func() {
		e := xpb.GetEncoder()
		m.MarshalTo(e)
		_ = e.Bytes()
		xpb.PutEncoder(e)
	})
	if allocs != 0 {
		t.Fatalf("pooled encode allocs/run = %v, want 0 (allocation regression)", allocs)
	}
}

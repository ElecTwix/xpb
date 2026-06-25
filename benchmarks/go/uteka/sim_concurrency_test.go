package utekabench

import (
	"bytes"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"unsafe"

	"github.com/ElecTwix/xpb/benchmarks/go/uteka/val"
	"github.com/ElecTwix/xpb/runtime/go/xpb"
)

// This file adds a SUSTAINED concurrency stress simulation over the two pieces
// of shared mutable / shared-aliased state in the Go runtime + codegen:
//
//  1. the sync.Pool-backed encoder pool (xpb.GetEncoder / xpb.PutEncoder), and
//  2. the zero-copy value-style decode path, where many decoded structs may
//     alias one underlying buffer (read-only fan-out).
//
// Unlike the single-shot TestRace_PooledEncodeAndZeroCopyDecode in race_test.go,
// these tests churn the pool from many goroutines for a bounded ITERATION count
// (never a wall-clock duration) so they are CI-stable and flake-free, and they
// run under the Makefile `test-race` target (`go test -race
// ./benchmarks/go/uteka/...`). Everything here is self-contained — it does not
// reuse helpers from the other test files, so it cannot collide with parallel
// edits to them.

// simConcurrencyWorkers is the goroutine fan-out: GOMAXPROCS*4, the canonical
// "more goroutines than cores" oversubscription that maximises scheduler churn
// (and therefore pool Get/Put contention) under the race detector.
func simConcurrencyWorkers() int {
	n := runtime.GOMAXPROCS(0) * 4
	if n < 4 {
		n = 4
	}
	return n
}

// simConcurrencyMessages returns a small table of DISTINCT messages so that
// successive pool Get/MarshalTo/Put cycles encode different lengths and field
// sets rather than re-encoding one fixed shape. Varying the message across pool
// cycles means a reused pooled encoder must correctly produce the right bytes
// for whatever message it next serves — cross-goroutine corruption of the shared
// pooled buffer would surface as both a -race report and a byte mismatch against
// the canonical encode. Each message is independently owned (no shared backing
// slices) so goroutines can read the table concurrently without any write hazard.
func simConcurrencyMessages() []*val.UtekaMessage {
	return []*val.UtekaMessage{
		{
			Type: 1, Id: "req-0001",
			Method: "market.subscribe", HasMethod: true,
			Payload: []byte(`{"topic":"prices"}`), HasPayload: true,
			Timestamp: 1735128000000,
			SessionId: "sess_a", HasSessionId: true,
		},
		{
			Type: 2, Id: "req-0002-longer-identifier",
			Payload: []byte(`{"user":"alice","action":"subscribe","topic":"prices","limit":100}`), HasPayload: true,
			Timestamp: 1735128000123,
			Seq:       42, Flags: 7,
		},
		{
			// No optionals present at all — shortest encoding.
			Type: 3, Id: "x", Timestamp: 1,
		},
		{
			Type: 4, Id: "req-0004",
			Method: "stream.open", HasMethod: true,
			Error: "rate limited", HasError: true,
			StreamId: "stream-deadbeef", HasStreamId: true,
			Timestamp: 1735128000999,
			Seq:       9001, Flags: 255,
			SessionId: "sess_d_with_a_rather_long_value_to_vary_length", HasSessionId: true,
		},
	}
}

// simConcurrencyAliases reports whether the backing array of `sub` lies entirely
// within the backing array of `buf` — i.e. `sub` was produced by aliasing `buf`
// rather than by copying. This makes the zero-copy property an explicit,
// checkable assertion: if the value-style decode ever regressed from
// ReadBytesUnsafeAt (aliasing) to a copying read, this returns false even though
// the decoded bytes would still compare equal.
func simConcurrencyAliases(sub, buf []byte) bool {
	if len(sub) == 0 || len(buf) == 0 {
		return false
	}
	bufStart := uintptr(unsafe.Pointer(unsafe.SliceData(buf)))
	bufEnd := bufStart + uintptr(len(buf))
	subStart := uintptr(unsafe.Pointer(unsafe.SliceData(sub)))
	subEnd := subStart + uintptr(len(sub))
	return subStart >= bufStart && subEnd <= bufEnd
}

// simConcurrencyEqual compares two decoded messages field-for-field. Used to
// prove that each produced buffer round-trips back to the exact source message.
func simConcurrencyEqual(got, want *val.UtekaMessage) bool {
	if got.Type != want.Type || got.Id != want.Id || got.Timestamp != want.Timestamp {
		return false
	}
	if got.HasMethod != want.HasMethod || (got.HasMethod && got.Method != want.Method) {
		return false
	}
	if got.HasPayload != want.HasPayload || (got.HasPayload && !bytes.Equal(got.Payload, want.Payload)) {
		return false
	}
	if got.HasError != want.HasError || (got.HasError && got.Error != want.Error) {
		return false
	}
	if got.HasStreamId != want.HasStreamId || (got.HasStreamId && got.StreamId != want.StreamId) {
		return false
	}
	if got.Seq != want.Seq || got.Flags != want.Flags {
		return false
	}
	if got.HasSessionId != want.HasSessionId || (got.HasSessionId && got.SessionId != want.SessionId) {
		return false
	}
	return true
}

// TestSimConcurrency_PoolChurn runs GOMAXPROCS*4 goroutines, each looping a
// bounded number of iterations:
//
//	GetEncoder -> MarshalTo(random message) -> use bytes -> PutEncoder
//
// It asserts (a) no data race — the pool must hand each goroutine an independent
// encoder and reused buffers must not bleed across goroutines — and (b) that
// every produced buffer decodes back to exactly the message that produced it.
//
// The loop is bounded purely by ITERATION COUNT (not wall-clock), and each
// goroutine uses its own deterministically-seeded RNG, so the test is
// reproducible and CI-stable.
func TestSimConcurrency_PoolChurn(t *testing.T) {
	const iterations = 2000

	msgs := simConcurrencyMessages()

	// Precompute the canonical wire bytes for each message once, single-threaded,
	// so the concurrent section only READS these immutable slices when checking
	// that a pooled encode produced the identical bytes.
	canonical := make([][]byte, len(msgs))
	for i, m := range msgs {
		b, err := m.Marshal()
		if err != nil {
			t.Fatalf("precompute marshal[%d]: %v", i, err)
		}
		canonical[i] = b
	}

	workers := simConcurrencyWorkers()
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(seed) + 1))
			for i := 0; i < iterations; i++ {
				idx := rng.Intn(len(msgs))
				src := msgs[idx]

				// Pooled encode.
				enc := xpb.GetEncoder()
				src.MarshalTo(enc)
				encoded := enc.Bytes()

				// The pooled encode must match the canonical single-threaded
				// encode of the same message (no stale-byte bleed from a reused
				// buffer, no cross-goroutine corruption).
				if !bytes.Equal(encoded, canonical[idx]) {
					t.Errorf("worker %d iter %d: pooled encode of msg %d mismatch:\n got=%x\nwant=%x",
						seed, i, idx, encoded, canonical[idx])
				}

				// Copy the bytes out into a goroutine-private buffer BEFORE
				// returning the encoder, because the pooled buffer may be reused
				// by another goroutine immediately after Put.
				buf := append([]byte(nil), encoded...)
				xpb.PutEncoder(enc)

				// Decode the private buffer and assert a full field round-trip.
				var dec val.UtekaMessage
				if err := dec.Unmarshal(buf); err != nil {
					t.Errorf("worker %d iter %d: unmarshal msg %d: %v", seed, i, idx, err)
					continue
				}
				if !simConcurrencyEqual(&dec, src) {
					t.Errorf("worker %d iter %d: round-trip of msg %d did not match source", seed, i, idx)
				}
			}
		}(w)
	}
	wg.Wait()
}

// TestSimConcurrency_ReaderFanout proves the zero-copy decode path is safe under
// a read-only fan-out: ONE shared buffer is decoded concurrently by many
// goroutines. Because the value-style decode aliases the input buffer
// (ReadBytesUnsafeAt), every decoded Payload slice points into the SAME shared
// backing array. As long as nobody writes, concurrent reads of that array must
// be race-free, and every goroutine must observe the same correct values.
//
// Bounded by iteration count (not wall-clock) so it is CI-stable.
func TestSimConcurrency_ReaderFanout(t *testing.T) {
	const iterations = 2000

	// The single message whose buffer every reader will alias. It carries a
	// distinctive Payload so we can assert the zero-copy slice is correct.
	src := &val.UtekaMessage{
		Type: 7, Id: "shared-buffer-id",
		Method: "fanout.read", HasMethod: true,
		Payload:    []byte(`{"shared":"read-only","aliased":true}`),
		HasPayload: true,
		Timestamp:  1735128001234,
		Seq:        1234, Flags: 9,
		SessionId: "sess_shared", HasSessionId: true,
	}

	// One shared, immutable buffer. It is written exactly once here, before any
	// reader goroutine starts, and is never mutated afterwards.
	shared, err := src.Marshal()
	if err != nil {
		t.Fatalf("marshal shared buffer: %v", err)
	}

	// Prove the zero-copy contract single-threaded BEFORE the fan-out: the
	// decoded Payload must alias the shared buffer, not be a copy of it. This is
	// the precondition that makes the fan-out a true "many readers alias one
	// buffer" scenario. If the value-style decode ever stopped aliasing, this
	// fails even though the byte contents would still match.
	var probe val.UtekaMessage
	if err := probe.Unmarshal(shared); err != nil {
		t.Fatalf("probe unmarshal: %v", err)
	}
	if !probe.HasPayload || !simConcurrencyAliases(probe.Payload, shared) {
		t.Fatalf("decoded Payload does not alias the shared buffer; zero-copy fan-out precondition violated")
	}

	readers := simConcurrencyWorkers()
	var wg sync.WaitGroup
	wg.Add(readers)
	for r := 0; r < readers; r++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// Each reader decodes the SAME shared buffer into its OWN struct.
				// The decoded Payload aliases `shared`; many goroutines therefore
				// concurrently READ one backing array. -race must report nothing.
				var dec val.UtekaMessage
				if err := dec.Unmarshal(shared); err != nil {
					t.Errorf("reader %d iter %d: unmarshal: %v", id, i, err)
					continue
				}
				if !simConcurrencyEqual(&dec, src) {
					t.Errorf("reader %d iter %d: decoded values do not match source", id, i)
				}
				// Touch the aliased payload bytes (a pure read) to make any
				// concurrent-read hazard on the shared backing array observable
				// to the race detector, and confirm this reader's Payload really
				// does alias the shared buffer (zero-copy), not a private copy.
				if !dec.HasPayload || !bytes.Equal(dec.Payload, src.Payload) {
					t.Errorf("reader %d iter %d: aliased payload mismatch", id, i)
				}
				if !simConcurrencyAliases(dec.Payload, shared) {
					t.Errorf("reader %d iter %d: Payload did not alias shared buffer (zero-copy broken)", id, i)
				}
			}
		}(r)
	}
	wg.Wait()

	// The shared buffer must be unchanged after the read-only fan-out: re-decode
	// once more and confirm it still round-trips.
	var final val.UtekaMessage
	if err := final.Unmarshal(shared); err != nil {
		t.Fatalf("post-fanout unmarshal: %v", err)
	}
	if !simConcurrencyEqual(&final, src) {
		t.Fatalf("shared buffer changed during read-only fan-out")
	}
}

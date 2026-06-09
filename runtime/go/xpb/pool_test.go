package xpb

import (
	"sync"
	"testing"
)

// TestPool_RecycledEncoderIsReset verifies that an encoder fetched from the
// pool never carries leftover bytes from a previous user. We force the same
// encoder object back through the pool and assert GetEncoder hands it back
// reset (Len == 0). This is the property an attacker / sloppy caller would
// otherwise exploit to leak prior message bytes.
func TestPool_RecycledEncoderIsReset(t *testing.T) {
	e1 := GetEncoder()
	e1.WriteString("leftover secret data")
	e1.WriteInt64(0xDEADBEEF)
	if e1.Len() == 0 {
		t.Fatal("setup: encoder should have data")
	}
	PutEncoder(e1)

	// Drain a handful of Gets; whichever object we receive must be empty.
	for i := 0; i < 8; i++ {
		e := GetEncoder()
		if e.Len() != 0 {
			t.Fatalf("recycled encoder not reset: Len = %d, bytes = %x", e.Len(), e.Bytes())
		}
		// Re-dirty and return so the pooled object keeps cycling.
		e.WriteInt32(int32(i))
		PutEncoder(e)
	}
}

// TestPool_ConcurrentGetPut hammers the pool from many goroutines doing
// Get -> encode -> Put. It must run clean under -race (no data races on the
// pool or the encoders) and every encoder must start empty.
func TestPool_ConcurrentGetPut(t *testing.T) {
	const goroutines = 64
	const iterations = 2000

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				e := GetEncoder()
				if e.Len() != 0 {
					// t.Error is goroutine-safe; report and bail this iteration.
					t.Errorf("g%d iter%d: encoder from pool not empty (Len=%d)", id, i, e.Len())
				}
				e.WriteInt32(int32(id))
				e.WriteInt64(int64(i))
				e.WriteString("payload")

				// Verify the data we wrote decodes back correctly (sanity that the
				// buffer wasn't shared/corrupted across goroutines).
				dec := NewDecoder(e.Bytes())
				if v, err := dec.ReadInt32(); err != nil || v != int32(id) {
					t.Errorf("g%d iter%d: int32 got %d err %v", id, i, v, err)
				}
				if v, err := dec.ReadInt64(); err != nil || v != int64(i) {
					t.Errorf("g%d iter%d: int64 got %d err %v", id, i, v, err)
				}
				if v, err := dec.CloneString(); err != nil || v != "payload" {
					t.Errorf("g%d iter%d: string got %q err %v", id, i, v, err)
				}

				PutEncoder(e)
			}
		}(g)
	}
	wg.Wait()
}

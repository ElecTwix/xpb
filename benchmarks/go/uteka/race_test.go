package utekabench

import (
	"bytes"
	"sync"
	"testing"

	"github.com/ElecTwix/xpb/benchmarks/go/uteka/val"
	"github.com/ElecTwix/xpb/runtime/go/xpb"
)

// TestRace_PooledEncodeAndZeroCopyDecode hammers the encoder pool and the
// zero-copy value-style decode path from many goroutines at once. Run with
// `go test -race ./benchmarks/go/uteka/...` it must report no data races: the
// pool must hand each goroutine an independent encoder, and each goroutine's
// zero-copy decode must alias only its own private buffer (no shared mutable
// state across goroutines).
func TestRace_PooledEncodeAndZeroCopyDecode(t *testing.T) {
	const goroutines = 32
	const iterations = 500

	// A canonical message every goroutine encodes; each goroutine owns its own
	// decode buffer so the zero-copy aliasing never crosses goroutines.
	base := sampleVal()
	canonical, err := base.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// Pooled encode.
				enc := xpb.GetEncoder()
				base.MarshalTo(enc)
				encoded := enc.Bytes()
				if !bytes.Equal(encoded, canonical) {
					t.Errorf("g%d iter%d: pooled encode mismatch", id, i)
				}
				// Copy out before returning the encoder to the pool, since the
				// pooled buffer may be reused by another goroutine.
				buf := append([]byte(nil), encoded...)
				xpb.PutEncoder(enc)

				// Zero-copy decode into a private struct over a private buffer.
				var m val.UtekaMessage
				if err := m.Unmarshal(buf); err != nil {
					t.Errorf("g%d iter%d: unmarshal: %v", id, i, err)
					continue
				}
				if !m.HasPayload || !bytes.Equal(m.Payload, []byte(payload)) {
					t.Errorf("g%d iter%d: payload mismatch", id, i)
				}
				// Mutate the private buffer; the aliased payload must follow,
				// proving zero-copy, and the race detector must see no sharing.
				if len(buf) > 0 {
					off := bytes.Index(buf, []byte(payload))
					if off >= 0 {
						buf[off] ^= 0xFF
						if m.Payload[0] != (byte(payload[0]) ^ 0xFF) {
							t.Errorf("g%d iter%d: zero-copy alias broken", id, i)
						}
					}
				}
			}
		}(g)
	}
	wg.Wait()
}

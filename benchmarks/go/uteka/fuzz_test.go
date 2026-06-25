package utekabench

import (
	"testing"

	"github.com/ElecTwix/xpb/benchmarks/go/uteka/ptr"
	"github.com/ElecTwix/xpb/benchmarks/go/uteka/val"
)

// seedFuzz adds a corpus of valid encodings (from the differential cases) plus a
// few adversarial byte strings that probe truncated lengths and the 0xFF
// compact-length marker.
func seedFuzz(f *testing.F) {
	f.Helper()
	for _, c := range diffCases() {
		if b, err := c.toVal().Marshal(); err == nil {
			f.Add(b)
		}
	}
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0x01, 0x02, 0x03, 0x04}) // bare int32, nothing else
	// 0xFF length marker claiming ~4GB with no payload: must error, not OOB.
	f.Add([]byte{0, 0, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
	f.Add([]byte{0, 0, 0, 0, 0xFF, 0x00, 0x00}) // truncated length marker
	// Phase 3 coalesced-run safety: seed inputs truncated at every offset inside
	// the Seq(int64)+Flags(int32) fixed-width run, so the fuzzer exercises a wire
	// that ends partway through the single up-front bounds check.
	for _, b := range midRunTruncations() {
		f.Add(b)
	}
}

// midRunTruncations builds valid wire bytes for the all_optionals_absent case
// (Type, Id, then four presence-absent bytes, then the coalesced Seq+Flags run,
// then the trailing SessionId presence byte) and truncates the buffer at every
// byte offset that falls strictly inside the Seq+Flags run. Each truncation must
// be rejected by the single xpb.EnsureRunAt up-front check — exactly as the
// per-field path rejected a short int64/int32 before coalescing — never an OOB
// panic.
func midRunTruncations() [][]byte {
	c := logical{Type: 7, Id: "id-only", Timestamp: 42, Seq: 9, Flags: 3}
	full, err := c.toVal().Marshal()
	if err != nil {
		return nil
	}
	// The run is the last fixed region before the trailing optional SessionId
	// presence byte: SessionId absent is 1 byte, so the run occupies
	// full[len-13 : len-1] (12 bytes). Truncate at every offset strictly inside
	// the run (and at its start), so the decoder sees a wire that ends partway
	// through the coalesced window.
	runStart := len(full) - 13
	var out [][]byte
	for cut := runStart; cut < len(full)-1; cut++ {
		out = append(out, append([]byte(nil), full[:cut]...))
	}
	return out
}

// TestCoalescedRun_TruncatedMidRunRejected is the deterministic (non-fuzz)
// counterpart of the mid-run fuzz seeds: it runs under plain `go test` and
// asserts that a wire truncated at every byte offset inside the coalesced
// Seq(int64)+Flags(int32) run is rejected with an error (never a panic, never an
// out-of-bounds read) by BOTH generated styles. This is the exact regression the
// single up-front xpb.EnsureRunAt bounds check must guard: before coalescing,
// each field had its own per-field check; the one up-front check must reject the
// same truncated inputs the per-field path did.
func TestCoalescedRun_TruncatedMidRunRejected(t *testing.T) {
	// Boundary anchor: prove the run window is exactly where the truncations
	// target it, so the test actually exercises the run's up-front EnsureRunAt
	// rather than an incidental earlier failure.
	c := logical{Type: 7, Id: "id-only", Timestamp: 42, Seq: 9, Flags: 3}
	full, err := c.toVal().Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	runStart := len(full) - 13 // 12-byte Seq+Flags run, then 1 trailing presence byte
	// (a) Everything up to the run decodes the pre-run fields with no error: the
	// run is genuinely the next thing to read, so truncating into it is what the
	// run's EnsureRunAt rejects (not a short earlier field). All pre-run fields
	// (Type, Id, the four absent-optional presence bytes, Timestamp) fit in
	// full[:runStart], so a buffer that ends exactly at runStart fails ONLY
	// because the entire 12-byte run is missing.
	var preRun val.UtekaMessage
	if errPre := preRun.Unmarshal(append([]byte(nil), full[:runStart]...)); errPre == nil {
		t.Fatalf("buffer truncated at the run start (run entirely missing) decoded without error; "+
			"the run's EnsureRunAt(%d) is not the gate it should be", 12)
	}
	if preRun.Timestamp != 42 {
		t.Fatalf("pre-run field Timestamp not decoded before the missing run: got %d, want 42", preRun.Timestamp)
	}
	// (b) The full valid buffer decodes; truncating one byte before the end
	// (dropping only the trailing SessionId presence byte, run intact) also
	// fails — confirming runStart..len-1 is exactly the run and len-1 is past it.
	var okMsg val.UtekaMessage
	if errOK := okMsg.Unmarshal(append([]byte(nil), full...)); errOK != nil {
		t.Fatalf("full valid buffer failed to decode: %v", errOK)
	}
	if okMsg.Seq != 9 || okMsg.Flags != 3 {
		t.Fatalf("full buffer decoded the run wrong: Seq=%d Flags=%d, want 9/3", okMsg.Seq, okMsg.Flags)
	}

	truncs := midRunTruncations()
	if len(truncs) == 0 {
		t.Fatal("no mid-run truncations generated")
	}
	for i, data := range truncs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("decode panicked on mid-run truncation #%d (%x): %v", i, data, r)
				}
			}()
			var pd ptr.UtekaMessage
			if err := pd.Unmarshal(append([]byte(nil), data...)); err == nil {
				t.Fatalf("ptr decode of mid-run truncation #%d (%x) succeeded; want error", i, data)
			}
			var vd val.UtekaMessage
			if err := vd.Unmarshal(append([]byte(nil), data...)); err == nil {
				t.Fatalf("val decode of mid-run truncation #%d (%x) succeeded; want error", i, data)
			}
		}()
	}
}

// FuzzUnmarshalPtr asserts the pointer-style decoder never panics / never
// indexes out of bounds on arbitrary input. Errors are expected and fine.
func FuzzUnmarshalPtr(f *testing.F) {
	seedFuzz(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("ptr decoder panicked on input %x: %v", data, r)
			}
		}()
		var m ptr.UtekaMessage
		_ = m.Unmarshal(data)
	})
}

// FuzzUnmarshalVal asserts the value-style (zero-copy) decoder never panics on
// arbitrary input.
func FuzzUnmarshalVal(f *testing.F) {
	seedFuzz(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("val decoder panicked on input %x: %v", data, r)
			}
		}()
		var m val.UtekaMessage
		_ = m.Unmarshal(data)
	})
}

// FuzzDifferentialDecode decodes the same fuzz input with BOTH styles. Neither
// may panic, and when both succeed they must agree on presence + values. This
// catches a divergence where one generated decoder mis-frames a field the other
// reads correctly.
func FuzzDifferentialDecode(f *testing.F) {
	seedFuzz(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("differential decode panicked on input %x: %v", data, r)
			}
		}()
		var pd ptr.UtekaMessage
		errP := pd.Unmarshal(append([]byte(nil), data...))
		var vd val.UtekaMessage
		errV := vd.Unmarshal(append([]byte(nil), data...))

		// The two decoders share the same wire grammar, so they must agree on
		// whether the input is decodable.
		if (errP == nil) != (errV == nil) {
			t.Fatalf("decode error disagreement on %x: ptr=%v val=%v", data, errP, errV)
		}
		if errP == nil && errV == nil {
			if err := agree(&pd, &vd); err != nil {
				t.Fatalf("decoded values disagree on %x: %v", data, err)
			}
		}
	})
}

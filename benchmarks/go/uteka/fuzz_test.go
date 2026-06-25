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

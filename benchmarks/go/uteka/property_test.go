package utekabench

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/ElecTwix/xpb/benchmarks/go/uteka/ptr"
	"github.com/ElecTwix/xpb/benchmarks/go/uteka/val"
)

// randLogical builds a random-but-valid logical message. String/byte lengths
// straddle the 254/255 compact-length boundary so the property loop exercises
// both the 1-byte and 0xFF 4-byte length encodings. Optional presence is random
// per field, including present-but-empty.
func randLogical(rng *rand.Rand) logical {
	randStr := func() string {
		// Bias toward small lengths but reach past the 254 boundary regularly.
		n := rng.Intn(260)
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		return string(b)
	}
	randBytes := func() []byte {
		n := rng.Intn(260)
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		return b
	}
	l := logical{
		Type:      int32(rng.Uint32()),
		Id:        randStr(),
		Timestamp: int64(rng.Uint64()),
		Seq:       int64(rng.Uint64()),
		Flags:     int32(rng.Uint32()),
	}
	if rng.Intn(2) == 0 {
		l.Method, l.HasMethod = randStr(), true
	}
	if rng.Intn(2) == 0 {
		l.Payload, l.HasPayload = randBytes(), true
	}
	if rng.Intn(2) == 0 {
		l.Error, l.HasError = randStr(), true
	}
	if rng.Intn(2) == 0 {
		l.StreamId, l.HasStreamId = randStr(), true
	}
	if rng.Intn(2) == 0 {
		l.SessionId, l.HasSessionId = randStr(), true
	}
	return l
}

// TestProperty_PtrRoundTrip generates random valid messages, Marshals then
// Unmarshals via the pointer style, and asserts a deep round-trip. Mirrors the
// runtime/go/xpb/property_test.go approach, applied to the generated types.
func TestProperty_PtrRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(0xA11CE))
	for iter := 0; iter < 2000; iter++ {
		c := randLogical(rng)
		in := c.toPtr()
		data, err := in.Marshal()
		if err != nil {
			t.Fatalf("iter %d marshal: %v", iter, err)
		}
		var out ptr.UtekaMessage
		if err := out.Unmarshal(append([]byte(nil), data...)); err != nil {
			t.Fatalf("iter %d unmarshal: %v", iter, err)
		}
		if !ptrEqual(in, &out) {
			t.Fatalf("iter %d (%+v): ptr round-trip mismatch\n in =%+v\n out=%+v", iter, c, *in, out)
		}
	}
}

// TestProperty_ValRoundTrip is the value-style analogue.
func TestProperty_ValRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(0xB0B))
	for iter := 0; iter < 2000; iter++ {
		c := randLogical(rng)
		in := c.toVal()
		data, err := in.Marshal()
		if err != nil {
			t.Fatalf("iter %d marshal: %v", iter, err)
		}
		// Decode into a buffer copy because value-style decode aliases bytes
		// fields; the copy keeps the round-trip self-contained.
		var out val.UtekaMessage
		if err := out.Unmarshal(append([]byte(nil), data...)); err != nil {
			t.Fatalf("iter %d unmarshal: %v", iter, err)
		}
		if !valEqual(in, &out) {
			t.Fatalf("iter %d (%+v): val round-trip mismatch\n in =%+v\n out=%+v", iter, c, *in, out)
		}
	}
}

func ptrEqual(a, b *ptr.UtekaMessage) bool {
	if a.Type != b.Type || a.Id != b.Id || a.Timestamp != b.Timestamp || a.Seq != b.Seq || a.Flags != b.Flags {
		return false
	}
	if !optStrEqual(a.Method, b.Method) || !optStrEqual(a.Error, b.Error) ||
		!optStrEqual(a.StreamId, b.StreamId) || !optStrEqual(a.SessionId, b.SessionId) {
		return false
	}
	return optBytesEqual(a.Payload, b.Payload)
}

func optStrEqual(a, b *string) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	return a == nil || *a == *b
}

func optBytesEqual(a, b *[]byte) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	return a == nil || bytes.Equal(*a, *b)
}

func valEqual(a, b *val.UtekaMessage) bool {
	if a.Type != b.Type || a.Id != b.Id || a.Timestamp != b.Timestamp || a.Seq != b.Seq || a.Flags != b.Flags {
		return false
	}
	if a.HasMethod != b.HasMethod || (a.HasMethod && a.Method != b.Method) {
		return false
	}
	if a.HasPayload != b.HasPayload || (a.HasPayload && !bytes.Equal(a.Payload, b.Payload)) {
		return false
	}
	if a.HasError != b.HasError || (a.HasError && a.Error != b.Error) {
		return false
	}
	if a.HasStreamId != b.HasStreamId || (a.HasStreamId && a.StreamId != b.StreamId) {
		return false
	}
	if a.HasSessionId != b.HasSessionId || (a.HasSessionId && a.SessionId != b.SessionId) {
		return false
	}
	return true
}

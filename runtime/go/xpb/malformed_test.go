package xpb

import (
	"errors"
	"io"
	"testing"
)

// runNoPanic runs fn and fails the test if it panics. It returns the panic
// value (nil if none) so callers can also assert "old behavior would panic".
func runNoPanic(t *testing.T, name string, fn func()) (panicked any) {
	t.Helper()
	defer func() {
		panicked = recover()
		if panicked != nil {
			t.Fatalf("%s: panicked (want clean error): %v", name, panicked)
		}
	}()
	fn()
	return nil
}

func TestMalformed_TruncatedLengthMarker(t *testing.T) {
	// 0xFF marker says "4-byte length follows" but fewer than 4 bytes remain.
	cases := [][]byte{
		{0xFF},
		{0xFF, 0x01},
		{0xFF, 0x01, 0x02},
		{0xFF, 0x01, 0x02, 0x03},
	}
	for _, data := range cases {
		runNoPanic(t, "ReadString truncated marker", func() {
			dec := NewDecoder(data)
			_, err := dec.ReadString()
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("ReadString(%x) err = %v, want ErrUnexpectedEOF", data, err)
			}
		})
		runNoPanic(t, "ReadBytes truncated marker", func() {
			dec := NewDecoder(data)
			_, err := dec.ReadBytes()
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("ReadBytes(%x) err = %v, want ErrUnexpectedEOF", data, err)
			}
		})
	}
}

func TestMalformed_LengthLargerThanBuffer(t *testing.T) {
	// Compact length 10 but only 1 payload byte present.
	data := []byte{0x0A, 0x41}
	runNoPanic(t, "ReadString oversized len", func() {
		dec := NewDecoder(data)
		_, err := dec.ReadString()
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("ReadString err = %v, want ErrUnexpectedEOF", err)
		}
	})
	runNoPanic(t, "CloneString oversized len", func() {
		dec := NewDecoder(data)
		_, err := dec.CloneString()
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("CloneString err = %v, want ErrUnexpectedEOF", err)
		}
	})
	runNoPanic(t, "ReadBytes oversized len", func() {
		dec := NewDecoder(data)
		_, err := dec.ReadBytes()
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("ReadBytes err = %v, want ErrUnexpectedEOF", err)
		}
	})
	runNoPanic(t, "ReadBytesUnsafe oversized len", func() {
		dec := NewDecoder(data)
		_, err := dec.ReadBytesUnsafe()
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("ReadBytesUnsafe err = %v, want ErrUnexpectedEOF", err)
		}
	})
	runNoPanic(t, "ReadMessageBytes oversized len", func() {
		dec := NewDecoder(data)
		_, err := dec.ReadMessageBytes()
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("ReadMessageBytes err = %v, want ErrUnexpectedEOF", err)
		}
	})
}

// TestMalformed_HugeLengthPrefixNoOOM feeds a ~4GB length prefix with an
// almost-empty buffer. The decoder must return an error, NOT attempt to
// make([]byte, 4GB) (OOM) or panic. This guards against alloc-before-bounds.
func TestMalformed_HugeLengthPrefixNoOOM(t *testing.T) {
	// 0xFF marker + length 0xFFFFFFFE (~4GB) little-endian, then 1 payload byte.
	data := []byte{0xFF, 0xFE, 0xFF, 0xFF, 0xFF, 0x00}
	runNoPanic(t, "ReadBytes huge len", func() {
		dec := NewDecoder(data)
		_, err := dec.ReadBytes()
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("ReadBytes huge len err = %v, want ErrUnexpectedEOF", err)
		}
	})
	runNoPanic(t, "ReadString huge len", func() {
		dec := NewDecoder(data)
		_, err := dec.ReadString()
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("ReadString huge len err = %v, want ErrUnexpectedEOF", err)
		}
	})
}

func TestMalformed_ReadPastEOF(t *testing.T) {
	// Buffer with exactly 3 bytes: every fixed-width read of >=4 must error.
	data := []byte{0x01, 0x02, 0x03}
	runNoPanic(t, "reads past EOF", func() {
		dec := NewDecoder(data)
		if _, err := dec.ReadInt32(); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("ReadInt32 err = %v, want ErrUnexpectedEOF", err)
		}
		if _, err := dec.ReadInt64(); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("ReadInt64 err = %v, want ErrUnexpectedEOF", err)
		}
		if _, err := dec.ReadFloat32(); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("ReadFloat32 err = %v, want ErrUnexpectedEOF", err)
		}
		if _, err := dec.ReadFloat64(); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("ReadFloat64 err = %v, want ErrUnexpectedEOF", err)
		}
	})
}

func TestMalformed_RepeatedReadsAfterEOF(t *testing.T) {
	data := []byte{0xAA} // 1 byte
	runNoPanic(t, "repeated reads after EOF", func() {
		dec := NewDecoder(data)
		if _, err := dec.ReadBool(); err != nil {
			t.Fatalf("first ReadBool err = %v, want nil", err)
		}
		// Now at EOF. Hammer reads; each must return an error, never panic.
		for i := 0; i < 100; i++ {
			if _, err := dec.ReadBool(); !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("ReadBool after EOF err = %v, want ErrUnexpectedEOF", err)
			}
			if _, err := dec.ReadInt32(); !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("ReadInt32 after EOF err = %v, want ErrUnexpectedEOF", err)
			}
			if _, err := dec.ReadString(); !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("ReadString after EOF err = %v, want ErrUnexpectedEOF", err)
			}
		}
	})
}

// TestMalformed_NegativeSkip is the regression test for the Skip negative-n bug.
//
// Before the fix, Skip(-4) passed the `d.pos+n > len(d.buf)` check (because a
// negative n makes the sum SMALLER), set d.pos = -4, and the next read indexed
// d.buf[-4:] and panicked. After the fix, Skip rejects n < 0 with an error and
// leaves pos untouched, so subsequent reads behave normally.
func TestMalformed_NegativeSkip(t *testing.T) {
	enc := NewEncoder(16)
	enc.WriteInt32(42)
	data := enc.Bytes()

	runNoPanic(t, "negative skip", func() {
		dec := NewDecoder(data)
		// Advance so pos is non-zero, making a negative skip dangerous.
		if _, err := dec.ReadInt32(); err != nil {
			t.Fatalf("setup ReadInt32: %v", err)
		}
		err := dec.Skip(-4)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("Skip(-4) err = %v, want ErrUnexpectedEOF", err)
		}
		// pos must be untouched (still at EOF), so reads cleanly error.
		if _, err := dec.ReadInt32(); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("ReadInt32 after rejected skip err = %v, want ErrUnexpectedEOF", err)
		}
	})

	// Also test a negative skip from pos 0 (most extreme: would index buf[-100:]).
	runNoPanic(t, "negative skip from start", func() {
		dec := NewDecoder(data)
		if err := dec.Skip(-100); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("Skip(-100) err = %v, want ErrUnexpectedEOF", err)
		}
		// Buffer intact: a valid read still works.
		v, err := dec.ReadInt32()
		if err != nil {
			t.Fatalf("ReadInt32 after rejected skip: %v", err)
		}
		if v != 42 {
			t.Fatalf("ReadInt32 = %d, want 42", v)
		}
	})
}

func TestMalformed_SkipPastEnd(t *testing.T) {
	data := []byte{0x00, 0x01}
	runNoPanic(t, "skip past end", func() {
		dec := NewDecoder(data)
		if err := dec.Skip(3); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("Skip(3) err = %v, want ErrUnexpectedEOF", err)
		}
		// pos untouched; valid skip still works.
		if err := dec.Skip(2); err != nil {
			t.Fatalf("Skip(2) err = %v, want nil", err)
		}
		if !dec.EOF() {
			t.Fatalf("expected EOF after skipping all bytes")
		}
	})
}

func TestMalformed_NestedMessageLengthExceedsBuffer(t *testing.T) {
	// Outer claims a nested message of length 50 but only 2 bytes follow.
	data := []byte{50, 0xAA, 0xBB}
	runNoPanic(t, "nested message oversized", func() {
		dec := NewDecoder(data)
		_, err := dec.ReadMessageBytes()
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("ReadMessageBytes err = %v, want ErrUnexpectedEOF", err)
		}
	})
}

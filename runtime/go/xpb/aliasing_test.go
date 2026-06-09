package xpb

import "testing"

// These tests document and enforce the unsafe zero-copy contract:
//
//   - ReadString returns a string that ALIASES the decoder's underlying buffer
//     (no allocation/copy). Mutating that buffer afterwards is visible through
//     the returned string. This is fast but the caller must treat the buffer as
//     immutable for as long as the string is used.
//   - CloneString returns a string that OWNS a fresh copy. Mutating the buffer
//     afterwards does NOT change it.
//   - ReadBytesUnsafe aliases the buffer; ReadBytes copies.
//
// The tests are in-package so they can write directly into the decoder's buf
// field to simulate "the underlying buffer changed underneath you".

func TestAliasing_ReadStringAliasesBuffer(t *testing.T) {
	enc := NewEncoder(64)
	enc.WriteString("AAAA")
	// Take an independent copy so we own a mutable backing array.
	buf := append([]byte(nil), enc.Bytes()...)

	dec := NewDecoder(buf)
	s, err := dec.ReadString()
	if err != nil {
		t.Fatalf("ReadString: %v", err)
	}
	if s != "AAAA" {
		t.Fatalf("ReadString = %q, want AAAA", s)
	}

	// Mutate the payload bytes in the decoder's buffer. The compact length is 1
	// byte (4), so payload starts at index 1.
	for i := 1; i < len(dec.buf); i++ {
		dec.buf[i] = 'B'
	}

	// Because ReadString aliases the buffer, the returned string now reflects
	// the mutation. If this ever fails, ReadString stopped being zero-copy.
	if s != "BBBB" {
		t.Fatalf("ReadString did not alias buffer: got %q, want BBBB (mutation should be visible)", s)
	}
}

func TestAliasing_CloneStringDoesNotAlias(t *testing.T) {
	enc := NewEncoder(64)
	enc.WriteString("AAAA")
	buf := append([]byte(nil), enc.Bytes()...)

	dec := NewDecoder(buf)
	s, err := dec.CloneString()
	if err != nil {
		t.Fatalf("CloneString: %v", err)
	}
	if s != "AAAA" {
		t.Fatalf("CloneString = %q, want AAAA", s)
	}

	// Mutate the underlying buffer.
	for i := 1; i < len(dec.buf); i++ {
		dec.buf[i] = 'B'
	}

	// CloneString owns its memory; the mutation must NOT be visible.
	if s != "AAAA" {
		t.Fatalf("CloneString aliased buffer: got %q, want AAAA (must be an independent copy)", s)
	}
}

func TestAliasing_ReadBytesUnsafeAliasesBuffer(t *testing.T) {
	enc := NewEncoder(64)
	enc.WriteBytes([]byte{1, 2, 3, 4})
	buf := append([]byte(nil), enc.Bytes()...)

	dec := NewDecoder(buf)
	b, err := dec.ReadBytesUnsafe()
	if err != nil {
		t.Fatalf("ReadBytesUnsafe: %v", err)
	}

	// Mutate the underlying buffer; the aliased slice must reflect it.
	dec.buf[1] = 0xFF // first payload byte (index 0 is the length)
	if b[0] != 0xFF {
		t.Fatalf("ReadBytesUnsafe did not alias buffer: b[0] = %#x, want 0xFF", b[0])
	}
}

func TestAliasing_ReadBytesCopies(t *testing.T) {
	enc := NewEncoder(64)
	enc.WriteBytes([]byte{1, 2, 3, 4})
	buf := append([]byte(nil), enc.Bytes()...)

	dec := NewDecoder(buf)
	b, err := dec.ReadBytes()
	if err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}

	// Mutate the underlying buffer; the copied slice must NOT change.
	dec.buf[1] = 0xFF
	if b[0] != 1 {
		t.Fatalf("ReadBytes aliased buffer: b[0] = %#x, want 1 (must be a copy)", b[0])
	}
}

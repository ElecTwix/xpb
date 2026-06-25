package utekabench

// This file makes the 0.5.0 zero-copy buffer-reuse footgun a lived, tested
// scenario. It simulates the canonical streaming-reader pattern — read each
// framed message into ONE reused scratch buffer, decode it, and move on — and
// proves three things behaviorally:
//
//  1. HAZARD: with the DEFAULT value+zero-copy fixture (val), a decoded field
//     (Payload []byte and string fields) ALIASES the reused buffer. Retaining
//     such a field past the next buffer fill silently CORRUPTS it.
//  2. SAFE-BY-CLONE: cloning the retained field (append([]byte(nil), x...) for
//     bytes, an explicit string copy for strings) survives buffer reuse.
//  3. SAFE-BY-OPT-OUT: the --go-safe-bytes opt-out fixture (ptr) COPIES bytes
//     on decode, so a retained *Payload does NOT alias and survives reuse.
//
// A positive decode-use-discard-in-scope loop shows that aliasing is harmless
// when the decoded field never outlives the buffer it points into.
//
// Everything is in-process and deterministic: messages are pre-encoded once,
// then streamed through a fixed-capacity scratch buffer. No real network, no
// timing, no randomness — the corruption is forced by an explicit overwrite,
// not raced for.

import (
	"bytes"
	"testing"

	"github.com/ElecTwix/xpb/benchmarks/go/uteka/ptr"
	"github.com/ElecTwix/xpb/benchmarks/go/uteka/val"
)

// simFrameA and simFrameB are two distinct logical messages we stream through a
// single reused buffer. They are intentionally byte-for-byte different in the
// fields we retain (Id string + Payload bytes) so an alias into frame A becomes
// observably wrong once frame B overwrites the buffer.
func simFrameA() *val.UtekaMessage {
	return &val.UtekaMessage{
		Type:    1,
		Id:      "stream-msg-AAAAAAAA",
		Payload: []byte("payload-AAAAAAAAAAAA"), HasPayload: true,
		Timestamp: 1000,
		Seq:       1,
	}
}

func simFrameB() *val.UtekaMessage {
	return &val.UtekaMessage{
		Type:    2,
		Id:      "stream-msg-BBBBBBBB",
		Payload: []byte("payload-BBBBBBBBBBBB"), HasPayload: true,
		Timestamp: 2000,
		Seq:       2,
	}
}

// streamBuf models the reusable scratch buffer a streaming reader fills from the
// wire. fill copies the next frame's bytes into the SAME backing array (the
// length is reset to the frame size) so any slice/string aliasing the previous
// contents now observes the new frame's bytes — exactly what a real
// read-into-reused-buffer loop does.
type streamBuf struct{ b []byte }

func newStreamBuf(capHint int) *streamBuf { return &streamBuf{b: make([]byte, 0, capHint)} }

// fill overwrites the buffer's backing array with frame and returns the active
// slice. The capacity is fixed up-front so no reallocation happens between
// frames; reuse of the identical backing array is what triggers the hazard.
func (s *streamBuf) fill(frame []byte) []byte {
	if cap(s.b) < len(frame) {
		// Deterministic guard: the test sizes the buffer to fit every frame, so
		// this branch must never run. If it did, a realloc would move the array
		// and accidentally hide the aliasing hazard.
		panic("streamBuf capacity too small: would reallocate and mask aliasing")
	}
	s.b = s.b[:len(frame)]
	copy(s.b, frame)
	return s.b
}

// encodeFrames returns the wire bytes for the two frames and a buffer sized to
// hold the larger of them, so streaming reuse never reallocates.
func encodeFrames(t *testing.T) (wireA, wireB []byte, buf *streamBuf) {
	t.Helper()
	var err error
	wireA, err = simFrameA().Marshal()
	if err != nil {
		t.Fatalf("marshal frame A: %v", err)
	}
	wireB, err = simFrameB().Marshal()
	if err != nil {
		t.Fatalf("marshal frame B: %v", err)
	}
	// The hazard tests pin the corrupted alias to frame B's EXACT bytes at the
	// same offsets. That only holds when the two frames encode to identical
	// length, so the in-place overwrite lands the new Id/Payload at the same
	// positions the frame-A aliases still point at. Assert the invariant loudly
	// here: if a future edit changes a retained field's length, this fails with
	// a clear message instead of producing a confusing false failure downstream.
	if len(wireA) != len(wireB) {
		t.Fatalf("frames must encode to equal length for same-offset overwrite: len(A)=%d len(B)=%d", len(wireA), len(wireB))
	}
	return wireA, wireB, newStreamBuf(len(wireA))
}

// TestSimBufReuse_ValRetainedFieldCorruptedOnReuse proves the hazard is real:
// retaining a DEFAULT (val) zero-copy decoded field across a buffer reuse
// corrupts it, because the field aliases the scratch buffer that the next frame
// overwrites.
func TestSimBufReuse_ValRetainedFieldCorruptedOnReuse(t *testing.T) {
	wireA, wireB, buf := encodeFrames(t)

	// Frame A arrives into the reused buffer and is decoded.
	var mA val.UtekaMessage
	if err := mA.Unmarshal(buf.fill(wireA)); err != nil {
		t.Fatalf("decode frame A: %v", err)
	}

	// The caller naively retains decoded fields past the read loop iteration.
	retainedPayload := mA.Payload // aliases buf
	retainedID := mA.Id           // aliases buf (zero-copy string)

	// Sanity: at this instant the retained values are correct.
	wantPayload := []byte("payload-AAAAAAAAAAAA")
	wantID := "stream-msg-AAAAAAAA"
	if !bytes.Equal(retainedPayload, wantPayload) || retainedID != wantID {
		t.Fatalf("frame A decoded wrong before reuse: id=%q payload=%q", retainedID, retainedPayload)
	}

	// Frame B arrives into the SAME buffer, overwriting frame A's bytes.
	var mB val.UtekaMessage
	if err := mB.Unmarshal(buf.fill(wireB)); err != nil {
		t.Fatalf("decode frame B: %v", err)
	}

	// HAZARD: the retained aliases now observe frame B's bytes — they are
	// corrupted. If either survived intact, the default decode stopped aliasing
	// and this lived hazard test would no longer be guarding anything.
	if bytes.Equal(retainedPayload, wantPayload) {
		t.Fatalf("expected retained val Payload to be CORRUPTED after buffer reuse, but it survived intact: %q", retainedPayload)
	}
	if retainedID == wantID {
		t.Fatalf("expected retained val Id to be CORRUPTED after buffer reuse, but it survived intact: %q", retainedID)
	}

	// And it is corrupted specifically by aliasing frame B's contents.
	if !bytes.Equal(retainedPayload, []byte("payload-BBBBBBBBBBBB")) {
		t.Fatalf("retained Payload should now alias frame B bytes, got %q", retainedPayload)
	}
	if retainedID != "stream-msg-BBBBBBBB" {
		t.Fatalf("retained Id should now alias frame B bytes, got %q", retainedID)
	}
}

// TestSimBufReuse_CloningSurvivesReuse proves the documented SAFE workaround:
// cloning a retained val field before the next read makes it independent of the
// scratch buffer, so it survives reuse intact.
func TestSimBufReuse_CloningSurvivesReuse(t *testing.T) {
	wireA, wireB, buf := encodeFrames(t)

	var mA val.UtekaMessage
	if err := mA.Unmarshal(buf.fill(wireA)); err != nil {
		t.Fatalf("decode frame A: %v", err)
	}

	// Clone before retaining: bytes via append([]byte(nil), x...); string via an
	// explicit copy that allocates fresh backing bytes.
	clonedPayload := append([]byte(nil), mA.Payload...)
	clonedID := string(append([]byte(nil), mA.Id...))

	wantPayload := []byte("payload-AAAAAAAAAAAA")
	wantID := "stream-msg-AAAAAAAA"
	if !bytes.Equal(clonedPayload, wantPayload) || clonedID != wantID {
		t.Fatalf("frame A clone wrong before reuse: id=%q payload=%q", clonedID, clonedPayload)
	}

	// Reuse the buffer for frame B.
	var mB val.UtekaMessage
	if err := mB.Unmarshal(buf.fill(wireB)); err != nil {
		t.Fatalf("decode frame B: %v", err)
	}

	// Independent control: prove the scratch buffer was genuinely overwritten
	// with frame B's bytes, so "survives reuse" below is not vacuously true (it
	// would also hold if fill became a no-op or the frames shared bytes).
	if !bytes.Equal(buf.b, wireB) {
		t.Fatalf("buffer was not overwritten with frame B; reuse premise broken: %q", buf.b)
	}

	// SAFE: the clones own independent memory, so buffer reuse cannot touch them.
	if !bytes.Equal(clonedPayload, wantPayload) {
		t.Fatalf("cloned Payload must survive buffer reuse, got %q want %q", clonedPayload, wantPayload)
	}
	if clonedID != wantID {
		t.Fatalf("cloned Id must survive buffer reuse, got %q want %q", clonedID, wantID)
	}
}

// TestSimBufReuse_PtrSafeBytesSurvivesReuse proves the --go-safe-bytes opt-out
// (ptr) decode does NOT alias for the BYTES field: the decoded *Payload owns a
// copy, so it survives buffer reuse without any caller-side cloning.
//
// Scope note: --go-safe-bytes only makes []byte fields copy. STRING fields in
// the ptr fixture still decode zero-copy (xpb.ReadStringAt), so a retained ptr
// Id would still be corrupted by buffer reuse exactly like the val fixture —
// the opt-out is bytes-only by design. This test therefore asserts only the
// Payload bytes survive, which is precisely the contract the flag controls.
func TestSimBufReuse_PtrSafeBytesSurvivesReuse(t *testing.T) {
	wireA, wireB, buf := encodeFrames(t)

	var mA ptr.UtekaMessage
	if err := mA.Unmarshal(buf.fill(wireA)); err != nil {
		t.Fatalf("decode frame A (ptr): %v", err)
	}
	if mA.Payload == nil {
		t.Fatalf("frame A (ptr) decoded nil Payload")
	}

	// Retain the safe-bytes Payload directly — no clone needed.
	retainedPayload := *mA.Payload
	wantPayload := []byte("payload-AAAAAAAAAAAA")
	if !bytes.Equal(retainedPayload, wantPayload) {
		t.Fatalf("frame A (ptr) Payload wrong before reuse: %q", retainedPayload)
	}

	// Reuse the buffer for frame B.
	var mB ptr.UtekaMessage
	if err := mB.Unmarshal(buf.fill(wireB)); err != nil {
		t.Fatalf("decode frame B (ptr): %v", err)
	}

	// Independent control: prove the scratch buffer was genuinely overwritten so
	// "survives reuse" below is a real negative control, not vacuously true.
	if !bytes.Equal(buf.b, wireB) {
		t.Fatalf("buffer was not overwritten with frame B; reuse premise broken: %q", buf.b)
	}

	// SAFE: safe-bytes copies on decode, so the retained slice is independent of
	// the reused buffer and survives intact.
	if !bytes.Equal(retainedPayload, wantPayload) {
		t.Fatalf("safe-bytes (ptr) Payload must survive buffer reuse without cloning, got %q want %q", retainedPayload, wantPayload)
	}
}

// TestSimBufReuse_DecodeUseDiscardInScopeIsSafe is the positive pattern: even
// with the aliasing default, decoding and FULLY consuming each frame within the
// same loop iteration — before the buffer is reused for the next frame — is
// always correct. The aliased fields never outlive the buffer they point into.
func TestSimBufReuse_DecodeUseDiscardInScopeIsSafe(t *testing.T) {
	wireA, wireB, buf := encodeFrames(t)

	frames := [][]byte{wireA, wireB}
	wantIDs := []string{"stream-msg-AAAAAAAA", "stream-msg-BBBBBBBB"}
	wantPayloads := []string{"payload-AAAAAAAAAAAA", "payload-BBBBBBBBBBBB"}

	// Accumulate a result derived from each frame, but only from values consumed
	// in-scope (not by retaining the aliasing fields themselves).
	var seenIDs []string
	var totalPayloadLen int
	for i, frame := range frames {
		var m val.UtekaMessage
		if err := m.Unmarshal(buf.fill(frame)); err != nil {
			t.Fatalf("decode frame %d: %v", i, err)
		}
		// Use the aliased fields right here, while the buffer still holds this
		// frame. This is the safe, idiomatic streaming pattern.
		if m.Id != wantIDs[i] {
			t.Fatalf("frame %d Id = %q, want %q", i, m.Id, wantIDs[i])
		}
		if !bytes.Equal(m.Payload, []byte(wantPayloads[i])) {
			t.Fatalf("frame %d Payload = %q, want %q", i, m.Payload, wantPayloads[i])
		}
		// Derive independent (copied) results to carry past the iteration.
		seenIDs = append(seenIDs, string(append([]byte(nil), m.Id...)))
		totalPayloadLen += len(m.Payload)
	}

	if len(seenIDs) != len(wantIDs) {
		t.Fatalf("processed %d frames, want %d", len(seenIDs), len(wantIDs))
	}
	for i := range wantIDs {
		if seenIDs[i] != wantIDs[i] {
			t.Fatalf("in-scope result %d = %q, want %q", i, seenIDs[i], wantIDs[i])
		}
	}
	wantTotal := len(wantPayloads[0]) + len(wantPayloads[1])
	if totalPayloadLen != wantTotal {
		t.Fatalf("total payload length = %d, want %d", totalPayloadLen, wantTotal)
	}
}

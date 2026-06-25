package xpb_test

import (
	"os/exec"
	"strings"
	"testing"
)

// hotInlineHelpers are the runtime encode/decode helpers that MUST stay
// inlinable for the zero-alloc / hot-path performance characteristics the rest
// of this arsenal locks in. They are the fixed-width scalar Read*/Write*
// methods plus the length-prefixed write helpers, all of which the Go inliner
// currently reports "can inline". If a future change pushes one of these over
// the inliner's cost budget (e.g. by adding a branch or a call), this guard
// fails loudly instead of letting a silent perf regression through.
//
// Note: ReadString / CloneString / ReadBytes / ReadBytesUnsafe are intentionally
// NOT listed -- they call unsafe.String / make / copy and do not inline; that is
// expected and not a regression.
var hotInlineHelpers = []string{
	"(*Encoder).WriteBool",
	"(*Encoder).WriteInt32",
	"(*Encoder).WriteInt64",
	"(*Encoder).WriteUint32",
	"(*Encoder).WriteUint64",
	"(*Encoder).WriteFloat32",
	"(*Encoder).WriteFloat64",
	"(*Encoder).WriteString",
	"(*Encoder).WriteBytes",
	"(*Encoder).writeCompactLength",
	"(*Decoder).ReadBool",
	"(*Decoder).ReadInt32",
	"(*Decoder).ReadInt64",
	"(*Decoder).ReadUint32",
	"(*Decoder).ReadUint64",
	"(*Decoder).ReadFloat32",
	"(*Decoder).ReadFloat64",
	"(*Decoder).readCompactLength",
	// Stateless cursor read helpers (Phase 1): the fixed-width scalar *At
	// helpers, the compact-length helper, and the nested-message envelope
	// helper are the register-local-cursor counterparts threaded through
	// generated decode. They must stay inlinable for generated unmarshalAt to
	// reach the hand-written local-cursor ceiling. ReadMessageBytesAt is the
	// per-nested-message envelope read and inlines because it is a thin alias
	// of ReadBytesUnsafeAt.
	//
	// Intentionally NOT listed (and not a regression):
	//   - ReadStringAt / ReadBytesAt / ReadBytesUnsafeAt: like their Decoder
	//     counterparts they call unsafe.String / make / copy and do not inline.
	//   - ReadArrayCountAt: like the stateful (*Decoder).ReadArrayCount it
	//     calls fmt.Errorf on the validation paths and does not inline; it runs
	//     once per repeated/map field, not on the per-scalar hot path.
	"ReadBoolAt",
	"ReadInt32At",
	"ReadInt64At",
	"ReadUint32At",
	"ReadUint64At",
	"ReadFloat32At",
	"ReadFloat64At",
	"readCompactLengthAt",
	"ReadMessageBytesAt",
	// Stateless cursor append helpers (Phase 2): the register-local-buffer
	// counterparts of the Encoder.Write* methods, threaded through generated
	// Marshal/MarshalTo. They must stay inlinable for generated encode to reach
	// the hand-written local-buffer ceiling. The fixed-width scalar *To helpers
	// are thin wrappers over encoding/binary; the length-prefixed string/bytes/
	// message helpers reduce to a compact-length append plus append(b, v...),
	// which (unlike the read side's make/copy) inlines. GrowBuf is a one-line
	// slices.Grow wrapper, and Buf/SetBuf are the encoder accessors that bind
	// and write back the local buffer once per message.
	"(*Encoder).Buf",
	"(*Encoder).SetBuf",
	"GrowBuf",
	"AppendBoolTo",
	"AppendInt32To",
	"AppendInt64To",
	"AppendUint32To",
	"AppendUint64To",
	"AppendFloat32To",
	"AppendFloat64To",
	"AppendCompactLengthTo",
	"AppendStringTo",
	"AppendBytesTo",
	"AppendMessageTo",
}

// TestInliningGuard_HotHelpers builds the runtime package with -gcflags=-m and
// asserts every helper in hotInlineHelpers is reported inlinable by the compiler.
// This is the cheap inlining guard from the perf arsenal: it documents and
// enforces the inlining contract of the hot path without depending on a
// benchmark threshold.
func TestInliningGuard_HotHelpers(t *testing.T) {
	// `go build -gcflags=-m` prints inlining decisions to stderr. Build the
	// runtime package itself (".") relative to this test's package directory.
	cmd := exec.Command("go", "build", "-gcflags=-m", ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build -gcflags=-m failed: %v\n%s", err, out)
	}

	report := string(out)
	canInline := make(map[string]bool)
	for _, line := range strings.Split(report, "\n") {
		// Lines look like: ".../xpb.go:86:6: can inline (*Encoder).WriteInt32"
		idx := strings.Index(line, "can inline ")
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(line[idx+len("can inline "):])
		canInline[name] = true
	}

	for _, h := range hotInlineHelpers {
		if !canInline[h] {
			t.Errorf("hot helper %s is no longer inlinable (inlining regression); "+
				"-gcflags=-m did not report \"can inline %s\"", h, h)
		}
	}
}

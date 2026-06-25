package golang

// Mutation-kill tests (ticket T-13).
//
// These tests KILL mutants that survived a gremlins run over emitter.go. The
// emitter produces Go source text, so each test asserts on the exact generated
// substring that the surviving mutant would change (a missing/extra GrowBuf, a
// wrong map element-min-bytes argument, a pointer where a value type belongs, a
// dropped doc comment, a wrong capacity hint). See docs/MUTATION.md for the
// survivor->test mapping and the equivalent mutant that is accepted (not killed).

import (
	"strings"
	"testing"

	"github.com/ElecTwix/xpb/pkg/ast"
)

func genOrFatal(t *testing.T, file *ast.File) string {
	t.Helper()
	src, err := Generate(file)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	return string(src)
}

// TestKill_GrowBuf_OnlyWhenLowerBoundPositive pins the guard in
// generateMarshalBody: `if lb := g.fixedSizeLowerBound(msg); lb > 0`. A message
// whose only field is variable-length has lower bound 0 and must NOT emit a
// useless `xpb.GrowBuf(buf, 0)`; a message with a fixed-width field must emit a
// positive grow. Kills the CONDITIONALS_BOUNDARY mutant `lb > 0` -> `lb >= 0`,
// which would emit GrowBuf(buf, 0).
func TestKill_GrowBuf_OnlyWhenLowerBoundPositive(t *testing.T) {
	varOnly := genOrFatal(t, &ast.File{
		Package: "test",
		Messages: []*ast.Message{{
			Name:   "VarOnly",
			Fields: []*ast.Field{{Number: 1, Name: "s", Type: ast.FieldType{Kind: ast.TypeString}}},
		}},
	})
	if strings.Contains(varOnly, "xpb.GrowBuf(buf, 0)") {
		t.Errorf("variable-only message must not emit GrowBuf(buf, 0), got:\n%s", varOnly)
	}

	fixed := genOrFatal(t, &ast.File{
		Package: "test",
		Messages: []*ast.Message{{
			Name:   "Fixed",
			Fields: []*ast.Field{{Number: 1, Name: "n", Type: ast.FieldType{Kind: ast.TypeInt32}}},
		}},
	})
	if !strings.Contains(fixed, "xpb.GrowBuf(buf, 4)") {
		t.Errorf("fixed int32 message must emit GrowBuf(buf, 4), got:\n%s", fixed)
	}
}

// TestKill_MapElementMinBytes pins the elementMinBytes argument the map decoder
// passes to ReadArrayCountAt: `keyMin+valMin`. For map<int32,int32> that is
// 4+4 = 8. Kills the ARITHMETIC_BASE mutant on `keyMin+valMin` (`+` -> `-`/`*`/`/`
// yields 0/16/1), which would weaken or change the per-element buffer-bound
// validation of an untrusted map count.
func TestKill_MapElementMinBytes(t *testing.T) {
	out := genOrFatal(t, &ast.File{
		Package: "test",
		Messages: []*ast.Message{{
			Name: "M",
			Fields: []*ast.Field{{
				Number: 1, Name: "kv",
				Type: ast.FieldType{
					Kind:    ast.TypeMap,
					KeyType: &ast.FieldType{Kind: ast.TypeInt32},
					ValType: &ast.FieldType{Kind: ast.TypeInt32},
				},
			}},
		}},
	})
	if !strings.Contains(out, "xpb.ReadArrayCountAt(data, pos, 8,") {
		t.Errorf("map<int32,int32> decode must pass elementMinBytes 8 (keyMin+valMin), got:\n%s", out)
	}
}

// TestKill_EnumBaseTypeIsValueNotPointer pins the enum detection in
// goBaseTypeName: `t.Kind == ast.TypeMessage && g.enums[t.Message]`. An enum
// referenced by name (the parser models it as TypeMessage) used as a map value
// must render as the bare enum type `Color`, not the nested-message pointer
// `*Color`. Kills the CONDITIONALS_NEGATION mutant `==` -> `!=`, which would
// fall through to the message branch and emit `*Color`.
func TestKill_EnumBaseTypeIsValueNotPointer(t *testing.T) {
	out := genOrFatal(t, &ast.File{
		Package: "test",
		Enums: []*ast.Enum{{
			Name:   "Color",
			Values: []*ast.EnumValue{{Name: "RED", Number: 0}, {Name: "GREEN", Number: 1}},
		}},
		Messages: []*ast.Message{{
			Name: "Palette",
			Fields: []*ast.Field{{
				Number: 1, Name: "by_name",
				Type: ast.FieldType{
					Kind:    ast.TypeMap,
					KeyType: &ast.FieldType{Kind: ast.TypeString},
					// Enum referenced by name == TypeMessage with the enum's name.
					ValType: &ast.FieldType{Kind: ast.TypeMessage, Message: "Color"},
				},
			}},
		}},
	})
	if !strings.Contains(out, "map[string]Color") {
		t.Errorf("enum map value must render as map[string]Color, got:\n%s", out)
	}
	if strings.Contains(out, "map[string]*Color") {
		t.Errorf("enum map value must NOT render as a pointer map[string]*Color, got:\n%s", out)
	}
}

// TestKill_ToCamelCase_EmptyPartGuard pins the guard in toCamelCase:
// `if len(parts[i]) > 0`. A field name with a doubled underscore splits into an
// empty middle part; the guard prevents slicing "" (which panics). Kills the
// CONDITIONALS_BOUNDARY mutant `> 0` -> `>= 0`, which makes the guard always
// true and panics on the empty part during code generation.
func TestKill_ToCamelCase_EmptyPartGuard(t *testing.T) {
	out := genOrFatal(t, &ast.File{
		Package: "test",
		Messages: []*ast.Message{{
			Name:   "Weird",
			Fields: []*ast.Field{{Number: 1, Name: "foo__bar", Type: ast.FieldType{Kind: ast.TypeInt32}}},
		}},
	})
	// "foo__bar" -> ["foo","","bar"] -> "FooBar"; the empty part is skipped.
	if !strings.Contains(out, "FooBar int32") {
		t.Errorf("field foo__bar must camel-case to FooBar, got:\n%s", out)
	}
}

// TestKill_MapDocComment pins messageHasMapField: `field.Type.Kind == ast.TypeMap`.
// A message WITH a map field gets the non-canonical-encoding NOTE doc comment; a
// message without one must not. Kills the CONDITIONALS_NEGATION mutant `==` ->
// `!=`, which flips the comment onto exactly the wrong messages.
func TestKill_MapDocComment(t *testing.T) {
	const note = "contains a map field"

	withMap := genOrFatal(t, &ast.File{
		Package: "test",
		Messages: []*ast.Message{{
			Name: "HasMap",
			Fields: []*ast.Field{{
				Number: 1, Name: "kv",
				Type: ast.FieldType{
					Kind:    ast.TypeMap,
					KeyType: &ast.FieldType{Kind: ast.TypeString},
					ValType: &ast.FieldType{Kind: ast.TypeInt32},
				},
			}},
		}},
	})
	if !strings.Contains(withMap, note) {
		t.Errorf("message with a map field must carry the map-nondeterminism NOTE, got:\n%s", withMap)
	}

	noMap := genOrFatal(t, &ast.File{
		Package: "test",
		Messages: []*ast.Message{{
			Name:   "NoMap",
			Fields: []*ast.Field{{Number: 1, Name: "n", Type: ast.FieldType{Kind: ast.TypeInt32}}},
		}},
	})
	if strings.Contains(noMap, note) {
		t.Errorf("message without a map field must NOT carry the map NOTE, got:\n%s", noMap)
	}
}

// TestKill_EstimateSize_FloorIs64 pins the capacity floor in estimateSize:
// `if size < 64 { return 64 }`. A small message (estimated size 4 for one int32)
// must still hint a 64-byte encoder. Kills the CONDITIONALS_NEGATION mutant
// `size < 64` -> `size >= 64`, which would emit NewEncoder(4).
//
// NOTE: the sibling CONDITIONALS_BOUNDARY mutant `size < 64` -> `size <= 64` is
// an EQUIVALENT mutant and is intentionally NOT killed here: it only differs at
// size == 64, where both the original and the mutant return 64. See
// docs/MUTATION.md.
func TestKill_EstimateSize_FloorIs64(t *testing.T) {
	out := genOrFatal(t, &ast.File{
		Package: "test",
		Messages: []*ast.Message{{
			Name:   "Tiny",
			Fields: []*ast.Field{{Number: 1, Name: "n", Type: ast.FieldType{Kind: ast.TypeInt32}}},
		}},
	})
	if !strings.Contains(out, "xpb.NewEncoder(64)") {
		t.Errorf("small message must hint NewEncoder(64) (size floor), got:\n%s", out)
	}
}

// TestKill_OptionalMessage_PresenceEncode covers the optional nested-message
// encode path -- the "optional presence flag" emit that was previously not
// covered by any test. An optional *T message field must emit a 1-byte presence
// flag derived from the pointer being non-nil, then encode the body only when
// non-nil. This brings that branch under test and pins the presence-flag emit.
func TestKill_OptionalMessage_PresenceEncode(t *testing.T) {
	out := genOrFatal(t, &ast.File{
		Package: "test",
		Messages: []*ast.Message{
			{
				Name:   "Inner",
				Fields: []*ast.Field{{Number: 1, Name: "x", Type: ast.FieldType{Kind: ast.TypeInt32}}},
			},
			{
				Name: "Outer",
				Fields: []*ast.Field{{
					Number: 1, Name: "inner", Optional: true,
					Type: ast.FieldType{Kind: ast.TypeMessage, Message: "Inner"},
				}},
			},
		},
	})
	if !strings.Contains(out, "xpb.AppendBoolTo(buf, m.Inner != nil)") {
		t.Errorf("optional message must emit a presence flag from the non-nil pointer, got:\n%s", out)
	}
	if !strings.Contains(out, "if m.Inner != nil {") {
		t.Errorf("optional message body must be guarded by a non-nil check, got:\n%s", out)
	}
	// Discriminate the message-optional branch (emitter.go ~332) from the
	// scalar/string/bytes pointer-optional fallthrough (~350): both emit the
	// SAME presence flag + nil guard, so the assertions above cannot tell them
	// apart. The message branch encodes the body via the pointer directly
	// (`m.Inner.MarshalTo(...)`); the scalar fallthrough would deref it
	// (`*m.Inner`). Asserting the un-deref'd MarshalTo and the ABSENCE of a
	// `*m.Inner` deref makes this test fail if the branch selection regresses
	// (e.g. the `Kind == TypeMessage` check is negated).
	if !strings.Contains(out, "m.Inner.MarshalTo(") {
		t.Errorf("optional message must encode the body via m.Inner.MarshalTo(...), got:\n%s", out)
	}
	if strings.Contains(out, "*m.Inner") {
		t.Errorf("optional message must NOT deref (*m.Inner): that is the scalar pointer-optional fallthrough, got:\n%s", out)
	}
}

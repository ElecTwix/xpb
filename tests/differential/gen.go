// Package differential is the cross-LANGUAGE differential fuzzer for XPB V2.
//
// Why this exists (T-9): the in-repo ptr-vs-val differential and the property
// tests all share the Go runtime, so a bug in the Go wire logic is invisible to
// them -- it is "correct relative to itself". The committed cross-language
// conformance suite (tests/conformance + the per-runtime harnesses driven by
// cmd/ci) is a much stronger oracle, but it only exercises a FIXED, hand-written
// set of golden vectors. This package closes the remaining gap: it generates
// RANDOM valid messages from Go, encodes them with the Go reference encoder, and
// hands the bytes to every OTHER language runtime to decode + re-encode, then
// asserts the re-encoded bytes are byte-identical to the Go encoding.
//
// The transport is the exact same language-neutral manifest the conformance
// suite uses: a temp directory containing a vectors.json manifest plus one .bin
// file per random vector (see tests/conformance/manifest.go for the value
// model). Each runtime is driven by a small, NEW, corpus-dir-parameterised
// runner that reuses that runtime's own Encoder/Decoder library -- the existing
// conformance harnesses are left untouched.
//
// Field-kind coverage (the ticket's "all field kinds"): bool, int32/int64,
// uint32/uint64, float32/float64 (bit-exact, incl. NaN/-0/inf), string, bytes,
// optional present + absent (encoded as a presence byte then the value, per
// docs/WIRE_FORMAT.md), repeated (array), map, nested message, and enum
// (encoded as int32 on the wire).
package differential

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/ElecTwix/xpb/tests/conformance"
)

// genConfig bounds the random generator so a single message stays small enough
// to encode/decode quickly across five runtimes while still reaching every
// field kind and the documented length-prefix and nesting edges.
type genConfig struct {
	maxDepth      int // nested-message recursion cap (matches MaxDecodeDepth headroom)
	maxFields     int // top-level ops per message
	maxCollection int // elements in an array / entries in a map
}

func defaultGenConfig() genConfig {
	return genConfig{maxDepth: 4, maxFields: 8, maxCollection: 5}
}

// scalarKinds are the leaf op kinds an array element, map key/value, or optional
// payload may take. Composite kinds (array/map/message) only appear at the
// top level or inside a nested message, which keeps element encoding uniform
// with the conformance value model (arrays/maps are homogeneous scalar runs).
var scalarKinds = []string{
	conformance.TypeBool,
	conformance.TypeInt32,
	conformance.TypeInt64,
	conformance.TypeUint32,
	conformance.TypeUint64,
	conformance.TypeFloat32,
	conformance.TypeFloat64,
	conformance.TypeString,
	conformance.TypeBytes,
}

// mapKeyKinds are the scalar kinds usable as a map key (every scalar except the
// composites; floats are allowed because the wire is bit-exact).
var mapKeyKinds = scalarKinds

// Canonical quiet-NaN bit patterns. Every runtime preserves these byte-for-byte;
// SIGNALING NaNs are not portable -- the float->double->float widening some
// runtimes perform on decode (notably the Lua runtime's string.unpack("f")) can
// set the quiet bit and silently change the payload, which would make a
// signaling-NaN vector spuriously mismatch. We therefore collapse every random
// NaN draw to the canonical quiet NaN so the float wire path is exercised
// without that cross-runtime ambiguity.
const (
	quietNaN32 = uint32(0x7FC00000)
	quietNaN64 = uint64(0x7FF8000000000000)
)

// canonF32 returns the bits unchanged unless they encode a NaN, in which case it
// returns the canonical quiet NaN.
func canonF32(bits uint32) uint32 {
	const expMask = uint32(0x7F800000)
	const fracMask = uint32(0x007FFFFF)
	if bits&expMask == expMask && bits&fracMask != 0 {
		return quietNaN32
	}
	return bits
}

// canonF64 is the float64 analogue of canonF32.
func canonF64(bits uint64) uint64 {
	const expMask = uint64(0x7FF0000000000000)
	const fracMask = uint64(0x000FFFFFFFFFFFFF)
	if bits&expMask == expMask && bits&fracMask != 0 {
		return quietNaN64
	}
	return bits
}

// randScalar builds one random scalar op of the given kind.
func randScalar(r *rand.Rand, kind string) conformance.Op {
	switch kind {
	case conformance.TypeBool:
		return conformance.OpBool(r.Intn(2) == 1)
	case conformance.TypeInt32:
		return conformance.OpInt32(int32(r.Uint32()))
	case conformance.TypeInt64:
		return conformance.OpInt64(int64(r.Uint64()))
	case conformance.TypeUint32:
		return conformance.OpUint32(r.Uint32())
	case conformance.TypeUint64:
		return conformance.OpUint64(r.Uint64())
	case conformance.TypeFloat32:
		// Mix in the bit-exact specials so the float wire path is exercised.
		switch r.Intn(8) {
		case 0:
			return conformance.OpFloat32Bits(quietNaN32) // canonical quiet NaN
		case 1:
			return conformance.OpFloat32Bits(0x80000000) // -0.0
		case 2:
			return conformance.OpFloat32(float32(math.Inf(1)))
		case 3:
			return conformance.OpFloat32(float32(math.Inf(-1)))
		default:
			return conformance.OpFloat32Bits(canonF32(r.Uint32()))
		}
	case conformance.TypeFloat64:
		switch r.Intn(8) {
		case 0:
			return conformance.OpFloat64Bits(quietNaN64) // canonical quiet NaN
		case 1:
			return conformance.OpFloat64Bits(0x8000000000000000) // -0.0
		case 2:
			return conformance.OpFloat64(math.Inf(1))
		case 3:
			return conformance.OpFloat64(math.Inf(-1))
		default:
			return conformance.OpFloat64Bits(canonF64(r.Uint64()))
		}
	case conformance.TypeString:
		return conformance.OpString(randString(r))
	case conformance.TypeBytes:
		return conformance.OpBytes(randBytes(r))
	default:
		panic("randScalar: unknown kind " + kind)
	}
}

// randEnum models an enum field: encoded as int32 on the wire (docs/WIRE_FORMAT
// "Enums are encoded as int32 values"), but constrained to a small value set so
// it is semantically an enum rather than an arbitrary int.
func randEnum(r *rand.Rand) conformance.Op {
	// Values 0..4 plus a deliberately large variant to exercise wide int32.
	vals := []int32{0, 1, 2, 3, math.MaxInt32}
	return conformance.OpInt32(vals[r.Intn(len(vals))])
}

// randString returns a random string, occasionally hitting the compact-length
// boundary (254/255/256 bytes) and the empty case.
func randString(r *rand.Rand) string {
	switch r.Intn(10) {
	case 0:
		return ""
	case 1:
		return strings.Repeat("a", 254) // last 1-byte length prefix
	case 2:
		return strings.Repeat("b", 255) // first 5-byte length prefix (0xFF marker)
	case 3:
		return strings.Repeat("c", 256)
	default:
		n := r.Intn(40)
		var b strings.Builder
		for i := 0; i < n; i++ {
			// Printable ASCII + the occasional multibyte rune.
			if r.Intn(8) == 0 {
				b.WriteRune('世')
			} else {
				b.WriteByte(byte(0x20 + r.Intn(0x5F)))
			}
		}
		return b.String()
	}
}

// randBytes returns a random byte slice, occasionally hitting empty, the 255-len
// 0xFF length-prefix boundary, and an all-0xFF payload (the ticket's
// ">254-byte 0xFF length" edge spans both length-prefix and content 0xFF).
func randBytes(r *rand.Rand) []byte {
	switch r.Intn(10) {
	case 0:
		return []byte{}
	case 1:
		return repeatByte(0xFF, 255) // 5-byte 0xFF length prefix + all-0xFF content
	case 2:
		return repeatByte(0x00, 8) // all-zero bytes
	default:
		n := r.Intn(40)
		out := make([]byte, n)
		for i := range out {
			out[i] = byte(r.Intn(256))
		}
		return out
	}
}

// randArray builds a homogeneous repeated field of a random scalar element type.
func randArray(r *rand.Rand, cfg genConfig) conformance.Op {
	elemType := scalarKinds[r.Intn(len(scalarKinds))]
	n := r.Intn(cfg.maxCollection + 1) // include the empty array
	elems := make([]conformance.Op, n)
	for i := range elems {
		elems[i] = randScalar(r, elemType)
	}
	return conformance.OpArray(elemType, elems...)
}

// randMap builds a map with random scalar key/value types. Keys are made unique
// so the decoded-value comparison is well-defined (the wire itself does not
// dedupe, but distinct keys keep the value-equality check unambiguous).
//
// Some key types have tiny cardinality (bool has exactly two distinct values),
// so requesting more entries than the key space allows is impossible. A bounded
// attempt budget guards against spinning forever: once fresh keys run out the
// map simply ends up smaller, which is itself a valid (and worth testing) case.
func randMap(r *rand.Rand, cfg genConfig) conformance.Op {
	keyType := mapKeyKinds[r.Intn(len(mapKeyKinds))]
	valType := scalarKinds[r.Intn(len(scalarKinds))]
	n := r.Intn(cfg.maxCollection + 1) // include the empty map
	entries := make([]conformance.MapEntry, 0, n)
	seen := map[string]bool{}
	attempts := 0
	maxAttempts := (n + 1) * 8 // generous budget; bounded so generation always terminates
	for len(entries) < n && attempts < maxAttempts {
		attempts++
		k := randScalar(r, keyType)
		key := keyHexKey(k)
		if seen[key] {
			continue
		}
		seen[key] = true
		entries = append(entries, conformance.MapEntry{K: k, V: randScalar(r, valType)})
	}
	return conformance.OpMap(keyType, valType, entries...)
}

// keyHexKey returns a stable identity string for a scalar op used as a map key,
// so duplicate keys can be filtered out during generation.
func keyHexKey(o conformance.Op) string {
	b := conformance.Encode([]conformance.Op{o})
	return o.Type + ":" + hex.EncodeToString(b)
}

// randMessage builds a nested length-prefixed message, recursing until depth 0.
func randMessage(r *rand.Rand, cfg genConfig, depth int) conformance.Op {
	return conformance.OpMessage(randOps(r, cfg, depth)...)
}

// randOps builds a sequence of top-level ops for a message body. At depth 0 only
// scalars/optionals are emitted (no further composites) to bound size.
func randOps(r *rand.Rand, cfg genConfig, depth int) []conformance.Op {
	n := 1 + r.Intn(cfg.maxFields)
	ops := make([]conformance.Op, 0, n)
	for i := 0; i < n; i++ {
		ops = append(ops, randField(r, cfg, depth)...)
	}
	return ops
}

// randField returns the op(s) for one random field. Optional fields expand to a
// presence byte (a bool op) optionally followed by the value op, exactly as the
// wire format encodes `?T` fields.
func randField(r *rand.Rand, cfg genConfig, depth int) []conformance.Op {
	// 1-in-4 fields are optional; absent vs present is then 50/50, guaranteeing
	// both the "absent" (presence byte only) and "present" arms are reached.
	if r.Intn(4) == 0 {
		present := r.Intn(2) == 1
		if !present {
			return []conformance.Op{conformance.OpBool(false)} // absent: presence byte only
		}
		return []conformance.Op{conformance.OpBool(true), randValue(r, cfg, depth)}
	}
	return []conformance.Op{randValue(r, cfg, depth)}
}

// randValue picks a random field value. Composite kinds (array/map/message,
// plus enum) are only reachable while depth budget remains.
func randValue(r *rand.Rand, cfg genConfig, depth int) conformance.Op {
	if depth <= 0 {
		// Leaf: scalar or enum only.
		if r.Intn(6) == 0 {
			return randEnum(r)
		}
		return randScalar(r, scalarKinds[r.Intn(len(scalarKinds))])
	}
	switch r.Intn(12) {
	case 0, 1:
		return randArray(r, cfg)
	case 2, 3:
		return randMap(r, cfg)
	case 4, 5:
		return randMessage(r, cfg, depth-1)
	case 6:
		return randEnum(r)
	default:
		return randScalar(r, scalarKinds[r.Intn(len(scalarKinds))])
	}
}

// containsMap reports whether any op in the tree is (or contains) a map. Such
// vectors are excluded from the byte-identity arm because map ordering is
// non-canonical across runtimes (T-7); they go through the decoded-value arm
// instead, handled by each runtime's runner.
func containsMap(ops []conformance.Op) bool {
	for _, o := range ops {
		if o.Type == conformance.TypeMap {
			return true
		}
		if o.Type == conformance.TypeMessage && containsMap(o.Ops) {
			return true
		}
		if o.Type == conformance.TypeArray && containsMap(o.Elems) {
			return true
		}
	}
	return false
}

func repeatByte(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

// --- corpus generation -----------------------------------------------------

// genVectors builds n random vectors from the given seed. The generator is
// fully deterministic in seed, so a failing fuzz input reproduces exactly.
func genVectors(seed int64, n int) []conformance.Vector {
	r := rand.New(rand.NewSource(seed))
	cfg := defaultGenConfig()
	out := make([]conformance.Vector, 0, n+len(edgeVectors()))

	// Always lead with the explicit edge-case seeds so every corpus exercises
	// them regardless of the random draw.
	out = append(out, edgeVectors()...)

	for i := 0; i < n; i++ {
		ops := randOps(r, cfg, cfg.maxDepth)
		out = append(out, conformance.Vector{
			Name: vecName("rand", i),
			Ops:  ops,
		})
	}
	return out
}

// edgeVectors are the hand-written corner cases the ticket calls out, expressed
// in the same op model so they ride the same cross-language pipeline.
func edgeVectors() []conformance.Vector {
	maxNest := func(depth int) conformance.Op {
		// Build a message nested `depth` deep with a scalar at the bottom.
		inner := conformance.OpInt32(0x7FFFFFFF)
		for i := 0; i < depth; i++ {
			inner = conformance.OpMessage(conformance.OpString("d"), inner)
		}
		return inner
	}
	return []conformance.Vector{
		// >254-byte 0xFF length prefix on bytes, with all-0xFF content.
		{Name: "edge_bytes_ff255", Ops: []conformance.Op{conformance.OpBytes(repeatByte(0xFF, 255))}},
		// >254-byte 0xFF length on a string (300 bytes -> 5-byte length prefix).
		{Name: "edge_string_300", Ops: []conformance.Op{conformance.OpString(strings.Repeat("x", 300))}},
		// Empty string + empty bytes back to back.
		{Name: "edge_empty_str_bytes", Ops: []conformance.Op{conformance.OpString(""), conformance.OpBytes([]byte{})}},
		// All-zero scalars across every fixed-width type.
		{Name: "edge_all_zero", Ops: []conformance.Op{
			conformance.OpBool(false), conformance.OpInt32(0), conformance.OpInt64(0),
			conformance.OpUint32(0), conformance.OpUint64(0),
			conformance.OpFloat32(0), conformance.OpFloat64(0),
		}},
		// Optional absent then optional present, exercising the presence byte.
		{Name: "edge_optional_both", Ops: []conformance.Op{
			conformance.OpBool(false),                                 // absent optional
			conformance.OpBool(true), conformance.OpString("present"), // present optional
		}},
		// Enum field (int32 wire) at a boundary value.
		{Name: "edge_enum", Ops: []conformance.Op{conformance.OpInt32(math.MaxInt32)}},
		// Deeply nested message at depth 60. NOTE: this drives 60 levels of
		// length-prefixed nested-message encode/decode/re-encode recursion through
		// every runtime's harness (a genuine deep-nesting wire-path stress); it
		// does NOT exercise the runtimes' MaxDecodeDepth=64 codegen guard, which is
		// only enforced by the generated unmarshalAt(depth) helpers, not by the
		// manifest-driven harness (which decodes each level with a fresh decoder).
		{Name: "edge_max_nesting", Ops: []conformance.Op{maxNest(60)}},
	}
}

// vecName builds a unique, zero-padded vector name. Using fmt.Sprintf with a
// %0*d width means the index is never truncated (a fixed-width buffer would wrap
// past 9999 and collide two vectors onto the same .bin filename, producing a
// spurious mismatch); the width is wide enough that names still sort naturally.
func vecName(prefix string, i int) string {
	return fmt.Sprintf("%s_%06d", prefix, i)
}

// writeCorpus encodes every vector with the Go reference encoder and writes a
// conformance-format corpus (vectors.json + one .bin per vector) into dir. The
// returned vectors carry their computed File/Hex (the Go reference bytes).
func writeCorpus(dir string, vecs []conformance.Vector) ([]conformance.Vector, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	for i := range vecs {
		v := &vecs[i]
		data := conformance.Encode(v.Ops)
		v.File = v.Name + ".bin"
		v.Hex = hex.EncodeToString(data)
		if err := os.WriteFile(filepath.Join(dir, v.File), data, 0o644); err != nil {
			return nil, err
		}
	}
	m := conformance.Manifest{
		Format:  "differential corpus (Go reference encoder); same value model as testdata/conformance/vectors.json",
		Vectors: vecs,
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	b = append(b, '\n')
	if err := os.WriteFile(filepath.Join(dir, "vectors.json"), b, 0o644); err != nil {
		return nil, err
	}
	return vecs, nil
}

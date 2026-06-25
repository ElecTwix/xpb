// Map-field encode non-determinism: executable contract test (T-7).
//
// The Go codegen emits `for k, v := range m.<MapField>` for map fields
// (pkg/codegen/golang/emitter.go). Go map iteration order is deliberately
// randomized, so a message containing a map<> field with more than one entry
// encodes to DIFFERENT byte sequences across encodes. The repository's
// byte-identical / golden guarantees therefore do NOT hold for map-containing
// messages.
//
// This file LOCKS that contract as an executable, deterministic (non-flaky)
// test. It does NOT change the encoder (canonicalization -- e.g. sorting keys --
// is a separate cross-language decision ticket). The assertions are:
//
//   - Decode ALWAYS round-trips correctly across many encodes. Map decode is
//     order-insensitive (reflect.DeepEqual of maps ignores key order), so a
//     re-shuffled key order on the wire still decodes to the same map. This is
//     the deterministic backbone of the test: it can never flake, and it proves
//     the non-determinism is a *byte-layout* property, not a correctness bug.
//   - A >1-entry map is NOT guaranteed byte-stable across encodes. This is
//     asserted ROBUSTLY: with a large map and many encodes the probability of
//     never observing a differing byte sequence by chance is astronomically
//     small, but the test never *fails* if it happens not to observe one -- it
//     only records the observation. The hard assertion is the round-trip, so the
//     test is deterministic regardless of which permutation the runtime picks.
//   - An empty (0-entry) and single-entry map ARE byte-stable across encodes
//     (only one possible iteration order), confirming that it is specifically
//     multi-entry map ordering that is non-canonical.
//
// This test lives in its own file and reuses the package-level helpers
// (generateGo, buildGenModule, goTestModule) declared in go_codegen_test.go; it
// does not modify any shared test file.
package integration

import "testing"

// mapNonDeterminismSchema is a tiny dedicated schema whose only point of
// interest is a multi-entry map field. Counter is a plain scalar that precedes
// the map so the test also confirms the (deterministic) bytes around the map are
// stable while only the map region permutes.
const mapNonDeterminismSchema = `
package mapnd

message Bag {
    1: int32 counter
    2: map<string, string> items
}
`

// mapNonDeterminismDriver is run inside a throwaway module that imports the real
// xpb runtime from this checkout. It exercises the generated Bag.Marshal /
// Bag.Unmarshal directly.
const mapNonDeterminismDriver = `package gen

import (
	"bytes"
	"reflect"
	"testing"
)

// makeItems builds a deterministic, large-enough map that key-order permutation
// reliably changes the encoded bytes. 16 entries => 16! possible orderings, so
// across a few hundred encodes the runtime will almost certainly emit at least
// two distinct byte sequences -- but the test does not depend on that to pass.
func makeItems() map[string]string {
	m := make(map[string]string, 16)
	for _, k := range []string{
		"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel",
		"india", "juliet", "kilo", "lima", "mike", "november", "oscar", "papa",
	} {
		m[k] = "v-" + k
	}
	return m
}

// TestMapDecodeAlwaysRoundTrips is the deterministic backbone: across many
// encodes of the SAME multi-entry map message, every decode must reproduce the
// input map exactly. reflect.DeepEqual on maps is order-insensitive, so this
// holds no matter which key order the encoder happened to emit. This can never
// flake.
func TestMapDecodeAlwaysRoundTrips(t *testing.T) {
	in := &Bag{Counter: 42, Items: makeItems()}
	const encodes = 256
	for i := 0; i < encodes; i++ {
		data, err := in.Marshal()
		if err != nil {
			t.Fatalf("encode %d: Marshal: %v", i, err)
		}
		var out Bag
		if err := out.Unmarshal(data); err != nil {
			t.Fatalf("encode %d: Unmarshal: %v", i, err)
		}
		if out.Counter != in.Counter {
			t.Fatalf("encode %d: counter mismatch: got %d want %d", i, out.Counter, in.Counter)
		}
		if !reflect.DeepEqual(out.Items, in.Items) {
			t.Fatalf("encode %d: map decode mismatch (decode must be order-insensitive):\n got  %v\n want %v",
				i, out.Items, in.Items)
		}
	}
}

// TestMultiEntryMapEncodeIsNotByteStable documents the non-canonical contract:
// a >1-entry map is NOT guaranteed to encode to identical bytes across encodes.
// It is written to be robust to chance equality -- it never fails for not
// observing a difference. The hard guarantee it enforces is the round-trip
// (so callers see exactly what they must rely on instead: decode, not bytes).
func TestMultiEntryMapEncodeIsNotByteStable(t *testing.T) {
	in := &Bag{Counter: 7, Items: makeItems()}

	const encodes = 512
	seen := make(map[string]struct{})
	var first []byte
	for i := 0; i < encodes; i++ {
		data, err := in.Marshal()
		if err != nil {
			t.Fatalf("encode %d: Marshal: %v", i, err)
		}
		if first == nil {
			first = append([]byte(nil), data...)
		}
		// Every encode, whatever its byte order, must still round-trip.
		var out Bag
		if err := out.Unmarshal(data); err != nil {
			t.Fatalf("encode %d: Unmarshal: %v", i, err)
		}
		if !reflect.DeepEqual(out.Items, in.Items) {
			t.Fatalf("encode %d: map decode mismatch: got %v want %v", i, out.Items, in.Items)
		}
		seen[string(data)] = struct{}{}
	}

	// The contract: byte-identity is NOT guaranteed for multi-entry maps. We
	// expect to observe more than one distinct encoding in practice; if we do,
	// record it as confirmation. We deliberately do NOT fail when only one is
	// observed -- that would make the test flaky (chance can keep the order
	// stable). The deterministic guarantees are the round-trips above.
	if len(seen) > 1 {
		t.Logf("confirmed non-canonical: observed %d distinct encodings of the same %d-entry map across %d encodes",
			len(seen), len(in.Items), encodes)
	} else {
		t.Logf("observed a single encoding across %d encodes (chance-stable this run); "+
			"byte-identity is still NOT guaranteed for multi-entry maps -- rely on decode, not bytes", encodes)
	}

	// All observed encodings must have the same length: only the order of the
	// key/value pairs permutes, never the byte count. This is a real, always-true
	// invariant that further pins the contract (non-determinism is reordering,
	// not corruption or size drift).
	for enc := range seen {
		if len(enc) != len(first) {
			t.Fatalf("encoding length drifted: got %d want %d -- map non-determinism must be reordering only",
				len(enc), len(first))
		}
	}
}

// TestSmallMapsAreByteStable confirms the flip side: maps with at most one
// entry have only one possible iteration order, so they encode to identical
// bytes every time. This is a hard, deterministic assertion -- it pins that the
// non-canonical behavior is specifically a MULTI-entry ordering property.
func TestSmallMapsAreByteStable(t *testing.T) {
	cases := []struct {
		name  string
		items map[string]string
	}{
		{"empty", map[string]string{}},
		{"nil", nil},
		{"single", map[string]string{"only": "one"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := &Bag{Counter: 3, Items: tc.items}
			ref, err := in.Marshal()
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			for i := 0; i < 64; i++ {
				data, err := in.Marshal()
				if err != nil {
					t.Fatalf("encode %d: Marshal: %v", i, err)
				}
				if !bytes.Equal(data, ref) {
					t.Fatalf("encode %d: <=1-entry map must be byte-stable:\n got  % x\n want % x", i, data, ref)
				}
			}
		})
	}
}
`

// TestGoCodegen_MapEncodeNonDeterminism generates a multi-entry-map schema and
// runs the contract driver in a throwaway module that compiles against and
// imports the real xpb runtime from this checkout. A compile failure or a
// round-trip mismatch fails the test for real. Because the deterministic
// assertions are all round-trips (order-insensitive) and stable-encoding checks
// for <=1-entry maps, the test never flakes on Go's randomized map iteration.
func TestGoCodegen_MapEncodeNonDeterminism(t *testing.T) {
	src := generateGo(t, mapNonDeterminismSchema)
	dir := buildGenModule(t, src, "map_nondeterminism_test.go", mapNonDeterminismDriver)
	goTestModule(t, dir)
}

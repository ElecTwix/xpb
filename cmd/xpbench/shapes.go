package main

import (
	"math"
	"strings"

	"github.com/ElecTwix/xpb/tests/conformance"
)

// shape is one canonical message shape exercised, byte-identically, by every
// runtime. The ops are the language-neutral conformance value model (the same
// model the cross-language conformance + differential suites use), so a shape
// defined once here encodes to one fixed byte string via the Go reference
// encoder and that exact byte string is what every runtime decodes.
type shape struct {
	name string
	ops  []conformance.Op
}

// canonicalShapes is the single source of truth for the cross-runtime shape
// set. The shapes span the dimensions that dominate codec cost -- a small fixed
// scalar record, a hot single string, homogeneous scalar/string arrays, a
// string-keyed map, nested-message recursion, and a multi-kilobyte mixed
// message -- so the table compares each runtime over the same realistic spread.
//
// They are aligned in spirit with the per-language benchmark categories already
// in the repo (Small / Large / StringArray / Int32Array / StringMap) and with
// the conformance value model used by T-16's shapes work; the canonical
// definition lives here so the cross-runtime comparison cannot drift per
// language.
func canonicalShapes() []shape {
	return []shape{
		{name: "scalars", ops: scalarsOps()},
		{name: "string", ops: []conformance.Op{conformance.OpString("the quick brown fox jumps over the lazy dog")}},
		{name: "int32_array", ops: []conformance.Op{intArrayOps(128)}},
		{name: "string_array", ops: []conformance.Op{stringArrayOps(64)}},
		{name: "string_map", ops: []conformance.Op{stringMapOps(32)}},
		{name: "nested", ops: []conformance.Op{nestedOps(4)}},
		{name: "large_mixed", ops: largeMixedOps()},
	}
}

// scalarsOps is a compact record touching every fixed-width scalar kind plus a
// short string and a small byte string -- the "Small" shape.
func scalarsOps() []conformance.Op {
	return []conformance.Op{
		conformance.OpBool(true),
		conformance.OpInt32(-42),
		conformance.OpInt64(math.MaxInt64 / 3),
		conformance.OpUint32(4242),
		conformance.OpUint64(math.MaxUint64 / 7),
		conformance.OpFloat32(3.14159),
		conformance.OpFloat64(2.718281828459045),
		conformance.OpString("benchmark"),
		conformance.OpBytes([]byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x7F, 0x80, 0xFF}),
	}
}

// intArrayOps builds an int32 array of n sequential elements.
func intArrayOps(n int) conformance.Op {
	elems := make([]conformance.Op, n)
	for i := range elems {
		elems[i] = conformance.OpInt32(int32(i * 7))
	}
	return conformance.OpArray(conformance.TypeInt32, elems...)
}

// stringArrayOps builds a string array of n short elements.
func stringArrayOps(n int) conformance.Op {
	elems := make([]conformance.Op, n)
	for i := range elems {
		elems[i] = conformance.OpString("item-" + itoa(i))
	}
	return conformance.OpArray(conformance.TypeString, elems...)
}

// stringMapOps builds a map<string,int32> with n ordered entries. Entries are
// given in a fixed slice order (not a Go map), so every runtime re-encodes the
// same byte string -- the cross-runtime map-order non-determinism that the
// differential suite works around does not arise here.
func stringMapOps(n int) conformance.Op {
	entries := make([]conformance.MapEntry, n)
	for i := range entries {
		entries[i] = conformance.MapEntry{
			K: conformance.OpString("key-" + itoa(i)),
			V: conformance.OpInt32(int32(i)),
		}
	}
	return conformance.OpMap(conformance.TypeString, conformance.TypeInt32, entries...)
}

// nestedOps builds a message nested `depth` levels deep, each level carrying a
// label string and an int, exercising length-prefixed nested-message recursion.
func nestedOps(depth int) conformance.Op {
	inner := conformance.OpMessage(conformance.OpString("leaf"), conformance.OpInt32(0))
	for i := 0; i < depth; i++ {
		inner = conformance.OpMessage(
			conformance.OpString("level-"+itoa(i)),
			conformance.OpInt32(int32(i)),
			inner,
		)
	}
	return inner
}

// largeMixedOps builds a multi-kilobyte message combining a big int array, a
// string array, a string map, and a nested message -- the "Large" shape that
// stresses sustained throughput rather than per-call overhead.
func largeMixedOps() []conformance.Op {
	return []conformance.Op{
		conformance.OpString("large-mixed-record"),
		conformance.OpInt64(1 << 40),
		intArrayOps(256),
		stringArrayOps(64),
		stringMapOps(32),
		nestedOps(4),
		conformance.OpBytes([]byte(strings.Repeat("\xAB", 512))),
	}
}

// itoa is a tiny base-10 formatter used to build deterministic element labels
// without dragging strconv into the shape definitions.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

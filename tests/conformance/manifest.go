// Package conformance defines a language-neutral manifest of encoded XPB V2
// vectors used for cross-language conformance testing (Go <-> Rust <-> TS).
//
// The manifest is a single JSON file (testdata/conformance/vectors.json) plus a
// set of .bin files (one per vector). The Go runtime is the reference encoder:
// the .bin bytes are produced by the Go encoder and every other runtime must
// decode them, re-encode, and match byte-for-byte.
//
// Value model (kept deliberately simple and language-neutral):
//   - int32/uint32: JSON number (fits exactly in a float64)
//   - int64/uint64: decimal STRING (avoids JS Number precision loss)
//   - float32/float64: hex bit-pattern STRING, e.g. "0x7FF0000000000000".
//     This is mandatory so NaN, -0.0, +/-inf survive exactly across languages.
//   - bool: JSON bool
//   - string: JSON string (UTF-8)
//   - bytes: lowercase hex STRING (no 0x prefix), "" means empty
//   - array: {elemType, elems:[Op...]} encoded as int32 count then elements
//   - map: {keyType, valType, entries:[{k,v}...]} encoded as int32 count then k/v pairs
//   - message: {ops:[Op...]} encoded as a length-prefixed nested message
package conformance

// Op type tags.
const (
	TypeBool    = "bool"
	TypeInt32   = "int32"
	TypeInt64   = "int64"
	TypeUint32  = "uint32"
	TypeUint64  = "uint64"
	TypeFloat32 = "float32"
	TypeFloat64 = "float64"
	TypeString  = "string"
	TypeBytes   = "bytes"
	TypeArray   = "array"
	TypeMap     = "map"
	TypeMessage = "message"
)

// Op is a single typed value. Exactly one of the value fields is meaningful,
// selected by Type. Encoded in the sequence given by a Vector.
type Op struct {
	Type string `json:"type"`

	// Scalars. For int64/uint64 the decimal string form is used; for
	// float32/float64 the hex bit-pattern string form is used.
	Bool   *bool   `json:"bool,omitempty"`
	Int32  *int32  `json:"int32,omitempty"`
	Uint32 *uint32 `json:"uint32,omitempty"`
	Int64  string  `json:"int64,omitempty"`
	Uint64 string  `json:"uint64,omitempty"`
	// FloatBits is the IEEE-754 bit pattern in hex (e.g. "0x40490FDB" for
	// float32, "0x7FF0000000000000" for float64). Used for both float types.
	FloatBits string `json:"floatBits,omitempty"`

	String string `json:"string,omitempty"`
	Bytes  string `json:"bytes,omitempty"` // lowercase hex, "" = empty

	// Array: int32 count prefix then each element.
	ElemType string `json:"elemType,omitempty"`
	Elems    []Op   `json:"elems,omitempty"`

	// Map: int32 count prefix then key,value pairs.
	KeyType string     `json:"keyType,omitempty"`
	ValType string     `json:"valType,omitempty"`
	Entries []MapEntry `json:"entries,omitempty"`

	// Message: a length-prefixed nested sequence of ops.
	Ops []Op `json:"ops,omitempty"`
}

// MapEntry is one key/value pair in a map op.
type MapEntry struct {
	K Op `json:"k"`
	V Op `json:"v"`
}

// Vector is one named conformance case.
type Vector struct {
	Name string `json:"name"`
	// File is the .bin filename (relative to the manifest directory).
	File string `json:"file"`
	// Hex is the expected encoding, lowercase hex (the same bytes as File).
	Hex string `json:"hex"`
	// Ops is the sequence of typed values, in encode order.
	Ops []Op `json:"ops"`
}

// Manifest is the top-level JSON document.
type Manifest struct {
	// Format is a human-readable note about the value model.
	Format  string   `json:"format"`
	Vectors []Vector `json:"vectors"`
}

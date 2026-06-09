package conformance

import (
	"math"
	"strings"
)

// Vectors returns the full set of conformance vectors (without File/Hex, which
// are filled in by the generator). Keep this the single source of truth for the
// value model; the generator computes the reference bytes from the Go encoder.
func Vectors() []Vector {
	// Helper string boundaries (lengths chosen to exercise compact-length edges).
	str254 := strings.Repeat("a", 254) // last 1-byte length
	str255 := strings.Repeat("b", 255) // first 5-byte length
	str256 := strings.Repeat("c", 256) // 5-byte length
	strLong := strings.Repeat("Z", 1000)

	return []Vector{
		// --- bool ---
		{Name: "bool_true", Ops: []Op{OpBool(true)}},
		{Name: "bool_false", Ops: []Op{OpBool(false)}},

		// --- int32 ---
		{Name: "int32_zero", Ops: []Op{OpInt32(0)}},
		{Name: "int32_neg1", Ops: []Op{OpInt32(-1)}},
		{Name: "int32_max", Ops: []Op{OpInt32(math.MaxInt32)}},
		{Name: "int32_min", Ops: []Op{OpInt32(math.MinInt32)}},
		{Name: "int32_sample", Ops: []Op{OpInt32(30)}},

		// --- int64 ---
		{Name: "int64_zero", Ops: []Op{OpInt64(0)}},
		{Name: "int64_neg1", Ops: []Op{OpInt64(-1)}},
		{Name: "int64_max", Ops: []Op{OpInt64(math.MaxInt64)}},
		{Name: "int64_min", Ops: []Op{OpInt64(math.MinInt64)}},

		// --- uint32 ---
		{Name: "uint32_zero", Ops: []Op{OpUint32(0)}},
		{Name: "uint32_max", Ops: []Op{OpUint32(math.MaxUint32)}},

		// --- uint64 ---
		{Name: "uint64_zero", Ops: []Op{OpUint64(0)}},
		{Name: "uint64_max", Ops: []Op{OpUint64(math.MaxUint64)}},

		// --- float32 (bit-exact) ---
		{Name: "float32_zero", Ops: []Op{OpFloat32(0.0)}},
		{Name: "float32_neg_zero", Ops: []Op{OpFloat32Bits(0x80000000)}},
		{Name: "float32_pi", Ops: []Op{OpFloat32(3.14)}},
		{Name: "float32_pos_inf", Ops: []Op{OpFloat32(float32(math.Inf(1)))}},
		{Name: "float32_neg_inf", Ops: []Op{OpFloat32(float32(math.Inf(-1)))}},
		{Name: "float32_nan", Ops: []Op{OpFloat32Bits(0x7FC00000)}}, // canonical quiet NaN

		// --- float64 (bit-exact) ---
		{Name: "float64_zero", Ops: []Op{OpFloat64(0.0)}},
		{Name: "float64_neg_zero", Ops: []Op{OpFloat64Bits(0x8000000000000000)}},
		{Name: "float64_pi", Ops: []Op{OpFloat64(3.141592653589793)}},
		{Name: "float64_pos_inf", Ops: []Op{OpFloat64(math.Inf(1))}},
		{Name: "float64_neg_inf", Ops: []Op{OpFloat64(math.Inf(-1))}},
		{Name: "float64_nan", Ops: []Op{OpFloat64Bits(0x7FF8000000000000)}}, // canonical quiet NaN

		// --- strings ---
		{Name: "string_empty", Ops: []Op{OpString("")}},
		{Name: "string_ascii", Ops: []Op{OpString("Alice")}},
		{Name: "string_utf8", Ops: []Op{OpString("héllo 世界 🚀")}},
		{Name: "string_len254", Ops: []Op{OpString(str254)}},
		{Name: "string_len255", Ops: []Op{OpString(str255)}},
		{Name: "string_len256", Ops: []Op{OpString(str256)}},
		{Name: "string_long", Ops: []Op{OpString(strLong)}},

		// --- bytes ---
		{Name: "bytes_empty", Ops: []Op{OpBytes([]byte{})}},
		{Name: "bytes_nonempty", Ops: []Op{OpBytes([]byte{0x00, 0x01, 0xFE, 0xFF, 0x7F, 0x80})}},
		// bytes boundary: exactly 255 bytes -> 5-byte length prefix.
		{Name: "bytes_len255", Ops: []Op{OpBytes(repeatByte(0xAB, 255))}},

		// --- arrays ---
		{Name: "array_int32_empty", Ops: []Op{OpArray(TypeInt32)}},
		{Name: "array_int32", Ops: []Op{OpArray(TypeInt32, OpInt32(1), OpInt32(2), OpInt32(3))}},
		{Name: "array_string", Ops: []Op{OpArray(TypeString, OpString("a"), OpString("bb"), OpString(""))}},
		{Name: "array_float64", Ops: []Op{OpArray(TypeFloat64, OpFloat64(1.5), OpFloat64(-2.25))}},

		// --- maps ---
		{Name: "map_empty", Ops: []Op{OpMap(TypeString, TypeInt32)}},
		{Name: "map_string_int32", Ops: []Op{OpMap(TypeString, TypeInt32,
			MapEntry{K: OpString("a"), V: OpInt32(1)},
			MapEntry{K: OpString("b"), V: OpInt32(2)},
		)}},
		{Name: "map_int64_string", Ops: []Op{OpMap(TypeInt64, TypeString,
			MapEntry{K: OpInt64(100), V: OpString("hundred")},
			MapEntry{K: OpInt64(-7), V: OpString("neg")},
		)}},

		// --- nested message ---
		{Name: "message_simple", Ops: []Op{OpMessage(
			OpString("Bob"), OpInt32(25), OpBool(true),
		)}},
		{Name: "message_nested", Ops: []Op{
			OpString("Alice"),
			OpMessage(OpString("NYC"), OpString("USA")),
		}},
		{Name: "message_empty", Ops: []Op{OpMessage()}},

		// --- mixed / realistic struct ---
		{Name: "mixed_record", Ops: []Op{
			OpString("user-42"),
			OpInt32(42),
			OpBool(true),
			OpFloat64(98.6),
			OpArray(TypeString, OpString("admin"), OpString("user")),
			OpMessage(OpInt64(math.MaxInt64), OpUint64(math.MaxUint64), OpBytes([]byte{0xDE, 0xAD, 0xBE, 0xEF})),
		}},
	}
}

func repeatByte(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

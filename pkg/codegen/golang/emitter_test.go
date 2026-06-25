package golang

import (
	"testing"

	"github.com/ElecTwix/xpb/pkg/ast"
)

func TestGenerate_SimpleMessage(t *testing.T) {
	file := &ast.File{
		Package: "test",
		Messages: []*ast.Message{
			{
				Name: "User",
				Fields: []*ast.Field{
					{Number: 1, Name: "name", Type: ast.FieldType{Kind: ast.TypeString}},
					{Number: 2, Name: "age", Type: ast.FieldType{Kind: ast.TypeInt32}},
				},
			},
		},
	}

	src, err := Generate(file)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)
	if output == "" {
		t.Error("Generate returned empty output")
	}

	// Verify key elements are present
	if !contains(output, "package test") {
		t.Error("Output should contain 'package test'")
	}
	if !contains(output, "type User struct") {
		t.Error("Output should contain 'type User struct'")
	}
	if !contains(output, "func (m *User) Marshal()") {
		t.Error("Output should contain Marshal method")
	}
	if !contains(output, "func (m *User) Unmarshal") {
		t.Error("Output should contain Unmarshal method")
	}

	// Decode (Phase 1) threads a register-local int cursor (`pos`) through the
	// stateless xpb.*At helpers instead of constructing a stateful Decoder.
	// This is the core of the local-cursor decode rewrite: no `dec :=
	// xpb.NewDecoder(data)`, a `pos := 0` local, and *At reads.
	if contains(output, "xpb.NewDecoder(") {
		t.Error("local-cursor decode must not construct a stateful Decoder (xpb.NewDecoder)")
	}
	if !contains(output, "pos := 0") {
		t.Errorf("decode should declare a local cursor `pos := 0`, got:\n%s", output)
	}
	if !contains(output, "xpb.ReadStringAt(data, pos)") {
		t.Error("string decode should use the stateless xpb.ReadStringAt(data, pos) helper")
	}
	if !contains(output, "xpb.ReadInt32At(data, pos)") {
		t.Error("int32 decode should use the stateless xpb.ReadInt32At(data, pos) helper")
	}
}

func TestGenerate_WithEnum(t *testing.T) {
	file := &ast.File{
		Package: "test",
		Enums: []*ast.Enum{
			{
				Name: "Status",
				Values: []*ast.EnumValue{
					{Name: "ACTIVE", Number: 1},
					{Name: "INACTIVE", Number: 2},
				},
			},
		},
		Messages: []*ast.Message{
			{
				Name: "User",
				Fields: []*ast.Field{
					{Number: 1, Name: "name", Type: ast.FieldType{Kind: ast.TypeString}},
					{Number: 2, Name: "status", Type: ast.FieldType{Kind: ast.TypeEnum}},
				},
			},
		},
	}

	src, err := Generate(file)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)

	// Verify enum is generated
	if !contains(output, "type Status int32") {
		t.Error("Output should contain 'type Status int32'")
	}
	if !contains(output, "Status_ACTIVE") {
		t.Error("Output should contain Status_ACTIVE constant")
	}
	if !contains(output, "Status_INACTIVE") {
		t.Error("Output should contain Status_INACTIVE constant")
	}
}

func TestGenerate_AllTypes(t *testing.T) {
	file := &ast.File{
		Package: "test",
		Messages: []*ast.Message{
			{
				Name: "AllTypes",
				Fields: []*ast.Field{
					{Number: 1, Name: "b", Type: ast.FieldType{Kind: ast.TypeBool}},
					{Number: 2, Name: "i32", Type: ast.FieldType{Kind: ast.TypeInt32}},
					{Number: 3, Name: "i64", Type: ast.FieldType{Kind: ast.TypeInt64}},
					{Number: 4, Name: "u32", Type: ast.FieldType{Kind: ast.TypeUint32}},
					{Number: 5, Name: "u64", Type: ast.FieldType{Kind: ast.TypeUint64}},
					{Number: 6, Name: "f32", Type: ast.FieldType{Kind: ast.TypeFloat32}},
					{Number: 7, Name: "f64", Type: ast.FieldType{Kind: ast.TypeFloat64}},
					{Number: 8, Name: "s", Type: ast.FieldType{Kind: ast.TypeString}},
					{Number: 9, Name: "data", Type: ast.FieldType{Kind: ast.TypeBytes}},
				},
			},
		},
	}

	src, err := Generate(file)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)

	// AllTypes is seven contiguous fixed-width fields (bool+int32+int64+uint32+
	// uint64+float32+float64 = 37 bytes) followed by string and bytes. Phase 3
	// coalesces that run: encode extends the local buffer ONCE via xpb.ExtendRun
	// and writes each fixed field with an unchecked Put*At writer at its known
	// offset; decode bounds-checks the whole run ONCE via xpb.EnsureRunAt and
	// reads each fixed field with an unchecked Run*At reader. The trailing
	// variable-length string/bytes still use the per-field Append*To/Read*At
	// helpers, since their byte span is data-dependent.
	runEncodeTests := []struct {
		name    string
		pattern string
	}{
		{"Bool", "xpb.PutBoolAt(buf, runOff+0, m.B)"},
		{"Int32", "xpb.PutInt32At(buf, runOff+1, m.I32)"},
		{"Int64", "xpb.PutInt64At(buf, runOff+5, m.I64)"},
		{"Uint32", "xpb.PutUint32At(buf, runOff+13, m.U32)"},
		{"Uint64", "xpb.PutUint64At(buf, runOff+17, m.U64)"},
		{"Float32", "xpb.PutFloat32At(buf, runOff+25, m.F32)"},
		{"Float64", "xpb.PutFloat64At(buf, runOff+29, m.F64)"},
	}
	for _, tt := range runEncodeTests {
		if !contains(output, tt.pattern) {
			t.Errorf("coalesced encode should write %s field via %q, got:\n%s", tt.name, tt.pattern, output)
		}
	}
	if !contains(output, "xpb.ExtendRun(buf, 37)") {
		t.Errorf("coalesced encode should extend the local buffer once by the run width (37), got:\n%s", output)
	}

	runDecodeTests := []struct {
		name    string
		pattern string
	}{
		{"Bool", "m.B = xpb.RunBoolAt(data, pos+0)"},
		{"Int32", "m.I32 = xpb.RunInt32At(data, pos+1)"},
		{"Int64", "m.I64 = xpb.RunInt64At(data, pos+5)"},
		{"Uint32", "m.U32 = xpb.RunUint32At(data, pos+13)"},
		{"Uint64", "m.U64 = xpb.RunUint64At(data, pos+17)"},
		{"Float32", "m.F32 = xpb.RunFloat32At(data, pos+25)"},
		{"Float64", "m.F64 = xpb.RunFloat64At(data, pos+29)"},
	}
	for _, tt := range runDecodeTests {
		if !contains(output, tt.pattern) {
			t.Errorf("coalesced decode should read %s field via %q, got:\n%s", tt.name, tt.pattern, output)
		}
	}
	if !contains(output, "xpb.EnsureRunAt(data, pos, 37)") {
		t.Errorf("coalesced decode should bounds-check the whole run once (37 bytes), got:\n%s", output)
	}

	// A coalesced fixed-width run must NOT emit the per-field Append*To/*At
	// helpers for the fields inside the run — that would mean the run was not
	// coalesced (the very regression Phase 3 fixes).
	for _, banned := range []string{
		"xpb.AppendBoolTo(buf, ", "xpb.AppendInt32To(buf, ", "xpb.AppendInt64To(buf, ",
		"xpb.AppendUint32To(buf, ", "xpb.AppendUint64To(buf, ",
		"xpb.AppendFloat32To(buf, ", "xpb.AppendFloat64To(buf, ",
	} {
		if contains(output, banned) {
			t.Errorf("fields inside a coalesced fixed-width run must not use the per-field helper %q", banned)
		}
	}

	// The trailing variable-length fields still use the per-field append helpers.
	if !contains(output, "xpb.AppendStringTo(buf, ") {
		t.Error("string (var-length, breaks the run) should still use xpb.AppendStringTo")
	}
	if !contains(output, "xpb.AppendBytesTo(buf, ") {
		t.Error("bytes (var-length, breaks the run) should still use xpb.AppendBytesTo")
	}

	// The local-buffer encode must NOT call the per-field stateful Encoder
	// Write* methods (each of which does enc.buf = append(enc.buf, ...),
	// reloading the 3-word slice header through memory every field).
	for _, banned := range []string{
		"enc.WriteBool(", "enc.WriteInt32(", "enc.WriteInt64(",
		"enc.WriteString(", "enc.WriteBytes(",
	} {
		if contains(output, banned) {
			t.Errorf("local-buffer encode must not call the stateful %q; use the Append*To helpers", banned)
		}
	}

	// Marshal binds the local once, grows once, and writes back exactly once.
	if !contains(output, "buf := enc.Buf()") {
		t.Error("encode should bind a register-local buffer via `buf := enc.Buf()`")
	}
	if !contains(output, "xpb.GrowBuf(buf, ") {
		t.Error("encode should grow the local buffer once up front via xpb.GrowBuf")
	}
	if !contains(output, "enc.SetBuf(buf)") {
		t.Error("encode should write the local buffer back to the encoder once via enc.SetBuf(buf)")
	}
}

// TestCoalesceRuns_MinLengthAndBreaks proves the Phase 3 run detection:
//   - a contiguous run of >= 2 fixed-width fields coalesces into one
//     ExtendRun/EnsureRunAt block;
//   - a single isolated fixed-width field (run of length 1, e.g. one separated by
//     a variable-length field on both sides) keeps the per-field Append*To/Read*At
//     path — coalescing a run of one would obscure the code for no gain;
//   - variable-length and optional fields break runs.
func TestCoalesceRuns_MinLengthAndBreaks(t *testing.T) {
	// id(int32) | sep(string) | a(int32) b(int64) [run of 2] | tail(string) |
	// c(int32) [run of 1, isolated].
	file := &ast.File{
		Package: "test",
		Messages: []*ast.Message{
			{
				Name: "Runs",
				Fields: []*ast.Field{
					{Number: 1, Name: "id", Type: ast.FieldType{Kind: ast.TypeInt32}},
					{Number: 2, Name: "sep", Type: ast.FieldType{Kind: ast.TypeString}},
					{Number: 3, Name: "a", Type: ast.FieldType{Kind: ast.TypeInt32}},
					{Number: 4, Name: "b", Type: ast.FieldType{Kind: ast.TypeInt64}},
					{Number: 5, Name: "tail", Type: ast.FieldType{Kind: ast.TypeString}},
					{Number: 6, Name: "c", Type: ast.FieldType{Kind: ast.TypeInt32}},
				},
			},
		},
	}
	src, err := Generate(file)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	output := string(src)

	// a(4)+b(8) is the only coalescable run: 12 bytes.
	if !contains(output, "xpb.EnsureRunAt(data, pos, 12)") {
		t.Errorf("the a+b run (12 bytes) should be coalesced on decode, got:\n%s", output)
	}
	if !contains(output, "xpb.ExtendRun(buf, 12)") {
		t.Errorf("the a+b run (12 bytes) should be coalesced on encode, got:\n%s", output)
	}
	if !contains(output, "m.A = xpb.RunInt32At(data, pos+0)") || !contains(output, "m.B = xpb.RunInt64At(data, pos+4)") {
		t.Errorf("coalesced decode offsets for a/b are wrong, got:\n%s", output)
	}

	// id (isolated by the following string) and c (isolated, trailing) are runs
	// of length 1: they keep the per-field cursor helpers, NOT a Run*At reader.
	if !contains(output, "m.Id = v") || !contains(output, "xpb.ReadInt32At(data, pos)") {
		t.Errorf("isolated int32 fields (run of 1) should keep the per-field xpb.ReadInt32At path, got:\n%s", output)
	}
	// There must be exactly one coalesced run; the isolated fields must not be
	// folded into a Run*At read.
	if contains(output, "m.Id = xpb.RunInt32At") || contains(output, "m.C = xpb.RunInt32At") {
		t.Errorf("a run of length 1 must not be coalesced, got:\n%s", output)
	}
}

// TestCoalesceRuns_RepeatedAndMapBreakRuns proves that repeated and map fields
// break a coalesced fixed-width run: a fixed scalar placed immediately adjacent
// to a repeated field or a map field must NOT have that variable-length field's
// span folded into the run width (which would corrupt the offset arithmetic).
// The fixedRunWidth guard returns (0,false) for Repeated/TypeMap, but the only
// previously-executed coverage placed repeated/map fields next to other
// variable-length fields; this pins the fixed-adjacent-to-repeated/map case.
func TestCoalesceRuns_RepeatedAndMapBreakRuns(t *testing.T) {
	// before(int32) [run of 1, broken by the following repeated] |
	// scores([]int32) | mid(int32) [run of 1, broken by the following map] |
	// counts(map<string,int32>) | a(int32) b(int32) [run of 2, the only run].
	mapType := ast.FieldType{
		Kind:    ast.TypeMap,
		KeyType: &ast.FieldType{Kind: ast.TypeString},
		ValType: &ast.FieldType{Kind: ast.TypeInt32},
	}
	file := &ast.File{
		Package: "test",
		Messages: []*ast.Message{
			{
				Name: "RM",
				Fields: []*ast.Field{
					{Number: 1, Name: "before", Type: ast.FieldType{Kind: ast.TypeInt32}},
					{Number: 2, Name: "scores", Type: ast.FieldType{Kind: ast.TypeInt32}, Repeated: true},
					{Number: 3, Name: "mid", Type: ast.FieldType{Kind: ast.TypeInt32}},
					{Number: 4, Name: "counts", Type: mapType},
					{Number: 5, Name: "a", Type: ast.FieldType{Kind: ast.TypeInt32}},
					{Number: 6, Name: "b", Type: ast.FieldType{Kind: ast.TypeInt32}},
				},
			},
		},
	}
	src, err := Generate(file)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	output := string(src)

	// The ONLY coalesced run is a+b (two int32 = 8 bytes). If a repeated or map
	// field were wrongly folded into a run, a different (larger) EnsureRunAt/
	// ExtendRun width would appear, or `before`/`mid` would become Run*At reads.
	if !contains(output, "xpb.EnsureRunAt(data, pos, 8)") || !contains(output, "xpb.ExtendRun(buf, 8)") {
		t.Errorf("a+b should be the only coalesced run (8 bytes), got:\n%s", output)
	}
	// `before` (followed by a repeated) and `mid` (followed by a map) are runs of
	// length 1 and must keep the per-field path, never a coalesced Run*At read.
	if contains(output, "m.Before = xpb.RunInt32At") {
		t.Errorf("a fixed field adjacent to a repeated field must not be coalesced, got:\n%s", output)
	}
	if contains(output, "m.Mid = xpb.RunInt32At") {
		t.Errorf("a fixed field adjacent to a map field must not be coalesced, got:\n%s", output)
	}
	// Repeated/map decode still goes through the bounds-checked array-count helper.
	if !contains(output, "xpb.ReadArrayCountAt(") {
		t.Errorf("repeated/map decode must still use xpb.ReadArrayCountAt, got:\n%s", output)
	}
}

// TestFixedSizeLowerBound asserts the exact grow-once lower bound the emitter
// computes, per the contract that it sums only the provably-emitted fixed bytes
// (fixed-width scalars/enums at their wire width, one presence byte per optional,
// a 4-byte count per repeated/map) and counts variable-length string/bytes/
// message fields as 0. A regression that under-counts (silently re-grown by the
// Append*To helpers) or over-counts (wastes capacity past what the message
// provably needs) is caught here, not just that "xpb.GrowBuf(buf, " appears.
// (Review finding: the prior round asserted the call exists but never its value.)
func TestFixedSizeLowerBound(t *testing.T) {
	cases := []struct {
		name   string
		fields []*ast.Field
		want   int
	}{
		{
			// All fixed-width scalars: bool(1)+int32(4)+int64(8)+uint32(4)+
			// uint64(8)+float32(4)+float64(8) = 37.
			name: "all_fixed_scalars",
			fields: []*ast.Field{
				{Name: "b", Type: ast.FieldType{Kind: ast.TypeBool}},
				{Name: "i32", Type: ast.FieldType{Kind: ast.TypeInt32}},
				{Name: "i64", Type: ast.FieldType{Kind: ast.TypeInt64}},
				{Name: "u32", Type: ast.FieldType{Kind: ast.TypeUint32}},
				{Name: "u64", Type: ast.FieldType{Kind: ast.TypeUint64}},
				{Name: "f32", Type: ast.FieldType{Kind: ast.TypeFloat32}},
				{Name: "f64", Type: ast.FieldType{Kind: ast.TypeFloat64}},
			},
			want: 37,
		},
		{
			// Variable-length only: string + bytes both count 0.
			name: "var_length_only",
			fields: []*ast.Field{
				{Name: "s", Type: ast.FieldType{Kind: ast.TypeString}},
				{Name: "data", Type: ast.FieldType{Kind: ast.TypeBytes}},
			},
			want: 0,
		},
		{
			// Mixed: int32(4) + optional string(1 presence byte, value var) +
			// repeated int32(4-byte count, elements var) + int64(8) = 17.
			name: "mixed_required_optional_repeated",
			fields: []*ast.Field{
				{Name: "id", Type: ast.FieldType{Kind: ast.TypeInt32}},
				{Name: "method", Type: ast.FieldType{Kind: ast.TypeString}, Optional: true},
				{Name: "scores", Type: ast.FieldType{Kind: ast.TypeInt32}, Repeated: true},
				{Name: "ts", Type: ast.FieldType{Kind: ast.TypeInt64}},
			},
			want: 17,
		},
		{
			// The uteka message: Type(4)+Id(0)+Method opt(1)+Payload opt(1)+
			// Timestamp(8)+Error opt(1)+StreamId opt(1)+Seq(8)+Flags(4)+
			// SessionId opt(1) = 29. This is the value the generated benchmark
			// fixtures must carry.
			name: "uteka_message",
			fields: []*ast.Field{
				{Name: "Type", Type: ast.FieldType{Kind: ast.TypeInt32}},
				{Name: "Id", Type: ast.FieldType{Kind: ast.TypeString}},
				{Name: "Method", Type: ast.FieldType{Kind: ast.TypeString}, Optional: true},
				{Name: "Payload", Type: ast.FieldType{Kind: ast.TypeBytes}, Optional: true},
				{Name: "Timestamp", Type: ast.FieldType{Kind: ast.TypeInt64}},
				{Name: "Error", Type: ast.FieldType{Kind: ast.TypeString}, Optional: true},
				{Name: "StreamId", Type: ast.FieldType{Kind: ast.TypeString}, Optional: true},
				{Name: "Seq", Type: ast.FieldType{Kind: ast.TypeInt64}},
				{Name: "Flags", Type: ast.FieldType{Kind: ast.TypeInt32}},
				{Name: "SessionId", Type: ast.FieldType{Kind: ast.TypeString}, Optional: true},
			},
			want: 29,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &Generator{enums: make(map[string]bool)}
			got := g.fixedSizeLowerBound(&ast.Message{Name: "M", Fields: tc.fields})
			if got != tc.want {
				t.Errorf("fixedSizeLowerBound = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestFixedSizeLowerBound_EnumCountsAsInt32 proves an enum field (which shares
// Kind==TypeMessage in the AST but resolves via the Generator's enum set)
// contributes a fixed 4 bytes to the lower bound, since enums encode as int32.
func TestFixedSizeLowerBound_EnumCountsAsInt32(t *testing.T) {
	g := &Generator{enums: map[string]bool{"Status": true}}
	msg := &ast.Message{
		Name: "M",
		Fields: []*ast.Field{
			{Name: "id", Type: ast.FieldType{Kind: ast.TypeInt32}},                          // 4
			{Name: "status", Type: ast.FieldType{Kind: ast.TypeMessage, Message: "Status"}}, // enum -> 4
		},
	}
	if got := g.fixedSizeLowerBound(msg); got != 8 {
		t.Errorf("fixedSizeLowerBound with enum = %d, want 8", got)
	}
}

func TestGenerate_EmptyPackage(t *testing.T) {
	file := &ast.File{
		Messages: []*ast.Message{
			{
				Name:   "Empty",
				Fields: []*ast.Field{},
			},
		},
	}

	src, err := Generate(file)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)
	// Default package name should be "main"
	if !contains(output, "package main") {
		t.Error("Output should default to 'package main'")
	}
	// A bodyless message decodes nothing, so its unmarshalAt must not declare
	// an unused decoder (that is a hard Go compile error). The authoritative
	// compile check lives in tests/integration/go_codegen_test.go; this is a
	// fast guard on the emitted text.
	if contains(output, "dec := xpb.NewDecoder(data)") {
		t.Error("empty message should not emit an unused decoder in unmarshalAt")
	}
	if !contains(output, "_ = data") {
		t.Error("empty message unmarshalAt should emit `_ = data` to use the parameter")
	}
}

func TestGenerate_RepeatedFields(t *testing.T) {
	file := &ast.File{
		Package: "test",
		Messages: []*ast.Message{
			{
				Name: "Container",
				Fields: []*ast.Field{
					{Number: 1, Name: "tags", Type: ast.FieldType{Kind: ast.TypeString}, Repeated: true},
					{Number: 2, Name: "scores", Type: ast.FieldType{Kind: ast.TypeInt32}, Repeated: true},
				},
			},
		},
	}

	src, err := Generate(file)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)
	// Verify repeated fields generate correct code
	if !contains(output, "Tags") {
		t.Error("Output should contain camelCase field name 'Tags'")
	}
	if !contains(output, "Scores") {
		t.Error("Output should contain camelCase field name 'Scores'")
	}
}

func TestGenerate_OptionalFields(t *testing.T) {
	file := &ast.File{
		Package: "test",
		Messages: []*ast.Message{
			{
				Name: "Profile",
				Fields: []*ast.Field{
					{Number: 1, Name: "bio", Type: ast.FieldType{Kind: ast.TypeString}},
					{Number: 2, Name: "avatar_url", Type: ast.FieldType{Kind: ast.TypeString}, Optional: true},
				},
			},
		},
	}

	src, err := Generate(file)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)
	// Verify optional fields generate correct code
	if !contains(output, "AvatarUrl") {
		t.Error("Output should contain camelCase field name 'AvatarUrl'")
	}
}

func TestGenerate_MultipleMessages(t *testing.T) {
	file := &ast.File{
		Package: "test",
		Messages: []*ast.Message{
			{
				Name: "User",
				Fields: []*ast.Field{
					{Number: 1, Name: "name", Type: ast.FieldType{Kind: ast.TypeString}},
				},
			},
			{
				Name: "Address",
				Fields: []*ast.Field{
					{Number: 1, Name: "city", Type: ast.FieldType{Kind: ast.TypeString}},
					{Number: 2, Name: "country", Type: ast.FieldType{Kind: ast.TypeString}},
				},
			},
		},
	}

	src, err := Generate(file)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)

	// Verify both messages are generated
	if !contains(output, "type User struct") {
		t.Error("Output should contain User struct")
	}
	if !contains(output, "type Address struct") {
		t.Error("Output should contain Address struct")
	}
	if !contains(output, "func (m *User) Marshal()") {
		t.Error("Output should contain User Marshal method")
	}
	if !contains(output, "func (m *Address) Marshal()") {
		t.Error("Output should contain Address Marshal method")
	}
}

func TestGenerate_NestedMessage(t *testing.T) {
	file := &ast.File{
		Package: "test",
		Messages: []*ast.Message{
			{
				Name: "Address",
				Fields: []*ast.Field{
					{Number: 1, Name: "city", Type: ast.FieldType{Kind: ast.TypeString}},
				},
			},
			{
				Name: "User",
				Fields: []*ast.Field{
					{Number: 1, Name: "name", Type: ast.FieldType{Kind: ast.TypeString}},
					{Number: 2, Name: "addr", Type: ast.FieldType{Kind: ast.TypeMessage, Message: "Address"}},
				},
			},
		},
	}

	src, err := Generate(file)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)

	// Verify nested message generates correct type (pointer)
	if !contains(output, "Addr *Address") {
		t.Error("Output should contain 'Addr *Address' field type")
	}

	// Nested-message decode must guard the recursive unmarshalAt on the
	// envelope body length (`len(mb) > 0`). Without the guard, a 0-length
	// envelope (which a caller of the encode side produces when the field is
	// nil) triggers `unexpected EOF` at the nested type's first ReadString /
	// ReadBytes. The local-cursor decode reads the envelope into `mb` via
	// xpb.ReadMessageBytesAt and recurses with depth+1.
	if !contains(output, "if len(mb) > 0 {") {
		t.Error("Output should guard nested unmarshalAt on len(mb) > 0 to round-trip nil pointers")
	}
	if !contains(output, "xpb.ReadMessageBytesAt(data, pos)") {
		t.Error("Output should read the nested-message envelope via xpb.ReadMessageBytesAt(data, pos)")
	}

	// Nested-message encode must nil-guard MarshalTo. Without the guard,
	// a caller passing a nil pointer (an absent optional field, or a nil
	// entry inside a repeated/map slice) would panic at `nil.MarshalTo`.
	// With the guard, a nil pointer emits a 0-length envelope, which the
	// decode side maps back to nil. (Check the prefix only; gofmt may
	// re-break the single-line `if X { Y }` into a multi-line block.)
	if !contains(output, "if m.Addr != nil") || !contains(output, "m.Addr.MarshalTo(nestedEnc)") {
		t.Error("Output should guard nested MarshalTo on `m.Field != nil` to handle nil pointers without panicking")
	}
	// The nested-message envelope is appended into the parent's local buffer
	// via the length-prefixed xpb.AppendMessageTo helper (Phase 2 local-buffer
	// encode), not the stateful enc.WriteMessage.
	if !contains(output, "xpb.AppendMessageTo(buf, nestedEnc.Bytes())") {
		t.Error("Output should append the nested-message envelope via xpb.AppendMessageTo(buf, nestedEnc.Bytes())")
	}
	if contains(output, "enc.WriteMessage(") {
		t.Error("local-buffer encode must not call the stateful enc.WriteMessage")
	}
}

func TestGenerator_DefaultPackage(t *testing.T) {
	// Test that empty package name defaults to "main"
	file := &ast.File{
		Messages: []*ast.Message{
			{
				Name: "Test",
				Fields: []*ast.Field{
					{Number: 1, Name: "value", Type: ast.FieldType{Kind: ast.TypeInt32}},
				},
			},
		},
	}

	src, err := Generate(file)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)
	if !contains(output, "package main") {
		t.Error("Output should default to 'package main'")
	}
}

// valueOptFile builds a message with one required string and one optional
// string + one optional bytes, used by the option tests below.
func valueOptFile() *ast.File {
	return &ast.File{
		Package: "test",
		Messages: []*ast.Message{
			{
				Name: "Msg",
				Fields: []*ast.Field{
					{Number: 1, Name: "id", Type: ast.FieldType{Kind: ast.TypeString}},
					{Number: 2, Name: "method", Type: ast.FieldType{Kind: ast.TypeString}, Optional: true},
					{Number: 3, Name: "payload", Type: ast.FieldType{Kind: ast.TypeBytes}, Optional: true},
				},
			},
		},
	}
}

func TestGenerate_DefaultOptionalIsPointer(t *testing.T) {
	src, err := Generate(valueOptFile())
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	output := string(src)
	if !contains(output, "*string") {
		t.Errorf("default style should keep pointer optional (*string), got:\n%s", output)
	}
	if contains(output, "HasMethod") {
		t.Error("default style must not emit a presence bool field")
	}
}

func TestGenerate_ValueOptionalStyle(t *testing.T) {
	src, err := GenerateWithOptions(valueOptFile(), Options{OptionalStyle: OptionalValue})
	if err != nil {
		t.Fatalf("GenerateWithOptions failed: %v", err)
	}
	output := string(src)

	// Struct: presence bool fields exist (gofmt aligns the type column, so
	// match field names, not exact spacing).
	if !contains(output, "HasMethod") {
		t.Errorf("value style should emit a HasMethod field, got:\n%s", output)
	}
	if !contains(output, "HasPayload") {
		t.Error("value style should emit a HasPayload field")
	}
	if contains(output, "*string") {
		t.Error("value style must not produce pointer optionals (*string)")
	}
	if contains(output, "*[]byte") {
		t.Error("value style must not produce pointer optionals (*[]byte)")
	}

	// Encode (Phase 2, local buffer): presence driven by Has<Field> via the
	// stateless append helper, value appended directly (no pointer deref).
	if !contains(output, "xpb.AppendBoolTo(buf, m.HasMethod)") {
		t.Error("encode should gate on m.HasMethod via xpb.AppendBoolTo(buf, ...)")
	}
	if !contains(output, "xpb.AppendStringTo(buf, m.Method)") {
		t.Error("encode should append m.Method directly via xpb.AppendStringTo(buf, ...) (no deref)")
	}

	// Decode: set value + presence bool.
	if !contains(output, "m.HasMethod = true") {
		t.Error("decode should set m.HasMethod = true when present")
	}
}

func TestGenerate_ZeroCopyBytes(t *testing.T) {
	src, err := GenerateWithOptions(valueOptFile(), Options{ZeroCopyBytes: true})
	if err != nil {
		t.Fatalf("GenerateWithOptions failed: %v", err)
	}
	output := string(src)
	if !contains(output, "ReadBytesUnsafe") {
		t.Errorf("zero-copy should decode bytes via ReadBytesUnsafe, got:\n%s", output)
	}
}

func TestGenerate_DefaultBytesCopies(t *testing.T) {
	src, err := Generate(valueOptFile())
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if contains(string(src), "ReadBytesUnsafe") {
		t.Error("default should use copying ReadBytes, not ReadBytesUnsafe")
	}
}

func BenchmarkGenerate_Simple(b *testing.B) {
	file := &ast.File{
		Package: "test",
		Messages: []*ast.Message{
			{
				Name: "User",
				Fields: []*ast.Field{
					{Number: 1, Name: "name", Type: ast.FieldType{Kind: ast.TypeString}},
					{Number: 2, Name: "age", Type: ast.FieldType{Kind: ast.TypeInt32}},
					{Number: 3, Name: "active", Type: ast.FieldType{Kind: ast.TypeBool}},
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Generate(file)
	}
}

func BenchmarkGenerate_Medium(b *testing.B) {
	file := &ast.File{
		Package: "test",
		Messages: []*ast.Message{
			{
				Name: "User",
				Fields: []*ast.Field{
					{Number: 1, Name: "name", Type: ast.FieldType{Kind: ast.TypeString}},
					{Number: 2, Name: "email", Type: ast.FieldType{Kind: ast.TypeString}},
					{Number: 3, Name: "age", Type: ast.FieldType{Kind: ast.TypeInt32}},
					{Number: 4, Name: "score", Type: ast.FieldType{Kind: ast.TypeFloat64}},
					{Number: 5, Name: "active", Type: ast.FieldType{Kind: ast.TypeBool}},
					{Number: 6, Name: "tags", Type: ast.FieldType{Kind: ast.TypeString}, Repeated: true},
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Generate(file)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

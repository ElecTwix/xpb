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

	// Verify all types are handled
	typeTests := []struct {
		name    string
		pattern string
	}{
		{"Bool", "WriteBool"},
		{"Int32", "WriteInt32"},
		{"Int64", "WriteInt64"},
		{"Uint32", "WriteUint32"},
		{"Uint64", "WriteUint64"},
		{"Float32", "WriteFloat32"},
		{"Float64", "WriteFloat64"},
		{"String", "WriteString"},
		{"Bytes", "WriteBytes"},
	}

	for _, tt := range typeTests {
		if !contains(output, tt.pattern) {
			t.Errorf("Output should contain %s method call", tt.name)
		}
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

	// Encode: presence driven by Has<Field>, value passed directly.
	if !contains(output, "enc.WriteBool(m.HasMethod)") {
		t.Error("encode should gate on m.HasMethod")
	}
	if !contains(output, "enc.WriteString(m.Method)") {
		t.Error("encode should write m.Method directly (no deref)")
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

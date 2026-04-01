package typescript

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
	if !contains(output, "export interface UserData") {
		t.Error("Output should contain 'export interface UserData'")
	}
	if !contains(output, "export class User") {
		t.Error("Output should contain 'export class User'")
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
	if !contains(output, "export enum Status") {
		t.Error("Output should contain 'export enum Status'")
	}
	if !contains(output, "ACTIVE = 1") {
		t.Error("Output should contain Status.ACTIVE = 1")
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
		{"Bool", "writeBool"},
		{"Int32", "writeInt32"},
		{"Int64", "writeInt64"},
		{"Uint32", "writeUint32"},
		{"Uint64", "writeUint64"},
		{"Float32", "writeFloat32"},
		{"Float64", "writeFloat64"},
		{"String", "writeString"},
		{"Bytes", "writeBytes"},
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
	// Empty message should still generate interface and class
	if !contains(output, "export interface EmptyData") {
		t.Error("Output should contain 'export interface EmptyData'")
	}
	if !contains(output, "export class Empty") {
		t.Error("Output should contain 'export class Empty'")
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
	if !contains(output, "tags: string[]") {
		t.Error("Output should contain 'tags: string[]'")
	}
	if !contains(output, "scores: number[]") {
		t.Error("Output should contain 'scores: number[]'")
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
	if !contains(output, "avatarUrl?: string") {
		t.Error("Output should contain 'avatarUrl?: string'")
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
	if !contains(output, "export interface UserData") {
		t.Error("Output should contain UserData interface")
	}
	if !contains(output, "export interface AddressData") {
		t.Error("Output should contain AddressData interface")
	}
	if !contains(output, "export class User") {
		t.Error("Output should contain User class")
	}
	if !contains(output, "export class Address") {
		t.Error("Output should contain Address class")
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

	// Verify nested message generates correct type
	if !contains(output, "addr: Address") {
		t.Error("Output should contain 'addr: Address' field type")
	}
}

func TestGenerate_MapField(t *testing.T) {
	file := &ast.File{
		Package: "test",
		Messages: []*ast.Message{
			{
				Name: "Container",
				Fields: []*ast.Field{
					{Number: 1, Name: "scores", Type: ast.FieldType{
						Kind:    ast.TypeMap,
						KeyType: &ast.FieldType{Kind: ast.TypeString},
						ValType: &ast.FieldType{Kind: ast.TypeInt32},
					}},
				},
			},
		},
	}

	_, err := Generate(file)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
}

func TestGenerator_Export(t *testing.T) {
	file := &ast.File{
		Package: "testpkg",
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
	// Verify exports
	if !contains(output, "export") {
		t.Error("Output should contain 'export' keyword")
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

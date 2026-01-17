// Package integration contains end-to-end tests for the TypeScript codegen.
package integration

import (
	"strings"
	"testing"

	"github.com/anthropic/xpb/pkg/ast"
	"github.com/anthropic/xpb/pkg/codegen/typescript"
)

func TestTSCodegen_SimpleMessage(t *testing.T) {
	schema := &ast.File{
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

	src, err := typescript.Generate(schema)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)
	if output == "" {
		t.Fatal("Generate returned empty output")
	}

	// Verify generated code structure
	checks := []struct {
		name    string
		pattern string
	}{
		{"import statement", "import { Encoder, Decoder }"},
		{"interface", "export interface UserData"},
		{"class", "export class User"},
		{"constructor", "constructor("},
		{"encode method", "encode(): Uint8Array"},
		{"decode method", "static decode("},
	}

	for _, check := range checks {
		if !strings.Contains(output, check.pattern) {
			t.Errorf("Missing %s: pattern '%s'", check.name, check.pattern)
		}
	}
}

func TestTSCodegen_WithEnum(t *testing.T) {
	schema := &ast.File{
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

	src, err := typescript.Generate(schema)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)

	// Verify enum generation
	if !strings.Contains(output, "export enum Status") {
		t.Error("Missing 'export enum Status'")
	}
	if !strings.Contains(output, "ACTIVE = 1") {
		t.Error("Missing 'ACTIVE = 1' in enum")
	}
}

func TestTSCodegen_AllTypes(t *testing.T) {
	schema := &ast.File{
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

	src, err := typescript.Generate(schema)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)

	// Verify all types generate correct TypeScript types (lowercase field names)
	checks := []struct {
		name    string
		pattern string
	}{
		{"bool in interface", "b: boolean"},
		{"int32 in interface", "i32: number"},
		{"int64 in interface", "i64: bigint"},
		{"uint32 in interface", "u32: number"},
		{"uint64 in interface", "u64: bigint"},
		{"float32 in interface", "f32: number"},
		{"float64 in interface", "f64: number"},
		{"string in interface", "s: string"},
		{"bytes in interface", "data: Uint8Array"},
	}

	for _, check := range checks {
		if !strings.Contains(output, check.pattern) {
			t.Errorf("Missing %s: pattern '%s'", check.name, check.pattern)
		}
	}
}

func TestTSCodegen_RepeatedFields(t *testing.T) {
	schema := &ast.File{
		Package: "test",
		Messages: []*ast.Message{
			{
				Name: "Container",
				Fields: []*ast.Field{
					{Number: 1, Name: "name", Type: ast.FieldType{Kind: ast.TypeString}},
					{Number: 2, Name: "tags", Type: ast.FieldType{Kind: ast.TypeString}, Repeated: true},
					{Number: 3, Name: "scores", Type: ast.FieldType{Kind: ast.TypeInt32}, Repeated: true},
				},
			},
		},
	}

	src, err := typescript.Generate(schema)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)

	// Verify repeated fields generate array types (lowercase field names)
	if !strings.Contains(output, "tags: string[]") {
		t.Error("Missing 'tags: string[]' in interface")
	}
	if !strings.Contains(output, "scores: number[]") {
		t.Error("Missing 'scores: number[]' in interface")
	}
}

func TestTSCodegen_OptionalFields(t *testing.T) {
	schema := &ast.File{
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

	src, err := typescript.Generate(schema)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)

	// Verify optional fields have ? in TypeScript (lowercase field name)
	if !strings.Contains(output, "avatarUrl?: string") {
		t.Error("Missing 'avatarUrl?: string' for optional field")
	}
}

func TestTSCodegen_NestedMessages(t *testing.T) {
	schema := &ast.File{
		Package: "test",
		Messages: []*ast.Message{
			{
				Name: "Address",
				Fields: []*ast.Field{
					{Number: 1, Name: "city", Type: ast.FieldType{Kind: ast.TypeString}},
				},
			},
			{
				Name: "Person",
				Fields: []*ast.Field{
					{Number: 1, Name: "name", Type: ast.FieldType{Kind: ast.TypeString}},
					{Number: 2, Name: "addr", Type: ast.FieldType{Kind: ast.TypeMessage, Message: "Address"}},
				},
			},
		},
	}

	src, err := typescript.Generate(schema)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)

	// Verify both messages are generated
	if !strings.Contains(output, "export interface AddressData") {
		t.Error("Missing 'export interface AddressData'")
	}
	if !strings.Contains(output, "export interface PersonData") {
		t.Error("Missing 'export interface PersonData'")
	}
	if !strings.Contains(output, "addr: Address") {
		t.Error("Missing 'addr: Address' field type")
	}
}

func TestTSCodegen_MultipleMessages(t *testing.T) {
	schema := &ast.File{
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
				},
			},
		},
	}

	src, err := typescript.Generate(schema)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)

	// Verify both messages are generated
	if !strings.Contains(output, "export interface UserData") {
		t.Error("Missing 'export interface UserData'")
	}
	if !strings.Contains(output, "export interface AddressData") {
		t.Error("Missing 'export interface AddressData'")
	}
	if !strings.Contains(output, "export class User") {
		t.Error("Missing 'export class User'")
	}
	if !strings.Contains(output, "export class Address") {
		t.Error("Missing 'export class Address'")
	}
}

func TestTSCodegen_MapField(t *testing.T) {
	schema := &ast.File{
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

	_, err := typescript.Generate(schema)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	// Map field generation should not error
}

func TestTSCodegen_TypeScriptSyntax(t *testing.T) {
	schema := &ast.File{
		Package: "test",
		Messages: []*ast.Message{
			{
				Name: "Test",
				Fields: []*ast.Field{
					{Number: 1, Name: "value", Type: ast.FieldType{Kind: ast.TypeInt32}},
				},
			},
		},
	}

	src, err := typescript.Generate(schema)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(src)

	// Verify TypeScript syntax elements
	checks := []struct {
		name    string
		pattern string
	}{
		{"export keyword", "export "},
		{"interface keyword", "interface "},
		{"class keyword", "class "},
		{"constructor keyword", "constructor"},
		{"type annotations", ":"},
		{"method definitions", "):"},
	}

	for _, check := range checks {
		if !strings.Contains(output, check.pattern) {
			t.Errorf("Missing %s: pattern '%s'", check.name, check.pattern)
		}
	}
}

// BenchmarkTSCodegen_Simple benchmarks simple message generation
func BenchmarkTSCodegen_Simple(b *testing.B) {
	schema := &ast.File{
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = typescript.Generate(schema)
	}
}

// BenchmarkTSCodegen_Medium benchmarks medium message generation
func BenchmarkTSCodegen_Medium(b *testing.B) {
	schema := &ast.File{
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
		_, _ = typescript.Generate(schema)
	}
}

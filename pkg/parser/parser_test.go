package parser

import (
	"testing"

	"github.com/anthropic/xpb/pkg/ast"
)

func TestLexer_BasicTokens(t *testing.T) {
	input := `package myapp

message User {
    1: string name
    2: int32 age
    3: ?bool active
}
`
	lexer := NewLexer(input)
	expected := []struct {
		kind  TokenKind
		value string
	}{
		{TokenPackage, "package"},
		{TokenIdent, "myapp"},
		{TokenNewline, "\n"},
		{TokenNewline, "\n"},
		{TokenMessage, "message"},
		{TokenIdent, "User"},
		{TokenLBrace, "{"},
		{TokenNewline, "\n"},
		{TokenNumber, "1"},
		{TokenColon, ":"},
		{TokenIdent, "string"},
		{TokenIdent, "name"},
		{TokenNewline, "\n"},
		{TokenNumber, "2"},
		{TokenColon, ":"},
		{TokenIdent, "int32"},
		{TokenIdent, "age"},
		{TokenNewline, "\n"},
		{TokenNumber, "3"},
		{TokenColon, ":"},
		{TokenQuestion, "?"},
		{TokenIdent, "bool"},
		{TokenIdent, "active"},
		{TokenNewline, "\n"},
		{TokenRBrace, "}"},
		{TokenNewline, "\n"},
		{TokenEOF, ""},
	}

	for i, exp := range expected {
		tok := lexer.Next()
		if tok.Kind != exp.kind {
			t.Errorf("token %d: kind = %v, want %v", i, tok.Kind, exp.kind)
		}
		if tok.Value != exp.value {
			t.Errorf("token %d: value = %q, want %q", i, tok.Value, exp.value)
		}
	}
}

func TestLexer_Comments(t *testing.T) {
	input := `// This is a comment
package myapp
// Another comment
message User {
    1: string name // inline comment
}
`
	lexer := NewLexer(input)

	// First token should be comment
	tok := lexer.Next()
	if tok.Kind != TokenComment {
		t.Errorf("expected comment, got %v", tok.Kind)
	}

	// Then newline
	tok = lexer.Next()
	if tok.Kind != TokenNewline {
		t.Errorf("expected newline, got %v", tok.Kind)
	}

	// Then package
	tok = lexer.Next()
	if tok.Kind != TokenPackage {
		t.Errorf("expected package, got %v", tok.Kind)
	}
}

func TestParser_BasicMessage(t *testing.T) {
	input := `package myapp

message User {
    1: string name
    2: int32 age
    3: bool active
}
`
	file, err := ParseFile(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if file.Package != "myapp" {
		t.Errorf("Package = %q, want %q", file.Package, "myapp")
	}

	if len(file.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(file.Messages))
	}

	msg := file.Messages[0]
	if msg.Name != "User" {
		t.Errorf("Message.Name = %q, want %q", msg.Name, "User")
	}

	if len(msg.Fields) != 3 {
		t.Fatalf("len(Fields) = %d, want 3", len(msg.Fields))
	}

	// Check first field
	f := msg.Fields[0]
	if f.Number != 1 || f.Name != "name" || f.Type.Kind != ast.TypeString || f.Optional {
		t.Errorf("Field 0 = %+v, unexpected", f)
	}

	// Check second field
	f = msg.Fields[1]
	if f.Number != 2 || f.Name != "age" || f.Type.Kind != ast.TypeInt32 || f.Optional {
		t.Errorf("Field 1 = %+v, unexpected", f)
	}

	// Check third field
	f = msg.Fields[2]
	if f.Number != 3 || f.Name != "active" || f.Type.Kind != ast.TypeBool || f.Optional {
		t.Errorf("Field 2 = %+v, unexpected", f)
	}
}

func TestParser_OptionalField(t *testing.T) {
	input := `package test

message Profile {
    1: string bio
    2: ?string avatar_url
}
`
	file, err := ParseFile(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	msg := file.Messages[0]
	if len(msg.Fields) != 2 {
		t.Fatalf("len(Fields) = %d, want 2", len(msg.Fields))
	}

	// First field is required
	if msg.Fields[0].Optional {
		t.Error("Field 0 should not be optional")
	}

	// Second field is optional
	if !msg.Fields[1].Optional {
		t.Error("Field 1 should be optional")
	}
}

func TestParser_MultipleMessages(t *testing.T) {
	input := `package test

message User {
    1: string name
}

message Address {
    1: string city
    2: string country
}
`
	file, err := ParseFile(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if len(file.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(file.Messages))
	}

	if file.Messages[0].Name != "User" {
		t.Errorf("Message 0 name = %q, want User", file.Messages[0].Name)
	}

	if file.Messages[1].Name != "Address" {
		t.Errorf("Message 1 name = %q, want Address", file.Messages[1].Name)
	}
}

func TestParser_AllBasicTypes(t *testing.T) {
	input := `package test

message AllTypes {
    1: bool b
    2: int32 i32
    3: int64 i64
    4: uint32 u32
    5: uint64 u64
    6: float32 f32
    7: float64 f64
    8: string s
    9: bytes data
}
`
	file, err := ParseFile(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	msg := file.Messages[0]
	expectedTypes := []ast.TypeKind{
		ast.TypeBool,
		ast.TypeInt32,
		ast.TypeInt64,
		ast.TypeUint32,
		ast.TypeUint64,
		ast.TypeFloat32,
		ast.TypeFloat64,
		ast.TypeString,
		ast.TypeBytes,
	}

	if len(msg.Fields) != len(expectedTypes) {
		t.Fatalf("len(Fields) = %d, want %d", len(msg.Fields), len(expectedTypes))
	}

	for i, exp := range expectedTypes {
		if msg.Fields[i].Type.Kind != exp {
			t.Errorf("Field %d type = %v, want %v", i, msg.Fields[i].Type.Kind, exp)
		}
	}
}

func TestParser_Error_MissingBrace(t *testing.T) {
	input := `package test

message User {
    1: string name
`
	_, err := ParseFile(input)
	if err == nil {
		t.Error("expected error for missing closing brace")
	}
}

func TestParser_Error_InvalidFieldNumber(t *testing.T) {
	input := `package test

message User {
    abc: string name
}
`
	_, err := ParseFile(input)
	if err == nil {
		t.Error("expected error for invalid field number")
	}
}

func TestParser_RepeatedField(t *testing.T) {
	input := `package test

message User {
    1: string name
    2: []string tags
    3: []int32 scores
}
`
	file, err := ParseFile(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	msg := file.Messages[0]
	if len(msg.Fields) != 3 {
		t.Fatalf("len(Fields) = %d, want 3", len(msg.Fields))
	}

	// First field is not repeated
	if msg.Fields[0].Repeated {
		t.Error("Field 0 should not be repeated")
	}

	// Second field is repeated string
	if !msg.Fields[1].Repeated {
		t.Error("Field 1 should be repeated")
	}
	if msg.Fields[1].Type.Kind != ast.TypeString {
		t.Errorf("Field 1 type = %v, want TypeString", msg.Fields[1].Type.Kind)
	}

	// Third field is repeated int32
	if !msg.Fields[2].Repeated {
		t.Error("Field 2 should be repeated")
	}
	if msg.Fields[2].Type.Kind != ast.TypeInt32 {
		t.Errorf("Field 2 type = %v, want TypeInt32", msg.Fields[2].Type.Kind)
	}
}

func TestParser_MapField(t *testing.T) {
	input := `package test

message User {
    1: string name
    2: map<string, int32> scores
    3: map<string, string> metadata
}
`
	file, err := ParseFile(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	msg := file.Messages[0]
	if len(msg.Fields) != 3 {
		t.Fatalf("len(Fields) = %d, want 3", len(msg.Fields))
	}

	// Second field is map<string, int32>
	f := msg.Fields[1]
	if f.Type.Kind != ast.TypeMap {
		t.Errorf("Field 1 type = %v, want TypeMap", f.Type.Kind)
	}
	if f.Type.KeyType == nil || f.Type.KeyType.Kind != ast.TypeString {
		t.Error("Field 1 should have string key type")
	}
	if f.Type.ValType == nil || f.Type.ValType.Kind != ast.TypeInt32 {
		t.Errorf("Field 1 value type = %v, want TypeInt32", f.Type.ValType.Kind)
	}

	// Third field is map<string, string>
	f = msg.Fields[2]
	if f.Type.Kind != ast.TypeMap {
		t.Errorf("Field 2 type = %v, want TypeMap", f.Type.Kind)
	}
	if f.Type.KeyType == nil || f.Type.KeyType.Kind != ast.TypeString {
		t.Error("Field 2 should have string key type")
	}
	if f.Type.ValType == nil || f.Type.ValType.Kind != ast.TypeString {
		t.Errorf("Field 2 value type = %v, want TypeString", f.Type.ValType.Kind)
	}
}

func TestParser_Enum(t *testing.T) {
	input := `package test

enum Status {
    ACTIVE = 1
    INACTIVE = 2
    PENDING = 3
}

message User {
    1: string name
    2: Status status
}
`
	file, err := ParseFile(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Check enum
	if len(file.Enums) != 1 {
		t.Fatalf("len(Enums) = %d, want 1", len(file.Enums))
	}
	enum := file.Enums[0]
	if enum.Name != "Status" {
		t.Errorf("Enum name = %q, want %q", enum.Name, "Status")
	}
	if len(enum.Values) != 3 {
		t.Fatalf("len(Enum.Values) = %d, want 3", len(enum.Values))
	}
	if enum.Values[0].Name != "ACTIVE" || enum.Values[0].Number != 1 {
		t.Errorf("Enum value 0 = %+v", enum.Values[0])
	}
	if enum.Values[1].Name != "INACTIVE" || enum.Values[1].Number != 2 {
		t.Errorf("Enum value 1 = %+v", enum.Values[1])
	}
	if enum.Values[2].Name != "PENDING" || enum.Values[2].Number != 3 {
		t.Errorf("Enum value 2 = %+v", enum.Values[2])
	}

	// Check message with enum field
	// Note: Enum types are parsed as message types since parser doesn't resolve enums
	msg := file.Messages[0]
	if len(msg.Fields) != 2 {
		t.Fatalf("len(Fields) = %d, want 2", len(msg.Fields))
	}
	if msg.Fields[1].Type.Kind != ast.TypeMessage {
		t.Errorf("Field 1 type = %v, want TypeMessage (enums parsed as messages)", msg.Fields[1].Type.Kind)
	}
	if msg.Fields[1].Type.Message != "Status" {
		t.Errorf("Field 1 message name = %q, want %q", msg.Fields[1].Type.Message, "Status")
	}
}

func TestParser_PackageOnly(t *testing.T) {
	input := `package myapp
`
	file, err := ParseFile(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if file.Package != "myapp" {
		t.Errorf("Package = %q, want %q", file.Package, "myapp")
	}
	if len(file.Messages) != 0 {
		t.Errorf("len(Messages) = %d, want 0", len(file.Messages))
	}
	if len(file.Enums) != 0 {
		t.Errorf("len(Enums) = %d, want 0", len(file.Enums))
	}
}

func TestParser_MultiplePackages(t *testing.T) {
	input := `package foo

message Foo {
    1: string name
}

package bar

message Bar {
    1: int32 id
}
`
	file, err := ParseFile(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Parser keeps all messages, but last package wins
	if file.Package != "bar" {
		t.Errorf("Package = %q, want %q", file.Package, "bar")
	}
	if len(file.Messages) != 2 {
		t.Errorf("len(Messages) = %d, want 2", len(file.Messages))
	}
	if file.Messages[0].Name != "Foo" {
		t.Errorf("Message 0 name = %q, want %q", file.Messages[0].Name, "Foo")
	}
	if file.Messages[1].Name != "Bar" {
		t.Errorf("Message 1 name = %q, want %q", file.Messages[1].Name, "Bar")
	}
}

func TestParser_UnknownTypeAsMessage(t *testing.T) {
	input := `package test

message User {
    1: unknowntype name
}
`
	file, err := ParseFile(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Unknown types are treated as message types
	msg := file.Messages[0]
	if len(msg.Fields) != 1 {
		t.Fatalf("len(Fields) = %d, want 1", len(msg.Fields))
	}
	if msg.Fields[0].Type.Kind != ast.TypeMessage {
		t.Errorf("Field 0 type = %v, want TypeMessage", msg.Fields[0].Type.Kind)
	}
	if msg.Fields[0].Type.Message != "unknowntype" {
		t.Errorf("Field 0 message = %q, want %q", msg.Fields[0].Type.Message, "unknowntype")
	}
}

func TestParser_Error_MissingType(t *testing.T) {
	input := `package test

message User {
    1: string
}
`
	_, err := ParseFile(input)
	if err == nil {
		t.Error("expected error for missing field name")
	}
}

func TestParser_Error_MissingFieldName(t *testing.T) {
	input := `package test

message User {
    1: string
}
`
	_, err := ParseFile(input)
	if err == nil {
		t.Error("expected error for missing field name")
	}
}

func TestParser_Error_MissingMapValueType(t *testing.T) {
	input := `package test

message User {
    1: map<string,> data
}
`
	_, err := ParseFile(input)
	if err == nil {
		t.Error("expected error for missing map value type")
	}
}

func TestParser_Error_InvalidEnumValue(t *testing.T) {
	input := `package test

enum Status {
    ACTIVE = notanumber
}
`
	_, err := ParseFile(input)
	if err == nil {
		t.Error("expected error for invalid enum value")
	}
}

func TestParser_ZeroFieldNumber(t *testing.T) {
	input := `package test

message User {
    0: string name
}
`
	file, err := ParseFile(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	msg := file.Messages[0]
	if msg.Fields[0].Number != 0 {
		t.Errorf("Field number = %d, want 0", msg.Fields[0].Number)
	}
}

func TestParser_EmptyMessage(t *testing.T) {
	input := `package test

message Empty {
}
`
	file, err := ParseFile(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	msg := file.Messages[0]
	if len(msg.Fields) != 0 {
		t.Errorf("len(Fields) = %d, want 0", len(msg.Fields))
	}
}

func TestParser_EmptyEnum(t *testing.T) {
	input := `package test

enum Empty {
}
`
	file, err := ParseFile(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	enum := file.Enums[0]
	if len(enum.Values) != 0 {
		t.Errorf("len(Enum.Values) = %d, want 0", len(enum.Values))
	}
}

func TestParser_WhitespaceOnly(t *testing.T) {
	input := `   

`
	file, err := ParseFile(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if file.Package != "" {
		t.Errorf("Package = %q, want empty", file.Package)
	}
}

func TestParser_MessageWithBytesField(t *testing.T) {
	input := `package test

message File {
    1: string name
    2: bytes data
}
`
	file, err := ParseFile(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	msg := file.Messages[0]
	if len(msg.Fields) != 2 {
		t.Fatalf("len(Fields) = %d, want 2", len(msg.Fields))
	}
	if msg.Fields[1].Type.Kind != ast.TypeBytes {
		t.Errorf("Field 1 type = %v, want TypeBytes", msg.Fields[1].Type.Kind)
	}
}

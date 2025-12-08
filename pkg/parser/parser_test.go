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

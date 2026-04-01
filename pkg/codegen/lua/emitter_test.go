package lua

import (
	"strings"
	"testing"

	"github.com/ElecTwix/xpb/pkg/parser"
)

func TestGenerate_SimpleMessage(t *testing.T) {
	schema := `package test

message User {
    1: string name
    2: int32 age
}
`
	file, err := parser.ParseFile(schema)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	code, err := Generate(file)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(code)
	if !contains(output, "local xpb = require 'xpb'") {
		t.Error("Output should require xpb module")
	}
	if !contains(output, "User = {}") {
		t.Error("Output should contain User table")
	}
	if !contains(output, "function User.new()") {
		t.Error("Output should contain new method")
	}
	if !contains(output, "function User:Marshal()") {
		t.Error("Output should contain Marshal method")
	}
	if !contains(output, "function User.Unmarshal(data)") {
		t.Error("Output should contain Unmarshal method")
	}
	if !contains(output, "enc:write_string(self.name)") {
		t.Error("Output should write string field")
	}
	if !contains(output, "enc:write_int32(self.age)") {
		t.Error("Output should write int32 field")
	}
}

func TestGenerate_AllTypes(t *testing.T) {
	schema := `package test

message AllTypes {
    1: bool active
    2: int32 count
    3: int64 big
    4: uint32 value
    5: uint64 bigValue
    6: float32 rate
    7: float64 precision
    8: string name
    9: bytes data
}
`
	file, err := parser.ParseFile(schema)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	code, err := Generate(file)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(code)
	if !contains(output, "enc:write_bool(self.active)") {
		t.Error("Output should write bool field")
	}
	if !contains(output, "enc:write_int32(self.count)") {
		t.Error("Output should write int32 field")
	}
	if !contains(output, "enc:write_int64(self.big)") {
		t.Error("Output should write int64 field")
	}
	if !contains(output, "enc:write_float64(self.precision)") {
		t.Error("Output should write float64 field")
	}
}

func TestGenerate_NestedMessage(t *testing.T) {
	schema := `package test

message Address {
    1: string city
    2: string country
}

message User {
    1: string name
    2: Address address
}
`
	file, err := parser.ParseFile(schema)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	code, err := Generate(file)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := string(code)
	if !contains(output, "Address = {}") {
		t.Error("Output should contain Address table")
	}
	if !contains(output, "User = {}") {
		t.Error("Output should contain User table")
	}
	if !contains(output, "enc:write_message(self.address:Marshal())") {
		t.Error("Output should marshal nested message")
	}
}

func BenchmarkGenerate(b *testing.B) {
	schema := `package test

message User {
    1: string name
    2: int32 age
    3: bool active
}
`
	file, _ := parser.ParseFile(schema)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Generate(file)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

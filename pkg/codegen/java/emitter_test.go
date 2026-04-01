package java

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
	if !contains(output, "package test;") {
		t.Error("Output should contain package declaration")
	}
	if !contains(output, "import xpb.Encoder;") {
		t.Error("Output should import Encoder")
	}
	if !contains(output, "public class User {") {
		t.Error("Output should contain User class")
	}
	if !contains(output, "public String name;") {
		t.Error("Output should contain String name field")
	}
	if !contains(output, "public int age;") {
		t.Error("Output should contain int age field")
	}
	if !contains(output, "public byte[] marshal() {") {
		t.Error("Output should contain marshal method")
	}
	if !contains(output, "public static User unmarshal(byte[] data)") {
		t.Error("Output should contain unmarshal method")
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
	if !contains(output, "public boolean active;") {
		t.Error("Output should contain boolean field")
	}
	if !contains(output, "public int count;") {
		t.Error("Output should contain int field")
	}
	if !contains(output, "public long big;") {
		t.Error("Output should contain long field")
	}
	if !contains(output, "public double precision;") {
		t.Error("Output should contain double field")
	}
	if !contains(output, "public byte[] data;") {
		t.Error("Output should contain byte[] field")
	}
}

func TestGenerate_WithEnum(t *testing.T) {
	schema := `package test

enum Status {
    ACTIVE = 1
    INACTIVE = 2
}

message User {
    1: string name
    2: Status status
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
	if !contains(output, "public enum Status {") {
		t.Error("Output should contain Status enum")
	}
	if !contains(output, "ACTIVE(1)") {
		t.Error("Output should contain ACTIVE value")
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
	if !contains(output, "public class Address {") {
		t.Error("Output should contain Address class")
	}
	if !contains(output, "public class User {") {
		t.Error("Output should contain User class")
	}
	if !contains(output, "enc.writeMessage(address.marshal())") {
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

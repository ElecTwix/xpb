package c

import (
	"os"
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
	if !contains(output, "#ifndef TEST_H") {
		t.Error("Output should contain include guard")
	}
	if !contains(output, "#include <xpb/xpb.h>") {
		t.Error("Output should include xpb header")
	}
	if !contains(output, "typedef struct {") {
		t.Error("Output should contain typedef struct")
	}
	if !contains(output, "char* name;") {
		t.Error("Output should contain char* name field")
	}
	if !contains(output, "int32_t age;") {
		t.Error("Output should contain int32_t age field")
	}
	if !contains(output, "User_marshal") {
		t.Error("Output should contain marshal function")
	}
	if !contains(output, "User_unmarshal") {
		t.Error("Output should contain unmarshal function")
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
	if !contains(output, "bool active;") {
		t.Error("Output should contain bool field")
	}
	if !contains(output, "int32_t count;") {
		t.Error("Output should contain int32_t field")
	}
	if !contains(output, "int64_t big;") {
		t.Error("Output should contain int64_t field")
	}
	if !contains(output, "uint32_t value;") {
		t.Error("Output should contain uint32_t field")
	}
	if !contains(output, "uint64_t bigValue;") {
		t.Error("Output should contain uint64_t field")
	}
	if !contains(output, "float rate;") {
		t.Error("Output should contain float field")
	}
	if !contains(output, "double precision;") {
		t.Error("Output should contain double field")
	}
	if !contains(output, "char* name;") {
		t.Error("Output should contain char* name field")
	}
	if !contains(output, "uint8_t* data;") {
		t.Error("Output should contain uint8_t* data field")
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
	if !contains(output, "typedef enum {") {
		t.Error("Output should contain typedef enum")
	}
	if !contains(output, "Status_ACTIVE = 1") {
		t.Error("Output should contain ACTIVE value")
	}
	if !contains(output, "Status_INACTIVE = 2") {
		t.Error("Output should contain INACTIVE value")
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
	if !contains(output, "typedef struct {") {
		t.Error("Output should contain Address typedef")
	}
	if !contains(output, "Address address;") {
		t.Error("Output should contain Address field in User")
	}
	if !contains(output, "Address_marshal") {
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

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

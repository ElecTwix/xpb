package cpp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	xpbast "github.com/anthropic/xpb/pkg/ast"
	"github.com/anthropic/xpb/pkg/parser"
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
	if !contains(output, "#ifndef TEST_HPP") {
		t.Error("Output should contain include guard")
	}
	if !contains(output, "#include <xpb/xpb.hpp>") {
		t.Error("Output should include xpb header")
	}
	if !contains(output, "struct User {") {
		t.Error("Output should contain struct User")
	}
	if !contains(output, "std::string name") {
		t.Error("Output should contain std::string name field")
	}
	if !contains(output, "int32_t age") {
		t.Error("Output should contain int32_t age field")
	}
	if !contains(output, "Marshal()") {
		t.Error("Output should contain Marshal method")
	}
	if !contains(output, "Unmarshal") {
		t.Error("Output should contain Unmarshal method")
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
	if !contains(output, "bool active") {
		t.Error("Output should contain bool field")
	}
	if !contains(output, "int32_t count") {
		t.Error("Output should contain int32_t field")
	}
	if !contains(output, "int64_t big") {
		t.Error("Output should contain int64_t field")
	}
	if !contains(output, "uint32_t value") {
		t.Error("Output should contain uint32_t field")
	}
	if !contains(output, "uint64_t bigValue") {
		t.Error("Output should contain uint64_t field")
	}
	if !contains(output, "float rate") {
		t.Error("Output should contain float field")
	}
	if !contains(output, "double") {
		t.Error("Output should contain double field")
	}
	if !contains(output, "std::string name") {
		t.Error("Output should contain std::string field")
	}
	if !contains(output, "std::vector<uint8_t> data") {
		t.Error("Output should contain bytes field")
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
	if !contains(output, "enum class Status") {
		t.Error("Output should contain enum Status")
	}
	if !contains(output, "ACTIVE = 1") {
		t.Error("Output should contain ACTIVE value")
	}
	if !contains(output, "INACTIVE = 2") {
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
	if !contains(output, "struct Address {") {
		t.Error("Output should contain Address struct")
	}
	if !contains(output, "struct User {") {
		t.Error("Output should contain User struct")
	}
	if !contains(output, "Address address") {
		t.Error("Output should contain Address field in User")
	}
	if !contains(output, "enc.writeMessage(address.Marshal())") {
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
	// Run tests
	os.Exit(m.Run())
}

var _ = xpbast.File{}
var _ = filepath.Join

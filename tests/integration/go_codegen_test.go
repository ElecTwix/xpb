// Package integration contains end-to-end tests for the Go codegen.
//
// These tests do REAL work: they parse a schema, generate Go source, write it
// into a throwaway module that imports the actual xpb runtime from this
// checkout (via a `replace` directive), then `go test` that module. The driver
// constructs the generated struct, Marshals it, Unmarshals into a fresh struct,
// and asserts round-trip equality. A failure to compile or a round-trip
// mismatch fails the test for real -- substring checks alone cannot catch a
// broken generator.
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ElecTwix/xpb/pkg/codegen/golang"
	"github.com/ElecTwix/xpb/pkg/parser"
)

// repoRoot returns the absolute path to the repository root, computed from this
// test file's own location so it is independent of the working directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate repo root")
	}
	// This file lives at <root>/tests/integration/go_codegen_test.go.
	root, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("computed repo root %q has no go.mod: %v", root, err)
	}
	return root
}

// buildGenModule writes a self-contained module under a temp dir:
//   - generated.go: the generated package (rewritten to `package gen`)
//   - <driverName>: a driver in the same package that exercises Marshal/Unmarshal
//   - go.mod with a replace directive pointing the xpb import at this checkout
//
// It returns the module directory.
func buildGenModule(t *testing.T, genSrc []byte, driverName, driverSrc string) string {
	t.Helper()

	root := repoRoot(t)
	dir := t.TempDir()

	// Force the generated code into package `gen` regardless of the schema's
	// declared package, so the driver can live in the same package.
	src := string(genSrc)
	src = rewritePackageClause(t, src, "gen")

	if err := os.WriteFile(filepath.Join(dir, "generated.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write generated.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, driverName), []byte(driverSrc), 0o644); err != nil {
		t.Fatalf("write %s: %v", driverName, err)
	}

	// A minimal module that resolves the xpb runtime import to THIS checkout.
	goMod := "module xpbgentest\n\n" +
		"go 1.23\n\n" +
		"require github.com/ElecTwix/xpb v0.0.0\n\n" +
		"replace github.com/ElecTwix/xpb => " + root + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	return dir
}

// rewritePackageClause replaces the `package X` line in generated Go source with
// the desired package name.
func rewritePackageClause(t *testing.T, src, pkg string) string {
	t.Helper()
	lines := strings.Split(src, "\n")
	for i, ln := range lines {
		if strings.HasPrefix(ln, "package ") {
			lines[i] = "package " + pkg
			return strings.Join(lines, "\n")
		}
	}
	t.Fatalf("generated source has no package clause:\n%s", src)
	return ""
}

// goTestModule runs `go test` in the given module directory and fails the test
// (with full output) if it does not pass. `go test` builds the package, so this
// is the authoritative compile + run check.
func goTestModule(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("go", "test", "-count=1", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated module failed `go test`: %v\n--- output ---\n%s\n--- generated.go ---\n%s",
			err, out, readFile(t, filepath.Join(dir, "generated.go")))
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return "<unreadable: " + err.Error() + ">"
	}
	return string(b)
}

// generateGo parses a schema and returns the generated Go source using default
// (pointer-style, copying) options, failing on error.
func generateGo(t *testing.T, schema string) []byte {
	t.Helper()
	return generateGoWith(t, schema, golang.Options{})
}

// generateGoWith parses a schema and returns the generated Go source for the
// given codegen options, failing on error. This lets the matrix tests exercise
// the value-optional and zero-copy-bytes codegen paths, not just the default.
func generateGoWith(t *testing.T, schema string, opts golang.Options) []byte {
	t.Helper()
	file, err := parser.ParseFile(schema)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	src, err := golang.GenerateWithOptions(file, opts)
	if err != nil {
		t.Fatalf("generate failed: %v\n%s", err, src)
	}
	return src
}

// --- Lightweight syntax pre-checks (kept; the authoritative check is the real build) ---

func TestGoCodegen_SimpleSchema(t *testing.T) {
	schema := `
package test

message User {
    1: string name
    2: int32 age
    3: bool active
}
`
	src := generateGo(t, schema)
	if !strings.Contains(string(src), "type User struct") {
		t.Fatalf("missing User struct in generated output:\n%s", src)
	}

	driver := `package gen

import "testing"

func TestRoundTrip(t *testing.T) {
	in := &User{Name: "alice", Age: 30, Active: true}
	data, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out User
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Name != in.Name || out.Age != in.Age || out.Active != in.Active {
		t.Fatalf("round-trip mismatch: got %+v want %+v", out, *in)
	}
}
`
	dir := buildGenModule(t, src, "roundtrip_test.go", driver)
	goTestModule(t, dir)
}

// TestGoCodegen_EmptyMessage guards the bodyless-message codegen path. A
// message with no fields decodes nothing, so the generated unmarshalAt must
// not declare an unused decoder (`dec := xpb.NewDecoder(data)` with no reads
// is a hard Go compile error: "declared and not used"). Compiling the
// generated module here is what catches the regression -- a substring check
// would not. The empty message is paired with a non-empty one to confirm the
// normal decoder path still emits `dec`.
func TestGoCodegen_EmptyMessage(t *testing.T) {
	schema := `
package test

message Ping {
}

message Wrapped {
    1: string note
}
`
	src := generateGo(t, schema)

	driver := `package gen

import "testing"

func TestRoundTrip(t *testing.T) {
	data, err := (&Ping{}).Marshal()
	if err != nil {
		t.Fatalf("Marshal empty: %v", err)
	}
	var p Ping
	if err := p.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal empty: %v", err)
	}

	in := &Wrapped{Note: "hi"}
	wd, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal wrapped: %v", err)
	}
	var out Wrapped
	if err := out.Unmarshal(wd); err != nil {
		t.Fatalf("Unmarshal wrapped: %v", err)
	}
	if out.Note != in.Note {
		t.Fatalf("round-trip mismatch: got %q want %q", out.Note, in.Note)
	}
}
`
	dir := buildGenModule(t, src, "roundtrip_test.go", driver)
	goTestModule(t, dir)
}

// TestGoCodegen_AllTypes covers every scalar type and asserts a full round-trip,
// including values where wrong field ordering would corrupt the decode.
func TestGoCodegen_AllTypes(t *testing.T) {
	schema := `
package test

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
	src := generateGo(t, schema)

	driver := `package gen

import (
	"bytes"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	in := &AllTypes{
		B:    true,
		I32:  -123456,
		I64:  -9000000000,
		U32:  4000000000,
		U64:  18000000000000000000,
		F32:  3.5,
		F64:  2.718281828,
		S:    "hello world",
		Data: []byte{0xde, 0xad, 0xbe, 0xef},
	}
	data, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out AllTypes
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.B != in.B || out.I32 != in.I32 || out.I64 != in.I64 ||
		out.U32 != in.U32 || out.U64 != in.U64 || out.F32 != in.F32 ||
		out.F64 != in.F64 || out.S != in.S || !bytes.Equal(out.Data, in.Data) {
		t.Fatalf("round-trip mismatch:\n got  %+v\n want %+v", out, *in)
	}
}
`
	dir := buildGenModule(t, src, "roundtrip_test.go", driver)
	goTestModule(t, dir)
}

// TestGoCodegen_FieldOrder uses two adjacent same-width fields with distinct
// values. The XPB V2 format is tagless and positional, so a generator that
// emitted fields in the wrong order would silently swap these values; the
// round-trip assertion catches it.
func TestGoCodegen_FieldOrder(t *testing.T) {
	schema := `
package test

message Ordered {
    1: int32 first
    2: int32 second
    3: int32 third
    4: string label
    5: int32 fourth
}
`
	src := generateGo(t, schema)

	driver := `package gen

import "testing"

func TestRoundTrip(t *testing.T) {
	in := &Ordered{First: 1, Second: 2, Third: 3, Label: "x", Fourth: 4}
	data, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out Ordered
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	// Distinct values per field: any ordering bug corrupts these.
	if out.First != 1 || out.Second != 2 || out.Third != 3 || out.Label != "x" || out.Fourth != 4 {
		t.Fatalf("field-order round-trip mismatch: got %+v", out)
	}
}
`
	dir := buildGenModule(t, src, "roundtrip_test.go", driver)
	goTestModule(t, dir)
}

func TestGoCodegen_RepeatedFields(t *testing.T) {
	schema := `
package test

message Container {
    1: string name
    2: []string tags
    3: []int32 scores
}
`
	src := generateGo(t, schema)

	// Security regression (XPB-001/002): repeated-field counts must go through
	// dec.ReadArrayCount (which bounds the count against the remaining buffer),
	// not a raw ReadInt32 + unchecked make([]T, count).
	if !strings.Contains(string(src), "dec.ReadArrayCount(") {
		t.Error("repeated-field decode must use dec.ReadArrayCount; got raw ReadInt32")
	}

	driver := `package gen

import (
	"reflect"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	in := &Container{
		Name:   "box",
		Tags:   []string{"a", "bb", "ccc"},
		Scores: []int32{-1, 0, 7, 1000000},
	}
	data, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out Container
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Name != in.Name || !reflect.DeepEqual(out.Tags, in.Tags) || !reflect.DeepEqual(out.Scores, in.Scores) {
		t.Fatalf("round-trip mismatch:\n got  %+v\n want %+v", out, *in)
	}
}
`
	dir := buildGenModule(t, src, "roundtrip_test.go", driver)
	goTestModule(t, dir)
}

func TestGoCodegen_NestedMessages(t *testing.T) {
	schema := `
package test

message Point {
    1: int32 x
    2: int32 y
}

message Rectangle {
    1: Point top_left
    2: Point bottom_right
}
`
	src := generateGo(t, schema)

	driver := `package gen

import "testing"

func TestRoundTrip(t *testing.T) {
	in := &Rectangle{
		TopLeft:     &Point{X: 1, Y: 2},
		BottomRight: &Point{X: 30, Y: 40},
	}
	data, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out Rectangle
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.TopLeft == nil || out.BottomRight == nil {
		t.Fatalf("nested messages not decoded: %+v", out)
	}
	if *out.TopLeft != *in.TopLeft || *out.BottomRight != *in.BottomRight {
		t.Fatalf("round-trip mismatch: got {%+v %+v} want {%+v %+v}",
			*out.TopLeft, *out.BottomRight, *in.TopLeft, *in.BottomRight)
	}
}
`
	dir := buildGenModule(t, src, "roundtrip_test.go", driver)
	goTestModule(t, dir)
}

func TestGoCodegen_WithEnum(t *testing.T) {
	schema := `
package test

enum Status {
    ACTIVE = 1
    INACTIVE = 2
}

message User {
    1: string name
    2: Status status
    3: int32 age
}
`
	src := generateGo(t, schema)

	driver := `package gen

import "testing"

func TestRoundTrip(t *testing.T) {
	in := &User{Name: "bob", Status: Status_INACTIVE, Age: 42}
	data, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out User
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Name != in.Name || out.Status != in.Status || out.Age != in.Age {
		t.Fatalf("round-trip mismatch: got %+v want %+v", out, *in)
	}
	if out.Status.String() != "INACTIVE" {
		t.Fatalf("enum String() = %q, want INACTIVE", out.Status.String())
	}
}
`
	dir := buildGenModule(t, src, "roundtrip_test.go", driver)
	goTestModule(t, dir)
}

func TestGoCodegen_MapField(t *testing.T) {
	schema := `
package test

message Config {
    1: string name
    2: map<string, int32> counts
}
`
	src := generateGo(t, schema)

	driver := `package gen

import (
	"reflect"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	in := &Config{
		Name:   "cfg",
		Counts: map[string]int32{"a": 1, "b": 2, "c": 3},
	}
	data, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out Config
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Name != in.Name || !reflect.DeepEqual(out.Counts, in.Counts) {
		t.Fatalf("round-trip mismatch:\n got  %+v\n want %+v", out, *in)
	}
}
`
	dir := buildGenModule(t, src, "roundtrip_test.go", driver)
	goTestModule(t, dir)
}

// TestGoCodegen_OptionalField confirms a schema using the `?` optional marker
// generates code that compiles and round-trips BOTH a present optional (value
// preserved) and an absent optional (decodes to nil) -- and that the field
// after an absent optional still decodes correctly, proving the 1-byte presence
// flag is consumed and does not corrupt the following field.
//
// Wire format: an optional field is encoded as a 1-byte presence flag (0x01 +
// value when present, 0x00 with no value bytes when absent). The Go codegen
// represents non-message optionals as pointers; nil == absent.
func TestGoCodegen_OptionalField(t *testing.T) {
	schema := `
package test

message Profile {
    1: string bio
    2: ?string avatar_url
    3: int32 followers
}
`
	src := generateGo(t, schema)
	// The optional scalar must be a pointer so absence is representable.
	if !strings.Contains(string(src), "AvatarUrl *string") {
		t.Errorf("optional scalar must be a *string pointer; got:\n%s", src)
	}

	driver := `package gen

import (
	"bytes"
	"testing"
)

func TestPresentRoundTrip(t *testing.T) {
	url := "http://x/y.png"
	in := &Profile{Bio: "hi", AvatarUrl: &url, Followers: 9}
	data, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out Profile
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Bio != in.Bio || out.AvatarUrl == nil || *out.AvatarUrl != url || out.Followers != in.Followers {
		t.Fatalf("present round-trip mismatch: got %+v (avatar=%v) want %+v", out, out.AvatarUrl, *in)
	}
}

func TestAbsentRoundTrip(t *testing.T) {
	// AvatarUrl left nil (absent). The presence byte must be consumed so the
	// FOLLOWING field (Followers) still decodes correctly.
	in := &Profile{Bio: "hi", AvatarUrl: nil, Followers: 9}
	data, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out Profile
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.AvatarUrl != nil {
		t.Fatalf("absent optional must decode to nil, got %q", *out.AvatarUrl)
	}
	if out.Bio != "hi" {
		t.Fatalf("field before optional corrupted: bio=%q", out.Bio)
	}
	if out.Followers != 9 {
		t.Fatalf("field after absent optional corrupted (presence byte not consumed): followers=%d, want 9", out.Followers)
	}
}

// TestAbsentOptionalConsumesExactlyOneByte builds {?string a, int32 b} with a
// absent, and asserts the absent optional adds exactly one byte over a bare
// int32 -- i.e. the presence flag and nothing else.
func TestAbsentOptionalConsumesExactlyOneByte(t *testing.T) {
	in := &Pair{B: 1234}
	data, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// b alone is a 4-byte int32; the absent optional contributes 1 presence byte.
	if len(data) != 5 {
		t.Fatalf("absent-optional encoding = %d bytes (% x), want 5 (1 presence + 4 int32)", len(data), data)
	}
	if data[0] != 0x00 {
		t.Fatalf("absent presence byte = 0x%02x, want 0x00", data[0])
	}
	// b = 1234 = 0x04D2, little-endian after the presence byte.
	if !bytes.Equal(data[1:], []byte{0xD2, 0x04, 0x00, 0x00}) {
		t.Fatalf("int32 after absent optional mis-encoded: % x", data[1:])
	}
	var out Pair
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.A != nil || out.B != 1234 {
		t.Fatalf("Pair round-trip: a=%v b=%d, want a=nil b=1234", out.A, out.B)
	}
}
`
	// Add a second message {?string a, int32 b} for the exact-byte test.
	schema2 := schema + `
message Pair {
    1: ?string a
    2: int32 b
}
`
	src = generateGo(t, schema2)
	dir := buildGenModule(t, src, "roundtrip_test.go", driver)
	goTestModule(t, dir)
}

// comprehensiveSchema mirrors testdata/comprehensive.xpb: it has an enum
// (Status), optional scalars (?string email / avatar_url / postal_code), and an
// optional MESSAGE field (?Address address). The optional message stays *T in
// both Go optional styles, while the optional scalars switch between *T and
// value+Has<Field>. Address also carries a required `bytes raw` field so the
// value+zero-copy matrix row actually emits and round-trips the zero-copy
// ReadBytesUnsafe decode path (a schema with no bytes field would never
// exercise the flag). This is the schema the value-style + zero-copy matrix
// exercises end to end.
const comprehensiveSchema = `
package myapp

enum Status {
    UNKNOWN = 0
    ACTIVE = 1
    INACTIVE = 2
    PENDING = 3
}

message User {
    1: string name
    2: int32 age
    3: bool active
    4: ?string email
    5: []string tags
    6: Status status
}

message Address {
    1: string city
    2: string country
    3: ?string postal_code
    4: bytes raw
}

message Profile {
    1: string bio
    2: ?string avatar_url
    3: User user
    4: ?Address address
    5: []int32 scores
    6: map<string, string> metadata
}
`

// TestGoCodegen_ComprehensiveMatrix extends the throwaway-module compile +
// round-trip verifier to cover the value-optional and zero-copy-bytes codegen
// paths on the comprehensive schema, not just the default pointer style. Each
// matrix entry generates the schema with its options, writes a style-specific
// driver, and `go test`s the throwaway module for a real compile + round-trip.
//
// The driver differs per optional style because an optional scalar is `*T`
// (pointer) vs `T` + `Has<Field>` (value); the optional MESSAGE field
// (?Address) and the enum field are identical across styles and are asserted in
// every variant.
func TestGoCodegen_ComprehensiveMatrix(t *testing.T) {
	cases := []struct {
		name   string
		opts   golang.Options
		driver string
	}{
		{
			name:   "default_pointer",
			opts:   golang.Options{},
			driver: comprehensivePtrDriver,
		},
		{
			name:   "value_optionals",
			opts:   golang.Options{OptionalStyle: golang.OptionalValue},
			driver: comprehensiveValDriver,
		},
		{
			name:   "value_optionals_zero_copy",
			opts:   golang.Options{OptionalStyle: golang.OptionalValue, ZeroCopyBytes: true},
			driver: comprehensiveValDriver,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := generateGoWith(t, comprehensiveSchema, tc.opts)
			dir := buildGenModule(t, src, "roundtrip_test.go", tc.driver)
			goTestModule(t, dir)
		})
	}
}

// comprehensivePtrDriver round-trips Profile under the default pointer style:
// optional scalars are *string, the optional Address message is *Address, and
// the enum is exercised. It covers both present and absent optional cases,
// including the optional MESSAGE field, which is the path comprehensive.xpb adds
// over the simpler schemas.
const comprehensivePtrDriver = `package gen

import (
	"bytes"
	"reflect"
	"testing"
)

func TestComprehensivePresent(t *testing.T) {
	email := "a@b.com"
	postal := "94016"
	avatar := "http://x/a.png"
	raw := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	in := &Profile{
		Bio:       "hi",
		AvatarUrl: &avatar,
		User:      &User{Name: "alice", Age: 30, Active: true, Email: &email, Tags: []string{"x", "y"}, Status: Status_ACTIVE},
		Address:   &Address{City: "SF", Country: "US", PostalCode: &postal, Raw: raw},
		Scores:    []int32{1, 2, 3},
		Metadata:  map[string]string{"k": "v"},
	}
	data, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out Profile
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.AvatarUrl == nil || *out.AvatarUrl != avatar {
		t.Fatalf("AvatarUrl: %v", out.AvatarUrl)
	}
	if out.User == nil || out.User.Status != Status_ACTIVE || out.User.Email == nil || *out.User.Email != email {
		t.Fatalf("User: %+v", out.User)
	}
	if out.Address == nil || out.Address.City != "SF" || out.Address.PostalCode == nil || *out.Address.PostalCode != postal {
		t.Fatalf("Address: %+v", out.Address)
	}
	if !bytes.Equal(out.Address.Raw, raw) {
		t.Fatalf("Address.Raw = % x, want % x", out.Address.Raw, raw)
	}
	if !reflect.DeepEqual(out.Scores, in.Scores) || !reflect.DeepEqual(out.Metadata, in.Metadata) {
		t.Fatalf("scores/metadata mismatch: %+v", out)
	}
}

func TestComprehensiveAbsentMessage(t *testing.T) {
	// Optional Address message ABSENT; the field after it (Scores) must still
	// decode, proving the presence flag is consumed.
	in := &Profile{
		Bio:    "hi",
		User:   &User{Name: "bob", Status: Status_PENDING},
		Scores: []int32{9, 8, 7},
	}
	data, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out Profile
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Address != nil {
		t.Fatalf("absent optional message must decode to nil, got %+v", out.Address)
	}
	if out.AvatarUrl != nil {
		t.Fatalf("absent optional scalar must decode to nil, got %q", *out.AvatarUrl)
	}
	if !reflect.DeepEqual(out.Scores, in.Scores) {
		t.Fatalf("field after absent optional corrupted: %+v", out.Scores)
	}
	if out.User == nil || out.User.Status != Status_PENDING {
		t.Fatalf("enum mismatch: %+v", out.User)
	}
}
`

// comprehensiveValDriver round-trips Profile under the value-optional style:
// optional scalars are value + Has<Field>, while the optional Address message
// stays *Address. Address.Raw is a required bytes field, so under the zero-copy
// matrix variant its decode is emitted via ReadBytesUnsafe (aliasing the input)
// rather than ReadBytes (copy). The driver round-trips Raw to prove the
// zero-copy bytes decode path compiles and decodes correctly with the flag set,
// alongside value optionals + the enum + the optional message.
const comprehensiveValDriver = `package gen

import (
	"bytes"
	"reflect"
	"testing"
)

func TestComprehensivePresent(t *testing.T) {
	raw := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	in := &Profile{
		Bio:       "hi",
		AvatarUrl: "http://x/a.png", HasAvatarUrl: true,
		User:    &User{Name: "alice", Age: 30, Active: true, Email: "a@b.com", HasEmail: true, Tags: []string{"x", "y"}, Status: Status_ACTIVE},
		Address: &Address{City: "SF", Country: "US", PostalCode: "94016", HasPostalCode: true, Raw: raw},
		Scores:  []int32{1, 2, 3},
		Metadata: map[string]string{"k": "v"},
	}
	data, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out Profile
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !out.HasAvatarUrl || out.AvatarUrl != "http://x/a.png" {
		t.Fatalf("AvatarUrl: has=%v val=%q", out.HasAvatarUrl, out.AvatarUrl)
	}
	if out.User == nil || out.User.Status != Status_ACTIVE || !out.User.HasEmail || out.User.Email != "a@b.com" {
		t.Fatalf("User: %+v", out.User)
	}
	if out.Address == nil || out.Address.City != "SF" || !out.Address.HasPostalCode || out.Address.PostalCode != "94016" {
		t.Fatalf("Address: %+v", out.Address)
	}
	if !bytes.Equal(out.Address.Raw, raw) {
		t.Fatalf("Address.Raw = % x, want % x", out.Address.Raw, raw)
	}
	if !reflect.DeepEqual(out.Scores, in.Scores) || !reflect.DeepEqual(out.Metadata, in.Metadata) {
		t.Fatalf("scores/metadata mismatch: %+v", out)
	}
}

func TestComprehensiveAbsentMessage(t *testing.T) {
	in := &Profile{
		Bio:    "hi",
		User:   &User{Name: "bob", Status: Status_PENDING},
		Scores: []int32{9, 8, 7},
	}
	data, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out Profile
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Address != nil {
		t.Fatalf("absent optional message must decode to nil, got %+v", out.Address)
	}
	if out.HasAvatarUrl {
		t.Fatalf("absent optional scalar must have HasAvatarUrl=false, got %q", out.AvatarUrl)
	}
	if !reflect.DeepEqual(out.Scores, in.Scores) {
		t.Fatalf("field after absent optional corrupted: %+v", out.Scores)
	}
	if out.User == nil || out.User.Status != Status_PENDING {
		t.Fatalf("enum mismatch: %+v", out.User)
	}
}
`

// BenchmarkGoCodegen_Simple benchmarks simple message generation.
func BenchmarkGoCodegen_Simple(b *testing.B) {
	schema := `
package test

message User {
    1: string name
    2: int32 age
}
`
	file, err := parser.ParseFile(schema)
	if err != nil {
		b.Fatalf("Parse error: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = golang.Generate(file)
	}
}

// BenchmarkGoCodegen_Medium benchmarks medium message generation.
func BenchmarkGoCodegen_Medium(b *testing.B) {
	schema := `
package test

message User {
    1: string name
    2: string email
    3: int32 age
    4: float64 score
    5: bool active
    6: []string tags
}
`
	file, err := parser.ParseFile(schema)
	if err != nil {
		b.Fatalf("Parse error: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = golang.Generate(file)
	}
}

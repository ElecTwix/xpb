// Package integration contains end-to-end tests for the TypeScript codegen.
//
// These tests do REAL work: they generate TypeScript from a schema, write it
// next to a tsconfig that resolves the `@xpb/runtime` import to this checkout's
// TypeScript runtime, then run a real `tsc --noEmit` type-check. Type errors
// fail the test. When `bun` (or `node`) is available the tests also execute a
// round-trip (encode then decode) and assert the decoded values match.
//
// If the toolchain (tsc / bun / node) is genuinely unavailable, the tests skip
// with a clear reason rather than reporting a false pass.
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ElecTwix/xpb/pkg/codegen/typescript"
	"github.com/ElecTwix/xpb/pkg/parser"
)

// tsRuntimeDir returns the path to the TypeScript runtime in this checkout.
func tsRuntimeDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "runtime", "ts")
}

// localTSC returns the path to the project-local `tsc` binary, or "" if absent.
func localTSC(t *testing.T) string {
	t.Helper()
	p := filepath.Join(tsRuntimeDir(t), "node_modules", ".bin", "tsc")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	if p, err := exec.LookPath("tsc"); err == nil {
		return p
	}
	return ""
}

// tsRunner returns a runtime capable of executing TypeScript directly, preferring
// bun, then node (with --experimental-strip-types). Returns ("", "") if none.
func tsRunner() (bin string, name string) {
	if p, err := exec.LookPath("bun"); err == nil {
		return p, "bun"
	}
	if p, err := exec.LookPath("node"); err == nil {
		return p, "node"
	}
	return "", ""
}

// generateTS parses a schema and returns the generated TypeScript source.
func generateTS(t *testing.T, schema string) []byte {
	t.Helper()
	file, err := parser.ParseFile(schema)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	src, err := typescript.Generate(file)
	if err != nil {
		t.Fatalf("generate failed: %v\n%s", err, src)
	}
	return src
}

// writeTSProject writes the generated TS plus a tsconfig that resolves
// `@xpb/runtime` to this checkout's built declaration file, and returns the
// project directory and the path to the generated .ts file.
func writeTSProject(t *testing.T, genSrc []byte) (dir, genFile string) {
	t.Helper()
	rt := tsRuntimeDir(t)
	distDecl := filepath.Join(rt, "dist", "index.d.ts")
	if _, err := os.Stat(distDecl); err != nil {
		t.Skipf("TS runtime not built (%s missing): run `npm run build` in runtime/ts", distDecl)
	}

	dir = t.TempDir()
	genFile = filepath.Join(dir, "generated.ts")
	if err := os.WriteFile(genFile, genSrc, 0o644); err != nil {
		t.Fatalf("write generated.ts: %v", err)
	}

	// Map the bare `@xpb/runtime` import to the built declaration file so the
	// type-check validates the generated code against the runtime's real,
	// published public API.
	tsconfig := `{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "lib": ["ES2022", "DOM"],
    "strict": true,
    "noEmit": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "baseUrl": ".",
    "paths": {
      "@xpb/runtime": ["` + jsonEscape(distDecl) + `"]
    }
  },
  "include": ["*.ts"]
}
`
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("write tsconfig.json: %v", err)
	}
	return dir, genFile
}

func jsonEscape(s string) string {
	return strings.ReplaceAll(s, `\`, `\\`)
}

// typeCheckTS runs `tsc --noEmit` over the project and fails on any type error.
// Skips if no tsc is available.
func typeCheckTS(t *testing.T, dir string) {
	t.Helper()
	tsc := localTSC(t)
	if tsc == "" {
		t.Skip("tsc not available (no runtime/ts/node_modules/.bin/tsc and none on PATH)")
	}
	cmd := exec.Command(tsc, "--noEmit", "--project", filepath.Join(dir, "tsconfig.json"))
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated TypeScript failed `tsc --noEmit`: %v\n--- tsc output ---\n%s\n--- generated.ts ---\n%s",
			err, out, readFile(t, filepath.Join(dir, "generated.ts")))
	}
}

// runTSRoundTrip writes a driver that imports the generated module (with the
// import rewritten to the runtime's built JS so it can execute), runs it, and
// fails if the driver exits non-zero. Skips if no JS runtime is available.
func runTSRoundTrip(t *testing.T, dir, genFile, driverBody string) {
	t.Helper()
	bin, name := tsRunner()
	if bin == "" {
		t.Skip("no TypeScript runner available (bun/node not on PATH)")
	}

	rt := tsRuntimeDir(t)
	distJS := filepath.Join(rt, "dist", "index.js")
	if _, err := os.Stat(distJS); err != nil {
		t.Skipf("TS runtime not built (%s missing)", distJS)
	}

	// Produce an executable copy of the generated module whose runtime import
	// resolves to the built JS file (the type-check used the .d.ts already).
	genSrc := readFile(t, genFile)
	execSrc := strings.Replace(genSrc, "'@xpb/runtime'", "'"+distJS+"'", 1)
	execFile := filepath.Join(dir, "gen_exec.ts")
	if err := os.WriteFile(execFile, []byte(execSrc), 0o644); err != nil {
		t.Fatalf("write gen_exec.ts: %v", err)
	}

	driverFile := filepath.Join(dir, "roundtrip.ts")
	if err := os.WriteFile(driverFile, []byte(driverBody), 0o644); err != nil {
		t.Fatalf("write roundtrip.ts: %v", err)
	}

	var cmd *exec.Cmd
	if name == "node" {
		// node needs explicit type-stripping for .ts files.
		cmd = exec.Command(bin, "--experimental-strip-types", driverFile)
	} else {
		cmd = exec.Command(bin, "run", driverFile)
	}
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("TS round-trip failed (%s): %v\n--- output ---\n%s\n--- driver ---\n%s",
			name, err, out, driverBody)
	}
	if !strings.Contains(string(out), "OK") {
		t.Fatalf("TS round-trip did not report OK (%s):\n%s", name, out)
	}
}

// --- Tests ---

func TestTSCodegen_SimpleMessage(t *testing.T) {
	schema := `
package test

message User {
    1: string name
    2: int32 age
    3: bool active
}
`
	src := generateTS(t, schema)
	if !strings.Contains(string(src), "export class User") {
		t.Fatalf("missing User class:\n%s", src)
	}
	dir, genFile := writeTSProject(t, src)
	typeCheckTS(t, dir)

	driver := `import { User } from './gen_exec.ts';
const input = new User({ name: "alice", age: 30, active: true });
const out = User.decode(input.encode());
if (out.name !== "alice" || out.age !== 30 || out.active !== true) {
  console.error("FAIL", JSON.stringify(out));
  process.exit(1);
}
console.log("OK");
`
	runTSRoundTrip(t, dir, genFile, driver)
}

// TestTSCodegen_AllTypes covers every scalar type and round-trips them.
func TestTSCodegen_AllTypes(t *testing.T) {
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
	src := generateTS(t, schema)
	dir, genFile := writeTSProject(t, src)
	typeCheckTS(t, dir)

	driver := `import { AllTypes } from './gen_exec.ts';
const input = new AllTypes({
  b: true, i32: -123456, i64: -9000000000n, u32: 4000000000, u64: 18000000000000000000n,
  f32: 1.5, f64: 2.718281828, s: "héllo world", data: new Uint8Array([0xde,0xad,0xbe,0xef]),
});
const out = AllTypes.decode(input.encode());
function fail(m: string): never { console.error("FAIL: " + m); process.exit(1); }
if (out.b !== input.b) fail("b");
if (out.i32 !== input.i32) fail("i32 " + out.i32);
if (out.i64 !== input.i64) fail("i64 " + out.i64);
if (out.u32 !== input.u32) fail("u32 " + out.u32);
if (out.u64 !== input.u64) fail("u64 " + out.u64);
if (Math.abs(out.f32 - input.f32) > 1e-6) fail("f32 " + out.f32);
if (out.f64 !== input.f64) fail("f64 " + out.f64);
if (out.s !== input.s) fail("s " + out.s);
if (out.data.join(",") !== input.data.join(",")) fail("data " + out.data.join(","));
console.log("OK");
`
	runTSRoundTrip(t, dir, genFile, driver)
}

// TestTSCodegen_FieldOrder asserts positional fields round-trip in order. XPB V2
// is tagless, so a wrong-order generator would swap these distinct values.
func TestTSCodegen_FieldOrder(t *testing.T) {
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
	src := generateTS(t, schema)
	dir, genFile := writeTSProject(t, src)
	typeCheckTS(t, dir)

	driver := `import { Ordered } from './gen_exec.ts';
const input = new Ordered({ first: 1, second: 2, third: 3, label: "x", fourth: 4 });
const out = Ordered.decode(input.encode());
if (out.first !== 1 || out.second !== 2 || out.third !== 3 || out.label !== "x" || out.fourth !== 4) {
  console.error("FAIL", JSON.stringify(out));
  process.exit(1);
}
console.log("OK");
`
	runTSRoundTrip(t, dir, genFile, driver)
}

func TestTSCodegen_RepeatedFields(t *testing.T) {
	schema := `
package test

message Container {
    1: string name
    2: []string tags
    3: []int32 scores
}
`
	src := generateTS(t, schema)
	if !strings.Contains(string(src), "tags: string[]") {
		t.Errorf("missing 'tags: string[]'")
	}
	// Security regression (XPB-005): repeated-field counts must go through
	// dec.readArrayCount (which bounds the count against the remaining buffer),
	// not a raw readInt32 + unchecked new Array(count).
	if !strings.Contains(string(src), "dec.readArrayCount(") {
		t.Error("repeated-field decode must use dec.readArrayCount; got raw readInt32")
	}
	dir, genFile := writeTSProject(t, src)
	typeCheckTS(t, dir)

	driver := `import { Container } from './gen_exec.ts';
const input = new Container({ name: "box", tags: ["a","bb","ccc"], scores: [-1,0,7,1000000] });
const out = Container.decode(input.encode());
if (out.name !== "box" || out.tags.join(",") !== "a,bb,ccc" || out.scores.join(",") !== "-1,0,7,1000000") {
  console.error("FAIL", JSON.stringify(out, (_,v)=>typeof v==="bigint"?v.toString():v));
  process.exit(1);
}
console.log("OK");
`
	runTSRoundTrip(t, dir, genFile, driver)
}

func TestTSCodegen_NestedMessages(t *testing.T) {
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
	src := generateTS(t, schema)
	dir, genFile := writeTSProject(t, src)
	typeCheckTS(t, dir)

	driver := `import { Rectangle, Point } from './gen_exec.ts';
const input = new Rectangle({ topLeft: new Point({x:1,y:2}), bottomRight: new Point({x:30,y:40}) });
const out = Rectangle.decode(input.encode());
if (out.topLeft.x !== 1 || out.topLeft.y !== 2 || out.bottomRight.x !== 30 || out.bottomRight.y !== 40) {
  console.error("FAIL", JSON.stringify(out));
  process.exit(1);
}
console.log("OK");
`
	runTSRoundTrip(t, dir, genFile, driver)
}

// TestTSCodegen_WithEnum is the regression test for the enum bug: enum-typed
// fields must encode/decode as int32 (matching the Go runtime), not as nested
// messages, and `Status` must be usable as a TypeScript type.
func TestTSCodegen_WithEnum(t *testing.T) {
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
	src := generateTS(t, schema)
	if strings.Contains(string(src), "Status.encode") || strings.Contains(string(src), "Status.decode") {
		t.Fatalf("enum field must not be treated as a message (found Status.encode/decode):\n%s", src)
	}
	dir, genFile := writeTSProject(t, src)
	typeCheckTS(t, dir)

	driver := `import { User, Status } from './gen_exec.ts';
const input = new User({ name: "bob", status: Status.INACTIVE, age: 42 });
const out = User.decode(input.encode());
if (out.name !== "bob" || out.status !== Status.INACTIVE || out.age !== 42) {
  console.error("FAIL", JSON.stringify(out));
  process.exit(1);
}
console.log("OK");
`
	runTSRoundTrip(t, dir, genFile, driver)
}

// TestTSCodegen_OptionalField round-trips BOTH a present optional (value
// preserved) and an absent optional (decodes to undefined) and proves that the
// field after an absent optional still decodes correctly -- i.e. the 1-byte
// presence flag is consumed and doesn't corrupt the following field.
//
// Wire format: an optional field is encoded as a 1-byte presence flag (0x01 +
// value when present, 0x00 with no value bytes when absent).
func TestTSCodegen_OptionalField(t *testing.T) {
	schema := `
package test

message Profile {
    1: string bio
    2: ?string avatar_url
    3: int32 followers
}

message Pair {
    1: ?string a
    2: int32 b
}
`
	src := generateTS(t, schema)
	if !strings.Contains(string(src), "avatarUrl?: string") {
		t.Errorf("missing optional 'avatarUrl?: string'")
	}
	dir, genFile := writeTSProject(t, src)
	typeCheckTS(t, dir)

	driver := `import { Profile, Pair } from './gen_exec.ts';
function fail(m: string): never { console.error("FAIL: " + m); process.exit(1); }

// Present optional round-trips its value.
{
  const input = new Profile({ bio: "hi", avatarUrl: "http://x/y.png", followers: 9 });
  const out = Profile.decode(input.encode());
  if (out.bio !== "hi" || out.avatarUrl !== "http://x/y.png" || out.followers !== 9) {
    fail("present " + JSON.stringify(out));
  }
}

// Absent optional decodes to undefined AND the following field still decodes.
{
  const input = new Profile({ bio: "hi", followers: 9 }); // avatarUrl omitted
  const out = Profile.decode(input.encode());
  if (out.avatarUrl !== undefined) fail("absent optional should be undefined, got " + out.avatarUrl);
  if (out.bio !== "hi") fail("field before optional corrupted: " + out.bio);
  if (out.followers !== 9) fail("field after absent optional corrupted (presence byte not consumed): " + out.followers);
}

// Exact-byte check on {?string a, int32 b} with a absent: 1 presence byte + 4 int32 bytes.
{
  const input = new Pair({ b: 1234 });
  const bytes = input.encode();
  if (bytes.length !== 5) fail("absent-optional Pair = " + bytes.length + " bytes, want 5");
  if (bytes[0] !== 0x00) fail("presence byte = " + bytes[0] + ", want 0");
  if (bytes[1] !== 0xD2 || bytes[2] !== 0x04 || bytes[3] !== 0 || bytes[4] !== 0) fail("int32 mis-encoded: " + Array.from(bytes).join(","));
  const out = Pair.decode(bytes);
  if (out.a !== undefined || out.b !== 1234) fail("Pair round-trip a=" + out.a + " b=" + out.b);
}

console.log("OK");
`
	runTSRoundTrip(t, dir, genFile, driver)
}

func TestTSCodegen_MapField(t *testing.T) {
	schema := `
package test

message Config {
    1: string name
    2: map<string, int32> counts
}
`
	src := generateTS(t, schema)
	dir, genFile := writeTSProject(t, src)
	typeCheckTS(t, dir)

	driver := `import { Config } from './gen_exec.ts';
const input = new Config({ name: "cfg", counts: new Map([["a",1],["b",2],["c",3]]) });
const out = Config.decode(input.encode());
if (out.name !== "cfg" || out.counts.get("a") !== 1 || out.counts.get("b") !== 2 || out.counts.get("c") !== 3 || out.counts.size !== 3) {
  console.error("FAIL", JSON.stringify(out, (_,v)=>v instanceof Map?[...v]:v));
  process.exit(1);
}
console.log("OK");
`
	runTSRoundTrip(t, dir, genFile, driver)
}

// BenchmarkTSCodegen_Simple benchmarks simple message generation.
func BenchmarkTSCodegen_Simple(b *testing.B) {
	file, err := parser.ParseFile(`
package test

message User {
    1: string name
    2: int32 age
}
`)
	if err != nil {
		b.Fatalf("parse: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = typescript.Generate(file)
	}
}

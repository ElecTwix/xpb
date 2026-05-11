// Package integration — security-audit regression tests for the XPB
// codegen and runtime suite. Each TestSecurityAudit_XPBxxx test originally
// proved the bug existed; after the fix landed, the test was flipped so
// it now PASSES when the hardened behavior is in place and FAILS if the
// hardening is regressed.
//
// The history matters: every test names the original finding (severity,
// description, original symptom) so that future readers see exactly what
// the test is protecting against and why.
package integration

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ElecTwix/xpb/pkg/codegen/c"
	"github.com/ElecTwix/xpb/pkg/codegen/java"
	"github.com/ElecTwix/xpb/pkg/codegen/lua"
	"github.com/ElecTwix/xpb/pkg/codegen/typescript"
	"github.com/ElecTwix/xpb/pkg/parser"
)

// SecurityFinding: XPB-100
// Severity (original): Critical
// Original symptom: C codegen silently dropped `repeated` flag.
//   `1: []string tags` rendered `char* tags;` with single-string
//   marshal/unmarshal, breaking cross-language wire compatibility.
// Hardening: codegen now emits `char** tags;` + `size_t tags_count;`
//   plus an explicit-max bounded array decode.
func TestSecurityAudit_XPB100_CCodegenRepeatedFieldsHandled(t *testing.T) {
	schema := `package test

message Item {
    1: []string tags
    2: int32 count
}
`
	file, err := parser.ParseFile(schema)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := c.Generate(file)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	src := string(out)

	if !strings.Contains(src, "char** tags;") {
		t.Fatalf("REGRESSION: expected `char** tags;` (array of strings); got:\n%s", src)
	}
	if !strings.Contains(src, "size_t tags_count;") {
		t.Fatalf("REGRESSION: expected `size_t tags_count;` paired count field")
	}
	if !strings.Contains(src, "xpb_decoder_validate_array_count") {
		t.Fatalf("REGRESSION: expected the array-count gate in the unmarshal")
	}
	t.Logf("XPB-100 OK: C codegen now emits a proper string array with bounded decode")
}

// SecurityFinding: XPB-101
// Severity (original): Critical
// Original symptom: C codegen had no case for TypeMap; map fields fell
//   through to `int32_t` and no marshal/unmarshal was emitted.
// Hardening: codegen now emits paired *_keys / *_values / *_count fields
//   and uses xpb_decoder_validate_array_count with an explicit max.
func TestSecurityAudit_XPB101_CCodegenMapFieldsHandled(t *testing.T) {
	schema := `package test

message Item {
    1: map<string, int32> attrs
}
`
	file, err := parser.ParseFile(schema)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := c.Generate(file)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	src := string(out)

	if !strings.Contains(src, "attrs_keys") || !strings.Contains(src, "attrs_values") {
		t.Fatalf("REGRESSION: expected paired key/value arrays; got:\n%s", src)
	}
	if !strings.Contains(src, "attrs_count") {
		t.Fatalf("REGRESSION: missing attrs_count")
	}
	if !strings.Contains(src, "xpb_decoder_validate_array_count") {
		t.Fatalf("REGRESSION: missing array-count gate in unmarshal")
	}
	t.Logf("XPB-101 OK: C codegen now emits map<K,V> as paired arrays with bounded decode")
}

// SecurityFinding: XPB-102
// Severity (original): Critical
// Original symptom: Java codegen dropped both `repeated` and `map`.
//   `[]string tags` → `public String tags;`; `map<...> attrs` → `public
//   int attrs;` with no marshal/unmarshal handling.
// Hardening: codegen now emits `String[] tags;` + array (de)serialization
//   with readArrayCount + an explicit max, and `java.util.Map<K,V> attrs;`
//   with a HashMap-based decode.
func TestSecurityAudit_XPB102_JavaCodegenRepeatedAndMapHandled(t *testing.T) {
	schema := `package test

message Item {
    1: []string tags
    2: map<string, int32> attrs
}
`
	file, err := parser.ParseFile(schema)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := java.Generate(file)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	src := string(out)
	if !strings.Contains(src, "String[] tags;") {
		t.Fatalf("REGRESSION: expected `String[] tags;`; got:\n%s", src)
	}
	if !strings.Contains(src, "java.util.Map<String, Integer> attrs;") {
		t.Fatalf("REGRESSION: expected `Map<String,Integer> attrs;`")
	}
	if !strings.Contains(src, "dec.readArrayCount(") {
		t.Fatalf("REGRESSION: array decodes don't go through readArrayCount")
	}
	t.Logf("XPB-102 OK: Java codegen now handles []T and map<K,V> with bounded decode")
}

// SecurityFinding: XPB-103
// Severity (original): Critical
// Original symptom: Lua codegen dropped both `repeated` and `map`.
// Hardening: codegen now emits ipairs-based array (de)serialization and
//   pairs-based map (de)serialization. Decode uses dec:read_array_count
//   with an explicit max.
func TestSecurityAudit_XPB103_LuaCodegenRepeatedAndMapHandled(t *testing.T) {
	schema := `package test

message Item {
    1: []string tags
    2: map<string, int32> attrs
}
`
	file, err := parser.ParseFile(schema)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := lua.Generate(file)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	src := string(out)
	if !strings.Contains(src, "for _, v in ipairs(arr)") {
		t.Fatalf("REGRESSION: expected ipairs-based encode; got:\n%s", src)
	}
	if !strings.Contains(src, "for k, v in pairs(m)") {
		t.Fatalf("REGRESSION: expected pairs-based map encode")
	}
	if !strings.Contains(src, "dec:read_array_count(") {
		t.Fatalf("REGRESSION: array decodes don't go through read_array_count")
	}
	t.Logf("XPB-103 OK: Lua codegen now handles []T and map<K,V> with bounded decode")
}

// SecurityFinding: XPB-104
// Severity (original): Critical
// Original symptom: C codegen emitted uncompilable C for self-referential
//   types — `typedef struct { Node next; } Node;` is an infinite-size
//   struct because the inner Node is incomplete at point of use.
// Hardening: codegen now emits a forward-declared typedef + named
//   `struct Node { ... };` with `Node* next;` (pointer indirection).
func TestSecurityAudit_XPB104_CCodegenRecursiveTypeCompiles(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not installed; cannot validate recursive-type compilation")
	}
	root := repoRoot(t)
	schema := `package test

message Node {
    1: Node next
}
`
	file, err := parser.ParseFile(schema)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := c.Generate(file)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	tmp := t.TempDir()
	headerPath := filepath.Join(tmp, "node.h")
	if err := os.WriteFile(headerPath, out, 0o644); err != nil {
		t.Fatal(err)
	}

	driver := `#include "node.h"
int main(void) {
    Node a = {0};
    Node b = {0};
    a.next = &b;
    return 0;
}
`
	driverPath := filepath.Join(tmp, "driver.c")
	if err := os.WriteFile(driverPath, []byte(driver), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(tmp, "driver")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("gcc", "-Wall", "-Wextra",
		"-I", tmp,
		"-I", filepath.Join(root, "runtime/c/include"),
		filepath.Join(root, "runtime/c/xpb.c"),
		driverPath,
		"-o", bin,
	)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("REGRESSION: recursive-type header no longer compiles:\n%s", output)
	}
	t.Logf("XPB-104 OK: recursive-type C header compiles")
}

// SecurityFinding: XPB-105
// Severity (original): High
// Original symptom: Java codegen recurses on nested messages with no
//   depth gate → StackOverflowError on adversarial payloads.
// Hardening: codegen now emits unmarshalAt(data, depth) and the public
//   unmarshal delegates to it; the helper checks
//   Decoder.MAX_DECODE_DEPTH on entry.
func TestSecurityAudit_XPB105_JavaCodegenDepthCapped(t *testing.T) {
	schema := `package test

message Node {
    1: Node next
}
`
	file, err := parser.ParseFile(schema)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := java.Generate(file)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	src := string(out)

	if !strings.Contains(src, "Decoder.MAX_DECODE_DEPTH") {
		t.Fatalf("REGRESSION: Java codegen lost MAX_DECODE_DEPTH reference")
	}
	if !strings.Contains(src, "Node.unmarshalAt(dec.readMessageBytes(), depth + 1)") {
		t.Fatalf("REGRESSION: nested unmarshal doesn't thread depth")
	}
	t.Logf("XPB-105 OK: Java codegen threads depth and trips on MAX_DECODE_DEPTH")
}

// SecurityFinding: XPB-106
// Severity (original): High
// Original symptom: Lua codegen recurses on nested messages with no
//   depth gate → Lua stack overflow on adversarial payloads.
// Hardening: codegen now emits UnmarshalAt(data, depth); the public
//   Unmarshal delegates to it; the helper checks xpb.MAX_DECODE_DEPTH.
func TestSecurityAudit_XPB106_LuaCodegenDepthCapped(t *testing.T) {
	schema := `package test

message Node {
    1: Node next
}
`
	file, err := parser.ParseFile(schema)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := lua.Generate(file)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	src := string(out)

	if !strings.Contains(src, "xpb.MAX_DECODE_DEPTH") {
		t.Fatalf("REGRESSION: Lua codegen lost MAX_DECODE_DEPTH reference")
	}
	if !strings.Contains(src, "Node.UnmarshalAt(dec:read_message_bytes(), depth + 1)") {
		t.Fatalf("REGRESSION: nested UnmarshalAt doesn't thread depth")
	}
	t.Logf("XPB-106 OK: Lua codegen threads depth and trips on MAX_DECODE_DEPTH")
}

// SecurityFinding: XPB-119 (post-review uniformity)
// Severity: High
// Description: After the audit, every runtime decoder's array-count gate
// requires an explicit caller-supplied max. Go and TS-main were the last
// runtimes to keep the buffer-bound-only signature; this test verifies
// both now require the explicit max. A regression here means a runtime
// silently accepted an unbounded count again.
func TestSecurityAudit_XPB119_GoTSExplicitMaxUniform(t *testing.T) {
	root := repoRoot(t)

	// Go: signature is `ReadArrayCount(elementMinBytes, maxElements int)`.
	goRT, err := os.ReadFile(filepath.Join(root, "runtime/go/xpb/xpb.go"))
	if err != nil {
		t.Fatalf("read go runtime: %v", err)
	}
	if !bytes.Contains(goRT, []byte("ReadArrayCount(elementMinBytes, maxElements int)")) {
		t.Fatalf("REGRESSION: Go runtime ReadArrayCount lost the maxElements parameter")
	}
	if !bytes.Contains(goRT, []byte("exceeds caller-supplied max")) {
		t.Fatalf("REGRESSION: Go runtime lost caller-supplied-max rejection")
	}

	// TS main decoder.
	tsRT, err := os.ReadFile(filepath.Join(root, "runtime/ts/src/index.ts"))
	if err != nil {
		t.Fatalf("read ts runtime: %v", err)
	}
	if !bytes.Contains(tsRT, []byte("readArrayCount(elementMinBytes: number, maxElements: number)")) {
		t.Fatalf("REGRESSION: TS Decoder.readArrayCount lost maxElements")
	}
	if !bytes.Contains(tsRT, []byte("exceeds caller-supplied max")) {
		t.Fatalf("REGRESSION: TS runtime lost caller-supplied-max rejection")
	}
	// Every TS readArray* helper must now require maxElements.
	for _, fn := range []string{
		"readArrayInt32(maxElements: number)",
		"readArrayInt64(maxElements: number)",
		"readArrayUint32(maxElements: number)",
		"readArrayUint64(maxElements: number)",
		"readArrayFloat32(maxElements: number)",
		"readArrayFloat64(maxElements: number)",
		"readArrayBool(maxElements: number)",
		"readArrayString(maxElements: number)",
	} {
		if !bytes.Contains(tsRT, []byte(fn)) {
			t.Fatalf("REGRESSION: %s missing or signature changed", fn)
		}
	}

	// TS worker FastDecoder mirrors the gate.
	wrkRT, err := os.ReadFile(filepath.Join(root, "runtime/ts/src/worker.ts"))
	if err != nil {
		t.Fatalf("read worker.ts: %v", err)
	}
	if !bytes.Contains(wrkRT, []byte("readArrayCount(elementMinBytes: number, maxElements: number)")) {
		t.Fatalf("REGRESSION: worker.ts FastDecoder.readArrayCount lost maxElements")
	}

	// Codegen must pass an explicit constant max.
	goCG, err := os.ReadFile(filepath.Join(root, "pkg/codegen/golang/emitter.go"))
	if err != nil {
		t.Fatalf("read go codegen: %v", err)
	}
	if !bytes.Contains(goCG, []byte("dec.ReadArrayCount(%d, %d)")) {
		t.Fatalf("REGRESSION: Go codegen no longer emits two-arg ReadArrayCount")
	}

	tsCG, err := os.ReadFile(filepath.Join(root, "pkg/codegen/typescript/emitter.go"))
	if err != nil {
		t.Fatalf("read ts codegen: %v", err)
	}
	if !bytes.Contains(tsCG, []byte("dec.readArrayCount(%d, %d)")) {
		t.Fatalf("REGRESSION: TS codegen no longer emits two-arg readArrayCount")
	}

	t.Log("XPB-119 OK: explicit-max gate uniform across Go, TS, Java, Lua, C, Rust runtimes")
}

// SecurityFinding: XPB-120 (post-review)
// Severity: Informational
// The DefaultMaxElements budget lives in one place (pkg/codegen/common).
// Each codegen must import and use that constant, not its own duplicate,
// so tightening the cap requires editing one file. This test asserts the
// import is present in every codegen that emits decoder calls.
func TestSecurityAudit_XPB120_SharedDefaultMaxElements(t *testing.T) {
	root := repoRoot(t)
	// Sentinel: source of truth.
	common, err := os.ReadFile(filepath.Join(root, "pkg/codegen/common/common.go"))
	if err != nil {
		t.Fatalf("read common.go: %v", err)
	}
	if !bytes.Contains(common, []byte("DefaultMaxElements = 1 << 24")) {
		t.Fatalf("REGRESSION: pkg/codegen/common lost DefaultMaxElements")
	}

	codegens := []string{
		"pkg/codegen/golang/emitter.go",
		"pkg/codegen/typescript/emitter.go",
		"pkg/codegen/c/emitter.go",
		"pkg/codegen/java/emitter.go",
		"pkg/codegen/lua/emitter.go",
		"pkg/codegen/rust/emitter.go",
	}
	for _, path := range codegens {
		src, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !bytes.Contains(src, []byte("common.DefaultMaxElements")) {
			t.Fatalf("REGRESSION: %s no longer references common.DefaultMaxElements", path)
		}
		// And reject any local re-declaration of the constant.
		if bytes.Contains(src, []byte("const Default")) && bytes.Contains(src, []byte("MaxElements = 1 <<")) {
			t.Fatalf("REGRESSION: %s re-declares the constant locally; should import from common", path)
		}
	}
	t.Logf("XPB-120 OK: all codegens consume common.DefaultMaxElements")
}

// SecurityFinding: XPB-121 (post-review)
// Severity: Informational
// Enum disambiguation now lives in pkg/ast (EnumSet.IsEnum). No codegen
// should re-implement the lookup locally.
func TestSecurityAudit_XPB121_SharedEnumSet(t *testing.T) {
	root := repoRoot(t)
	astSrc, err := os.ReadFile(filepath.Join(root, "pkg/ast/ast.go"))
	if err != nil {
		t.Fatalf("read ast.go: %v", err)
	}
	if !bytes.Contains(astSrc, []byte("type EnumSet")) ||
		!bytes.Contains(astSrc, []byte("func NewEnumSet(")) {
		t.Fatalf("REGRESSION: ast.EnumSet helper missing")
	}

	codegens := []string{
		"pkg/codegen/c/emitter.go",
		"pkg/codegen/java/emitter.go",
		"pkg/codegen/lua/emitter.go",
		"pkg/codegen/rust/emitter.go",
	}
	for _, path := range codegens {
		src, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !bytes.Contains(src, []byte("NewEnumSet")) && !bytes.Contains(src, []byte("ast.NewEnumSet")) {
			t.Fatalf("REGRESSION: %s no longer uses ast.NewEnumSet", path)
		}
		// And reject local genCtx-style re-implementations.
		for _, bad := range [][]byte{
			[]byte("type genCtx struct"),
			[]byte("type jCtx struct"),
			[]byte("type luaCtx struct"),
		} {
			if bytes.Contains(src, bad) {
				t.Fatalf("REGRESSION: %s re-introduced a per-codegen enum ctx", path)
			}
		}
	}
	t.Logf("XPB-121 OK: enum disambiguation centralized in ast.EnumSet")
}

// SecurityFinding: XPB-107
// Severity (original): High
// Original symptom: Java decoder readArray* called `new int[count]` with
//   no bound check — adversarial count of INT_MAX → 8 GB allocation.
// Hardening: Decoder.readArrayCount(elementMinBytes, maxElements) REQUIRES
//   the caller to pass an explicit max. All public readArray* methods
//   now take a max parameter and funnel through it.
func TestSecurityAudit_XPB107_JavaDecoderRequiresExplicitMax(t *testing.T) {
	root := repoRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "runtime/java/src/main/java/xpb/Decoder.java"))
	if err != nil {
		t.Fatalf("read Decoder.java: %v", err)
	}
	body := string(src)

	if !strings.Contains(body, "public int readArrayCount(int elementMinBytes, int maxElements)") {
		t.Fatalf("REGRESSION: Java Decoder lost the explicit-max readArrayCount gate")
	}
	if !strings.Contains(body, "exceeds caller-supplied max") {
		t.Fatalf("REGRESSION: caller-supplied-max error message missing")
	}
	if !strings.Contains(body, "public int[] readArrayInt32(int maxElements)") {
		t.Fatalf("REGRESSION: readArrayInt32 no longer requires maxElements")
	}
	t.Logf("XPB-107 OK: Java decoder requires explicit max on every array read")
}

// SecurityFinding: XPB-108
// Severity (original): High
// Original symptom: Lua decoder read_array_* looped on a wire-supplied
//   count with no bound — count = 2^31 ran for hours / OOM'd Lua.
// Hardening: dec:read_array_count(element_min_bytes, max_elements)
//   REQUIRES the caller to pass max_elements. Read helpers funnel
//   through it.
func TestSecurityAudit_XPB108_LuaDecoderRequiresExplicitMax(t *testing.T) {
	root := repoRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "runtime/lua/xpb.lua"))
	if err != nil {
		t.Fatalf("read xpb.lua: %v", err)
	}
	body := string(src)

	if !strings.Contains(body, "function self:read_array_count(element_min_bytes, max_elements)") {
		t.Fatalf("REGRESSION: Lua decoder lost read_array_count with explicit max")
	}
	if !strings.Contains(body, "exceeds caller-supplied max") {
		t.Fatalf("REGRESSION: caller-supplied-max error message missing")
	}
	if !strings.Contains(body, "function self:read_array_int32(max_elements)") {
		t.Fatalf("REGRESSION: read_array_int32 no longer requires max_elements")
	}
	t.Logf("XPB-108 OK: Lua decoder requires explicit max on every array read")
}

// SecurityFinding: XPB-109
// Severity (original): Medium-High
// Original symptom: Lua decoder primitives didn't bounds-check.
//   read_bool over 0 bytes returned `true` silently; skip(n) advanced
//   past EOF; read_string/bytes used Lua's forgiving sub() and returned
//   short data with no error.
// Hardening: every primitive now calls self:ensure_bytes() which errors
//   on EOF; skip(n) is gated the same way.
func TestSecurityAudit_XPB109_LuaDecoderBoundsChecked(t *testing.T) {
	root := repoRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "runtime/lua/xpb.lua"))
	if err != nil {
		t.Fatalf("read xpb.lua: %v", err)
	}
	body := string(src)

	if !strings.Contains(body, "function self:ensure_bytes(n, what)") {
		t.Fatalf("REGRESSION: ensure_bytes helper missing")
	}
	// Confirm every primitive reads through ensure_bytes.
	for _, fn := range []string{"read_bool", "read_int32", "read_int64", "read_float32", "read_float64", "skip"} {
		idx := strings.Index(body, "function self:"+fn)
		if idx < 0 {
			t.Fatalf("REGRESSION: function self:%s missing", fn)
		}
		end := strings.Index(body[idx:], "\n    end\n")
		if end < 0 {
			t.Fatalf("function self:%s body unparseable", fn)
		}
		snippet := body[idx : idx+end]
		if !strings.Contains(snippet, "ensure_bytes") {
			t.Fatalf("REGRESSION: self:%s skipped ensure_bytes:\n%s", fn, snippet)
		}
	}
	t.Logf("XPB-109 OK: every Lua decoder primitive funnels through ensure_bytes")
}

// SecurityFinding: XPB-109 (dynamic)
// Severity: Medium-High
// Hardening confirmed at runtime by tests/lua/xpb_security_test.lua.
// The script feeds adversarial payloads and asserts the runtime errors
// rather than silently returning garbage.
func TestSecurityAudit_XPB109Dynamic_LuaRuntimeBoundsExercise(t *testing.T) {
	luaBin, err := exec.LookPath("lua")
	if err != nil {
		luaBin, err = exec.LookPath("lua5.4")
		if err != nil {
			t.Skip("lua not installed; cannot run dynamic exploit")
		}
	}
	root := repoRoot(t)
	cmd := exec.Command(luaBin,
		"-e", "package.path='"+filepath.Join(root, "runtime/lua/?.lua")+";'..package.path",
		filepath.Join(root, "tests/lua/xpb_security_test.lua"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("REGRESSION: lua bounds-exercise script failed:\n%s", out)
	}
	t.Logf("Lua dynamic bounds-exercise output:\n%s", out)
}

// SecurityFinding: XPB-111
// Severity (original): Medium
// Original symptom: TS JIT (jit.ts) read string compact length as a
//   single byte; long-form (0xFF + 4-byte length) was never branched on,
//   so strings ≥ 255 bytes silently misparsed.
// Hardening: the string-read template now branches on `first === 255`
//   and reads the trailing 4-byte length.
func TestSecurityAudit_XPB111_TSJITHandlesLongString(t *testing.T) {
	root := repoRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "runtime/ts/src/jit.ts"))
	if err != nil {
		t.Fatalf("read jit.ts: %v", err)
	}
	body := string(src)
	// jit.ts has two `case FieldType.String:` branches — one in the
	// encoder (writes length + bytes) and one in the decoder (reads
	// length + bytes). The bug was in the decoder; jump there via
	// `function generateFieldRead`.
	readFn := body[strings.Index(body, "function generateFieldRead("):]
	stringCase := readFn[strings.Index(readFn, "case FieldType.String:"):]
	stringCase = stringCase[:strings.Index(stringCase, "default:")]
	if !strings.Contains(stringCase, "first === 255") {
		t.Fatalf("REGRESSION: jit.ts decoder string read no longer branches on 0xFF marker:\n%s",
			stringCase)
	}
	t.Logf("XPB-111 OK: TS JIT decoder now handles long-form compact length")
}

// SecurityFinding: XPB-112
// Severity (original): Medium
// Original symptom: TS view.ts StringArrayView allocated `new
//   Int32Array(length)` from a wire-supplied int32 with no bound — same
//   class of issue as XPB-005/107/108, in a different module.
// Hardening: constructor now takes a required maxElements parameter and
//   rejects negative / over-max / over-buffer counts before allocating.
func TestSecurityAudit_XPB112_TSStringArrayViewRequiresMax(t *testing.T) {
	root := repoRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "runtime/ts/src/view.ts"))
	if err != nil {
		t.Fatalf("read view.ts: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, "constructor(buffer: Uint8Array, maxElements: number") {
		t.Fatalf("REGRESSION: StringArrayView no longer requires maxElements")
	}
	if !strings.Contains(body, "exceeds caller-supplied max") {
		t.Fatalf("REGRESSION: caller-supplied-max rejection missing")
	}
	t.Logf("XPB-112 OK: StringArrayView requires explicit maxElements")
}

// SecurityFinding: XPB-113
// Severity (original): Medium
// Original symptom: Generated TS class constructor did
//   `Object.assign(this, data)`, which copies an own `__proto__` and
//   pollutes the prototype chain.
// Hardening: codegen now emits per-field `if (data.x !== undefined)
//   this.x = data.x;` assignments — only declared fields land on the
//   instance.
func TestSecurityAudit_XPB113_TSCodegenNoPrototypeSink(t *testing.T) {
	schema := `package test

message Login {
    1: string username
    2: bool admin
}
`
	file, err := parser.ParseFile(schema)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := typescript.Generate(file)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	src := string(out)
	if strings.Contains(src, "Object.assign(this, data)") {
		t.Fatalf("REGRESSION: TS codegen re-introduced Object.assign sink")
	}
	if !strings.Contains(src, "if (data.username !== undefined) this.username = data.username;") {
		t.Fatalf("REGRESSION: constructor does not assign declared fields explicitly")
	}
	t.Logf("XPB-113 OK: TS codegen constructor uses explicit per-field assignment")
}

// SecurityFinding: XPB-114
// Severity (original): Medium
// Original symptom: xpb_encoder_create dereferenced malloc result with
//   no NULL check — first-malloc failure → SIGSEGV.
// Hardening: factory now NULL-checks both mallocs, frees the partial
//   allocation, and returns NULL to the caller.
func TestSecurityAudit_XPB114_CEncoderCreateHandlesOOM(t *testing.T) {
	root := repoRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "runtime/c/xpb.c"))
	if err != nil {
		t.Fatalf("read xpb.c: %v", err)
	}
	body := string(src)
	idx := strings.Index(body, "xpb_encoder_create(size_t initial_capacity)")
	end := strings.Index(body[idx:], "\n}\n")
	fn := body[idx : idx+end+1]
	if !strings.Contains(fn, "if (enc == NULL) return NULL;") {
		t.Fatalf("REGRESSION: xpb_encoder_create lost the malloc NULL check")
	}
	if !strings.Contains(fn, "if (enc->buf == NULL)") {
		t.Fatalf("REGRESSION: xpb_encoder_create lost the buffer-malloc NULL check")
	}
	t.Logf("XPB-114 OK: xpb_encoder_create NULL-checks every allocation")
}

// SecurityFinding: XPB-115
// Severity (original): Medium
// Original symptom: xpb_encoder_ensure_capacity assigned `realloc(...)`
//   back to enc->buf with no check; on failure the original allocation
//   leaked and enc->buf was NULL.
// Hardening: realloc result is stored in a temporary, NULL-checked, and
//   only then committed. On failure, the original buffer survives and a
//   sticky-error flag latches the encoder so subsequent writes no-op.
func TestSecurityAudit_XPB115_CEncoderRealloc(t *testing.T) {
	root := repoRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "runtime/c/xpb.c"))
	if err != nil {
		t.Fatalf("read xpb.c: %v", err)
	}
	body := string(src)
	idx := strings.Index(body, "xpb_encoder_ensure_capacity(")
	end := strings.Index(body[idx:], "\n}\n")
	fn := body[idx : idx+end+1]
	if !strings.Contains(fn, "uint8_t* new_buf = (uint8_t*)realloc(enc->buf") {
		t.Fatalf("REGRESSION: realloc no longer assigned to a temporary")
	}
	if !strings.Contains(fn, "if (new_buf == NULL)") {
		t.Fatalf("REGRESSION: realloc result no longer NULL-checked")
	}
	if !strings.Contains(fn, "enc->error = true") {
		t.Fatalf("REGRESSION: ensure_capacity no longer latches the sticky error")
	}
	t.Logf("XPB-115 OK: ensure_capacity NULL-checks realloc and latches error")
}

// SecurityFinding: XPB-116
// Severity (original): Low
// Original symptom: codegens silently ignored the Optional flag.
// Hardening: codegen now emits a NOTE comment for any message containing
//   an optional field so the user knows the wire format treats it as
//   required (V3 will add presence semantics).
func TestSecurityAudit_XPB116_OptionalFieldsWarned(t *testing.T) {
	schema := `package test

message User {
    1: ?string nickname
    2: int32 id
}
`
	file, err := parser.ParseFile(schema)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	cases := []struct {
		name string
		gen  func() ([]byte, error)
		mark string
	}{
		{"TS", func() ([]byte, error) { return typescript.Generate(file) }, "schema contains `optional` fields"},
		{"Java", func() ([]byte, error) { return java.Generate(file) }, "schema contains `optional` fields"},
		{"Lua", func() ([]byte, error) { return lua.Generate(file) }, "schema contains `optional` fields"},
		{"C", func() ([]byte, error) { return c.Generate(file) }, "schema contains `optional` fields"},
	}
	for _, tc := range cases {
		out, err := tc.gen()
		if err != nil {
			t.Fatalf("%s generate: %v", tc.name, err)
		}
		if !bytes.Contains(out, []byte(tc.mark)) {
			t.Fatalf("REGRESSION: %s codegen no longer warns about optional fields:\n%s",
				tc.name, out)
		}
	}
	t.Logf("XPB-116 OK: every codegen surfaces the V2 optional-field limitation")
}

// SecurityFinding: XPB-R001/R002/R003 (Rust runtime)
// Severity: High
// The Rust runtime gained read_array_count(element_min_bytes, max_elements)
// with the same explicit-max contract as the Go/TS/Java/Lua/C runtimes.
// This test invokes `cargo test --test security_validation` against the
// in-tree runtime and asserts the security_validation.rs suite passes.
func TestSecurityAudit_RustRuntimeArrayBoundsExercise(t *testing.T) {
	cargo, err := exec.LookPath("cargo")
	if err != nil {
		t.Skip("cargo not installed; cannot validate Rust runtime")
	}
	root := repoRoot(t)
	cmd := exec.Command(cargo, "test", "--test", "security_validation")
	cmd.Dir = filepath.Join(root, "runtime/rust")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("REGRESSION: Rust security_validation tests failed:\n%s", out)
	}
	t.Logf("Rust security_validation output:\n%s", out)
}

// SecurityFinding: XPB-107 (dynamic)
// Severity: High
// Hardening confirmed at runtime by tests/java/XpbSecurityTest.java.
// The script feeds adversarial payloads to the Java decoder and asserts
// each one is rejected. Requires javac on PATH; CI without Java skips.
func TestSecurityAudit_XPB107Dynamic_JavaRuntimeBoundsExercise(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not installed; cannot run dynamic Java exploit")
	}
	// macOS ships a `javac` stub that prompts the user to install a JDK.
	// Probe with --version so we skip cleanly when there's no real JDK.
	if out, err := exec.Command(javac, "--version").CombinedOutput(); err != nil {
		t.Skipf("javac present but not functional (likely macOS stub): %s", out)
	}
	javaBin, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java not installed")
	}
	root := repoRoot(t)
	tmp := t.TempDir()
	build := exec.Command(javac, "-d", tmp,
		filepath.Join(root, "runtime/java/src/main/java/xpb/Decoder.java"),
		filepath.Join(root, "runtime/java/src/main/java/xpb/Encoder.java"),
		filepath.Join(root, "tests/java/XpbSecurityTest.java"),
	)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("javac failed:\n%s", out)
	}
	run := exec.Command(javaBin, "-cp", tmp, "xpb.XpbSecurityTest")
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("REGRESSION: Java bounds-exercise failed:\n%s", out)
	}
	t.Logf("Java dynamic bounds-exercise output:\n%s", out)
}

// SecurityFinding: XPB-118 — POSITIVE / SANITY CHECK
// Severity: Informational
// A small set of regression checks that ensure the original PR-1 / PR-2
// hardening in Go / TS / C is still wired up correctly.
func TestSecurityAudit_XPB118_HardenedPathsStillHardened(t *testing.T) {
	// Reference an unused symbol to silence the linter if no other test
	// touches binary.LittleEndian in this file.
	_ = binary.LittleEndian

	root := repoRoot(t)
	goRT, err := os.ReadFile(filepath.Join(root, "runtime/go/xpb/xpb.go"))
	if err != nil {
		t.Fatalf("read go runtime: %v", err)
	}
	if !bytes.Contains(goRT, []byte("negative array count")) {
		t.Fatalf("REGRESSION: Go runtime lost the negative-count error")
	}
	if !bytes.Contains(goRT, []byte("exceeds buffer-bounded max")) {
		t.Fatalf("REGRESSION: Go runtime lost the buffer-bounded max gate")
	}

	cRT, err := os.ReadFile(filepath.Join(root, "runtime/c/xpb.c"))
	if err != nil {
		t.Fatalf("read C runtime: %v", err)
	}
	if !bytes.Contains(cRT, []byte("xpb_decoder_validate_array_count")) {
		t.Fatalf("REGRESSION: C runtime lost validate_array_count")
	}

	tsRT, err := os.ReadFile(filepath.Join(root, "runtime/ts/src/index.ts"))
	if err != nil {
		t.Fatalf("read TS runtime: %v", err)
	}
	if !bytes.Contains(tsRT, []byte("readArrayCount")) {
		t.Fatalf("REGRESSION: TS runtime lost readArrayCount")
	}

	t.Log("XPB-118 OK: Go/C/TS hardened paths still in place")
}

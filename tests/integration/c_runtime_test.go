// Package integration shells out to gcc to compile and run the C-runtime
// tests. Without this wrapper, runtime/c/xpb.c and tests/c/*.c are not
// exercised by `go test ./...` and silently rot when the runtime changes.
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ElecTwix/xpb/pkg/codegen/c"
	"github.com/ElecTwix/xpb/pkg/parser"
)

// runCBinary builds {sources...} into a temp binary and runs it. Returns
// the combined output and any non-zero-exit error from the binary.
func runCBinary(t *testing.T, label string, sources []string, includes []string, extraFlags []string) ([]byte, error) {
	t.Helper()
	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Skipf("gcc not installed; skipping C %s test", label)
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, label)
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	args := []string{"-Wall", "-Wextra", "-O0", "-o", bin}
	for _, inc := range includes {
		args = append(args, "-I", inc)
	}
	args = append(args, sources...)
	args = append(args, extraFlags...)
	build := exec.Command(gcc, args...)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("gcc build failed for %s: %v\n%s", label, err, out)
	}
	run := exec.Command(bin)
	return run.CombinedOutput()
}

// TestC_RuntimeRoundTrip runs the long-standing tests/c/xpb_test.c
// round-trip suite against the in-tree runtime/c/xpb.c. Brings the C
// runtime under `go test ./...`.
func TestC_RuntimeRoundTrip(t *testing.T) {
	root := repoRoot(t)
	out, err := runCBinary(t, "xpb_runtime_roundtrip",
		[]string{
			filepath.Join(root, "runtime/c/xpb.c"),
			filepath.Join(root, "tests/c/xpb_test.c"),
		},
		[]string{filepath.Join(root, "runtime/c/include")},
		[]string{"-lm"},
	)
	t.Logf("C runtime test output:\n%s", out)
	if err != nil {
		t.Fatalf("xpb_test.c failed: %v", err)
	}
}

// TestC_SecurityValidation runs the new tests/c/xpb_security_test.c
// (XPB-007/008/009/010 cases) against the in-tree runtime. Without this
// wrapper, regressions in the bounds checks and sticky-error behavior
// are silent until someone manually rebuilds the C harness.
func TestC_SecurityValidation(t *testing.T) {
	root := repoRoot(t)
	out, err := runCBinary(t, "xpb_security_validation",
		[]string{
			filepath.Join(root, "runtime/c/xpb.c"),
			filepath.Join(root, "tests/c/xpb_security_test.c"),
		},
		[]string{filepath.Join(root, "runtime/c/include")},
		nil,
	)
	t.Logf("C security test output:\n%s", out)
	if err != nil {
		t.Fatalf("xpb_security_test.c failed: %v", err)
	}
}

// TestC_GeneratedUnmarshalReportsFailure exercises review finding F2:
// the C codegen previously emitted T_unmarshal as `T T_unmarshal(...)`
// returning by value with no error channel, so a 1-byte payload over a
// multi-field schema decoded into a partially-populated struct without
// the caller knowing. After the fix, T_unmarshal returns bool and
// surfaces sticky decoder errors. This test generates the schema, writes
// a tiny C driver that calls the generated function with a malformed
// payload, and asserts the driver exits zero (i.e. the generated code
// reported failure as expected).
func TestC_GeneratedUnmarshalReportsFailure(t *testing.T) {
	root := repoRoot(t)
	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("gcc not installed; skipping generated-C unmarshal test")
	}

	schema := `package login

message Login {
    1: bool admin
    2: string name
    3: int32 attempts
}
`
	file, err := parser.ParseFile(schema)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	header, err := c.Generate(file)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	tmp := t.TempDir()
	headerPath := filepath.Join(tmp, "login.h")
	if err := os.WriteFile(headerPath, header, 0o644); err != nil {
		t.Fatal(err)
	}

	// Driver:
	//   - Truncated payload (1 byte) feeds a 3-field schema. The first
	//     read (bool) succeeds, the second (string compact length) hits
	//     EOF and sets the decoder's sticky error. Pre-fix this returned
	//     a populated Login{admin=1, name=NULL, attempts=0}; post-fix
	//     T_unmarshal returns false and the driver exits 0.
	driver := `#include <stdio.h>
#include <stdlib.h>
#include <stdbool.h>
#include "login.h"

int main(void) {
    uint8_t payload[1] = { 0x01 };
    Login out;
    bool ok = Login_unmarshal(&out, payload, sizeof(payload));
    if (ok) {
        fprintf(stderr, "FIX REGRESSED: Login_unmarshal accepted a 1-byte payload over a 3-field schema\n");
        return 1;
    }
    /* Free anything the partial decode might have allocated before bailing. */
    if (out.name != NULL) free(out.name);
    return 0;
}
`
	driverPath := filepath.Join(tmp, "driver.c")
	if err := os.WriteFile(driverPath, []byte(driver), 0o644); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(tmp, "driver")
	build := exec.Command(gcc, "-Wall", "-Wextra", "-O0",
		"-I", tmp, // for login.h
		"-I", filepath.Join(root, "runtime/c/include"),
		filepath.Join(root, "runtime/c/xpb.c"),
		driverPath,
		"-o", bin,
	)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("gcc build failed: %v\nheader:\n%s\noutput: %s", err, header, out)
	}

	out, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("FIX VERIFICATION FAILED: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "FIX REGRESSED") {
		t.Fatalf("driver detected regression: %s", out)
	}
}

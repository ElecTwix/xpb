package differential

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// diffMode selects what a per-runtime runner asserts over a corpus.
//
//   - modeBytes: decode + verify values AND assert the re-encode is
//     byte-identical to the Go reference .bin. Used for the map-FREE corpus.
//   - modeValues: decode + verify values ONLY (no byte-identity check). Used
//     for the map-CONTAINING corpus, because map entry order is non-canonical
//     across runtimes (T-7); the decoded values are still a real cross-language
//     oracle, but the exact byte ordering is not required to match.
type diffMode string

const (
	modeBytes  diffMode = "bytes"
	modeValues diffMode = "values"
)

// repoRoot walks up from the package directory to the module root (the dir that
// holds go.mod). The test always runs with cwd == tests/differential.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod above cwd")
		}
		dir = parent
	}
}

func have(bin string) bool { _, err := exec.LookPath(bin); return err == nil }

// runResult is the per-runtime verdict for one corpus.
type runResult struct {
	name      string
	exercised bool   // true if the toolchain was present and the runner ran
	skipped   string // non-empty reason when the runtime was unavailable
	err       error  // non-nil when the runner ran and reported a mismatch/failure
}

// run executes bin+args in dir (empty = repo root) capturing combined output;
// the output is folded into any error so a differential mismatch is legible.
func run(dir, bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w\n%s", bin, strings.Join(args, " "), err, out)
	}
	return nil
}

// allRunners returns every per-language differential runner. Each is gated on
// toolchain availability and returns a runResult; a missing toolchain is a
// clean skip, never a failure. Each runner takes the corpus dir and the diff
// mode (byte-identity vs values-only).
func allRunners() []func(root, corpus string, mode diffMode) runResult {
	return []func(root, corpus string, mode diffMode) runResult{
		runRust,
		runTS,
		runC,
		runLua,
		runJava,
	}
}

// runRust drives the NEW Rust example binary (runtime/rust/examples/diff_runner.rs)
// over the corpus. cargo is required; absent -> skip.
func runRust(root, corpus string, mode diffMode) runResult {
	const name = "Rust"
	if !have("cargo") {
		return runResult{name: name, skipped: "cargo not on PATH"}
	}
	dir := filepath.Join(root, "runtime", "rust")
	err := run(dir, "cargo", "run", "--quiet", "--example", "diff_runner", "--", corpus, string(mode))
	return runResult{name: name, exercised: true, err: err}
}

// runTS drives the NEW Node script (tests/diff/ts_diff_runner.mjs). The TS
// runtime source uses extensionless ESM imports that raw Node cannot resolve, so
// rather than depend on a separate `tsc` dist build we bundle runtime/ts/src
// into a single self-contained ESM module with the project-local esbuild (a
// vitest dependency) and point the runner at it. Requires node + node_modules
// (with esbuild); otherwise this skips cleanly, matching cmd/ci's policy.
func runTS(root, corpus string, mode diffMode) runResult {
	const name = "TypeScript"
	if !have("node") {
		return runResult{name: name, skipped: "node not on PATH"}
	}
	tsDir := filepath.Join(root, "runtime", "ts")
	if _, err := os.Stat(filepath.Join(tsDir, "node_modules")); err != nil {
		return runResult{name: name, skipped: "runtime/ts/node_modules missing (run: npm --prefix runtime/ts install)"}
	}
	esbuild := filepath.Join(tsDir, "node_modules", ".bin", "esbuild")
	if _, err := os.Stat(esbuild); err != nil {
		return runResult{name: name, skipped: "runtime/ts/node_modules/.bin/esbuild missing (run: npm --prefix runtime/ts install)"}
	}

	work, err := os.MkdirTemp("", "xpb_diff_ts")
	if err != nil {
		return runResult{name: name, skipped: "mktemp: " + err.Error()}
	}
	defer os.RemoveAll(work)

	bundle := filepath.Join(work, "xpb_bundle.mjs")
	if err := run("", esbuild, filepath.Join(tsDir, "src", "index.ts"),
		"--bundle", "--format=esm", "--platform=node", "--outfile="+bundle); err != nil {
		return runResult{name: name, exercised: true, err: fmt.Errorf("esbuild bundle: %w", err)}
	}

	script := filepath.Join(root, "tests", "diff", "ts_diff_runner.mjs")
	err = run("", "node", script, corpus, bundle, string(mode))
	return runResult{name: name, exercised: true, err: err}
}

// runC compiles and runs the NEW generic C differential runner
// (tests/diff/xpb_diff_runner.c) under ASan/UBSan. It parses vectors.json at
// runtime so it is not tied to a fixed vector set. Needs a C compiler.
func runC(root, corpus string, mode diffMode) runResult {
	const name = "C"
	cc := os.Getenv("CC")
	if cc == "" {
		for _, cand := range []string{"clang", "cc", "gcc"} {
			if have(cand) {
				cc = cand
				break
			}
		}
	}
	if cc == "" || !have(cc) {
		return runResult{name: name, skipped: "no C compiler on PATH (set $CC or install clang/gcc)"}
	}
	work, err := os.MkdirTemp("", "xpb_diff_c")
	if err != nil {
		return runResult{name: name, skipped: "mktemp: " + err.Error()}
	}
	defer os.RemoveAll(work)

	runtimeC := filepath.Join(root, "runtime", "c", "xpb.c")
	incDir := filepath.Join(root, "runtime", "c", "include")
	src := filepath.Join(root, "tests", "diff", "xpb_diff_runner.c")
	bin := filepath.Join(work, "xpb_diff_runner")
	args := []string{
		"-g", "-O1", "-std=c11", "-Wall", "-Wextra", "-I" + incDir,
		"-fsanitize=address,undefined", "-fno-sanitize-recover=undefined",
		runtimeC, src, "-o", bin,
	}
	if err := run("", cc, args...); err != nil {
		return runResult{name: name, exercised: true, err: fmt.Errorf("compile: %w", err)}
	}
	return runResult{name: name, exercised: true, err: run("", bin, corpus, string(mode))}
}

// runLua drives the NEW corpus-parameterised Lua runner
// (tests/diff/lua_diff_runner.lua). Needs a Lua 5.3+ interpreter.
func runLua(root, corpus string, mode diffMode) runResult {
	const name = "Lua"
	var lua string
	for _, cand := range []string{"lua", "lua5.4", "lua5.3", "lua54", "lua53"} {
		if have(cand) {
			lua = cand
			break
		}
	}
	if lua == "" {
		return runResult{name: name, skipped: "no Lua 5.3+ interpreter on PATH"}
	}
	script := filepath.Join(root, "tests", "diff", "lua_diff_runner.lua")
	err := run("", lua, script, corpus, string(mode))
	return runResult{name: name, exercised: true, err: err}
}

// runJava compiles and runs the NEW xpb.DiffRunner (the committed
// ConformanceTest is byte-identity only and left untouched), passing the corpus
// via -DxpbConformanceDir and the assertion mode via -DxpbDiffMode=bytes|values.
// Needs a JDK.
func runJava(root, corpus string, mode diffMode) runResult {
	const name = "Java"
	if !have("javac") || !have("java") {
		return runResult{name: name, skipped: "no JDK (need javac and java on PATH)"}
	}
	build, err := os.MkdirTemp("", "xpb_diff_java")
	if err != nil {
		return runResult{name: name, skipped: "mktemp: " + err.Error()}
	}
	defer os.RemoveAll(build)

	jbase := filepath.Join(root, "runtime", "java", "src", "main", "java", "xpb")
	if err := run("", "javac", "-d", build,
		filepath.Join(jbase, "Encoder.java"),
		filepath.Join(jbase, "Decoder.java"),
		filepath.Join(root, "tests", "diff", "DiffRunner.java")); err != nil {
		return runResult{name: name, exercised: true, err: fmt.Errorf("compile: %w", err)}
	}
	err = run("", "java", "-cp", build, "-DxpbConformanceDir="+corpus,
		"-DxpbDiffMode="+string(mode), "xpb.DiffRunner")
	return runResult{name: name, exercised: true, err: err}
}

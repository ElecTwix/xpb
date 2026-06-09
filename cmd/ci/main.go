// Command ci is xpb's local CI runner: a single Go entrypoint that runs the
// full multi-language verification suite the way a cloud CI would, but on your
// machine. It replaces the former shell scripts (ci.sh, run_fuzz.sh,
// conformance_runner.sh) so orchestration logic lives in Go — no shell
// portability footguns (macOS bash 3.2 lacks mapfile, set -u crashes on empty
// arrays, word-splitting differs from zsh).
//
// Each toolchain is invoked via os/exec. A step whose toolchain is missing is
// reported SKIP (not a failure); any step that actually runs and fails makes
// the whole run exit non-zero.
//
//	go run ./cmd/ci              # run everything
//	go run ./cmd/ci --install-hook   # run ci on every `git push`
//	go run ./cmd/ci 15           # C fuzz campaign seconds (default 20)
//
// Note: benchmarks/go is excluded — its committed bench.pb.go panics at init
// against the installed protobuf runtime (pre-existing, unrelated).
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ---- colored reporter ----

var useColor = func() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}()

func c(code, s string) string {
	if !useColor {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

type reporter struct{ passed, failed, skipped []string }

func (r *reporter) pass(n string) {
	r.passed = append(r.passed, n)
	fmt.Printf("%s %s\n\n", c("32", "[PASS]"), n)
}
func (r *reporter) fail(n string) {
	r.failed = append(r.failed, n)
	fmt.Printf("%s %s\n\n", c("31", "[FAIL]"), n)
}
func (r *reporter) skip(n, why string) {
	r.skipped = append(r.skipped, n)
	fmt.Printf("%s==>%s %s\n%s %s: %s\n\n", c("1", ""), "", n, c("33", "[SKIP]"), n, why)
}

// step runs bin+args in dir (empty = repo root), streaming output, and records
// the result under name.
func (r *reporter) step(name, dir, bin string, args ...string) {
	fmt.Printf("%s %s\n", c("1", "==>"), name)
	if err := runCmd(dir, bin, args...); err != nil {
		r.fail(name)
	} else {
		r.pass(name)
	}
}

func runCmd(dir, bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func have(bin string) bool { _, err := exec.LookPath(bin); return err == nil }

// repoRoot walks up from cwd to the directory containing go.mod.
func repoRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			fmt.Fprintln(os.Stderr, "ci: could not find go.mod above", dir)
			os.Exit(2)
		}
		dir = parent
	}
}

func main() {
	root := repoRoot()
	if err := os.Chdir(root); err != nil {
		fmt.Fprintln(os.Stderr, "ci:", err)
		os.Exit(2)
	}

	fuzzSecs := 20
	for _, a := range os.Args[1:] {
		if a == "--install-hook" {
			installHook(root)
			return
		}
		if n, err := strconv.Atoi(a); err == nil {
			fuzzSecs = n
		}
	}

	r := &reporter{}

	goSteps(r)
	tsHasDeps := tsBuild(r, root) // build dist BEFORE go test so the integration tsc/bun tests run for real
	if have("go") {
		r.step("go test", "", "go", append([]string{"test", "-count=1"}, goPkgs()...)...)
	}
	if tsHasDeps {
		r.step("ts test (vitest)", filepath.Join(root, "runtime", "ts"),
			filepath.Join("node_modules", ".bin", "vitest"), "run", "src")
	}
	if have("cargo") {
		r.step("cargo test", filepath.Join(root, "runtime", "rust"), "cargo", "test", "--quiet")
	} else {
		r.skip("Rust", "cargo not on PATH")
	}
	cSuite(r, root, fuzzSecs)
	luaConformance(r, root)
	javaConformance(r, root)

	summary(r)
}

// ---- Go ----

// goPkgs returns all module packages except benchmarks (broken upstream).
func goPkgs() []string {
	out, err := exec.Command("go", "list", "./...").Output()
	if err != nil {
		return nil
	}
	var pkgs []string
	for _, p := range strings.Fields(string(out)) {
		if !strings.Contains(p, "/benchmarks") {
			pkgs = append(pkgs, p)
		}
	}
	return pkgs
}

func goSteps(r *reporter) {
	if !have("go") {
		r.skip("Go", "go not on PATH")
		return
	}
	// gofmt: fail if any non-benchmark .go file is unformatted.
	fmt.Printf("%s gofmt\n", c("1", "==>"))
	out, _ := exec.Command("gofmt", "-l", ".").Output()
	var bad []string
	for _, f := range strings.Fields(string(out)) {
		if !strings.HasPrefix(f, "benchmarks/") {
			bad = append(bad, f)
		}
	}
	if len(bad) > 0 {
		fmt.Println("unformatted:", strings.Join(bad, " "))
		r.fail("gofmt")
	} else {
		r.pass("gofmt")
	}

	pkgs := goPkgs()
	r.step("go vet", "", "go", append([]string{"vet"}, pkgs...)...)
	// `go build ./...` (not the explicit list): the wildcard is lenient about
	// test-only packages (tests, tests/integration), which `go build pkg` errors
	// on. Building benchmarks is harmless — only its runtime init panics.
	r.step("go build", "", "go", "build", "./...")
}

// ---- TypeScript ----

// tsBuild builds the TS runtime dist with the project-local tsc. Returns true
// if the toolchain (node_modules + node) is present so the caller can also run
// vitest.
func tsBuild(r *reporter, root string) bool {
	ts := filepath.Join(root, "runtime", "ts")
	if _, err := os.Stat(filepath.Join(ts, "node_modules")); err != nil || !have("node") {
		r.skip("TypeScript", "runtime/ts/node_modules missing or node not on PATH (run: npm --prefix runtime/ts install)")
		return false
	}
	r.step("ts build (tsc)", ts, filepath.Join("node_modules", ".bin", "tsc"))
	return true
}

// ---- C safety suite (ports run_fuzz.sh) ----

func cSuite(r *reporter, root string, fuzzSecs int) {
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
		r.skip("C suite", "no C compiler on PATH (set $CC or install clang/gcc)")
		return
	}

	runtimeC := filepath.Join(root, "runtime", "c", "xpb.c")
	incDir := filepath.Join(root, "runtime", "c", "include")
	dataDir := filepath.Join(root, "testdata", "conformance")
	tc := filepath.Join(root, "tests", "c")
	common := []string{"-g", "-O1", "-std=c11", "-Wall", "-Wextra", "-I" + incDir}
	san := []string{"-fsanitize=address,undefined", "-fno-sanitize-recover=undefined"}

	work, err := os.MkdirTemp("", "xpb_ci_c")
	if err != nil {
		r.skip("C suite", "mktemp: "+err.Error())
		return
	}
	defer os.RemoveAll(work)

	build := func(out string, flags, srcs []string) error {
		args := append([]string{}, common...)
		args = append(args, flags...)
		args = append(args, srcs...)
		args = append(args, "-o", filepath.Join(work, out))
		return runCmd("", cc, args...)
	}

	// 1. libFuzzer campaign (best effort; Apple Clang ships no fuzzer runtime).
	probe := filepath.Join(work, "probe.c")
	os.WriteFile(probe, []byte("#include <stdint.h>\n#include <stddef.h>\nint LLVMFuzzerTestOneInput(const uint8_t*d,size_t n){(void)d;(void)n;return 0;}\n"), 0o644)
	fuzzerOK := runCmd("", cc, "-g", "-O1", "-fsanitize=fuzzer,address,undefined", probe, "-o", filepath.Join(work, "probe")) == nil
	if fuzzerOK {
		if build("xpb_fuzz", []string{"-fsanitize=fuzzer,address,undefined"}, []string{runtimeC, filepath.Join(tc, "xpb_fuzz.c")}) == nil {
			corpus := filepath.Join(work, "corpus")
			os.MkdirAll(corpus, 0o755)
			copyBins(dataDir, corpus)
			r.step(fmt.Sprintf("c: libFuzzer (%ds)", fuzzSecs), "", filepath.Join(work, "xpb_fuzz"),
				"-max_total_time="+strconv.Itoa(fuzzSecs), "-rss_limit_mb=2048", "-print_final_stats=1", corpus)
		} else {
			r.fail("c: libFuzzer build")
		}
	} else {
		r.skip("c: libFuzzer", "clang libFuzzer runtime unavailable (Apple Clang ships none; use Homebrew LLVM or CI)")
	}

	// 2. Standalone ASan/UBSan replay of the fuzz harness (always).
	if build("xpb_fuzz_std", append([]string{"-DXPB_FUZZ_STANDALONE"}, san...), []string{runtimeC, filepath.Join(tc, "xpb_fuzz.c")}) == nil {
		r.step("c: asan/ubsan fuzz replay", "", filepath.Join(work, "xpb_fuzz_std"), bins(dataDir)...)
	} else {
		r.fail("c: asan fuzz replay build")
	}

	// 3. Existing tests under ASan/UBSan.
	for _, t := range []struct{ name, src string }{
		{"c: xpb_test (asan)", "xpb_test.c"},
		{"c: xpb_security_test (asan)", "xpb_security_test.c"},
	} {
		flags := append([]string{"-lm"}, san...)
		if build(t.name, flags, []string{runtimeC, filepath.Join(tc, t.src)}) == nil {
			r.step(t.name, "", filepath.Join(work, t.name))
		} else {
			r.fail(t.name + " build")
		}
	}

	// 4. Conformance harness under ASan/UBSan.
	if build("xpb_conformance", san, []string{runtimeC, filepath.Join(tc, "xpb_conformance.c")}) == nil {
		r.step("c: conformance (asan)", "", filepath.Join(work, "xpb_conformance"), dataDir)
	} else {
		r.fail("c: conformance build")
	}
}

func bins(dir string) []string {
	m, _ := filepath.Glob(filepath.Join(dir, "*.bin"))
	return m
}

func copyBins(src, dst string) {
	for _, f := range bins(src) {
		if b, err := os.ReadFile(f); err == nil {
			os.WriteFile(filepath.Join(dst, filepath.Base(f)), b, 0o644)
		}
	}
}

// ---- Lua / Java conformance (ports conformance_runner.sh) ----

func luaConformance(r *reporter, root string) {
	var lua string
	for _, cand := range []string{"lua", "lua5.4", "lua5.3", "lua54", "lua53"} {
		if have(cand) {
			lua = cand
			break
		}
	}
	if lua == "" {
		r.skip("Lua conformance", "no Lua 5.3+ interpreter (LuaJIT unsupported: xpb.lua needs 5.3+ features)")
		return
	}
	r.step("Lua conformance", "", lua, filepath.Join(root, "tests", "lua", "conformance.lua"))
}

func javaConformance(r *reporter, root string) {
	if !have("javac") || !have("java") {
		r.skip("Java conformance", "no JDK (need javac and java on PATH)")
		return
	}
	build, err := os.MkdirTemp("", "xpb_ci_java")
	if err != nil {
		r.skip("Java conformance", "mktemp: "+err.Error())
		return
	}
	defer os.RemoveAll(build)

	jbase := filepath.Join(root, "runtime", "java", "src", "main", "java", "xpb")
	dataDir := filepath.Join(root, "testdata", "conformance")
	fmt.Printf("%s Java conformance\n", c("1", "==>"))
	if err := runCmd("", "javac", "-d", build,
		filepath.Join(jbase, "Encoder.java"), filepath.Join(jbase, "Decoder.java"),
		filepath.Join(root, "tests", "java", "ConformanceTest.java")); err != nil {
		r.fail("Java conformance (compile)")
		return
	}
	if err := runCmd("", "java", "-cp", build, "-DxpbConformanceDir="+dataDir, "xpb.ConformanceTest"); err != nil {
		r.fail("Java conformance")
	} else {
		r.pass("Java conformance")
	}
}

// ---- hook install + summary ----

func installHook(root string) {
	if err := runCmd(root, "git", "config", "core.hooksPath", "scripts/hooks"); err != nil {
		fmt.Fprintln(os.Stderr, "ci:", err)
		os.Exit(1)
	}
	fmt.Printf("%s git pushes now run `go run ./cmd/ci` (core.hooksPath=scripts/hooks)\n", c("32", "[ok]"))
}

func summary(r *reporter) {
	fmt.Println(c("1", "================ CI SUMMARY ================"))
	fmt.Printf("%s: %d   %s: %d   %s: %d\n",
		c("32", "passed"), len(r.passed), c("33", "skipped"), len(r.skipped), c("31", "failed"), len(r.failed))
	if len(r.skipped) > 0 {
		fmt.Printf("%s %s\n", c("33", "skipped:"), strings.Join(r.skipped, ", "))
	}
	fmt.Printf("%s benchmarks/go excluded (pre-existing protobuf init panic).\n", c("33", "NOTE:"))
	if len(r.failed) > 0 {
		fmt.Printf("%s %s\n", c("31", "FAILED:"), strings.Join(r.failed, ", "))
		os.Exit(1)
	}
	fmt.Println(c("32", "ALL GREEN"))
}

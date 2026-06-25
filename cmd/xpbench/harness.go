package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// The per-runtime benchmark harnesses are committed under cmd/xpbench/harness/
// and embedded here. At run time the driver materializes the one it needs into
// a temp dir, compiles it where necessary, and runs it against the shared
// corpus. Each harness parses the same vectors.json + .bin corpus the Go
// reference encoder wrote, re-encodes the ops and decodes the bytes in timed
// loops (iteration count per vector taken from the manifest), and prints a JSON
// array of {name, encodeNs, decodeNs, wireSize} to stdout. They are the timed
// analogues of the proven differential runners in tests/diff/*.

//go:embed harness/rust_bench.rs
var rustBenchSrc string

//go:embed harness/c_bench.c
var cBenchSrc string

//go:embed harness/lua_bench.lua
var luaBenchSrc string

//go:embed harness/ts_bench.mjs
var tsBenchSrc string

//go:embed harness/JavaBench.java
var javaBenchSrc string

// writeTemp writes content to dir/name and returns the full path.
func writeTemp(dir, name, content string) (string, error) {
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return "", err
	}
	return p, nil
}

// runRust builds a throwaway cargo project with a path dependency on the
// repo's Rust runtime crate (no registry/network deps -- the harness hand-rolls
// its JSON parsing so it needs nothing beyond `xpb`) and runs it over the
// corpus. cargo absent -> clean skip.
func runRust(root, corpus string, _ []shapeMeta) ([]Row, runtimeStatus) {
	const name = "Rust"
	if !have("cargo") {
		return skipResult(name, "cargo not on PATH")
	}
	work, err := os.MkdirTemp("", "xpbench_rust")
	if err != nil {
		return skipResult(name, "mktemp: "+err.Error())
	}
	defer os.RemoveAll(work)

	crate := filepath.Join(root, "runtime", "rust")
	cargoToml := fmt.Sprintf(`[package]
name = "xpbench_rust"
version = "0.0.0"
edition = "2021"

[[bin]]
name = "xpbench_rust"
path = "src/main.rs"

[dependencies]
xpb = { path = %q }
`, crate)
	if _, err := writeTemp(work, "Cargo.toml", cargoToml); err != nil {
		return errorResult(name, "write Cargo.toml: "+err.Error())
	}
	if err := os.MkdirAll(filepath.Join(work, "src"), 0o755); err != nil {
		return errorResult(name, "mkdir src: "+err.Error())
	}
	if _, err := writeTemp(filepath.Join(work, "src"), "main.rs", rustBenchSrc); err != nil {
		return errorResult(name, "write main.rs: "+err.Error())
	}

	cmd := exec.Command("cargo", "run", "--quiet", "--release", "--offline",
		"--manifest-path", filepath.Join(work, "Cargo.toml"), "--", corpus)
	return runExternalHarness(name, cmd)
}

// runC compiles the C harness together with the C runtime at -O2 (a benchmark
// build -- no sanitizers) and runs it over the corpus. No C compiler -> skip.
func runC(root, corpus string, _ []shapeMeta) ([]Row, runtimeStatus) {
	const name = "C"
	cc := firstAvailable(os.Getenv("CC"), "clang", "cc", "gcc")
	if cc == "" {
		return skipResult(name, "no C compiler ($CC/clang/cc/gcc)")
	}
	work, err := os.MkdirTemp("", "xpbench_c")
	if err != nil {
		return skipResult(name, "mktemp: "+err.Error())
	}
	defer os.RemoveAll(work)

	src, err := writeTemp(work, "c_bench.c", cBenchSrc)
	if err != nil {
		return errorResult(name, "write c_bench.c: "+err.Error())
	}
	bin := filepath.Join(work, "c_bench")
	runtimeC := filepath.Join(root, "runtime", "c", "xpb.c")
	inc := filepath.Join(root, "runtime", "c", "include")
	build := exec.Command(cc, "-O2", "-std=c11", "-Wall", "-I"+inc, runtimeC, src, "-lm", "-o", bin)
	if out, berr := build.CombinedOutput(); berr != nil {
		return errorResult(name, fmt.Sprintf("compile: %v: %s", berr, truncate(string(out), 300)))
	}
	return runExternalHarness(name, exec.Command(bin, corpus))
}

// runLua runs the Lua harness, pointing it at the corpus and the runtime/lua
// module dir. Needs a Lua 5.3+ interpreter (LuaJIT is unsupported by the
// runtime). Absent -> skip.
func runLua(root, corpus string, _ []shapeMeta) ([]Row, runtimeStatus) {
	const name = "Lua"
	lua := firstAvailable("lua", "lua5.4", "lua5.3", "lua54", "lua53")
	if lua == "" {
		return skipResult(name, "no Lua 5.3+ interpreter on PATH")
	}
	work, err := os.MkdirTemp("", "xpbench_lua")
	if err != nil {
		return skipResult(name, "mktemp: "+err.Error())
	}
	defer os.RemoveAll(work)

	script, err := writeTemp(work, "lua_bench.lua", luaBenchSrc)
	if err != nil {
		return errorResult(name, "write lua_bench.lua: "+err.Error())
	}
	luaDir := filepath.Join(root, "runtime", "lua")
	return runExternalHarness(name, exec.Command(lua, script, corpus, luaDir))
}

// runTS bundles the TS runtime with the project-local esbuild (a vitest dep)
// and runs the Node harness against the bundle. Mirrors cmd/ci / the
// differential runner: needs node + runtime/ts/node_modules/.bin/esbuild;
// otherwise skips cleanly.
func runTS(root, corpus string, _ []shapeMeta) ([]Row, runtimeStatus) {
	const name = "TypeScript"
	if !have("node") {
		return skipResult(name, "node not on PATH")
	}
	tsDir := filepath.Join(root, "runtime", "ts")
	esbuild := filepath.Join(tsDir, "node_modules", ".bin", "esbuild")
	if _, err := os.Stat(esbuild); err != nil {
		return skipResult(name, "runtime/ts/node_modules/.bin/esbuild missing (run: npm --prefix runtime/ts install)")
	}
	work, err := os.MkdirTemp("", "xpbench_ts")
	if err != nil {
		return skipResult(name, "mktemp: "+err.Error())
	}
	defer os.RemoveAll(work)

	bundle := filepath.Join(work, "xpb_bundle.mjs")
	build := exec.Command(esbuild, filepath.Join(tsDir, "src", "index.ts"),
		"--bundle", "--format=esm", "--platform=node", "--outfile="+bundle)
	if out, berr := build.CombinedOutput(); berr != nil {
		return errorResult(name, fmt.Sprintf("esbuild bundle: %v: %s", berr, truncate(string(out), 300)))
	}
	script, err := writeTemp(work, "ts_bench.mjs", tsBenchSrc)
	if err != nil {
		return errorResult(name, "write ts_bench.mjs: "+err.Error())
	}
	return runExternalHarness(name, exec.Command("node", script, corpus, bundle))
}

// runJava compiles the Java harness together with the runtime Encoder/Decoder
// and runs it over the corpus (passed via -DxpbCorpusDir). Needs a JDK
// (javac + java). Absent -> skip.
func runJava(root, corpus string, _ []shapeMeta) ([]Row, runtimeStatus) {
	const name = "Java"
	if !have("javac") || !have("java") {
		return skipResult(name, "no JDK (need javac and java on PATH)")
	}
	work, err := os.MkdirTemp("", "xpbench_java")
	if err != nil {
		return skipResult(name, "mktemp: "+err.Error())
	}
	defer os.RemoveAll(work)

	src, err := writeTemp(work, "JavaBench.java", javaBenchSrc)
	if err != nil {
		return errorResult(name, "write JavaBench.java: "+err.Error())
	}
	build := filepath.Join(work, "classes")
	if err := os.MkdirAll(build, 0o755); err != nil {
		return errorResult(name, "mkdir classes: "+err.Error())
	}
	jbase := filepath.Join(root, "runtime", "java", "src", "main", "java", "xpb")
	compile := exec.Command("javac", "-d", build,
		filepath.Join(jbase, "Encoder.java"),
		filepath.Join(jbase, "Decoder.java"),
		src)
	if out, berr := compile.CombinedOutput(); berr != nil {
		return errorResult(name, fmt.Sprintf("javac: %v: %s", berr, truncate(string(out), 300)))
	}
	run := exec.Command("java", "-cp", build, "-DxpbCorpusDir="+corpus, "xpb.JavaBench")
	return runExternalHarness(name, run)
}

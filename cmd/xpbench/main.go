// Command xpbench is the XPB cross-runtime benchmark runner. It encodes a fixed
// set of canonical message shapes with the Go reference encoder and drives every
// AVAILABLE language runtime (Go, Rust, TypeScript, C, Lua, Java) over the SAME
// shapes, emitting one normalized table so encode/decode cost is directly
// comparable across runtimes:
//
//	runtime | shape | wire size | encode ns/op + MB/s | decode ns/op + MB/s | allocs/op (Go)
//
// Each non-Go runtime is gated on toolchain availability and SKIPPED cleanly if
// its toolchain is absent (mirroring cmd/ci) -- a missing toolchain is never a
// hard failure. Output is human-readable by default and machine-readable on
// request (--format json|csv, optionally --out FILE).
//
//	go run ./cmd/xpbench                      # human table for locally-available runtimes
//	go run ./cmd/xpbench --format json        # machine-readable JSON to stdout
//	go run ./cmd/xpbench --format csv --out r.csv   # CSV to file + human table on stdout
//	go run ./cmd/xpbench --runtimes go,rust   # restrict to a subset
//
// Note: this tool benchmarks only the XPB codec ACROSS runtimes; it deliberately
// does NOT touch the pre-existing-broken root benchmarks/go package.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// namedRunner pairs a runtime's display name with its driver.
type namedRunner struct {
	name string
	fn   func(root, corpus string, metas []shapeMeta) ([]Row, runtimeStatus)
}

// allRunners returns every runtime driver in display order. Go is in-process;
// the rest shell out to a per-runtime harness gated on toolchain availability.
func allRunners() []namedRunner {
	return []namedRunner{
		{"Go", runGo},
		{"Rust", runRust},
		{"TypeScript", runTS},
		{"C", runC},
		{"Lua", runLua},
		{"Java", runJava},
	}
}

// selectRunners filters allRunners by a comma-separated name list. An empty
// list or "all" selects everything; unknown names are ignored.
func selectRunners(spec string) []namedRunner {
	spec = strings.TrimSpace(spec)
	if spec == "" || strings.EqualFold(spec, "all") {
		return allRunners()
	}
	want := map[string]bool{}
	for _, n := range strings.Split(spec, ",") {
		want[strings.ToLower(strings.TrimSpace(n))] = true
	}
	var out []namedRunner
	for _, r := range allRunners() {
		if want[strings.ToLower(r.name)] {
			out = append(out, r)
		}
	}
	return out
}

// collectRows runs each selected runtime over the corpus and returns the
// aggregated rows plus the per-runtime statuses. Progress is written to
// progress (pass io.Discard to silence it).
func collectRows(root, corpus string, metas []shapeMeta, runners []namedRunner, progress io.Writer) ([]Row, []runtimeStatus) {
	var rows []Row
	statuses := make([]runtimeStatus, 0, len(runners))
	for _, r := range runners {
		fmt.Fprintf(progress, "running %s...\n", r.name)
		rr, st := r.fn(root, corpus, metas)
		rows = append(rows, rr...)
		statuses = append(statuses, st)
	}
	return rows, statuses
}

// runBenchmark writes the shared corpus to a temp dir and drives the selected
// runtimes over it. It is the testable core of the command.
func runBenchmark(root string, shapes []shape, runners []namedRunner, progress io.Writer) ([]Row, []runtimeStatus, []shapeMeta, error) {
	dir, err := os.MkdirTemp("", "xpbench_corpus")
	if err != nil {
		return nil, nil, nil, err
	}
	defer os.RemoveAll(dir)

	metas, err := writeCorpus(dir, shapes)
	if err != nil {
		return nil, nil, nil, err
	}
	rows, statuses := collectRows(root, dir, metas, runners, progress)
	return rows, statuses, metas, nil
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "xpbench:", err)
		os.Exit(1)
	}
}

// run is main minus os.Exit, so it is unit-testable. It parses flags, drives the
// benchmark, and renders the requested output.
func run(args []string, stdout, stderr io.Writer) error {
	var (
		format   = "table"
		outPath  = ""
		runtimes = "all"
	)
	fs := newFlagSet(&format, &outPath, &runtimes)
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch format {
	case "table", "json", "csv":
	default:
		return fmt.Errorf("unknown --format %q (want table|json|csv)", format)
	}

	root, err := repoRoot()
	if err != nil {
		return err
	}
	runners := selectRunners(runtimes)
	if len(runners) == 0 {
		return fmt.Errorf("no runtimes selected by %q", runtimes)
	}

	rows, statuses, metas, err := runBenchmark(root, canonicalShapes(), runners, stderr)
	if err != nil {
		return err
	}
	order := shapeOrderFromMeta(metas)

	if err := render(rows, order, format, outPath, stdout); err != nil {
		return err
	}
	writeSummary(stderr, statuses)
	return nil
}

// render writes the rows in the requested format. When --out is set, the chosen
// format is written to that file AND the human table is also printed to stdout
// (so one run yields both a saved machine artifact and an on-screen view).
func render(rows []Row, order map[string]int, format, outPath string, stdout io.Writer) error {
	if outPath != "" {
		f, err := os.Create(outPath)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := emit(f, rows, order, format); err != nil {
			return err
		}
		writeTable(stdout, rows, order)
		return nil
	}
	return emit(stdout, rows, order, format)
}

// emit writes rows to w in the given format.
func emit(w io.Writer, rows []Row, order map[string]int, format string) error {
	switch format {
	case "json":
		return writeJSON(w, rows, order)
	case "csv":
		return writeCSV(w, rows, order)
	default:
		writeTable(w, rows, order)
		return nil
	}
}

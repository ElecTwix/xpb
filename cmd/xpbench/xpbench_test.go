package main

import (
	"bytes"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ElecTwix/xpb/tests/conformance"
)

// --- shapes + corpus -------------------------------------------------------

// TestCanonicalShapesEncode proves the shared shape set is non-empty, uniquely
// named, and that every shape encodes to a non-empty byte string via the Go
// reference encoder (these are the SAME bytes every runtime later decodes).
func TestCanonicalShapesEncode(t *testing.T) {
	shapes := canonicalShapes()
	if len(shapes) < 5 {
		t.Fatalf("want a meaningful spread of shapes, got %d", len(shapes))
	}
	seen := map[string]bool{}
	for _, s := range shapes {
		if s.name == "" {
			t.Error("shape with empty name")
		}
		if seen[s.name] {
			t.Errorf("duplicate shape name %q", s.name)
		}
		seen[s.name] = true
		if got := conformance.Encode(s.ops); len(got) == 0 {
			t.Errorf("shape %q encoded to zero bytes", s.name)
		}
	}
}

// TestWriteCorpusManifest proves the corpus writer materializes a manifest plus
// one .bin per shape, that each .bin's length matches the reported wire size,
// and that every vector carries a usable iteration count. The .bin bytes are the
// cross-runtime input, so this is the contract every harness consumes.
func TestWriteCorpusManifest(t *testing.T) {
	dir := t.TempDir()
	shapes := canonicalShapes()
	metas, err := writeCorpus(dir, shapes)
	if err != nil {
		t.Fatalf("writeCorpus: %v", err)
	}
	if len(metas) != len(shapes) {
		t.Fatalf("metas %d != shapes %d", len(metas), len(shapes))
	}

	raw, err := os.ReadFile(filepath.Join(dir, "vectors.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var man benchManifest
	if err := json.Unmarshal(raw, &man); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if len(man.Vectors) != len(shapes) {
		t.Fatalf("manifest vectors %d != shapes %d", len(man.Vectors), len(shapes))
	}

	for i, m := range metas {
		if m.Iters < 1 {
			t.Errorf("shape %q: iters %d < 1", m.Name, m.Iters)
		}
		binPath := filepath.Join(dir, man.Vectors[i].File)
		b, err := os.ReadFile(binPath)
		if err != nil {
			t.Fatalf("read %s: %v", binPath, err)
		}
		if len(b) != m.WireSize {
			t.Errorf("shape %q: .bin len %d != wireSize %d", m.Name, len(b), m.WireSize)
		}
		// The manifest hex must decode to exactly the .bin bytes (so a harness
		// can use either representation interchangeably).
		decoded, err := hex.DecodeString(man.Vectors[i].Hex)
		if err != nil {
			t.Errorf("shape %q: manifest hex not decodable: %v", m.Name, err)
		} else if !bytes.Equal(decoded, b) {
			t.Errorf("shape %q: manifest hex != .bin bytes", m.Name)
		}
	}
}

// TestItersForClamp proves the per-shape iteration heuristic stays within its
// documented clamp so a full cross-runtime run is bounded.
func TestItersForClamp(t *testing.T) {
	cases := []struct{ wire, min, max int }{
		{1, 5_000, 200_000},
		{40, 5_000, 200_000},
		{2_000, 5_000, 200_000},
		{10_000_000, 5_000, 200_000},
	}
	for _, c := range cases {
		got := itersFor(c.wire)
		if got < c.min || got > c.max {
			t.Errorf("itersFor(%d)=%d out of [%d,%d]", c.wire, got, c.min, c.max)
		}
	}
}

// --- normalization ---------------------------------------------------------

// TestMBpsNormalization proves units are normalized so rows are comparable, and
// that a non-positive time yields 0 rather than Inf/NaN.
func TestMBpsNormalization(t *testing.T) {
	// 1000 bytes in 1000 ns == 1 byte/ns == 1000 MB/s.
	if got := mbps(1000, 1000); got != 1000 {
		t.Errorf("mbps(1000,1000)=%v want 1000", got)
	}
	if got := mbps(100, 0); got != 0 {
		t.Errorf("mbps with zero ns=%v want 0", got)
	}
	if got := mbps(100, -5); got != 0 {
		t.Errorf("mbps with negative ns=%v want 0", got)
	}
}

// --- Go runner -------------------------------------------------------------

// TestRunGoRows proves the in-process Go runner produces one comparable row per
// shape, reports it as exercised, normalizes MB/s consistently with ns/op, and —
// uniquely among runtimes — reports allocations per op (not the AllocsNA
// sentinel).
func TestRunGoRows(t *testing.T) {
	shapes := canonicalShapes()
	dir := t.TempDir()
	metas, err := writeCorpus(dir, shapes)
	if err != nil {
		t.Fatalf("writeCorpus: %v", err)
	}
	rows, st := runGo("", dir, metas)
	if st.state != stateExercised {
		t.Fatalf("Go state = %q want exercised", st.state)
	}
	if len(rows) != len(shapes) {
		t.Fatalf("Go rows %d != shapes %d", len(rows), len(shapes))
	}
	for _, r := range rows {
		if r.Runtime != "Go" || r.Skipped {
			t.Errorf("unexpected row %+v", r)
		}
		if r.WireSize <= 0 {
			t.Errorf("shape %q: non-positive wire size %d", r.Shape, r.WireSize)
		}
		if r.EncodeNsPerOp <= 0 || r.DecodeNsPerOp <= 0 {
			t.Errorf("shape %q: non-positive ns/op enc=%v dec=%v", r.Shape, r.EncodeNsPerOp, r.DecodeNsPerOp)
		}
		// MB/s must be the exact normalization of the measured ns/op.
		if want := mbps(r.WireSize, r.EncodeNsPerOp); want != r.EncodeMBps {
			t.Errorf("shape %q: EncodeMBps=%v want %v", r.Shape, r.EncodeMBps, want)
		}
		if want := mbps(r.WireSize, r.DecodeNsPerOp); want != r.DecodeMBps {
			t.Errorf("shape %q: DecodeMBps=%v want %v", r.Shape, r.DecodeMBps, want)
		}
		// Go is the only runtime that reports allocs.
		if r.EncodeAllocsPerOp == AllocsNA || r.DecodeAllocsPerOp == AllocsNA {
			t.Errorf("shape %q: Go must report allocs, got enc=%v dec=%v", r.Shape, r.EncodeAllocsPerOp, r.DecodeAllocsPerOp)
		}
	}
}

// --- runtime selection -----------------------------------------------------

// TestSelectRunners proves the --runtimes filter: all/empty select everything, a
// subset selects exactly that subset (case-insensitive), and unknown names are
// ignored rather than fatal.
func TestSelectRunners(t *testing.T) {
	if got := len(selectRunners("all")); got != 6 {
		t.Errorf("selectRunners(all)=%d want 6", got)
	}
	if got := len(selectRunners("")); got != 6 {
		t.Errorf("selectRunners(empty)=%d want 6", got)
	}
	sub := selectRunners("go,RUST,nope")
	if len(sub) != 2 {
		t.Fatalf("selectRunners subset=%d want 2", len(sub))
	}
	names := sub[0].name + "," + sub[1].name
	if names != "Go,Rust" {
		t.Errorf("subset order/names=%q want Go,Rust", names)
	}
}

// --- skip / error reporting (toolchain gating) -----------------------------

// TestCollectRowsSkipAndError proves a runtime whose toolchain is absent
// contributes exactly one clearly-marked skip row (never failing the run), an
// errored runtime is classified distinctly, and an exercised runtime passes its
// rows through — i.e. the engine that drives "skip cleanly if absent" works
// deterministically, independent of which toolchains the host actually has.
func TestCollectRowsSkipAndError(t *testing.T) {
	metas := []shapeMeta{{Name: "scalars", WireSize: 10, Iters: 1}}
	runners := []namedRunner{
		{"Always", func(_, _ string, _ []shapeMeta) ([]Row, runtimeStatus) {
			return []Row{{Runtime: "Always", Shape: "scalars", WireSize: 10, EncodeNsPerOp: 1, DecodeNsPerOp: 1}},
				runtimeStatus{name: "Always", state: stateExercised}
		}},
		{"Gone", func(_, _ string, _ []shapeMeta) ([]Row, runtimeStatus) {
			return skipResult("Gone", "toolchain not on PATH")
		}},
		{"Broke", func(_, _ string, _ []shapeMeta) ([]Row, runtimeStatus) {
			return errorResult("Broke", "harness blew up")
		}},
	}
	rows, statuses := collectRows("", "", metas, runners, io.Discard)

	byState := map[runtimeState]runtimeStatus{}
	for _, s := range statuses {
		byState[s.state] = s
	}
	if byState[stateExercised].name != "Always" {
		t.Errorf("exercised = %q want Always", byState[stateExercised].name)
	}
	if byState[stateSkipped].name != "Gone" {
		t.Errorf("skipped = %q want Gone", byState[stateSkipped].name)
	}
	if byState[stateError].name != "Broke" {
		t.Errorf("errored = %q want Broke", byState[stateError].name)
	}

	var skipRows, dataRows int
	for _, r := range rows {
		if r.Skipped {
			skipRows++
			if r.SkipReason == "" {
				t.Error("skipped row missing reason")
			}
		} else {
			dataRows++
		}
	}
	if dataRows != 1 {
		t.Errorf("data rows = %d want 1", dataRows)
	}
	if skipRows != 2 { // Gone (skip) + Broke (error) each emit one info row
		t.Errorf("info rows = %d want 2", skipRows)
	}
}

// --- rendering: human + machine-readable -----------------------------------

func sampleRows() []Row {
	return []Row{
		{Runtime: "Go", Shape: "scalars", WireSize: 56, EncodeNsPerOp: 200, EncodeMBps: mbps(56, 200),
			DecodeNsPerOp: 500, DecodeMBps: mbps(56, 500), EncodeAllocsPerOp: 2, DecodeAllocsPerOp: 12},
		{Runtime: "Rust", Shape: "scalars", WireSize: 56, EncodeNsPerOp: 240, EncodeMBps: mbps(56, 240),
			DecodeNsPerOp: 130, DecodeMBps: mbps(56, 130), EncodeAllocsPerOp: AllocsNA, DecodeAllocsPerOp: AllocsNA},
		{Runtime: "TypeScript", Shape: skipShape, Skipped: true, SkipReason: "esbuild missing",
			EncodeAllocsPerOp: AllocsNA, DecodeAllocsPerOp: AllocsNA},
	}
}

// TestWriteTableHuman proves the human-readable table carries the required
// columns and clearly marks a skipped runtime without distorting the numeric
// columns (the skip reason is trailing, so a long reason cannot widen ENC ns/op).
func TestWriteTableHuman(t *testing.T) {
	var buf bytes.Buffer
	order := map[string]int{"scalars": 0, skipShape: 1}
	writeTable(&buf, sampleRows(), order)
	out := buf.String()
	for _, want := range []string{"RUNTIME", "ENC ns/op", "ENC MB/s", "DEC ns/op", "DEC MB/s", "ALLOCS/op", "WIRE(B)"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing header %q\n%s", want, out)
		}
	}
	if !strings.Contains(out, "SKIPPED: esbuild missing") {
		t.Errorf("table does not mark the skipped runtime\n%s", out)
	}
	// Go's allocs appear; the non-Go runtime (Rust) must show the "-" sentinel,
	// NOT a misleading "0.0" (AC6: not-measured is distinct from a real zero).
	if !strings.Contains(out, "2.0 / 12.0") {
		t.Errorf("table missing Go allocs\n%s", out)
	}
	var rustLine string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Rust ") {
			rustLine = line
		}
	}
	if rustLine == "" {
		t.Fatalf("no Rust data row in table\n%s", out)
	}
	if !strings.HasSuffix(strings.TrimRight(rustLine, " "), "-") {
		t.Errorf("Rust allocs should render as the \"-\" sentinel: %q", rustLine)
	}
	if strings.Contains(rustLine, "0.0 / 0.0") {
		t.Errorf("Rust allocs must not render as 0.0/0.0 (not-measured != zero): %q", rustLine)
	}
	// No data line may be absurdly wide (regression guard for the tabwriter
	// distortion that a tab-terminated skip reason used to cause).
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Go ") && len(line) > 120 {
			t.Errorf("data row unexpectedly wide (%d cols): %q", len(line), line)
		}
	}
}

// TestWriteJSONMachine proves the JSON form round-trips into []Row with every
// metric preserved and skip state distinguishable.
func TestWriteJSONMachine(t *testing.T) {
	var buf bytes.Buffer
	order := map[string]int{"scalars": 0, skipShape: 1}
	if err := writeJSON(&buf, sampleRows(), order); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	var got []Row
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal JSON: %v\n%s", err, buf.String())
	}
	if len(got) != 3 {
		t.Fatalf("json rows = %d want 3", len(got))
	}
	var sawSkip, sawGo, sawRust bool
	for _, r := range got {
		if r.Runtime == "TypeScript" {
			sawSkip = true
			if !r.Skipped || r.SkipReason == "" {
				t.Errorf("TypeScript row not marked skipped: %+v", r)
			}
		}
		if r.Runtime == "Go" {
			sawGo = true
			if r.EncodeMBps != mbps(r.WireSize, r.EncodeNsPerOp) {
				t.Errorf("Go MB/s not preserved: %+v", r)
			}
			if r.EncodeAllocsPerOp == AllocsNA {
				t.Errorf("Go must carry real allocs in JSON, got sentinel: %+v", r)
			}
		}
		if r.Runtime == "Rust" {
			sawRust = true
			// AC6: a non-Go runtime's allocs must round-trip as the AllocsNA
			// sentinel (-1), distinct from a genuine 0.
			if r.EncodeAllocsPerOp != AllocsNA || r.DecodeAllocsPerOp != AllocsNA {
				t.Errorf("Rust allocs must be AllocsNA in JSON, got enc=%v dec=%v", r.EncodeAllocsPerOp, r.DecodeAllocsPerOp)
			}
			if r.EncodeAllocsPerOp == 0 {
				t.Errorf("Rust allocs sentinel must not be 0: %+v", r)
			}
		}
	}
	if !sawSkip || !sawGo || !sawRust {
		t.Errorf("json missing rows: sawSkip=%v sawGo=%v sawRust=%v", sawSkip, sawGo, sawRust)
	}
}

// TestWriteCSVMachine proves the CSV form has a stable header, one record per
// row, blank metric cells for a skipped runtime, and the reason preserved.
func TestWriteCSVMachine(t *testing.T) {
	var buf bytes.Buffer
	order := map[string]int{"scalars": 0, skipShape: 1}
	if err := writeCSV(&buf, sampleRows(), order); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}
	recs, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	if len(recs) != 4 { // header + 3 rows
		t.Fatalf("csv records = %d want 4", len(recs))
	}
	if recs[0][0] != "runtime" || recs[0][len(recs[0])-1] != "skipReason" {
		t.Errorf("unexpected CSV header: %v", recs[0])
	}
	// Derive column indices from the header by name so the AC6 check stays bound
	// to the columns it claims to verify even if columns are reordered/inserted.
	colOf := func(name string) int {
		for i, h := range recs[0] {
			if h == name {
				return i
			}
		}
		t.Fatalf("CSV header missing column %q: %v", name, recs[0])
		return -1
	}
	colEncNs := colOf("encodeNsPerOp")
	colEncAllocs := colOf("encodeAllocsPerOp")
	colDecAllocs := colOf("decodeAllocsPerOp")
	// Find the skipped row and assert its metric cells are blank but reason set.
	var foundSkip, foundRust bool
	for _, rec := range recs[1:] {
		switch rec[0] {
		case "TypeScript":
			foundSkip = true
			if rec[colEncNs] != "" {
				t.Errorf("skipped row should have blank metrics: %v", rec)
			}
			if rec[len(rec)-1] == "" {
				t.Errorf("skipped row should carry a reason: %v", rec)
			}
		case "Rust":
			foundRust = true
			// AC6: a non-Go EXERCISED runtime has real timings but BLANK alloc
			// cells (the not-measured sentinel renders empty, not "0").
			if rec[colEncNs] == "" {
				t.Errorf("exercised Rust row should have a real encode timing: %v", rec)
			}
			if rec[colEncAllocs] != "" || rec[colDecAllocs] != "" {
				t.Errorf("exercised non-Go row must have blank alloc cells (not 0): %v", rec)
			}
		}
	}
	if !foundSkip {
		t.Error("skipped runtime missing from CSV")
	}
	if !foundRust {
		t.Error("exercised Rust row missing from CSV")
	}
}

// TestWriteSummary proves the exercised-vs-skipped report the ticket requires.
func TestWriteSummary(t *testing.T) {
	var buf bytes.Buffer
	writeSummary(&buf, []runtimeStatus{
		{name: "Go", state: stateExercised},
		{name: "Rust", state: stateExercised},
		{name: "TypeScript", state: stateSkipped, detail: "esbuild missing"},
		{name: "Java", state: stateError, detail: "javac failed"},
	})
	out := buf.String()
	if !strings.Contains(out, "runtimes exercised: Go, Rust") {
		t.Errorf("summary missing exercised list:\n%s", out)
	}
	if !strings.Contains(out, "TypeScript (esbuild missing)") {
		t.Errorf("summary missing skip detail:\n%s", out)
	}
	if !strings.Contains(out, "Java (javac failed)") {
		t.Errorf("summary missing error detail:\n%s", out)
	}
}

// --- end-to-end ------------------------------------------------------------

// TestRunEndToEndGo proves the acceptance criterion: `go run ./cmd/xpbench`
// (here via run() with the always-available Go runtime) produces the
// cross-runtime table for available runtimes and reports what ran. The other
// runtimes are excluded here so the test is hermetic and fast.
func TestRunEndToEndGo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--runtimes", "go"}, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "XPB cross-runtime benchmark") {
		t.Errorf("missing table banner:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "runtimes exercised: Go") {
		t.Errorf("summary did not report Go exercised:\n%s", stderr.String())
	}

	// Prove exactly one Go data row per canonical shape exists (parse the
	// machine form so a dropped/duplicated row cannot hide behind a substring
	// match -- "string" is a substring of "string_array"/"string_map").
	var jsonOut, jsonErr bytes.Buffer
	if err := run([]string{"--runtimes", "go", "--format", "json"}, &jsonOut, &jsonErr); err != nil {
		t.Fatalf("run json: %v", err)
	}
	var rows []Row
	if err := json.Unmarshal(jsonOut.Bytes(), &rows); err != nil {
		t.Fatalf("parse rows: %v\n%s", err, jsonOut.String())
	}
	goShapes := map[string]int{}
	for _, r := range rows {
		if r.Runtime == "Go" && !r.Skipped {
			goShapes[r.Shape]++
		}
	}
	for _, s := range canonicalShapes() {
		if goShapes[s.name] != 1 {
			t.Errorf("want exactly one Go row for shape %q, got %d", s.name, goShapes[s.name])
		}
	}
	if len(goShapes) != len(canonicalShapes()) {
		t.Errorf("Go rows cover %d shapes, want %d", len(goShapes), len(canonicalShapes()))
	}
}

// TestRunJSONToFile proves --out writes the chosen machine format to a file AND
// still prints the human table to stdout (one run, both artifacts).
func TestRunJSONToFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "rows.json")
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--runtimes", "go", "--format", "json", "--out", out}, &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "XPB cross-runtime benchmark") {
		t.Errorf("stdout should still show the human table when --out is set:\n%s", stdout.String())
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read out file: %v", err)
	}
	var rows []Row
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatalf("out file is not valid JSON: %v", err)
	}
	if len(rows) == 0 {
		t.Error("out file has no rows")
	}
}

// TestRunRejectsBadFormat proves an unknown --format is a clean error.
func TestRunRejectsBadFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--format", "yaml", "--runtimes", "go"}, &stdout, &stderr); err == nil {
		t.Error("expected error for unknown --format")
	}
}

// TestRunNoRuntimesSelected proves run() errors cleanly when --runtimes names
// only unknown runtimes (it must not silently run nothing).
func TestRunNoRuntimesSelected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"--runtimes", "bogus,nope"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when no known runtimes are selected")
	}
	// Bind the assertion to the no-selection cause so it cannot pass for an
	// unrelated error (e.g. a repoRoot failure).
	if !strings.Contains(err.Error(), "no runtimes selected") {
		t.Errorf("error %q should name the no-selection cause", err.Error())
	}
}

// --- toolchain gating: the REAL runners --------------------------------------

// TestRealRunnersSkipWhenToolchainAbsent proves the detection half of AC2: with
// every external toolchain hidden, each real have()-gated runner takes its clean
// SKIP path (exactly one marked skip row, state=skipped) rather than failing,
// erroring, or panicking. This exercises the production gating logic in
// runners.go directly, not a synthetic stand-in.
func TestRealRunnersSkipWhenToolchainAbsent(t *testing.T) {
	// An empty PATH hides cargo/node/cc/lua/javac/java; CC="" stops runC from
	// resolving a compiler via the env.
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CC", "")
	metas := []shapeMeta{{Name: "scalars", WireSize: 10, Iters: 1}}

	for _, r := range []namedRunner{
		{"Rust", runRust},
		{"TypeScript", runTS},
		{"C", runC},
		{"Lua", runLua},
		{"Java", runJava},
	} {
		rows, st := r.fn("", "", metas)
		if st.state != stateSkipped {
			t.Errorf("%s: state=%q want skipped (detail=%q)", r.name, st.state, st.detail)
		}
		if len(rows) != 1 || !rows[0].Skipped || rows[0].SkipReason == "" {
			t.Errorf("%s: want exactly one marked skip row, got %+v", r.name, rows)
		}
	}
}

// --- external-harness ingestion (the Go side that consumes non-Go output) -----

// TestParseHarnessJSON proves the driver tolerates a leading banner before the
// JSON array and rejects output with no array / malformed array.
func TestParseHarnessJSON(t *testing.T) {
	res, err := parseHarnessJSON([]byte("compiling foo v0.1\n[{\"name\":\"x\",\"encodeNs\":1.5,\"decodeNs\":2.5,\"wireSize\":10}]"))
	if err != nil {
		t.Fatalf("valid (banner-prefixed) parse: %v", err)
	}
	if len(res) != 1 || res[0].Name != "x" || res[0].EncodeNs != 1.5 || res[0].DecodeNs != 2.5 || res[0].WireSize != 10 {
		t.Errorf("unexpected parse result: %+v", res)
	}
	if _, err := parseHarnessJSON([]byte("no array here")); err == nil {
		t.Error("want error when no JSON array is present")
	}
	if _, err := parseHarnessJSON([]byte("[not valid json")); err == nil {
		t.Error("want error on malformed JSON array")
	}
}

// TestRowsFromHarness proves a non-Go harness's raw timings are normalized to
// comparable rows (MB/s computed from wire size + ns/op) and that non-Go rows
// carry the AllocsNA sentinel (AC3 + AC6 for the 5 external runtimes).
func TestRowsFromHarness(t *testing.T) {
	rows := rowsFromHarness("Rust", []harnessResult{{Name: "scalars", EncodeNs: 200, DecodeNs: 500, WireSize: 56}})
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1", len(rows))
	}
	r := rows[0]
	if r.Runtime != "Rust" || r.Shape != "scalars" || r.WireSize != 56 {
		t.Errorf("unexpected row: %+v", r)
	}
	if r.EncodeMBps != mbps(56, 200) || r.DecodeMBps != mbps(56, 500) {
		t.Errorf("MB/s not normalized from ns/op: %+v", r)
	}
	if r.EncodeAllocsPerOp != AllocsNA || r.DecodeAllocsPerOp != AllocsNA {
		t.Errorf("non-Go row must use AllocsNA: %+v", r)
	}
}

// TestRunExternalHarnessError proves a harness command that cannot run is
// classified as an error (present-but-failing toolchain) with one marked info
// row, never crashing the whole tool.
func TestRunExternalHarnessError(t *testing.T) {
	cmd := exec.Command(filepath.Join(t.TempDir(), "definitely-not-a-real-binary"))
	rows, st := runExternalHarness("Bogus", cmd)
	if st.state != stateError {
		t.Errorf("state=%q want error", st.state)
	}
	if len(rows) != 1 || !rows[0].Skipped || rows[0].SkipReason == "" {
		t.Errorf("want one marked info row, got %+v", rows)
	}
}

// TestRunExternalHarnessSuccess proves the success-path glue that classifies an
// external runtime as exercised and normalizes its raw timings into rows, plus
// the empty-results guard. It feeds runExternalHarness a real subprocess (cat)
// echoing a harness JSON document, exercising parse -> normalize -> classify end
// to end (the part not covered by the unit tests of parseHarnessJSON /
// rowsFromHarness in isolation).
func TestRunExternalHarnessSuccess(t *testing.T) {
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not available")
	}
	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	if err := os.WriteFile(good, []byte(`[{"name":"scalars","encodeNs":200,"decodeNs":500,"wireSize":56}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, st := runExternalHarness("Rust", exec.Command("cat", good))
	if st.state != stateExercised {
		t.Fatalf("state=%q want exercised (detail=%q)", st.state, st.detail)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1", len(rows))
	}
	r := rows[0]
	if r.Runtime != "Rust" || r.Shape != "scalars" || r.WireSize != 56 {
		t.Errorf("unexpected row: %+v", r)
	}
	if r.EncodeMBps != mbps(56, 200) || r.DecodeMBps != mbps(56, 500) {
		t.Errorf("MB/s not normalized from harness output: %+v", r)
	}
	if r.EncodeAllocsPerOp != AllocsNA || r.DecodeAllocsPerOp != AllocsNA {
		t.Errorf("non-Go row must use AllocsNA: %+v", r)
	}

	// An empty results array is classified as an error (a present-but-empty
	// harness), never silently treated as exercised.
	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte(`[]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, st2 := runExternalHarness("Rust", exec.Command("cat", empty)); st2.state != stateError {
		t.Errorf("empty results: state=%q want error", st2.state)
	}
}

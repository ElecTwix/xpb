package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ElecTwix/xpb/runtime/go/xpb"
	"github.com/ElecTwix/xpb/tests/conformance"
)

// sinks defeat dead-code elimination so the timed encode/decode work is not
// optimized away by the Go compiler.
var (
	sinkBytes []byte
	sinkU64   uint64
)

// have reports whether bin resolves on PATH.
func have(bin string) bool { _, err := exec.LookPath(bin); return err == nil }

// firstAvailable returns the first candidate that resolves on PATH, or "".
func firstAvailable(cands ...string) string {
	for _, c := range cands {
		if c != "" && have(c) {
			return c
		}
	}
	return ""
}

// repoRoot walks up from cwd to the directory holding go.mod.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod above %s", dir)
		}
		dir = parent
	}
}

// timeLoop runs f iters times after a short warmup and returns ns/op.
func timeLoop(iters int, f func()) float64 {
	if iters < 1 {
		iters = 1
	}
	warm := iters / 10
	for i := 0; i < warm; i++ {
		f()
	}
	start := time.Now()
	for i := 0; i < iters; i++ {
		f()
	}
	return float64(time.Since(start).Nanoseconds()) / float64(iters)
}

// measureAllocs returns allocations per op (mallocs delta / iters). It uses a
// bounded iteration count so the second pass stays cheap; mallocs accounting is
// exact regardless of the count.
func measureAllocs(iters int, f func()) float64 {
	if iters > 50_000 {
		iters = 50_000
	}
	if iters < 1 {
		iters = 1
	}
	runtime.GC()
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)
	for i := 0; i < iters; i++ {
		f()
	}
	runtime.ReadMemStats(&m2)
	return float64(m2.Mallocs-m1.Mallocs) / float64(iters)
}

// skipResult / errorResult build the single info Row + status a non-exercised
// runtime contributes.
func skipResult(name, reason string) ([]Row, runtimeStatus) {
	return infoRow(name, reason), runtimeStatus{name: name, state: stateSkipped, detail: reason}
}

func errorResult(name, reason string) ([]Row, runtimeStatus) {
	return infoRow(name, reason), runtimeStatus{name: name, state: stateError, detail: reason}
}

func infoRow(name, reason string) []Row {
	return []Row{{
		Runtime:           name,
		Shape:             skipShape,
		Skipped:           true,
		SkipReason:        reason,
		EncodeAllocsPerOp: AllocsNA,
		DecodeAllocsPerOp: AllocsNA,
	}}
}

// runGo benchmarks the Go runtime in-process over the canonical shapes. It is
// the only runtime measured in-process, so it is also the only one that reports
// allocations per op. Go is always available (this is a Go program).
func runGo(_ string, _ string, metas []shapeMeta) ([]Row, runtimeStatus) {
	byName := map[string][]conformance.Op{}
	for _, s := range canonicalShapes() {
		byName[s.name] = s.ops
	}
	rows := make([]Row, 0, len(metas))
	for _, m := range metas {
		ops := byName[m.Name]
		data := conformance.Encode(ops)

		encNs := timeLoop(m.Iters, func() { sinkBytes = conformance.Encode(ops) })
		decNs := timeLoop(m.Iters, func() { sinkU64 += decodeGoOps(xpb.NewDecoder(data), ops) })
		encAllocs := measureAllocs(m.Iters, func() { sinkBytes = conformance.Encode(ops) })
		decAllocs := measureAllocs(m.Iters, func() { sinkU64 += decodeGoOps(xpb.NewDecoder(data), ops) })

		rows = append(rows, Row{
			Runtime:           "Go",
			Shape:             m.Name,
			WireSize:          m.WireSize,
			EncodeNsPerOp:     encNs,
			EncodeMBps:        mbps(m.WireSize, encNs),
			DecodeNsPerOp:     decNs,
			DecodeMBps:        mbps(m.WireSize, decNs),
			EncodeAllocsPerOp: encAllocs,
			DecodeAllocsPerOp: decAllocs,
		})
	}
	return rows, runtimeStatus{name: "Go", state: stateExercised}
}

// decodeGoOps reads every value of ops from d with the Go decoder and returns a
// small accumulator. It deliberately does NOT verify the decoded values and
// builds NO per-element diagnostic strings (unlike conformance.DecodeAndVerify,
// whose per-op fmt.Sprintf path-building would dominate the decode ns/op and
// allocs/op for array/map/nested shapes and make the Go column incomparable).
// This keeps the Go decode timing measuring decode work only, consistent with
// the external harnesses which likewise decode-only. Strings are cloned
// (materialized) to mirror the other runtimes, which all copy decoded strings
// out of their input buffer. The accumulator is fed to a package sink so the
// compiler cannot elide the decode in the timed loop.
func decodeGoOps(d *xpb.Decoder, ops []conformance.Op) uint64 {
	var acc uint64
	for i := range ops {
		acc += decodeGoOp(d, ops[i])
	}
	return acc
}

func decodeGoOp(d *xpb.Decoder, o conformance.Op) uint64 {
	switch o.Type {
	case conformance.TypeBool:
		v, _ := d.ReadBool()
		if v {
			return 1
		}
		return 0
	case conformance.TypeInt32:
		v, _ := d.ReadInt32()
		return uint64(uint32(v))
	case conformance.TypeUint32:
		v, _ := d.ReadUint32()
		return uint64(v)
	case conformance.TypeInt64:
		v, _ := d.ReadInt64()
		return uint64(v)
	case conformance.TypeUint64:
		v, _ := d.ReadUint64()
		return v
	case conformance.TypeFloat32:
		v, _ := d.ReadFloat32()
		return uint64(math.Float32bits(v))
	case conformance.TypeFloat64:
		v, _ := d.ReadFloat64()
		return math.Float64bits(v)
	case conformance.TypeString:
		s, _ := d.CloneString()
		return uint64(len(s))
	case conformance.TypeBytes:
		b, _ := d.ReadBytes()
		return uint64(len(b))
	case conformance.TypeArray:
		count, _ := d.ReadInt32()
		acc := uint64(uint32(count))
		for i := range o.Elems {
			acc += decodeGoOp(d, o.Elems[i])
		}
		return acc
	case conformance.TypeMap:
		count, _ := d.ReadInt32()
		acc := uint64(uint32(count))
		for i := range o.Entries {
			acc += decodeGoOp(d, o.Entries[i].K)
			acc += decodeGoOp(d, o.Entries[i].V)
		}
		return acc
	case conformance.TypeMessage:
		msg, _ := d.ReadMessageBytes()
		inner := xpb.NewDecoder(msg)
		return decodeGoOps(inner, o.Ops) + uint64(len(msg))
	}
	return 0
}

// parseHarnessJSON extracts the JSON array a harness prints to stdout (tolerant
// of any leading banner text a runtime might emit).
func parseHarnessJSON(stdout []byte) ([]harnessResult, error) {
	idx := bytes.IndexByte(stdout, '[')
	if idx < 0 {
		return nil, fmt.Errorf("no JSON array in harness output: %q", truncate(string(stdout), 200))
	}
	var out []harnessResult
	if err := json.Unmarshal(stdout[idx:], &out); err != nil {
		return nil, fmt.Errorf("parse harness JSON: %w (output: %q)", err, truncate(string(stdout), 200))
	}
	return out, nil
}

// rowsFromHarness normalizes a harness's raw timings into comparable Rows,
// computing MB/s from the wire size and ns/op. Non-Go runtimes do not report
// allocations.
func rowsFromHarness(name string, results []harnessResult) []Row {
	rows := make([]Row, 0, len(results))
	for _, r := range results {
		rows = append(rows, Row{
			Runtime:           name,
			Shape:             r.Name,
			WireSize:          r.WireSize,
			EncodeNsPerOp:     r.EncodeNs,
			EncodeMBps:        mbps(r.WireSize, r.EncodeNs),
			DecodeNsPerOp:     r.DecodeNs,
			DecodeMBps:        mbps(r.WireSize, r.DecodeNs),
			EncodeAllocsPerOp: AllocsNA,
			DecodeAllocsPerOp: AllocsNA,
		})
	}
	return rows
}

// runExternalHarness runs an already-built harness command, parses its JSON,
// and returns normalized rows. A run/parse failure is reported as an error
// status (toolchain present but the harness did not produce results) -- never a
// hard crash of the whole tool.
func runExternalHarness(name string, cmd *exec.Cmd) ([]Row, runtimeStatus) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return errorResult(name, fmt.Sprintf("run failed: %v: %s", err, truncate(stderr.String(), 300)))
	}
	results, err := parseHarnessJSON(stdout.Bytes())
	if err != nil {
		return errorResult(name, err.Error())
	}
	if len(results) == 0 {
		return errorResult(name, "harness produced no results")
	}
	return rowsFromHarness(name, results), runtimeStatus{name: name, state: stateExercised}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"text/tabwriter"
)

// mbps converts a wire size (bytes processed per op) and a per-op time in
// nanoseconds into decimal megabytes per second: bytes/ns * 1e9/1e6.
func mbps(wireSize int, nsPerOp float64) float64 {
	if nsPerOp <= 0 {
		return 0
	}
	return float64(wireSize) / nsPerOp * 1000.0
}

// runtimeOrder is the stable display order for runtimes in the human table.
var runtimeOrder = map[string]int{
	"Go": 0, "Rust": 1, "TypeScript": 2, "C": 3, "Lua": 4, "Java": 5,
}

// sortRows orders rows by shape (canonical order), then runtime, so the same
// shape's runtimes appear adjacent for side-by-side comparison.
func sortRows(rows []Row, shapeOrder map[string]int) {
	sort.SliceStable(rows, func(i, j int) bool {
		si, sj := shapeOrder[rows[i].Shape], shapeOrder[rows[j].Shape]
		if si != sj {
			return si < sj
		}
		return runtimeOrder[rows[i].Runtime] < runtimeOrder[rows[j].Runtime]
	})
}

// shapeOrderFromMeta maps each shape name to its position so the table sorts in
// definition order rather than alphabetically.
func shapeOrderFromMeta(metas []shapeMeta) map[string]int {
	m := make(map[string]int, len(metas))
	for i, meta := range metas {
		m[meta.Name] = i
	}
	// Skipped/errored rows use the sentinel shape; sort them last.
	m[skipShape] = len(metas)
	return m
}

const skipShape = "(all)"

func fmtFloat(v float64) string {
	if v == 0 {
		return "0"
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func fmtAllocs(v float64) string {
	if v == AllocsNA {
		return "-"
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

// writeTable renders the normalized, human-readable cross-runtime table. Each
// data row compares one runtime on one shape; skipped/errored runtimes appear
// as a clearly marked row so the reader sees what did not run.
func writeTable(w io.Writer, rows []Row, shapeOrder map[string]int) {
	sortRows(rows, shapeOrder)
	fmt.Fprintln(w, "XPB cross-runtime benchmark (same shapes, all runtimes; ns/op + MB/s, lower ns / higher MB/s is better)")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RUNTIME\tSHAPE\tWIRE(B)\tENC ns/op\tENC MB/s\tDEC ns/op\tDEC MB/s\tALLOCS/op (enc/dec)")
	fmt.Fprintln(tw, "-------\t-----\t-------\t---------\t--------\t---------\t--------\t-------------------")
	for _, r := range rows {
		if r.Skipped {
			// The skip reason is the trailing (un-tabbed) cell so its length does
			// not widen any aligned numeric column of the table.
			fmt.Fprintf(tw, "%s\t%s\tSKIPPED: %s\n", r.Runtime, skipShape, r.SkipReason)
			continue
		}
		allocs := "-"
		if r.EncodeAllocsPerOp != AllocsNA || r.DecodeAllocsPerOp != AllocsNA {
			allocs = fmtAllocs(r.EncodeAllocsPerOp) + " / " + fmtAllocs(r.DecodeAllocsPerOp)
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
			r.Runtime, r.Shape, r.WireSize,
			fmtFloat(r.EncodeNsPerOp), fmtFloat(r.EncodeMBps),
			fmtFloat(r.DecodeNsPerOp), fmtFloat(r.DecodeMBps),
			allocs)
	}
	//nolint:errcheck // tabwriter.Flush only errors if the underlying writer does; best-effort for a report.
	tw.Flush()
}

// writeJSON emits the machine-readable JSON array of rows.
func writeJSON(w io.Writer, rows []Row, shapeOrder map[string]int) error {
	sortRows(rows, shapeOrder)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

// writeCSV emits the machine-readable CSV form of the rows (one header row then
// one row per measurement). Skipped runtimes are emitted with empty metric
// cells and skipped=true so a spreadsheet consumer can filter them.
func writeCSV(w io.Writer, rows []Row, shapeOrder map[string]int) error {
	sortRows(rows, shapeOrder)
	cw := csv.NewWriter(w)
	header := []string{
		"runtime", "shape", "wireSizeBytes",
		"encodeNsPerOp", "encodeMBps", "decodeNsPerOp", "decodeMBps",
		"encodeAllocsPerOp", "decodeAllocsPerOp", "skipped", "skipReason",
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		rec := []string{
			r.Runtime, r.Shape, strconv.Itoa(r.WireSize),
			csvNum(r.EncodeNsPerOp, r.Skipped), csvNum(r.EncodeMBps, r.Skipped),
			csvNum(r.DecodeNsPerOp, r.Skipped), csvNum(r.DecodeMBps, r.Skipped),
			csvAllocs(r.EncodeAllocsPerOp), csvAllocs(r.DecodeAllocsPerOp),
			strconv.FormatBool(r.Skipped), r.SkipReason,
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func csvNum(v float64, skipped bool) string {
	if skipped {
		return ""
	}
	return strconv.FormatFloat(v, 'f', 3, 64)
}

func csvAllocs(v float64) string {
	if v == AllocsNA {
		return ""
	}
	return strconv.FormatFloat(v, 'f', 3, 64)
}

// writeSummary prints the exercised-vs-skipped runtime report the ticket
// requires, so a run in a minimal environment is never mistaken for a full one.
func writeSummary(w io.Writer, statuses []runtimeStatus) {
	var exercised, skipped, errored []string
	for _, s := range statuses {
		switch s.state {
		case stateExercised:
			exercised = append(exercised, s.name)
		case stateSkipped:
			skipped = append(skipped, fmt.Sprintf("%s (%s)", s.name, s.detail))
		case stateError:
			errored = append(errored, fmt.Sprintf("%s (%s)", s.name, s.detail))
		}
	}
	fmt.Fprintln(w, "\nruntimes exercised:", joinOrNone(exercised))
	fmt.Fprintln(w, "runtimes skipped:  ", joinOrNone(skipped))
	if len(errored) > 0 {
		fmt.Fprintln(w, "runtimes errored:  ", joinOrNone(errored))
	}
	if len(exercised) <= 1 {
		fmt.Fprintln(w, "NOTE: at most the Go runtime ran; install cargo / node+esbuild / a C compiler / lua / a JDK to exercise the others.")
	}
}

func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	out := items[0]
	for _, s := range items[1:] {
		out += ", " + s
	}
	return out
}

package main

// AllocsNA is the sentinel stored in a Row's alloc fields when a runtime does
// not report allocations (every runtime except Go). It renders as "-" in the
// human table and is preserved verbatim in the machine-readable output so a
// consumer can tell "not measured" apart from a genuine zero.
const AllocsNA = -1.0

// Row is one normalized cross-runtime benchmark measurement: a single runtime's
// encode/decode performance for a single canonical message shape. Every numeric
// field is in a unit that is directly comparable across runtimes -- ns/op and
// MB/s -- so two rows for the same Shape but different Runtime can be read side
// by side. A skipped or errored runtime contributes exactly one Row with
// Skipped=true and a populated SkipReason (its metric fields are left zero).
type Row struct {
	Runtime  string `json:"runtime"`
	Shape    string `json:"shape"`
	WireSize int    `json:"wireSizeBytes"`

	EncodeNsPerOp float64 `json:"encodeNsPerOp"`
	EncodeMBps    float64 `json:"encodeMBps"`
	DecodeNsPerOp float64 `json:"decodeNsPerOp"`
	DecodeMBps    float64 `json:"decodeMBps"`

	// Alloc accounting is Go-only (the one runtime we benchmark in-process and
	// can instrument); every other runtime stores AllocsNA.
	EncodeAllocsPerOp float64 `json:"encodeAllocsPerOp"`
	DecodeAllocsPerOp float64 `json:"decodeAllocsPerOp"`

	Skipped    bool   `json:"skipped"`
	SkipReason string `json:"skipReason,omitempty"`
}

// runtimeState classifies how a runtime fared in a run, for the exercised-vs-
// skipped summary the ticket requires.
type runtimeState string

const (
	stateExercised runtimeState = "exercised"
	stateSkipped   runtimeState = "skipped" // toolchain absent -- a clean skip
	stateError     runtimeState = "error"   // toolchain present but the harness failed
)

// runtimeStatus is the per-runtime verdict for one run.
type runtimeStatus struct {
	name   string
	state  runtimeState
	detail string // skip/error reason (empty when exercised)
}

// harnessResult is the JSON shape every external (non-Go) harness prints to
// stdout: one entry per canonical shape, with raw timings the Go driver then
// normalizes into a Row (computing MB/s from WireSize and ns/op).
type harnessResult struct {
	Name     string  `json:"name"`
	EncodeNs float64 `json:"encodeNs"`
	DecodeNs float64 `json:"decodeNs"`
	WireSize int     `json:"wireSize"`
}

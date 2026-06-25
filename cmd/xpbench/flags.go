package main

import "flag"

// newFlagSet builds the command's flag set, binding the destination variables.
// It is its own function so run() stays focused on orchestration and so the
// flag wiring can be exercised in isolation.
func newFlagSet(format, outPath, runtimes *string) *flag.FlagSet {
	fs := flag.NewFlagSet("xpbench", flag.ContinueOnError)
	fs.StringVar(format, "format", *format, "output format: table|json|csv")
	fs.StringVar(outPath, "out", *outPath, "write the chosen format to this file (also prints the human table to stdout)")
	fs.StringVar(runtimes, "runtimes", *runtimes, "comma-separated runtimes to run, or 'all' (go,rust,typescript,c,lua,java)")
	return fs
}

# uteka codegen benchmark baseline

This package compares the two Go codegen styles (`ptr/` default pointer
optionals + copying bytes, `val/` value optionals + zero-copy bytes) against
JSON and msgpack for a realistic control-plane RPC message.

## Capture a baseline before a perf refactor

```sh
# Stable numbers: pin GOMAXPROCS, raise the run count.
go test ./benchmarks/go/uteka/ -run '^$' -bench . -benchmem -count=10 \
    | tee baseline.txt
```

## Compare after the refactor

```sh
go install golang.org/x/perf/cmd/benchstat@latest

go test ./benchmarks/go/uteka/ -run '^$' -bench . -benchmem -count=10 \
    | tee after.txt

benchstat baseline.txt after.txt
```

`benchstat` reports the per-benchmark delta with a significance test, so a real
regression (or win) is distinguishable from run-to-run noise.

## Deterministic guards (run under plain `go test`, no benchstat needed)

These run on every `go test` and fail CI on a regression without depending on
wall-clock numbers:

- `alloc_test.go` — `TestZeroAlloc_ValDecode` and `TestZeroAlloc_PooledEncode`
  assert `testing.AllocsPerRun(1000, fn) == 0` for value-style decode and pooled
  encode. They catch a 0->N allocation regression deterministically.
- `runtime/go/xpb/inline_test.go` — `TestInliningGuard_HotHelpers` asserts the
  hot scalar `Read*`/`Write*` helpers still report "can inline" via
  `go build -gcflags=-m`, catching an inlining (and therefore hot-path) regression.

Run them all, including the race check over the encoder pool + zero-copy aliasing:

```sh
go test ./benchmarks/go/uteka/...           # normal gates
go test -race ./benchmarks/go/uteka/...      # pool + aliasing under the race detector
```

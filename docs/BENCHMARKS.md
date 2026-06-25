# Benchmark Measurement Protocol

This document describes how to **measure** Go performance reproducibly and how to
run the **profile-guided-optimization (PGO)** workflow. It is the measurement
counterpart to the deterministic correctness gates:

- **Deterministic gates** (allocs-per-op, wire size, inlining) live in the Go
  test suite (`benchmarks/go/uteka/alloc_test.go`,
  `runtime/go/xpb/inline_test.go`, and the per-shape gate tests). They run under
  plain `go test` / `make verify` and **fail CI** on a regression without
  depending on any wall-clock number.
- **Measurement** (ns/op throughput) is what this document and the `make bench*`
  targets cover. Raw ns/op is **never** a hard CI gate (see
  [Why ns/op is never CI-gated](#why-nsop-is-never-ci-gated)).

Pre-rendered result tables for a specific machine live in
[`BENCHMARK_REPORT.md`](BENCHMARK_REPORT.md); this file is about *how* to produce
trustworthy numbers, not a snapshot of them.

## Quick start

```bash
make benchstat-install   # one-time: install golang.org/x/perf/cmd/benchstat
make bench               # reproducible run; writes bench.out
make bench-compare       # benchstat the run against a committed baseline
make bench-pgo           # capture default.pgo and measure the PGO delta
```

All of these are **additive** and **not** wired into `make verify` / `make ci`.

## The targets

| Target | What it does |
|--------|--------------|
| `make bench` | Runs the Go benchmarks with the reproducible settings below over `./benchmarks/go/uteka/...` and `./benchmarks/go/matrix/...` (the matrix tree is included only when it exists), writing results to `bench.out`. |
| `make bench-compare` | Runs `bench`, then `benchstat`s the fresh run against a committed baseline and prints each delta with a confidence interval. Informational, not a gate. |
| `make bench-pgo` | Captures a CPU profile into `default.pgo`, then measures the PGO delta (bench with vs. without `-pgo`) via `benchstat`. |
| `make benchstat-install` | Installs `benchstat` into `$(go env GOPATH)/bin` (mirrors `make mutate-install`). |

Override the knobs on the command line, e.g.
`make bench BENCH_COUNT=20 BENCH_BENCHTIME=20000x`.

## Why these settings (reproducibility)

The point of a measurement is that **the same change produces the same verdict**
on the same machine. Three sources of variance are pinned away:

- **Fixed iteration count, not wall-clock time** — the targets pass
  `-benchtime=<N>x` (e.g. `5000x`), not the default time-based `-benchtime=1s`.
  A time-based run does *a different amount of work* every time (and a different
  amount on a faster/slower machine), so the iteration count — and thus the noise
  profile — drifts. A fixed `Nx` count makes every run execute identical work.
- **`GOMAXPROCS=1`** — pinning to a single OS thread removes goroutine-scheduler
  and cross-core migration jitter, which is the largest source of run-to-run
  variance for these tiny (single-digit-ns) operations. Measure single-core
  throughput first; concurrency scaling is a separate experiment.
- **`-count=10`** — repeating the whole run gives `benchstat` enough samples to
  compute a mean, a variation band, and a significance test. A single run cannot
  distinguish a real change from noise.
- **`-benchmem`** — always report `B/op` and `allocs/op`. Allocation counts are
  deterministic (see below) and are the most reliable signal in the output.

### Pinning the environment

ns/op is only comparable against numbers captured on the **same machine, in the
same power/thermal state**. For trustworthy comparisons:

- Capture the baseline and the candidate **back to back** on the same host.
- On a laptop, disable turbo / pin the governor to a fixed frequency if you can,
  plug in to AC, and let the machine reach thermal steady state.
- Record `goos`, `goarch`, `cpu`, and the Go version (`go test` prints the first
  three; they appear at the top of `bench.out`). Never compare numbers across
  different `cpu:` lines.
- Close other CPU-heavy work; `GOMAXPROCS=1` helps but does not isolate the box.

## Comparing with benchstat (confidence, not eyeballing)

Never compare two ns/op numbers by eye — a 5% difference between single runs is
almost always noise. `benchstat` takes the `-count=10` samples from each side and
reports the delta **with a confidence interval and a p-value**:

```bash
# Seed a committed baseline once (writes a .txt you can `git add`):
make bench BENCH_OUT=benchmarks/go/uteka/baseline.txt
git add benchmarks/go/uteka/baseline.txt

# Later, after a change:
make bench-compare       # benchstat baseline.txt bench.out
```

`bench-compare` prints something like `~` (no significant change) or a delta with
`(p=0.001 n=10)`. **Only a statistically-significant delta counts as a
regression or a win.** Everything else is noise. If no baseline is committed yet,
`bench-compare` still prints the current run's summary and tells you how to seed
one.

## Why ns/op is never CI-gated

Raw throughput (ns/op) depends on the host CPU, its thermal/power state,
neighbouring load, and the Go version. Gating CI on it would make the build flaky
(green on a cold runner, red on a hot one) and would punish contributors for the
CI machine's mood. So ns/op is **measured and reported for humans**, compared
with `benchstat`'s significance test, and discussed in review — but it never
fails the build.

What **is** gated (in the deterministic test suite, T-16, run by `make verify`):

- **`allocs/op` / `B/op`** — allocation counts are deterministic for a given code
  path. `TestZeroAlloc_ValDecode` / `TestZeroAlloc_PooledEncode` assert
  `testing.AllocsPerRun(...) == 0`, so a 0→N allocation regression fails CI
  immediately, independent of wall-clock speed.
- **Wire size** — the encoded byte length for a given message is fixed by the
  wire format. The conformance/golden vectors and size assertions catch any
  unintended size change.
- **Inlining of the hot helpers** — `TestInliningGuard_HotHelpers` asserts the
  hot `Read*`/`Write*` helpers still report "can inline", catching the
  optimization regressions that *cause* ns/op regressions — deterministically,
  without measuring time.

In short: gate the **deterministic** proxies of performance (allocs, size,
inlining); only **measure** the non-deterministic one (ns/op).

## Profile-guided optimization (PGO) workflow

[PGO](https://go.dev/doc/pgo) lets the compiler use a representative CPU profile
to drive inlining and devirtualization on the hot path. `make bench-pgo`
automates the capture-and-measure loop:

1. **Capture** a CPU profile from the uteka hot path (which exercises the
   `runtime/go/xpb` encode/decode primitives) and write it as `default.pgo`:

   ```bash
   GOMAXPROCS=1 go test -run '^$' -bench=. -benchtime=5000x \
       -cpuprofile=default.pgo ./benchmarks/go/uteka/
   ```

2. **Measure the delta** — run the benchmarks once without PGO and once with
   `-pgo=default.pgo`, then diff with `benchstat`:

   ```bash
   GOMAXPROCS=1 go test -run '^$' -bench=. -benchmem -count=10 \
       ./benchmarks/go/uteka/... > bench-nopgo.out
   GOMAXPROCS=1 go test -pgo=default.pgo -run '^$' -bench=. -benchmem -count=10 \
       ./benchmarks/go/uteka/... > bench-pgo.out
   benchstat bench-nopgo.out bench-pgo.out
   ```

   `make bench-pgo` runs all four steps for you.

3. **Ship the win** — Go auto-detects a file named `default.pgo` in a `main`
   package's directory at build time, or pass `-pgo=<path>` to any
   `go build`/`go test`. Re-measure after committing the profile to confirm the
   delta holds.

The profile (`default.pgo`) and the `*.out` result files are **local build
artifacts** — they should be added to `.gitignore` and not committed, except for
a deliberately curated `default.pgo` placed next to a `main` package you want PGO
to optimize.

## Future determinism options

Two techniques could make even the timing side more reproducible; they are noted
here as future work, not yet implemented:

- **`testing/synctest`** — for the concurrency-scaling benchmarks, a synthetic
  clock (Go 1.24+ `testing/synctest`) removes wall-clock and scheduler
  nondeterminism so concurrent ser/de can be measured deterministically rather
  than wall-clock-timed.
- **Instruction-count measurement** — counting retired instructions (e.g. via
  `perf stat` / hardware counters or a simulator like `cachegrind`) instead of
  nanoseconds yields a machine-independent, noise-free metric that *could* be
  CI-gated, unlike ns/op. This would let throughput regressions be caught
  deterministically the way allocs and wire size already are.

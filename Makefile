# Repository verify plan.
#
# `make verify` is the canonical "is the tree healthy?" gate. It mirrors the
# package selection that cmd/ci already uses: the root benchmarks/go package is
# EXCLUDED because its committed bench.pb.go panics at init against the installed
# protobuf runtime (a pre-existing, unrelated breakage). The uteka benchmark
# sub-packages (benchmarks/go/uteka/...) are NOT excluded -- their tests are part
# of the Go codegen perf-test arsenal and must run, including under -race for the
# encoder pool + zero-copy aliasing paths.

# GO_PKGS = every module package except the broken root benchmarks/go package.
# benchmarks/go/uteka and its children are kept (they only match the broader
# "/benchmarks/" path, which we do not exclude).
GO_PKGS := $(shell go list ./... | grep -v '/benchmarks/go$$' | grep -v '^github.com/ElecTwix/xpb/benchmarks/go$$')

.PHONY: verify verify-go test test-race fmt vet build ci mutate mutate-install \
        bench bench-compare bench-pgo benchstat-install

verify: verify-go

# verify-go: gofmt + vet + build + test over the non-broken package set, plus a
# race run over the uteka arsenal (encoder pool + zero-copy aliasing).
verify-go: fmt vet build test test-race

fmt:
	@unformatted=$$(gofmt -l . | grep -v '^benchmarks/' || true); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: unformatted files:"; echo "$$unformatted"; exit 1; \
	fi

vet:
	go vet $(GO_PKGS)

build:
	go build ./...

test:
	go test -count=1 $(GO_PKGS)

# Race over the uteka arsenal specifically (encoder pool + zero-copy aliasing).
test-race:
	go test -race -count=1 ./benchmarks/go/uteka/... ./runtime/go/xpb/...

# ci runs the full multi-language local CI runner (Go + TS + C + Lua + Java).
ci:
	go run ./cmd/ci

# --- Mutation testing (ticket T-13) -------------------------------------------
# `make mutate` runs a mutation tester over the two hot correctness surfaces --
# the Go runtime (runtime/go/xpb) and the Go codegen emitter (pkg/codegen/golang)
# -- to verify the TEST SUITE has teeth, not just that the code is correct. It is
# DELIBERATELY NOT part of `verify`/`ci`: mutation runs are slow and this ticket
# does not gate on a hard score. The target only measures and prints the score;
# raise the kill rate by adding tests (see runtime/go/xpb/mutation_kill_test.go
# and pkg/codegen/golang/mutation_kill_test.go). Full guide: docs/MUTATION.md.
#
# Tooling: gremlins (github.com/go-gremlins/gremlins) is preferred. If it is not
# on PATH the target looks in $(go env GOPATH)/bin, then falls back to
# go-mutesting, and finally prints install guidance. `make mutate-install`
# installs gremlins.
#
# TIMEOUT NOTE: gremlins derives each mutant's test timeout from the (very fast,
# ~0.3s) baseline test run, which is shorter than Go's per-mutant recompile -- so
# WITHOUT a raised --timeout-coefficient every mutant spuriously reports "TIMED
# OUT" and the score reads 0%. The coefficient below is REQUIRED for real results
# in this repo; bump it further on slower machines. See docs/MUTATION.md.
MUTATE_PKGS := ./runtime/go/xpb ./pkg/codegen/golang
GREMLINS_FLAGS := --timeout-coefficient 30 --workers 4

mutate:
	@gbin="$$(command -v gremlins 2>/dev/null || echo "$$(go env GOPATH)/bin/gremlins")"; \
	mbin="$$(command -v go-mutesting 2>/dev/null || echo "$$(go env GOPATH)/bin/go-mutesting")"; \
	if [ -x "$$gbin" ]; then \
		echo "==> gremlins mutation testing over: $(MUTATE_PKGS)"; \
		rc=0; \
		for pkg in $(MUTATE_PKGS); do \
			echo "--- $$pkg ---"; \
			"$$gbin" unleash $(GREMLINS_FLAGS) "$$pkg" || rc=$$?; \
		done; \
		exit $$rc; \
	elif [ -x "$$mbin" ]; then \
		echo "==> gremlins not found; falling back to go-mutesting over: $(MUTATE_PKGS)"; \
		for pkg in $(MUTATE_PKGS); do \
			echo "--- $$pkg ---"; \
			"$$mbin" "$$pkg" || true; \
		done; \
	else \
		echo "No mutation tester found on PATH or in \$$(go env GOPATH)/bin."; \
		echo "  install gremlins (preferred): make mutate-install"; \
		echo "  or go-mutesting (fallback):   go install github.com/zimmski/go-mutesting/cmd/go-mutesting@latest"; \
		echo "See docs/MUTATION.md for how to run and interpret the score."; \
		exit 1; \
	fi

# Install the preferred mutation tester (gremlins) into $(go env GOPATH)/bin.
mutate-install:
	go install github.com/go-gremlins/gremlins/cmd/gremlins@latest

# --- Benchmark MEASUREMENT tooling (ticket T-18) ------------------------------
# Reproducible Go performance MEASUREMENT, kept strictly SEPARATE from the
# deterministic allocs/wire-size GATES (those live in T-16 and run under plain
# `go test`, failing CI on a regression). These bench targets are DELIBERATELY
# NOT wired into `verify`/`ci`: raw ns/op is machine- and noise-dependent, so it
# is reported for humans/benchstat but is NEVER a hard CI gate. Full protocol and
# the PGO workflow: docs/BENCHMARKS.md.
#
# Reproducibility knobs (override on the command line, e.g.
#   make bench BENCH_COUNT=20 BENCH_BENCHTIME=20000x):
#   BENCH_BENCHTIME -- FIXED iteration count (-benchtime=Nx), NOT a wall-clock
#                      duration, so each run does identical work.
#   BENCH_COUNT     -- repeat the whole run N times so benchstat has enough
#                      samples for its significance test.
#   GOMAXPROCS=1 is pinned in the recipes to remove scheduler-driven variance.
BENCH_COUNT     ?= 10
BENCH_BENCHTIME ?= 5000x
# Result files default to *.out (already .gitignored) so a measurement run never
# dirties the tree. Override BENCH_OUT to seed a committable baseline (see below).
BENCH_OUT       ?= bench.out
BENCH_BASELINE  ?= benchmarks/go/uteka/baseline.txt
PGO_PROFILE     ?= default.pgo

# bench: run the Go benchmarks with reproducible settings over the uteka arsenal
# and (when present) the matrix arsenal. The matrix package is added by a parallel
# ticket and may not exist yet -- it is included ONLY if `go list` resolves it, so
# the target never hard-fails on a missing matrix path. The broken root
# benchmarks/go package is never included (only its uteka/matrix sub-trees are).
bench:
	@pkgs="$$(go list ./benchmarks/go/uteka/... 2>/dev/null)"; \
	matrix="$$(go list ./benchmarks/go/matrix/... 2>/dev/null)"; \
	if [ -n "$$matrix" ]; then \
		pkgs="$$pkgs $$matrix"; \
	else \
		echo "note: ./benchmarks/go/matrix not present yet -- benchmarking uteka only"; \
	fi; \
	if [ -z "$$pkgs" ]; then echo "no benchmark packages found"; exit 1; fi; \
	echo "==> bench (GOMAXPROCS=1, -benchtime=$(BENCH_BENCHTIME), -count=$(BENCH_COUNT), -benchmem)"; \
	echo "    packages: $$pkgs"; \
	GOMAXPROCS=1 go test -run '^$$' -bench=. -benchmem \
		-benchtime=$(BENCH_BENCHTIME) -count=$(BENCH_COUNT) $$pkgs > $(BENCH_OUT) 2>&1; \
	rc=$$?; cat $(BENCH_OUT); \
	echo "==> wrote $(BENCH_OUT)"; \
	exit $$rc

# bench-compare: benchstat the fresh run (BENCH_OUT, produced by the `bench`
# prerequisite) against a COMMITTED baseline. benchstat reports each delta with a
# confidence interval and a significance test, so only statistically-significant
# deltas count as regressions -- and even those are INFORMATIONAL, never a CI gate
# (allocs/wire-size are what get gated, in T-16). If no baseline is committed yet
# the target still produces output (the single-run summary) and prints how to seed
# one. Seeding a baseline writes outside this ticket's files, so it is left to a
# human:  make bench BENCH_OUT=$(BENCH_BASELINE) && git add $(BENCH_BASELINE)
bench-compare: bench
	@bs="$$(command -v benchstat 2>/dev/null || echo "$$(go env GOPATH)/bin/benchstat")"; \
	if [ ! -x "$$bs" ]; then \
		echo "benchstat not found on PATH or in \$$(go env GOPATH)/bin."; \
		echo "  install it: make benchstat-install"; \
		echo "  See docs/BENCHMARKS.md for the comparison protocol."; \
		exit 1; \
	fi; \
	if [ -f "$(BENCH_BASELINE)" ]; then \
		echo "==> benchstat (baseline=$(BENCH_BASELINE)  current=$(BENCH_OUT))"; \
		"$$bs" "$(BENCH_BASELINE)" "$(BENCH_OUT)"; \
	else \
		echo "==> no committed baseline at $(BENCH_BASELINE); showing the current run summary."; \
		echo "    Seed one (committable .txt, not the .out artifact):"; \
		echo "      make bench BENCH_OUT=$(BENCH_BASELINE) && git add $(BENCH_BASELINE)"; \
		"$$bs" "$(BENCH_OUT)"; \
	fi; \
	echo "NOTE: only statistically-significant deltas are regressions -- INFORMATIONAL, not a CI gate."

# bench-pgo: profile-guided-optimization workflow. Capture a CPU profile from the
# uteka hot path, write it as default.pgo, then measure the PGO delta by running
# the benchmarks with and without -pgo and diffing the two with benchstat. The
# resulting default.pgo can be committed next to a main package (Go auto-detects a
# file named default.pgo) or passed to any build with -pgo=$(PGO_PROFILE).
bench-pgo:
	@pkgs="$$(go list ./benchmarks/go/uteka/... 2>/dev/null)"; \
	matrix="$$(go list ./benchmarks/go/matrix/... 2>/dev/null)"; \
	if [ -n "$$matrix" ]; then pkgs="$$pkgs $$matrix"; fi; \
	if [ -z "$$pkgs" ]; then echo "no benchmark packages found"; exit 1; fi; \
	bs="$$(command -v benchstat 2>/dev/null || echo "$$(go env GOPATH)/bin/benchstat")"; \
	if [ ! -x "$$bs" ]; then \
		echo "benchstat not found; install it: make benchstat-install"; exit 1; \
	fi; \
	echo "==> [1/4] capture CPU profile -> $(PGO_PROFILE) (from ./benchmarks/go/uteka)"; \
	GOMAXPROCS=1 go test -run '^$$' -bench=. -benchtime=$(BENCH_BENCHTIME) \
		-cpuprofile=$(PGO_PROFILE) ./benchmarks/go/uteka/ >/dev/null || exit $$?; \
	echo "==> [2/4] baseline bench (no PGO) -> bench-nopgo.out"; \
	GOMAXPROCS=1 go test -run '^$$' -bench=. -benchmem \
		-benchtime=$(BENCH_BENCHTIME) -count=$(BENCH_COUNT) $$pkgs > bench-nopgo.out 2>&1 || exit $$?; \
	echo "==> [3/4] PGO bench (-pgo=$(PGO_PROFILE)) -> bench-pgo.out"; \
	GOMAXPROCS=1 go test -pgo=$(PGO_PROFILE) -run '^$$' -bench=. -benchmem \
		-benchtime=$(BENCH_BENCHTIME) -count=$(BENCH_COUNT) $$pkgs > bench-pgo.out 2>&1 || exit $$?; \
	echo "==> [4/4] PGO delta (benchstat bench-nopgo.out bench-pgo.out):"; \
	"$$bs" bench-nopgo.out bench-pgo.out; \
	echo "==> wrote profile $(PGO_PROFILE); rebuild a main package with -pgo=$(PGO_PROFILE) to ship the win."

# Install benchstat (golang.org/x/perf/cmd/benchstat) into $(go env GOPATH)/bin,
# the same way mutate-install installs gremlins.
benchstat-install:
	go install golang.org/x/perf/cmd/benchstat@latest

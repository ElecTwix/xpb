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

.PHONY: verify verify-go test test-race fmt vet build ci mutate mutate-install

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

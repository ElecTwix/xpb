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

.PHONY: verify verify-go test test-race fmt vet build ci

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

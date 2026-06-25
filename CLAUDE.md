# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

XPB is a tagless binary serialization format with a single Go-based code generator (`xpbc`) that emits message types for **six** target languages (Go, TypeScript, C, Lua, Java, Rust), each backed by a hand-written runtime. The wire format is the contract shared by all runtimes.

## Commands

`make verify` is the canonical health gate — run it before claiming work is done:

```bash
make verify        # gofmt + vet + build + go test + -race (the gate)
make ci            # full multi-language CI (go run ./cmd/ci): Go + TS + C + Lua + Java
make mutate        # mutation testing over runtime/go/xpb + pkg/codegen/golang (gremlins; see docs/MUTATION.md)
```

```bash
# Build the generator
go build -o xpbc ./cmd/xpbc

# Generate code. 0.5.0 Go defaults: value-style optionals + zero-copy bytes.
./xpbc --lang=go,ts,c,lua,java,rust schema.xpb
./xpbc --lang=go --go-optional-style=pointer --go-safe-bytes schema.xpb   # 0.4.x-equivalent Go

# Single Go test / package
go test -run TestName ./pkg/parser
go test ./pkg/codegen/golang/

# Go benchmarks — use the uteka sub-packages, NOT ./benchmarks/go (see gotcha)
go test -bench=. -benchmem -count=1 ./benchmarks/go/uteka/
```

Per-language runtime test/bench invocations (gcc for C, `lua5.4` with `package.path`, `javac`/`java`, `vitest` for TS) live in `AGENTS.md` and are wrapped by `make ci`.

## Critical gotchas

- **`go test ./...` PANICS.** The root `benchmarks/go` package's committed `bench.pb.go` panics at init against the installed protobuf runtime (pre-existing, unrelated). `make verify` and `cmd/ci` exclude exactly that package (`GO_PKGS` in the Makefile) while keeping `benchmarks/go/uteka/...`. Always go through `make verify`; never `go test ./...` blind.
- **Module path is `github.com/ElecTwix/xpb`.** Some docs (`AGENTS.md`, `README.md`, `docs/ARCHITECTURE.md`) show a stale `github.com/anthropic/xpb` import path and predate the 0.5.0 Go runtime/codegen changes below — trust the code over those docs where they diverge.

## Architecture

**Pipeline:** `.xpb` schema → `pkg/parser` (lexer + parser) → `pkg/ast` → one emitter per language in `pkg/codegen/{golang,typescript,c,lua,java,rust}`. All emitters consume the same AST and the shared constants in `pkg/wire`. There is **no reflection and no runtime schema** — generated code IS the API: each message gets `Marshal`/`Unmarshal` (Go), `encode`/`decode`, etc., that call hand-written primitives in `runtime/<lang>`.

**Wire format is the cross-runtime contract** (`docs/WIRE_FORMAT.md`, `pkg/wire/wire.go`):
- Tagless **struct mode** — fields encode in declaration order, no tags. (Schema field numbers like `1:` are syntactic; the wire ignores them, so field order/compatibility matters.)
- **Fixed-width little-endian** scalars (int32/uint32/float32 = 4B, int64/uint64/float64 = 8B, bool = 1B).
- **Compact length** for string/bytes/message: 1 byte if ≤254, else `0xFF` + 4-byte LE length.
- Optional fields carry a 1-byte presence flag (`0x00` absent / `0x01`+value).

**Cross-language conformance:** `testdata/conformance/` holds golden byte vectors produced by the Go reference encoder; every runtime must decode → re-encode them byte-identically. `cmd/ci` drives the whole multi-language suite. Changing encode/decode logic that shifts these bytes is a wire-format break — fix the code, never edit a golden vector to pass.

**Schema syntax** (`testdata/*.xpb`): `package x`; `message M { 1: type name }`; `?type` = optional, `[]type` = repeated, `map<k,v>` = map; `enum E { NAME = 0 }`. No `import` directive — one generated package per file, so nested-message decode is always intra-package.

## Go runtime/codegen specifics (the optimized hot path)

The Go runtime (`runtime/go/xpb/xpb.go`) was tuned for zero-allocation, register-local ser/de. **Do not "simplify" generated Go back toward per-field `Decoder` method calls** — that regresses the wins:
- Generated decode threads a register-local `pos int` through stateless `*At` helpers (`ReadInt32At(b,p)→(v,p,err)`, `ReadStringAt`, …), not the stateful `Decoder`.
- Generated encode threads a register-local `buf []byte` through `Append*To` helpers, and coalesces contiguous fixed-width field runs via `EnsureRunAt` / `Run*At` / `Put*At` (one bounds-check / one grow per run).
- The stateful `Encoder`/`Decoder` API (`GetEncoder`/`PutEncoder` pool, `NewDecoder`) is retained, unchanged, for streaming/manual use.

**0.5.0 Go defaults (breaking; `docs/MIGRATION.md`):**
- Optional scalar/string/bytes/enum → **value style** `X T` + `HasX bool` (not `*T`). Opt out: `--go-optional-style=pointer` / `golang.Options{OptionalStyle: OptionalPointer}`. Non-enum **message** optionals stay `*T`.
- `bytes` decode is **zero-copy by default** — the decoded `[]byte` (and, as always, strings) **aliases the decoder's input buffer**. Hazard: do not retain decoded data past a reused/overwritten input buffer; clone it (`append([]byte(nil), x...)`) or generate with `--go-safe-bytes`. `golang.Options.SafeBytes` is the opt-out field (false = zero-copy).
- **Map fields encode non-deterministically** (Go map iteration order) → NOT byte-stable. Don't hash/sign/byte-compare messages containing maps (decode is order-insensitive). Canonical map encoding is an open cross-language decision, not yet implemented.

Codegen options flow through `golang.GenerateWithOptions(file, golang.Options{...})`; `Generate(file)` uses the 0.5.0 defaults. The TypeScript emitter mirrors this with `typescript.GenerateWithOptions` (`--ts-runtime-import` sets the runtime module specifier).

## Reference docs

`docs/WIRE_FORMAT.md` (spec) · `docs/MIGRATION.md` (0.5.0 breaking changes) · `docs/MUTATION.md` (mutation testing) · `docs/SECURITY.md` · `AGENTS.md` (per-language code style + raw build commands).

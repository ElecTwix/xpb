# Mutation testing

Mutation testing answers a question coverage cannot: **does the test suite have
teeth?** A mutation tester makes small, behaviour-changing edits to the source
(a `>` becomes `>=`, a `+` becomes `-`, a `!=` becomes `==`) and re-runs the
tests. If a test fails, the mutant is *killed* — some test actually depended on
that behaviour. If every test still passes, the mutant *survived* — that line of
logic is exercised but never asserted on, a blind spot.

This repo scopes mutation testing to the two hot correctness surfaces:

- `runtime/go/xpb` — the wire encode/decode runtime (bounds checks, cursor math,
  compact-length 0xFF path, `ReadArrayCount` validation, coalesced-run helpers).
- `pkg/codegen/golang` — the Go code emitter (presence flags, map element
  sizing, type rendering, capacity hints).

## Run it

```sh
make mutate
```

`make mutate` is **not** part of `make verify` or `make ci`: mutation runs are
slow and we do not gate CI on a hard score. It only measures and prints the
score.

It uses [gremlins](https://github.com/go-gremlins/gremlins) if present (on
`PATH` or in `$(go env GOPATH)/bin`), falls back to
[go-mutesting](https://github.com/zimmski/go-mutesting), and otherwise prints
install guidance. To install the preferred tool:

```sh
make mutate-install   # go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
```

### Why `--timeout-coefficient` is required

gremlins derives each mutant's per-test timeout from the baseline test run. This
suite is very fast (~0.3s per package), and that derived timeout is **shorter
than Go's per-mutant recompile**. With the default coefficient every mutant
spuriously reports `TIMED OUT` and the score reads `0.00%`:

```
Killed: 0, Lived: 0, Not covered: 1
Timed out: 145, ...
Test efficacy: 0.00%
```

`make mutate` therefore passes `--timeout-coefficient 30 --workers 4` (see the
`GREMLINS_FLAGS` variable in the `Makefile`). Bump the coefficient further on a
slower machine if you still see spurious timeouts.

## Reading the score

gremlins reports two numbers:

- **Test efficacy** = `killed / (killed + lived)`. Of the mutants a test *could*
  have caught, what fraction did it catch? This is the headline "do the tests
  have teeth" number.
- **Mutator coverage** = `(killed + lived) / total`. What fraction of mutants
  were even reachable by the tests? A `NOT COVERED` mutant is a coverage gap (no
  test runs that line), distinct from a `LIVED` survivor (the line runs but
  nothing asserts on the mutated behaviour).

`NOT COVERED` and `NOT VIABLE` (the mutation does not compile) do not count
against efficacy.

## Score: before and after (ticket T-13)

Measured with `--timeout-coefficient 30 --workers 4`.

| Package | Before (efficacy / survivors) | After (efficacy / survivors) |
|---|---|---|
| `runtime/go/xpb` | 88.97% — 16 lived, 1 not covered | **100.00%** — 0 lived, 0 not covered |
| `pkg/codegen/golang` | 85.71% — 7 lived, 1 not covered | **98.00%** — 1 lived (equivalent, accepted), 0 not covered |

The killing tests live in `runtime/go/xpb/mutation_kill_test.go` and
`pkg/codegen/golang/mutation_kill_test.go`. They are ordinary `go test` tests
(run by `make verify`); each pins the exact boundary or operator a survivor
flipped.

### Survivors killed — `runtime/go/xpb`

| Site | Mutation | Killed by |
|---|---|---|
| `writeCompactLength` `<= 254` | boundary `<=`→`<` | compact-length 254 stays a single byte; 255 trips the 0xFF marker |
| `readCompactLength` `pos+4 > len` | boundary `>`→`>=` | a 4-byte length tail that ends exactly at EOF decodes |
| `Skip` `n < 0` / `pos+n > len` | boundary | `Skip(0)` is a no-op; skip-to-exact-end succeeds, one past fails |
| `ReadArrayCount` / `ReadArrayCountAt` `maxElements < 0`, `n < 0` | boundary `<`→`<=` | `maxElements==0` admits `count==0` |
| `ReadArrayCount` / `ReadArrayCountAt` buffer-bound `Remaining()/elem`, `(len-p)/elem` | arithmetic `/`→`*`, `-`→`+`, sign | with 8 bytes left and elem=4, count 2 passes and 3 is rejected (incl. a non-zero starting cursor to pin `len-p`) |
| `ReadInt64At` / `ReadUint32At` / `ReadUint64At` / `ReadFloat32At` / `ReadFloat64At` `p+w > len` | boundary `>`→`>=` | a buffer holding exactly one value (`p+w == len`) decodes |
| `RunBoolAt` `b[p] != 0` | negation `!=`→`==` | `0x01`→true, `0x00`→false |
| `ExtendRun` `off+n` | arithmetic (was also *not covered*) | returned offset is the old length and the slice grows by exactly `n`; pre-run bytes preserved |

### Survivors killed — `pkg/codegen/golang`

The emitter produces source text, so each test asserts on the generated
substring the mutant would change.

| Site | Mutation | Killed by |
|---|---|---|
| `generateMarshalBody` `lb > 0` | boundary `>`→`>=` | a variable-only message emits no `GrowBuf(buf, 0)`; a fixed field emits `GrowBuf(buf, 4)` |
| map decode `keyMin+valMin` | arithmetic on `+` | `map<int32,int32>` decode passes elementMinBytes `8` to `ReadArrayCountAt` |
| `goBaseTypeName` enum detect `Kind == TypeMessage && enums[...]` | negation `==`→`!=` | an enum map value renders as `map[string]Color`, not `map[string]*Color` |
| `toCamelCase` `len(parts[i]) > 0` | boundary `>`→`>=` | a `foo__bar` field name camel-cases to `FooBar` instead of panicking on the empty part |
| `messageHasMapField` `Kind == TypeMap` | negation `==`→`!=` | the map-nondeterminism NOTE appears for a message with a map and not for one without |
| `estimateSize` `size < 64` | negation `<`→`>=` | a tiny message still hints `NewEncoder(64)` |
| optional message presence (was *not covered*) | — | an optional `*T` field emits `AppendBoolTo(buf, m.Inner != nil)` then a non-nil-guarded body |

### Accepted survivor (equivalent mutant)

`pkg/codegen/golang/emitter.go` `estimateSize`: the `CONDITIONALS_BOUNDARY`
mutant `size < 64` → `size <= 64` is **equivalent** and is intentionally not
killed. The two branches differ only at `size == 64`, where the original returns
`size` (64) and the mutant returns the literal `64` — the same value. No test
can distinguish them because no observable behaviour changes. (The sibling
`CONDITIONALS_NEGATION` mutant `< 64` → `>= 64` at the same line *is* real and is
killed by `TestKill_EstimateSize_FloorIs64`.)

This leaves `pkg/codegen/golang` at 98.00% efficacy with one accepted equivalent
survivor, which is the correct end state — chasing an equivalent mutant to 100%
would require a contrived, behaviour-free assertion.

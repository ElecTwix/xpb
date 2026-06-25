# Changelog

All notable changes to xpb are documented here. Versions follow semantic
versioning; while pre-1.0, breaking changes bump the minor version.

## [0.5.0]

### Changed — BREAKING (Go codegen source/semantics; wire format UNCHANGED)

The Go code generator now emits the **fast path by default**. These are
source-level and decode-time changes only — the wire format is byte-identical,
so the cross-language conformance golden vectors are unchanged and data encoded
by older xpb versions still decodes. Both breaks fail at Go compile time (or at
an explicit aliasing-safety boundary), which is intended. See
[docs/MIGRATION.md](docs/MIGRATION.md) for before/after migration steps.

- **Optional scalar/string/bytes/enum fields now default to the VALUE style**
  (`m.X T` plus a generated `m.HasX bool`) instead of a pointer (`m.X *T`).
  `golang.Generate()` with a zero-value `Options{}` — and `xpbc` with no
  optional-style flag — now emit value-style optionals. Non-enum message
  optionals stay `*T`.
  - Migration: a call site that did `*m.X` (deref) must become `m.X`, and one
    that did `if m.X != nil` must become `if m.HasX`. Setting a field changes
    from `m.X = &v` to `m.X, m.HasX = v, true`. These fail to compile against
    the new structs, which surfaces every site that must change.
  - Opt out with `xpbc --go-optional-style=pointer` (or
    `golang.Options{OptionalStyle: golang.OptionalPointer}`) to keep `*T`.
- **`bytes` fields now decode ZERO-COPY by default**: the decoded `[]byte`
  ALIASES the decoder's input buffer (`ReadBytesUnsafe`) instead of copying it
  (`ReadBytes`). Decoded `string` fields already aliased the input; `bytes` now
  matches. `golang.Generate()` with a zero-value `Options{}` — and `xpbc` with
  no bytes flag — now emit the aliasing decode.
  - Migration: a caller that retains decoded `[]byte` (or `string`) past the
    point where the source buffer is reused or mutated must now clone it
    (`append([]byte(nil), b...)`) **or** opt out of zero-copy. This does NOT
    fail at compile time — it is a runtime aliasing hazard — so audit callers
    that hold decoded slices across buffer reuse.
  - Opt out with `xpbc --go-safe-bytes` (or `golang.Options{SafeBytes: true}`)
    to get a copying decode whose `[]byte` owns its memory.
- The previously-unreleased `golang.Options.ZeroCopyBytes bool` (zero-copy as an
  opt-IN) is **replaced** by `golang.Options.SafeBytes bool` (copy as an
  opt-OUT), so the `Options{}` zero value selects the fast path with no
  Go-bool "unset vs false" ambiguity. The corresponding `xpbc` flag is renamed
  from `--go-zero-copy-bytes` to `--go-safe-bytes`. (`ZeroCopyBytes` /
  `--go-zero-copy-bytes` never shipped in a release.)
- All committed generated Go (`testdata/gen/*.xpb.go`,
  `benchmarks/go/uteka/val/*`) is regenerated under the new defaults;
  `benchmarks/go/uteka/ptr/*` is regenerated with the explicit
  `--go-optional-style=pointer --go-safe-bytes` opt-out so it remains the
  pointer + copying variant the differential/aliasing tests compare against.

## [Unreleased]

### Changed

- **Generated Go decode now threads a register-local cursor** instead of the
  stateful `Decoder.pos` struct field. The `unmarshalAt` body declares a local
  `pos int` and advances it through new stateless `xpb.*At` runtime helpers
  (`ReadInt32At`, `ReadStringAt`, `ReadBytesAt`/`ReadBytesUnsafeAt`,
  `ReadArrayCountAt`, etc.) rather than constructing `xpb.NewDecoder` and
  calling per-field methods that reload/store `pos` and `len(buf)` through
  memory. This is a generated-code performance improvement only: the wire
  format is byte-identical, decode stays 0 allocations, and the stateful
  `Decoder` API is unchanged for streaming/manual callers.
- **Generated Go encode now threads a register-local buffer** instead of the
  stateful `Encoder.buf` struct field. `Marshal`/`MarshalTo` bind a local
  `buf := enc.Buf()`, grow it once up front by the message's fixed-size lower
  bound (`xpb.GrowBuf`), append each field into the local via the stateless
  `xpb.Append*To` helpers, then write the local back with `enc.SetBuf(buf)`
  exactly once — instead of the per-field `enc.Write*` calls that each did
  `enc.buf = append(enc.buf, ...)`, reloading/storing the 3-word slice header
  through memory every field. This is the symmetric encode counterpart of the
  decode cursor change: the wire format is byte-identical, pooled encode stays
  0 allocations, the nested-message envelope and pooling semantics are
  unchanged, and the stateful `Encoder` API (`NewEncoder`/`GetEncoder`/`Write*`/
  `MarshalTo`) is preserved for manual callers and the pool. On Apple M5 the
  value-style pooled encode drops from ~24 ns to ~13 ns/op (~1.9x, 0 allocs).
- **Generated Go decode/encode now coalesce contiguous fixed-width field runs.**
  A maximal run of two or more consecutive fields whose wire encoding is
  fixed-width and unconditional (bool/int32/uint32/float32/enum = 1/4 bytes,
  int64/uint64/float64 = 8 bytes; never optional, repeated, map, string, bytes,
  or nested message) is one contiguous little-endian byte region, so it is now
  bounds-checked once on decode (`xpb.EnsureRunAt`, then unchecked `xpb.Run*At`
  reads at known offsets) and grown once on encode (`xpb.ExtendRun`, then
  unchecked `xpb.Put*At` writes at known offsets), instead of one bounds
  check / capacity check per field. Runs of length 1 keep the per-field
  `*At`/`Append*To` path unchanged (coalescing one field buys nothing). The
  wire format is byte-identical, decode stays 0 allocations and pooled encode
  stays 0 allocations, and a truncated input that ends partway through a run is
  rejected by the single up-front `EnsureRunAt` exactly as the per-field path
  rejected a short field (covered by a new mid-run truncation test + fuzz
  seeds). The gain scales with run length: messages dominated by long
  fixed-width runs (e.g. all-scalar structs) collapse N per-field checks into
  one; the `uteka` benchmark message has only a single 2-field run
  (`Seq`+`Flags`) buried among optional/string fields, so its decode/encode are
  flat-to-slightly-faster (~9.0→~8.9 ns decode, ~12.7→~13.0 ns encode on Apple
  M5, both within run-to-run noise and still 0 allocs). Repeated fixed-width
  primitive arrays (`[]int32`/`[]float64`/…) bulk-`memmove` was scoped but
  **deferred**: it requires an `unsafe` slice reinterpret plus a big-endian
  fallback, which is not worth the correctness risk for this optional polish
  phase and is not exercised by the benchmark.

### Added

- Stateless cursor append helpers in `runtime/go/xpb` (`AppendBoolTo`,
  `AppendInt32To`, `AppendInt64To`, `AppendUint32To`, `AppendUint64To`,
  `AppendFloat32To`, `AppendFloat64To`, `AppendCompactLengthTo`,
  `AppendStringTo`, `AppendBytesTo`, `AppendMessageTo`), plus `GrowBuf` and the
  `(*Encoder).Buf`/`(*Encoder).SetBuf` accessors: the register-local-buffer
  counterparts of the `Encoder.Write*` methods, mirroring the
  `binary.LittleEndian.Append*` style, with identical little-endian fixed-width
  layout and compact-length (`0xFF`) framing. Added alongside the unchanged
  stateful `Encoder` API; threaded through generated `Marshal`/`MarshalTo`.
- Stateless cursor read helpers in `runtime/go/xpb` (`ReadBoolAt`,
  `ReadInt32At`, `ReadInt64At`, `ReadUint32At`, `ReadUint64At`,
  `ReadFloat32At`, `ReadFloat64At`, `ReadStringAt`, `ReadBytesAt`,
  `ReadBytesUnsafeAt`, `ReadMessageBytesAt`, `ReadArrayCountAt`): the
  register-local-cursor counterparts of the `Decoder.Read*` methods, with
  identical bounds, compact-length (`0xFF`), negative-length, and array-count
  validation. Added alongside the unchanged stateful `Decoder` API.
- Coalesced fixed-width run helpers in `runtime/go/xpb`: `EnsureRunAt` (one
  up-front bounds check for a whole fixed-width run) and the unchecked offset
  readers `RunBoolAt`/`RunInt32At`/`RunInt64At`/`RunUint32At`/`RunUint64At`/
  `RunFloat32At`/`RunFloat64At`; `ExtendRun` (grow the local encode buffer once
  by a run width, returning the run's base offset) and the unchecked offset
  writers `PutBoolAt`/`PutInt32At`/`PutInt64At`/`PutUint32At`/`PutUint64At`/
  `PutFloat32At`/`PutFloat64At`. The `Run*`/`Put*` accessors carry no per-field
  bounds/capacity check and are valid only inside a window already guarded by
  `EnsureRunAt` / extended by `ExtendRun`; all stay inlinable (guarded by
  `TestInliningGuard_HotHelpers`).
- `--go-optional-style` flag on `xpbc` (and `golang.Options.OptionalStyle`)
  selecting the optional scalar/string/bytes/enum representation: `value` (a
  value field plus a `Has<Field>` bool, the 0.5.0 default — see the BREAKING
  notes above) or `pointer` (`*T`, the opt-out). Non-enum message optionals
  stay `*T`. The wire format is identical between styles.
- `--go-safe-bytes` flag on `xpbc` (and `golang.Options.SafeBytes`): opts OUT of
  the 0.5.0 zero-copy bytes default, decoding `bytes` fields by copying
  (`ReadBytes`) so the decoded `[]byte` owns its memory. Default (off) decodes
  by aliasing the input buffer (`ReadBytesUnsafe`).
- `benchmarks/go/uteka/`: a realistic control-plane RPC message benchmark
  (`UTEKA_MESSAGE`) comparing both XPB codegen styles against JSON and msgpack.
  On Apple M5 the value+zero-copy style decodes in 14.5 ns / 0 allocs vs the
  default pointer style's 51 ns / 4 allocs (and vs JSON's ~1000 ns / 10 allocs).

## [0.4.0] - 2026-06-09

### Changed — BREAKING (wire format)

- **Optional (`?`) fields are now encoded with a 1-byte presence flag**:
  `0x00` when absent (no value bytes follow) or `0x01` followed by the value
  when present. Previously the Go, TypeScript, C, Java, and Lua generators
  wrote the value unconditionally with no presence indicator, so an absent
  optional was indistinguishable from a zero value and desynced every
  following field; only Rust wrote a presence flag. All six runtimes are now
  unified behind this spec. **Data containing optional fields produced by the
  pre-0.4.0 non-Rust generators is incompatible** and must be re-encoded.
  See `docs/WIRE_FORMAT.md` → "Optional Fields".

### Added

- Cross-language conformance suite: shared golden byte vectors generated from
  the Go reference encoder (`testdata/conformance/`), decoded/verified/
  re-encoded with byte-identity by all six runtimes (Go, Rust, TypeScript, C,
  Lua, Java).
- C runtime fuzzing (libFuzzer) plus AddressSanitizer/UBSan coverage of the
  existing C tests and conformance harness.
- Go and Rust decoder hardening: native fuzzing, malformed-input cases,
  property-based round-trips, float bit-pattern edges (NaN, -0.0, inf),
  unsafe-aliasing contract tests, and encoder-pool race tests.
- Real code-generation verification: generated Go is compiled and round-tripped
  in a throwaway module; generated TypeScript is type-checked with `tsc` and
  round-tripped with `bun` (replacing substring-only checks).
- `cmd/ci`: a single Go local-CI runner that runs the full multi-language suite
  (`go run ./cmd/ci`) and an optional `--install-hook` pre-push gate.
- `--ts-runtime-import` flag on `xpbc` (and `typescript.GenerateWithOptions`):
  overrides the module specifier in the generated TypeScript runtime import
  (`from '@xpb/runtime'`). Lets projects emit a vendored/relative runtime path
  directly instead of post-processing the output. Defaults to `@xpb/runtime`,
  so existing behavior is unchanged.

### Fixed

- C decoder: signed-integer-overflow UB in `read_le32`/`read_le64` (`byte << 24`
  with the high bit set), caught by UBSan on `0xFF`-prefixed values.
- `Decoder.Skip(n)` (Go): missing `n < 0` guard caused a negative position and a
  panic on the next read.
- Go codegen: a message with no fields generated an `unmarshalAt` that declared
  `dec := xpb.NewDecoder(data)` but never used it, which Go rejects at compile
  time (`declared and not used`). Bodyless messages now emit `_ = data` instead.
  Covered by a real compile + round-trip test in `tests/integration`.

### Note

- The Rust crate version, previously stranded at 0.1.0, is synced to 0.4.0.

## [0.3.0] and earlier

Released and tagged in git (`v0.1.0`, `v0.2.0`, `v0.3.0`); see the git history.
`v0.3.0` includes the comprehensive cross-language security-hardening audit
(array-count bounds, recursion-depth caps, runtime + codegen hardening).

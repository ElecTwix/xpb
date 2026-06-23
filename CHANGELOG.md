# Changelog

All notable changes to xpb are documented here. Versions follow semantic
versioning; while pre-1.0, breaking changes bump the minor version.

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

### Note

- The Rust crate version, previously stranded at 0.1.0, is synced to 0.4.0.

## [0.3.0] and earlier

Released and tagged in git (`v0.1.0`, `v0.2.0`, `v0.3.0`); see the git history.
`v0.3.0` includes the comprehensive cross-language security-hardening audit
(array-count bounds, recursion-depth caps, runtime + codegen hardening).

# XPB Security Model

The XPB runtimes and codegens have been hardened against the standard
class of decode-bomb attacks: small adversarial payloads that decode into
huge allocations. This document describes the contract every caller must
honor, the defense-in-depth layers the runtime provides, and the breaking
API changes that landed during the hardening pass.

## Threat model

An attacker controls the bytes passed to a `Decoder` and the
element-count fields embedded in those bytes. The decoder must not:

- Allocate memory proportional to an attacker-supplied count without an
  application-level budget.
- Read past the end of the supplied buffer.
- Recurse without bound on nested-message fields.
- Crash on malformed input — failures must be reportable to the caller.

## The `readArrayCount` contract

Every runtime's `Decoder.readArrayCount` (Go: `ReadArrayCount`,
Lua: `read_array_count`, etc.) takes **two** arguments:

| Parameter | Meaning |
|---|---|
| `elementMinBytes` | smallest possible on-wire size of one element. Pass 4 for `int32`, 1 for `bool`/variable-length. Pass 0 to disable the buffer bound (only safe for fully trusted input). |
| `maxElements` | **caller-supplied** hard cap. The runtime never picks a default. |

The validation order is fail-closed:

1. Negative wire counts are rejected.
2. Wire counts above `maxElements` are rejected.
3. Wire counts that cannot fit in the remaining buffer at
   `elementMinBytes` per element are rejected.

All three rejections produce errors with distinctive messages
(`negative array count`, `exceeds caller-supplied max`,
`exceeds buffer-bounded max`) so callers can route on them.

### Why explicit max?

A buffer-bound-only check (the original design) lets a 16 MB buffer
authorize 16 M one-byte string allocations or 4 M four-byte ints. That's
enough memory to OOM a small host. The explicit max forces every call
site to declare its policy: a streaming RPC handler picks something
small, a batch ingest path picks something larger.

The generated code passes
[`pkg/codegen/common.DefaultMaxElements`][common] (currently
`1 << 24` ≈ 16 M) as the max — application code that needs a tighter cap
should edit the generated source or call the runtime helpers directly.

[common]: ../pkg/codegen/common/common.go

## The `MaxDecodeDepth` contract

Every runtime defines `MaxDecodeDepth = 64`. The generated
`unmarshalAt(data, depth)` (or `unmarshal_at`, or `UnmarshalAt`) shims
increment `depth` on every nested-message call and refuse at the cap.
Without this, a self-referential schema with a deeply-nested adversarial
payload triggers stack overflow.

The public entry point — `Unmarshal(data)` / `unmarshal(data)` /
`decode(data)` — delegates to the depth-threaded helper with `depth=0`.

## The sticky-error model

The C runtime mirrors the contract on both sides:

- `Decoder`: once any read overflows or fails an allocation, an internal
  sticky-error flag is latched and every subsequent read returns
  `0`/`NULL` without side effects. Use `xpb_decoder_ok()` to test before
  trusting the values.
- `Encoder`: any internal `malloc`/`realloc` failure latches the same
  flag. Every subsequent `write_*` is a no-op, and `xpb_encoder_finish`
  returns `NULL` with `*out_len = 0`. Use `xpb_encoder_ok()` to verify
  before consuming the returned buffer.

This pattern means the caller can write a sequence of operations and
only check for failure at the end, instead of after every primitive.

## Breaking API changes (from the v2 hardening pass)

If you're upgrading from a pre-hardening checkout, the following
signatures changed. Every change adds an explicit `maxElements` (or
equivalent) parameter that the caller must supply.

### Go

```go
// before
func (d *Decoder) ReadArrayCount(elementMinBytes int) (int32, error)

// after
func (d *Decoder) ReadArrayCount(elementMinBytes, maxElements int) (int32, error)
```

### TypeScript

```ts
// before
readArrayCount(elementMinBytes: number): number
readArrayInt32(): number[]
// ...

// after
readArrayCount(elementMinBytes: number, maxElements: number): number
readArrayInt32(maxElements: number): number[]
// ...
```

`StringArrayView` constructor:

```ts
// before
new StringArrayView(buffer, startOffset?)

// after
new StringArrayView(buffer, maxElements, startOffset?)
```

### Java

```java
// before
public int[] readArrayInt32()
// ...

// after
public int readArrayCount(int elementMinBytes, int maxElements)
public int[] readArrayInt32(int maxElements)
// ...
```

### Lua

```lua
-- before
dec:read_array_int32()

-- after
dec:read_array_count(element_min_bytes, max_elements)
dec:read_array_int32(max_elements)
```

### C

```c
/* before */
int32_t* xpb_decoder_read_array_int32(struct xpb_decoder*, size_t* out);

/* after */
bool xpb_decoder_validate_array_count(struct xpb_decoder*,
    size_t element_min_bytes, size_t max_elements, size_t* out);
int32_t* xpb_decoder_read_array_int32(struct xpb_decoder*,
    size_t max_elements, size_t* out);
```

Encoder also gained `bool xpb_encoder_ok(const struct xpb_encoder*)`.

### Rust

```rust
// new
impl Decoder {
    pub fn read_array_count(
        &mut self,
        element_min_bytes: usize,
        max_elements: usize,
    ) -> Result<usize>;
}
```

### Generated code (all languages)

- Recursive message decoders now run through a `unmarshal_at(depth)`
  shim and refuse at `MaxDecodeDepth = 64`.
- Repeated and map fields now go through `readArrayCount` with an
  explicit max constant (`common.DefaultMaxElements = 1 << 24`).
- Schemas containing `optional` fields emit a `// NOTE: ...` warning
  because the V2 wire format has no presence bit; callers must agree on
  a sentinel value or wait for V3.
- The generated TypeScript class constructor uses explicit per-field
  assignment instead of `Object.assign`, eliminating a prototype-pollution
  sink for callers who feed `JSON.parse(userInput)` directly into
  `new MessageX(...)`.

## Audit traceability

Each hardening change is covered by a regression test in
`tests/integration/security_audit_test.go`. Each test names the original
finding (`XPB-001` … `XPB-121`) in its docstring so a reader can trace
backward from the test to the audit-time write-up. A test that starts
failing means the hardening was rolled back; the test's `REGRESSION:`
failure message tells the bisecter exactly what.

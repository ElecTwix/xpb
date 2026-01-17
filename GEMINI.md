# XPB V2 Technical Reference

## Wire Format Specification

### Integer Encoding

| Type | Size | Endian | Range |
|------|------|--------|-------|
| int32 | 4 bytes | Little | -2^31 to 2^31-1 |
| int64 | 8 bytes | Little | -2^63 to 2^63-1 |
| uint32 | 4 bytes | Little | 0 to 2^32-1 |
| uint64 | 8 bytes | Little | 0 to 2^64-1 |

Floats use IEEE 754:
- float32: 4 bytes
- float64: 8 bytes

### Compact Length Encoding

Length prefix for strings, bytes, and messages:

```
Length 0-254:  [length_byte]              // 1 byte
Length 255+:   [0xFF] [len_u32_le]        // 5 bytes total
```

### Message Structure

Messages encode fields in declaration order without tags:

```xpb
message User {
    1: string name    // length-prefixed string
    2: int32 age      // 4 bytes
    3: bool active    // 1 byte
}
```

Encoded bytes for `name="Alice", age=30, active=true`:
```
06 41 6C 69 63 65  1E 00 00 00  01
|  |__________|   |________|  |__|
|       |            |          |
|       |            |          +-- bool (1 byte)
|       |            +-- int32 (4 bytes LE = 0x0000001E)
|       +-- length (6) + "Alice" (5 bytes)
+-- length byte
```

### Repeated Fields (Arrays)

Arrays encode count followed by elements:

```
[int32 count] [element_0] [element_1] ... [element_n-1]
```

Example: `["go", "ts"]`
```
02 02 67 6F  02 74 73
|  |__| |____| |__| |__|
|    |    |     |    |
|    |    |     |    +-- "ts" (2 bytes)
|    |    |     +-- length 2 + "go"
|    |    +-- count = 2
|    +-- count = 2
```

### Map Fields

Maps encode count followed by key-value pairs:

```
[int32 count] [key_0] [value_0] [key_1] [value_1] ...
```

## Error Handling

Go runtime returns standard library errors:
- `io.ErrUnexpectedEOF` on incomplete data
- Custom errors: `xpb.ErrBufferTooSmall`, `xpb.ErrInvalidData`

TypeScript runtime throws `Error` with `xpb:` prefix:
- `Error('xpb: unexpected EOF reading int32')`

## Performance Characteristics

### Memory Allocation

Go:
- `NewEncoder`/`NewDecoder`: allocates new buffer
- `GetEncoder`/`PutEncoder`: uses `sync.Pool`
- Zero-copy decode via `unsafe.String`

TypeScript:
- Encoder grows buffer 2x when capacity exceeded
- Buffer.transfer() used when available (zero-copy resize)

### Benchmarking Guidelines

Run benchmarks with:
```bash
go test -bench=. -benchmem -count=1 ./benchmarks/go
```

Key metrics:
- ns/op: nanoseconds per operation
- B/op: bytes allocated per operation
- allocs/op: allocation count per operation

## Comparison with Other Formats

### Byte Layout Comparison (Small Message)

Message: `{name: "A", age: 30, active: true}`

| Format | Bytes | Notes |
|--------|-------|-------|
| XPB V2 | 11 | No field tags, sequential |
| Protobuf | 19 | Field tags (1 byte each) |
| MessagePack | 33 | Type indicator per value |
| JSON | 47 | Key names, whitespace |

### Encoding Speed Factors

1. XPB advantages:
   - No reflection/lookup
   - Sequential memory access
   - Minimal bounds checking in hot paths

2. XPB disadvantages:
   - No schema evolution support
   - No forward compatibility
   - Order-sensitive decoding

## Platform-Specific Optimizations

### Go

- `binary.LittleEndian.AppendUint32/64` for zero-allocation writes
- `unsafe.String` for zero-copy string reads
- `sync.Pool` for encoder/decoder reuse

### TypeScript

- Manual ASCII decoding for short strings (<64 chars)
- `TextEncoder` fallback for UTF-8
- Native `Uint8Array.fromBase64` when available (Chrome 133+)
- Buffer.transfer() when available for zero-copy resize

### Browser

- `setFromBase64` for zero-allocation Base64 writes
- Zero-copy accessors via `XPB.compileAccessor`

## File References

- Encoder/Decoder: `runtime/go/xpb/xpb.go`, `runtime/ts/src/index.ts`
- Parser: `pkg/parser/parser.go`
- Wire constants: `pkg/wire/wire.go`
- Benchmarks: `benchmarks/go/`, `docs/BENCHMARKS.md`

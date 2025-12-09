# XPB V2 - High-Performance Binary Serialization

A speed-optimized binary serialization format that beats JSON and Protobuf.

## Supported Platforms

| Platform    | Status          | Runtime              |
| :---------- | :-------------- | :------------------- |
| **Go**      | ✅ Full Support | `runtime/go/xpb`     |
| **Node.js** | ✅ Full Support | `runtime/ts/src`     |
| **Browser** | ✅ Full Support | `benchmarks/browser` |

## Features

- **Faster than JSON** - 3-5x faster encode, 1.4-2.9x faster decode
- **Smaller than JSON** - 2.5x smaller payloads
- **Multi-platform** - Go, Node.js, and Browser support
- **JIT Compilation** - Runtime-generated optimized encoders/decoders
- **V2 Wire Format** - Struct mode, fixed-width integers, compact lengths

## Performance (Best of 5 Rounds)

### Node.js

| Format     |    Encode |     Decode |     Size |
| :--------- | --------: | ---------: | -------: |
| **XPB V2** | **28 ns** | **133 ns** | **19 B** |
| JSON       |    149 ns |     378 ns |     47 B |

**XPB is 5.4x faster encode, 2.9x faster decode vs JSON**

### Browser (Chromium)

| Format     |    Encode |     Decode |     Size |
| :--------- | --------: | ---------: | -------: |
| **XPB V2** | **22 ns** | **137 ns** | **19 B** |
| JSON       |     81 ns |     194 ns |     47 B |

**XPB is 3.7x faster encode, 1.4x faster decode vs JSON**

### Go

| Format     |    Encode |    Decode |     Size |
| :--------- | --------: | --------: | -------: |
| **XPB V2** | **50 ns** | **40 ns** | **19 B** |
| Protobuf   |    169 ns |    247 ns |     19 B |
| JSON       |    259 ns |  1,501 ns |     47 B |

## Quick Start

```bash
# Build compiler
go build -o xpbc ./cmd/xpbc

# Generate code
./xpbc --lang=go,ts schema.xpb

# Run all benchmarks
./benchmarks/run-all.sh
```

### Schema Example

```xpb
package myapp

enum Status { ACTIVE = 1 }

message User {
    1: string name
    2: int32 age
    3: []string tags
    4: Status status
}
```

## V2 Wire Format

- **Struct Mode** - No field tags, fields in declaration order
- **Fixed-Width Integers** - 4 bytes for int32, 8 bytes for int64
- **Compact Lengths** - 1 byte if < 255, else 5 bytes
- **Little-Endian** - All multi-byte values

## License

MIT

# XPB V2 - High-Performance Binary Serialization

A speed-optimized binary serialization format that beats JSON and Protobuf.

## Supported Platforms

| Platform    | Status          | Runtime              |
| :---------- | :-------------- | :------------------- |
| **Go**      | ✅ Full Support | `runtime/go/xpb`     |
| **Node.js** | ✅ Full Support | `runtime/ts/src`     |
| **Browser** | ✅ Full Support | `benchmarks/browser` |

## Features

- **Blazing Fast** - Up to **5x faster encode** and **39x faster decode** than JSON
- **Tiny Payloads** - Up to **90% smaller** than JSON
- **Zero-Copy Decode** - (Go) Direct memory access for ultimate performance
- **Multi-platform** - Go, Node.js, and Browser support
- **JIT Compilation** - Runtime-generated optimized encoders/decoders for JS
- **V2 Wire Format** - Struct mode, fixed-width integers, compact lengths

## Performance (2025 Benchmarks)

Benchmarks run on Linux (Intel Core i9-13900H).

### 🏆 Executive Summary

| Platform    |     XPB Encode vs JSON     |   XPB Decode vs JSON   |   Size Savings    |
| ----------- | :------------------------: | :--------------------: | :---------------: |
| **Go**      |   ✅ **3.8-5.3x faster**   |  ✅ **14-39x faster**  | ✅ 37-90% smaller |
| **Node.js** |   ✅ **1.7-5.1x faster**   | ✅ **1.6-3.5x faster** | ✅ 37-90% smaller |
| **Browser** | ✅ **4.7x faster** (small) |       ~1x (tie)        | ✅ 37-90% smaller |

### 🚀 Small Message Benchmarks

| Format     | Go Encode | Go Decode | Node Encode | Node Decode |   Size   |
| :--------- | :-------: | :-------: | :---------: | :---------: | :------: |
| **XPB V2** | **40 ns** | **23 ns** |  **16 ns**  |  **61 ns**  | **19 B** |
| Protobuf   |   98 ns   |  164 ns   |   162 ns    |    86 ns    |   19 B   |
| JSON       |  153 ns   |  901 ns   |    83 ns    |   211 ns    |   47 B   |

> **Note**: For massive string arrays in JS, native JSON is currently faster. XPB excels at structured data and integers.

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

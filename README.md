# XPB - Compact Binary Serialization

A compact, protobuf-compatible binary serialization format with Go-based code generator.

## Features

- **Same size as Protobuf** - Uses identical wire format
- **Faster decode** - 2.5x faster than Protobuf in Go
- **Multi-language** - Go and TypeScript support
- **Extended types** - Repeated fields, maps, enums

## Quick Start

```bash
# Build compiler
go build -o xpbc ./cmd/xpbc

# Generate code
./xpbc --lang=go,ts schema.xpb
```

### Schema Example

```xpb
package myapp

enum Status { ACTIVE = 1 }

message User {
    1: string name
    2: int32 age
    3: []string tags       // repeated
    4: map<string,string> meta  // map
    5: Status status       // enum
}
```

## Benchmark Matrix

### Size Comparison (bytes)

| Format       |     Go | TypeScript |
| ------------ | -----: | ---------: |
| **XPB**      | **19** |     **19** |
| **Protobuf** | **19** |          - |
| Msgpack      |     37 |         33 |
| JSON         |     47 |         47 |

### Speed Comparison - Go (ns/op)

| Format   |  Encode |    Decode |
| -------- | ------: | --------: |
| **XPB**  | **100** | **52** 🏆 |
| Protobuf |     102 |       131 |
| JSON     |     151 |       849 |
| Msgpack  |     305 |       352 |

### Speed Comparison - TypeScript (ns/op)

| Format   |    Encode |     Decode |
| -------- | --------: | ---------: |
| XPB      |       724 |        394 |
| **JSON** | **85** 🏆 | **212** 🏆 |
| Msgpack  |      1246 |        350 |

> Note: V8 (Node.js) has native JSON optimization, making it faster than JavaScript-based binary parsers.

## Key Results

| Metric                 | Go               | TypeScript       |
| ---------------------- | ---------------- | ---------------- |
| XPB vs Protobuf decode | **2.5x faster**  | N/A              |
| XPB vs JSON size       | **2.5x smaller** | **2.5x smaller** |
| XPB vs Msgpack size    | **1.9x smaller** | **1.7x smaller** |

## Running Benchmarks

```bash
# All benchmarks
./benchmarks/run_all.sh

# Go only
go test -bench=. ./benchmarks/go/...

# TypeScript only
cd benchmarks/ts && npm run bench
```

## Wire Format

| Wire Type       |  ID | Used For                                 |
| --------------- | --: | ---------------------------------------- |
| Varint          |   0 | int32, int64, uint32, uint64, bool, enum |
| Fixed32         |   5 | float32                                  |
| Fixed64         |   1 | float64                                  |
| LengthDelimited |   2 | string, bytes, messages                  |

## License

MIT

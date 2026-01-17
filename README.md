# XPB V2 Binary Serialization

High-performance binary serialization for Go and TypeScript.

## Runtimes

| Platform | Location | Status |
|----------|----------|--------|
| Go | `runtime/go/xpb` | Active |
| TypeScript | `runtime/ts/src` | Active |
| Browser | `runtime/ts/src` (browser exports) | Active |

## Quick Start

```bash
# Build CLI
go build -o xpbc ./cmd/xpbc

# Generate code
./xpbc --lang=go,ts schema.xpb
```

## Go API

```go
import "github.com/anthropic/xpb/runtime/go/xpb"

// Encode
enc := xpb.NewEncoder(64)
enc.WriteString("Alice")
enc.WriteInt32(30)
data := enc.Bytes()

// Decode
dec := xpb.NewDecoder(data)
name, _ := dec.ReadString()
age, _ := dec.ReadInt32()
```

## TypeScript API

```typescript
import { Encoder, Decoder } from '@xpb/runtime'

// Encode
const enc = new Encoder(64)
enc.writeString("Alice")
enc.writeInt32(30)
const data = enc.finish()

// Decode
const dec = new Decoder(data)
const name = dec.readString()
const age = dec.readInt32()
```

## Wire Format

XPB V2 uses struct mode encoding:

- **int32**: 4 bytes, little-endian, two's complement
- **int64**: 8 bytes, little-endian, two's complement
- **uint32/uint64**: 4/8 bytes, little-endian
- **float32/float64**: 4/8 bytes, little-endian IEEE 754
- **string/bytes**: length prefix + data
  - Length < 255: 1 byte
  - Length >= 255: 0xFF marker + 4-byte length
- **bool**: 1 byte (0 or 1)

Fields are written/read in declaration order with no field tags.

## Schema Example

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

## Commands

```bash
# Run tests
go test ./...
cd runtime/ts && npm test

# Run benchmarks
go test -bench=. -benchmem ./benchmarks/go
cd runtime/ts && npm run bench

# Unified benchmark tool
go run ./cmd/xpbench
```

## Project Structure

```
xpb/
├── cmd/xpbc/           # CLI code generator
├── cmd/xpbench/        # Unified benchmark runner
├── pkg/
│   ├── ast/            # AST definitions
│   ├── parser/         # Lexer and parser
│   ├── codegen/        # Go and TypeScript generators
│   └── wire/           # Wire format constants
├── runtime/
│   ├── go/xpb/         # Go runtime
│   └── ts/src/         # TypeScript runtime
├── benchmarks/
│   ├── go/             # Go benchmarks
│   └── ts/             # Node.js benchmarks
└── tests/              # E2E tests
```

## Documentation

- [Wire Format Spec](docs/WIRE_FORMAT.md)
- [Architecture Overview](docs/ARCHITECTURE.md)
- [Benchmark Results](docs/BENCHMARKS.md)
- [Agent Guidelines](AGENTS.md)

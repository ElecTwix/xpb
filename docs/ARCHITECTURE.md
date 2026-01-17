# XPB V2 Architecture

XPB V2 is a high-performance binary serialization format with Go and TypeScript runtimes.

## Project Structure

```
xpb/
├── cmd/
│   ├── xpbc/              # CLI code generator
│   └── xpbench/           # Unified benchmark runner
├── pkg/
│   ├── ast/               # AST definitions for schema parsing
│   ├── parser/            # Lexer and parser for .xpb schema files
│   ├── codegen/           # Code generators
│   │   ├── golang/        # Go code generator
│   │   └── typescript/    # TypeScript code generator
│   └── wire/              # Wire format constants and utilities
├── runtime/
│   ├── go/xpb/            # Go runtime (Encoder/Decoder)
│   └── ts/src/            # TypeScript runtime
│       ├── index.ts       # Core Encoder/Decoder
│       ├── node.ts        # Node.js optimized runtime
│       ├── browser.ts     # Browser optimized runtime
│       ├── hybrid.ts      # Auto-selects optimal strategy
│       ├── jit.ts         # JIT-compiled encoding/decoding
│       ├── wasm.ts        # WebAssembly runtime
│       └── collections.ts # Collection utilities
├── tests/                 # End-to-end tests
└── benchmarks/            # Performance benchmarks
```

## Wire Format (V2)

### Design Goals

1. **Tagless encoding** - No field tags, fields written in declaration order
2. **Fixed-width integers** - 4 bytes for int32/uint32, 8 bytes for int64/uint64
3. **Compact lengths** - 1 byte for lengths < 255, 5 bytes (0xFF + 4 bytes) for larger
4. **Little-endian** - Consistent byte order across platforms

### Data Types

| Type    | Size    | Encoding                          |
|---------|---------|-----------------------------------|
| bool    | 1 byte  | 0x00 = false, 0x01 = true         |
| int32   | 4 bytes | Little-endian two's complement    |
| int64   | 8 bytes | Little-endian two's complement    |
| uint32  | 4 bytes | Little-endian unsigned            |
| uint64  | 8 bytes | Little-endian unsigned            |
| float32 | 4 bytes | IEEE 754 little-endian            |
| float64 | 8 bytes | IEEE 754 little-endian            |
| string  | N+1/5   | Compact length + UTF-8 bytes      |
| bytes   | N+1/5   | Compact length + raw bytes        |
| message | N+1/5   | Compact length + nested content   |

### Compact Length Encoding

```
Length < 255:  [length byte] [data...]
Length >= 255: [0xFF] [4-byte length (LE)] [data...]
```

## Go Runtime

### Core Types

```go
// Encoder writes typed values to a buffer
type Encoder struct {
    buf []byte
    pos int
}

// Decoder reads typed values from a buffer
type Decoder struct {
    data []byte
    pos  int
}
```

### Performance Optimizations

1. **sync.Pool** - Reuse Encoder/Decoder instances
2. **unsafe.Pointer** - Zero-copy operations where possible
3. **Pre-allocated buffers** - Avoid allocations for small messages

### Usage

```go
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

## TypeScript Runtime

### Core Types

```typescript
export class Encoder {
  private buf: Uint8Array;
  private view: DataView;
  private pos = 0;
}

export class Decoder {
  private data: Uint8Array;
  private view: DataView;
  private pos = 0;
}
```

### Performance Optimizations

1. **Zero-copy reads** - Return Uint8Array slices instead of copies
2. **TextEncoder caching** - Reuse TextEncoder/TextDecoder instances
3. **ASCII fast path** - Manual encoding for short ASCII strings
4. **Buffer transfer** - Use ArrayBuffer.transfer() when available

### Runtimes

| Runtime    | Use Case                    | Performance       |
|------------|-----------------------------|-------------------|
| index.ts   | Universal (Node/Browser)    | Good              |
| node.ts    | Node.js                     | Better (Buffer)   |
| browser.ts | Browser                    | Better (Web APIs) |
| hybrid.ts  | Auto-select                 | Best              |
| wasm.ts    | Large messages              | Best              |

## Code Generation

### Schema Syntax

```
package name

message MessageName {
    field_number: type field_name
}

enum EnumName {
    VALUE_ONE = 0
    VALUE_TWO = 1
}
```

### Generated Go Code

```go
type User struct {
    Name string
    Age  int32
}

func (m *User) Marshal() ([]byte, error) { ... }
func (m *User) Unmarshal([]byte) error   { ... }
```

### Generated TypeScript Code

```typescript
export interface UserData {
    name: string;
    age: number;
}

export class User {
    constructor(public data: UserData) {}
    encode(): Uint8Array { ... }
    static decode(data: Uint8Array): User { ... }
}
```

## Benchmarking

### Running Benchmarks

```bash
# Go benchmarks
go test -bench=. -benchmem ./benchmarks/go

# TypeScript benchmarks
cd runtime/ts && npm run bench

# Unified benchmarks
go run ./cmd/xpbench
```

### Key Metrics

- **Encode throughput** - MB/s for encoding
- **Decode throughput** - MB/s for decoding
- **Message size** - Encoded size vs JSON/Msgpack
- **Memory allocations** - Allocs/op for Go benchmarks

## CI/CD

### GitHub Actions

- **Test** - Runs Go and TypeScript tests with coverage
- **Lint** - Runs golangci-lint and ESLint

### Coverage Targets

- Go: > 70% line coverage
- TypeScript: > 50% line coverage (for core runtime)

## Future Work

1. **Varint encoding** - For smaller integers (1-10 bytes)
2. **Zigzag encoding** - For negative integers
3. **Streaming API** - For large messages
4. **Schema evolution** - Backwards compatibility support
5. **更多语言后端** - Rust, Python, Java

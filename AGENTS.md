# XPB V2 - Agent Guidelines

XPB V2 is a high-performance binary serialization format with Go and TypeScript runtimes. This document provides guidelines for agentic coding agents working in this repository.

## Project Structure

```
xpb/
├── cmd/xpbc/           # CLI code generator
├── cmd/xpbench/        # Unified benchmark runner
├── pkg/
│   ├── ast/            # AST definitions
│   ├── parser/         # Lexer and parser
│   ├── codegen/        # Go and TypeScript code generators
│   └── wire/           # V2 wire format constants
├── runtime/go/xpb/     # Go runtime (Encoder/Decoder)
├── runtime/ts/src/     # TypeScript runtime with JIT
├── benchmarks/
│   ├── go/             # Go benchmarks
│   ├── ts/             # Node.js benchmarks
│   └── browser/        # Browser benchmarks
└── tests/              # End-to-end tests
```

## Build Commands

### Go

```bash
# Build CLI tool
go build -o xpbc ./cmd/xpbc

# Run all tests
go test ./...

# Run single test
go test -run TestName ./...
go test -run TestName ./pkg/parser

# Run benchmarks
go test -bench=. -benchmem ./benchmarks/go
go test -bench=BenchmarkName ./benchmarks/go

# Run unified benchmark tool
go run ./cmd/xpbench
```

### TypeScript

```bash
# Build runtime
cd runtime/ts && npm run build

# Run tests
cd runtime/ts && npm test

# Run single test
cd runtime/ts && npx vitest run -t "test name"

# Run benchmarks
cd runtime/ts && npm run bench
```

## Code Style Guidelines

### Go

- **Formatting**: Run `go fmt` on all files before committing
- **Error Handling**: Use early returns with descriptive errors; prefer `fmt.Errorf` with context
- **Naming**: Use PascalCase for exported identifiers, camelCase for unexported; avoid abbreviations
- **Package Comments**: Every package must have a package-level comment
- **Tests**: Use `*testing.T` with `t.Fatalf` for errors, `t.Logf` for debug info
- **Performance**: Use `sync.Pool` for object pooling; prefer `unsafe` for zero-copy operations

```go
// Package xpb provides the XPB V2 runtime library for encoding and decoding.
package xpb

// Common errors.
var (
    ErrBufferTooSmall = errors.New("xpb: buffer too small")
    ErrInvalidData    = errors.New("xpb: invalid data")
)
```

### TypeScript

- **TypeScript Config**: Target ES2022, use strict mode, module: ESNext
- **JSDoc**: Add JSDoc comments for public APIs
- **Error Handling**: Throw `Error` with descriptive messages prefixed with `xpb:`
- **Naming**: Use PascalCase for classes, camelCase for functions/variables
- **Tests**: Use Vitest with descriptive test names

```typescript
/**
 * V2 Encoder - tagless, fixed-width, compact lengths.
 */
export class Encoder {
  /** Write bool as 1 byte */
  writeBool(v: boolean): void {
    // implementation
  }
}
```

## V2 Wire Format

XPB V2 uses:
- **Struct Mode**: No field tags, fields in declaration order
- **Fixed-Width Integers**: 4 bytes for int32, 8 bytes for int64, little-endian
- **Compact Lengths**: 1 byte if length < 255, else 0xFF + 4 bytes

## Performance Guidelines

- Go: Use `sync.Pool` for encoder/decoder reuse; prefer zero-copy methods (`ReadString`, `ReadBytesUnsafe`)
- TypeScript: Optimize for small messages (<256 bytes) with manual ASCII decoding; use native Base64 when available
- Benchmarks should use `-count=1` and `-benchmem` for accurate measurements

## Import Conventions

### Go

Standard library imports first, then third-party, then internal:

```go
import (
    "encoding/binary"
    "io"

    "github.com/vmihailenco/msgpack/v5"

    "github.com/anthropic/xpb/pkg/wire"
)
```

### TypeScript

Use ES module imports with explicit paths:

```typescript
import { Encoder, Decoder } from './index';
```

## Testing Guidelines

- Tests should verify round-trip encoding/decoding
- Include edge cases (empty strings, large values, boundary conditions)
- Log encoded sizes for debugging serialization format
- E2E tests are in `tests/e2e_test.go`

## Common Patterns

### Go Encoder/Decoder

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

### TypeScript Encoder/Decoder

```typescript
const enc = new Encoder(64);
enc.writeString("Alice");
enc.writeInt32(30);
const data = enc.finish();

const dec = new Decoder(data);
const name = dec.readString();
const age = dec.readInt32();
```

## Key Files

- `runtime/go/xpb/xpb.go` - Go Encoder/Decoder implementation
- `runtime/ts/src/index.ts` - TypeScript Encoder/Decoder
- `pkg/parser/parser.go` - Schema parser
- `pkg/wire/wire.go` - Wire format constants
- `benchmarks/go/comparison_test.go` - Performance comparisons

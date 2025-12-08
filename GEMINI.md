# XPB - Compact Binary Serialization

## Overview

XPB is a high-performance, protobuf-compatible binary serialization format with a Go-based code generator and multi-language runtime support.

## Project Structure

```
xpb/
├── cmd/xpbc/           # CLI code generator
├── pkg/
│   ├── ast/            # Abstract Syntax Tree
│   ├── parser/         # Lexer and Parser
│   ├── codegen/
│   │   ├── golang/     # Go code generator
│   │   └── typescript/ # TypeScript code generator
│   └── wire/           # Wire format utilities
├── runtime/
│   ├── go/xpb/         # Go runtime library
│   ├── ts/src/         # TypeScript runtimes
│   │   ├── index.ts    # Universal (Uint8Array)
│   │   ├── node.ts     # Node.js (Buffer optimized)
│   │   ├── browser.ts  # Browser (encodeInto optimized)
│   │   ├── ultra.ts    # Ultra-speed (pooling + fixed-size)
│   │   ├── hyper.ts    # Hyper-speed (inline + batch)
│   │   ├── wasm.ts     # WASM module (310 bytes)
│   │   └── hybrid.ts   # Auto-selects by message size
│   └── wasm/           # WAT source for WASM module
├── benchmarks/
│   ├── go/             # Go benchmarks (vs Protobuf, JSON, Msgpack)
│   └── ts/             # TypeScript benchmarks
├── testdata/           # Example .xpb schemas
└── tests/              # E2E tests
```

## Schema Language

```xpb
package myapp

enum Status { ACTIVE = 1, INACTIVE = 2 }

message User {
    1: string name
    2: int32 age
    3: optional bool active
    4: []string tags         // repeated
    5: map<string,string> meta
    6: Status status
}
```

## Wire Format

| Wire Type       |  ID | Used For                                 |
| --------------- | --: | ---------------------------------------- |
| Varint          |   0 | int32, int64, uint32, uint64, bool, enum |
| Fixed64         |   1 | float64                                  |
| LengthDelimited |   2 | string, bytes, messages                  |
| Fixed32         |   5 | float32                                  |

## TypeScript Runtime Tiers

| Runtime     |    Encode |     Decode | Best For                 |
| ----------- | --------: | ---------: | ------------------------ |
| **jit**     | **70 ns** | **114 ns** | **Peak perf, V8 only**   |
| **unsafe**  |    107 ns |     126 ns | Low overhead, trusted    |
| **ultra**   |     65 ns |      82 ns | Single messages, Node.js |
| **hyper**   | 72 ns/msg | 153 ns/msg | Batch operations         |
| **node**    |     80 ns |     125 ns | Node.js/Bun              |
| **index**   |    513 ns |     210 ns | Universal                |
| **browser** |    467 ns |     203 ns | Web browsers             |

## Commands

```bash
# Build CLI
go build -o xpbc ./cmd/xpbc

# Generate Go code
./xpbc --lang=go schema.xpb

# Generate TypeScript code
./xpbc --lang=ts schema.xpb

# Run Go tests
go test ./...

# Run TypeScript tests
cd runtime/ts && npm test

# Run benchmarks
cd benchmarks/ts && npm run bench
./benchmarks/run_all.sh
```

## Benchmark Results

### Go (vs Protobuf, JSON, Msgpack)

| Format   | Encode |    Decode | Size |
| -------- | -----: | --------: | ---: |
| **XPB**  | 100 ns | **52 ns** | 19 B |
| Protobuf | 102 ns |    131 ns | 19 B |
| JSON     | 151 ns |    849 ns | 47 B |
| Msgpack  | 305 ns |    352 ns | 37 B |

### TypeScript (Ultra mode)

| Format        |    Encode |    Decode | Size |
| ------------- | --------: | --------: | ---: |
| **XPB Ultra** | **65 ns** | **82 ns** | 19 B |
| JSON          |     81 ns |    218 ns | 47 B |

## Key Files

### Code Generator

- `cmd/xpbc/main.go` - CLI entry point
- `pkg/parser/lexer.go` - Tokenizer
- `pkg/parser/parser.go` - Parser
- `pkg/ast/ast.go` - AST types
- `pkg/codegen/golang/emitter.go` - Go generator
- `pkg/codegen/typescript/emitter.go` - TS generator

### Go Runtime

- `runtime/go/xpb/xpb.go` - Encoder/Decoder

### TypeScript Runtimes

- `runtime/ts/src/jit.ts` - JIT Compiler + Slab
- `runtime/ts/src/unsafe.ts` - Direct Uint8Array access
- `runtime/ts/src/ultra.ts` - Fastest single message
- `runtime/ts/src/hyper.ts` - Batch operations
- `runtime/ts/src/node.ts` - Node.js Buffer
- `runtime/ts/src/browser.ts` - Browser encodeInto
- `runtime/ts/src/index.ts` - Universal

## Usage Examples

### Go

```go
user := &User{Name: "Alice", Age: 30, Active: true}
data, _ := user.Marshal()

decoded := &User{}
decoded.Unmarshal(data)
```

### TypeScript (Ultra)

```typescript
import { getEncoder, releaseEncoder, UltraDecoder } from "@xpb/runtime/ultra";

const enc = getEncoder(64);
enc.writeString(1, "Alice");
enc.writeInt32(2, 30);
enc.writeBool(3, true);
const data = enc.finish();
releaseEncoder(enc);
```

### TypeScript (Hyper Batch)

```typescript
import { batchEncode, batchDecode, E, D } from "@xpb/runtime/hyper";

const data = batchEncode(
  users,
  (e, u) => {
    E.str(e, 1, u.name);
    E.i32(e, 2, u.age);
  },
  8192
);

const decoded = batchDecode(data, (d) => ({
  name: D.str(d),
  age: D.i32(d),
}));
```

## Testing

```bash
# Go tests
go test ./...
go test -v ./tests/...  # E2E tests

# TypeScript tests
cd runtime/ts && npm test

# Benchmarks
go test -bench=. ./benchmarks/go/...
cd benchmarks/ts && npx tsx src/ultra-bench.ts
```

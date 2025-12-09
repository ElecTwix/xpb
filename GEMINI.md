# XPB V2 - High-Performance Binary Serialization

## Overview

XPB V2 is a speed-optimized binary serialization format with Go and TypeScript runtimes.

**V2 Format:**

- Struct mode (no tags, fields in declaration order)
- Fixed-width integers (4/8 bytes, little-endian)
- Compact length encoding (1 byte if < 255, else 5 bytes)

## Project Structure

```
xpb/
├── cmd/xpbc/           # CLI code generator
├── pkg/
│   ├── ast/            # Abstract Syntax Tree
│   ├── parser/         # Lexer and Parser
│   ├── codegen/        # Go and TypeScript generators
│   └── wire/           # V2 wire format constants
├── runtime/
│   ├── go/xpb/         # Go runtime (Encoder/Decoder)
│   └── ts/src/         # TypeScript runtime + JIT compiler
├── benchmarks/         # Go and TypeScript benchmarks
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
    4: []string tags
    5: Status status
}
```

## Performance

### Go (V2 vs Other Formats)

| Format     |    Encode |    Decode |     Size |
| :--------- | --------: | --------: | -------: |
| **XPB V2** | **50 ns** | **40 ns** | **19 B** |
| Protobuf   |    169 ns |    247 ns |     19 B |
| JSON       |    259 ns |  1,501 ns |     47 B |

### TypeScript

| Runtime    | Encode | Decode | Size |
| :--------- | -----: | -----: | ---: |
| **JIT**    | 883 ns | 309 ns | 19 B |
| **Manual** | 827 ns | 310 ns | 19 B |
| JSON       | 150 ns | 378 ns | 47 B |

## Commands

```bash
# Build CLI
go build -o xpbc ./cmd/xpbc

# Generate Go code
./xpbc --lang=go schema.xpb

# Run Go tests and benchmarks
go test ./...
go test -bench=. ./benchmarks/go/...

# Run TypeScript tests and benchmarks
cd runtime/ts && npm test
cd benchmarks/ts && npm run bench
```

## Go Usage

```go
// Encode (V2: no field numbers)
enc := xpb.NewEncoder(64)
enc.WriteString("Alice")
enc.WriteInt32(30)
enc.WriteBool(true)
data := enc.Bytes()

// Decode (sequential order)
dec := xpb.NewDecoder(data)
name, _ := dec.ReadString()
age, _ := dec.ReadInt32()
active, _ := dec.ReadBool()
```

## TypeScript Usage

```typescript
import { Encoder, Decoder } from "@xpb/runtime";

// Encode
const enc = new Encoder(64);
enc.writeString("Alice");
enc.writeInt32(30);
enc.writeBool(true);
const data = enc.finish();

// Decode
const dec = new Decoder(data);
const name = dec.readString();
const age = dec.readInt32();
const active = dec.readBool();
```

## Key Files

- `pkg/wire/wire.go` - V2 constants (compact length)
- `runtime/go/xpb/xpb.go` - Go Encoder/Decoder
- `runtime/ts/src/index.ts` - TypeScript Encoder/Decoder
- `runtime/ts/src/jit.ts` - JIT Compiler
- `pkg/codegen/golang/emitter.go` - Go code generator

# XPB V2 - High-Performance Binary Serialization

## Overview

XPB V2 is a speed-optimized binary serialization format with runtimes for Go, Node.js, and Browser.

## Supported Platforms

| Platform    | Runtime              | JIT | Performance vs JSON      |
| :---------- | :------------------- | :-- | :----------------------- |
| **Go**      | `runtime/go/xpb`     | N/A | 5x encode, 38x decode    |
| **Node.js** | `runtime/ts/src`     | ✅  | 7x encode, 3.4x decode   |
| **Browser** | `benchmarks/browser` | ✅  | 3.6x encode, 1.4x decode |

## V2 Format

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
├── benchmarks/
│   ├── go/             # Go benchmarks
│   ├── ts/             # Node.js benchmarks
│   ├── browser/        # Browser benchmarks (Playwright)
│   └── run-all.sh      # Unified benchmark runner
└── tests/              # E2E tests
```

## Performance (Best of 5 Rounds)

### Node.js

| Format     |    Encode |     Decode |     Size |
| :--------- | --------: | ---------: | -------: |
| **XPB V2** | **21 ns** | **109 ns** | **19 B** |
| JSON       |    149 ns |     371 ns |     47 B |

### Browser (Chromium)

| Format     |    Encode |     Decode |     Size |
| :--------- | --------: | ---------: | -------: |
| **XPB V2** | **22 ns** | **138 ns** | **19 B** |
| JSON       |     79 ns |     194 ns |     47 B |

### Go

| Format     |    Encode |    Decode |     Size |
| :--------- | --------: | --------: | -------: |
| **XPB V2** | **53 ns** | **40 ns** | **19 B** |
| Protobuf   |    169 ns |    247 ns |     19 B |
| JSON       |    259 ns |  1,501 ns |     47 B |

## Commands

```bash
# Build CLI
go build -o xpbc ./cmd/xpbc

# Generate Go code
./xpbc --lang=go schema.xpb

# Run all benchmarks
./benchmarks/run-all.sh

# Run Go tests
go test ./...

# Run TypeScript tests
cd runtime/ts && npm test
```

## Go Usage

```go
enc := xpb.NewEncoder(64)
enc.WriteString("Alice")
enc.WriteInt32(30)
enc.WriteBool(true)
data := enc.Bytes()

dec := xpb.NewDecoder(data)
name, _ := dec.ReadString()
age, _ := dec.ReadInt32()
active, _ := dec.ReadBool()
```

## TypeScript Usage

```typescript
import { Encoder, Decoder } from "@xpb/runtime";

const enc = new Encoder(64);
enc.writeString("Alice");
enc.writeInt32(30);
enc.writeBool(true);
const data = enc.finish();

const dec = new Decoder(data);
const name = dec.readString();
const age = dec.readInt32();
const active = dec.readBool();
```

## Key Files

- `pkg/wire/wire.go` - V2 wire format constants
- `runtime/go/xpb/xpb.go` - Go Encoder/Decoder
- `runtime/ts/src/index.ts` - TypeScript Encoder/Decoder
- `runtime/ts/src/jit.ts` - JIT Compiler
- `benchmarks/browser/src/xpb-browser.ts` - Browser-optimized runtime

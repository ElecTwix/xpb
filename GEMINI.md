# XPB V2 - High-Performance Binary Serialization

## Overview

XPB V2 is a speed-optimized binary serialization format with runtimes for Go, Node.js, and Browser.

## Supported Platforms

| Platform    | Runtime              | JIT | Performance vs JSON        |
| :---------- | :------------------- | :-- | :------------------------- |
| **Go**      | `runtime/go/xpb`     | N/A | 5-6x encode, 20-40x decode |
| **Node.js** | `runtime/ts/src`     | ✅  | 6x encode, 3.4x decode     |
| **Browser** | `benchmarks/browser` | ✅  | 4x encode, 1x decode       |

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
│   ├── go/             # Go benchmarks (comparison, collections, size_scaling)
│   ├── ts/             # Node.js benchmarks
│   ├── browser/        # Browser benchmarks (Playwright)
│   └── run-all.sh      # Unified benchmark runner
└── tests/              # E2E tests
```

## Benchmark Coverage

The benchmark suite tests XPB V2 across multiple dimensions on all platforms:

| Benchmark Type              | Go  | Node.js | Browser |
| --------------------------- | :-: | :-----: | :-----: |
| Small Message (3 fields)    | ✅  |   ✅    |   ✅    |
| Large Message (7 fields)    | ✅  |   ✅    |   ✅    |
| String Array (100 elements) | ✅  |   ✅    |   ✅    |
| Int32 Array (100 elements)  | ✅  |   ✅    |   ✅    |
| String Map (100 entries)    | ✅  |   ✅    |   ✅    |
| Size Scaling (Tiny→XLarge)  | ✅  |   ✅    |   ✅    |
| Comparisons: JSON           | ✅  |   ✅    |   ✅    |
| Comparisons: MessagePack    | ✅  |   ✅    |   ✅    |
| Comparisons: Protobuf       | ✅  |   ✅    |    -    |

## Performance Results

### Small Message (3 fields: name, age, active)

#### Go

| Format      |    Encode |    Decode |     Size |
| :---------- | --------: | --------: | -------: |
| **XPB V2**  | **40 ns** | **23 ns** | **19 B** |
| Protobuf    |     98 ns |    164 ns |     19 B |
| JSON        |    153 ns |    901 ns |     47 B |
| MessagePack |    275 ns |    333 ns |     33 B |

#### Node.js (JIT)

| Format      |    Encode |    Decode |     Size |
| :---------- | --------: | --------: | -------: |
| **XPB V2**  | **16 ns** | **61 ns** | **19 B** |
| Protobuf    |    162 ns |     86 ns |     19 B |
| JSON        |     83 ns |    211 ns |     47 B |
| MessagePack |  1,389 ns |    304 ns |     33 B |

### Large Message (7 fields: id, name, email, age, score, active, description)

#### Go

| Format      |     Encode |     Decode |      Size |
| :---------- | ---------: | ---------: | --------: |
| **XPB V2**  | **105 ns** | **114 ns** | **121 B** |
| Protobuf    |     229 ns |     382 ns |     124 B |
| JSON        |     518 ns |   1,881 ns |     192 B |
| MessagePack |     741 ns |     715 ns |     165 B |

#### Node.js (JIT)

| Format      |     Encode |     Decode |      Size |
| :---------- | ---------: | ---------: | --------: |
| **XPB V2**  | **156 ns** | **267 ns** | **121 B** |
| Protobuf    |     548 ns |     248 ns |     124 B |
| JSON        |     258 ns |     436 ns |     192 B |
| MessagePack |   1,673 ns |     922 ns |     165 B |

### Size Scaling (XPB vs JSON)

XPB provides greatest size savings for smaller messages:

| Message Size   | XPB (B) | JSON (B) | Size Savings | Encode Speedup | Decode Speedup |
| :------------- | ------: | -------: | -----------: | -------------: | -------------: |
| Tiny (1 bool)  |       1 |       11 |    **90.9%** |           4.2x |         1,913x |
| Small (3 fld)  |      19 |       47 |    **59.6%** |           3.8x |            39x |
| Medium (8 fld) |     452 |      548 |    **17.5%** |           3.0x |            14x |
| Large (10KB)   |  10,604 |   10,982 |     **3.4%** |           4.3x |            12x |
| XLarge (50KB)  |  52,407 |   53,434 |     **1.9%** |           3.2x |             9x |

### Collection Types (100 elements, Go)

| Collection   | XPB Encode | JSON Encode |  Speedup | XPB Decode | JSON Decode |  Speedup |
| :----------- | ---------: | ----------: | -------: | ---------: | ----------: | -------: |
| String Array |     1.2 µs |      4.1 µs | **3.3x** |     3.7 µs |     17.4 µs | **4.7x** |
| Int32 Array  |     350 ns |      1.8 µs | **5.3x** |     358 ns |      9.9 µs |  **28x** |
| String Map   |     3.1 µs |     23.7 µs | **7.7x** |     9.7 µs |     42.7 µs | **4.4x** |

## Key Insights

1. **Encoding Speed**: XPB is consistently **3-5x faster** than JSON for encoding
2. **Decoding Speed**: XPB is **14-39x faster** than JSON for decoding (Go), with decode advantage increasing for smaller messages
3. **Size Efficiency**: XPB is most space-efficient for **small messages** (91% savings for tiny, 60% for small) due to eliminating field name overhead
4. **Int Arrays**: XPB excels at numeric arrays (5.3x encode, 28x decode) due to fixed-width encoding vs JSON's text representation

## Commands

```bash
# Build CLI
go build -o xpbc ./cmd/xpbc

# Generate Go code
./xpbc --lang=go schema.xpb

# Run all benchmarks
./benchmarks/run-all.sh

# Run selective benchmarks by platform
./benchmarks/run-all.sh --go              # Go only
./benchmarks/run-all.sh --nodejs          # Node.js only
./benchmarks/run-all.sh --browser         # Browser only
./benchmarks/run-all.sh --nodejs --go     # Multiple platforms

# Run selective benchmarks by test type
./benchmarks/run-all.sh --small           # Small message tests
./benchmarks/run-all.sh --large           # Large message tests
./benchmarks/run-all.sh --collections     # Collection tests (arrays/maps)
./benchmarks/run-all.sh --scaling         # Size scaling comparison

# Combine platform and test type filters
./benchmarks/run-all.sh --nodejs --small  # Node.js small tests only
./benchmarks/run-all.sh --go --collections # Go collection tests only

# Show benchmark help
./benchmarks/run-all.sh --help

# Run platform-specific benchmarks directly
cd benchmarks/go && go test -bench=. -count=1
cd benchmarks/ts && npm run bench
cd benchmarks/browser && npm run bench

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
- `benchmarks/go/comparison_test.go` - Go vs JSON/Protobuf/Msgpack
- `benchmarks/go/collections_test.go` - Array and Map benchmarks
- `benchmarks/go/size_scaling_test.go` - Size scaling benchmarks

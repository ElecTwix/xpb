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
| **XPB V2**  | **41 ns** | **36 ns** | **19 B** |
| Protobuf    |    193 ns |    238 ns |     19 B |
| JSON        |    265 ns |  1,389 ns |     47 B |
| MessagePack |    360 ns |    637 ns |     33 B |

#### Node.js (JIT)

| Format      |    Encode |     Decode |     Size |
| :---------- | --------: | ---------: | -------: |
| **XPB V2**  | **22 ns** | **110 ns** | **19 B** |
| Protobuf    |    255 ns |     153 ns |     19 B |
| JSON        |    140 ns |     378 ns |     47 B |
| MessagePack |  2,283 ns |     539 ns |     33 B |

### Large Message (7 fields: id, name, email, age, score, active, description)

#### Go

| Format      |     Encode |     Decode |      Size |
| :---------- | ---------: | ---------: | --------: |
| **XPB V2**  | **110 ns** | **123 ns** | **121 B** |
| Protobuf    |     331 ns |     442 ns |     124 B |
| JSON        |     729 ns |   3,195 ns |     192 B |
| MessagePack |     763 ns |   1,045 ns |     165 B |

#### Node.js (JIT)

| Format      |     Encode |     Decode |      Size |
| :---------- | ---------: | ---------: | --------: |
| **XPB V2**  | **275 ns** | **464 ns** | **121 B** |
| Protobuf    |     928 ns |     430 ns |     124 B |
| JSON        |     465 ns |     756 ns |     192 B |
| MessagePack |   2,656 ns |   1,525 ns |     165 B |

### Size Scaling (XPB vs JSON)

XPB provides greatest size savings for smaller messages:

| Message Size   | XPB (B) | JSON (B) | Size Savings | Encode Speedup | Decode Speedup |
| :------------- | ------: | -------: | -----------: | -------------: | -------------: |
| Tiny (1 bool)  |       1 |       11 |    **90.9%** |           5.3x |         2,127x |
| Small (3 fld)  |      21 |       48 |    **56.3%** |           5.9x |            41x |
| Medium (8 fld) |     452 |      548 |    **17.5%** |           4.5x |            21x |
| Large (10KB)   |  10,604 |   10,982 |     **3.4%** |           5.0x |            21x |
| XLarge (50KB)  |  52,407 |   53,434 |     **1.9%** |           3.1x |            15x |

### Collection Types (100 elements, Go)

| Collection   | XPB Encode | JSON Encode |  Speedup | XPB Decode | JSON Decode | Speedup |
| :----------- | ---------: | ----------: | -------: | ---------: | ----------: | ------: |
| String Array |     1.3 µs |      5.3 µs |   **4x** |     3.8 µs |     29.5 µs |  **8x** |
| Int32 Array  |     386 ns |      3.4 µs | **8.7x** |     357 ns |     16.9 µs | **47x** |
| String Map   |     3.8 µs |       39 µs |  **10x** |    10.9 µs |     72.5 µs |  **7x** |

## Key Insights

1. **Encoding Speed**: XPB is consistently **3-10x faster** than JSON for encoding
2. **Decoding Speed**: XPB is **15-2000x faster** than JSON for decoding (Go), with decode advantage increasing for smaller messages
3. **Size Efficiency**: XPB is most space-efficient for **small messages** (90% savings for tiny, 56% for small) due to eliminating field name overhead
4. **Int Arrays**: XPB excels at numeric arrays (8.7x encode, 47x decode) due to fixed-width encoding vs JSON's text representation

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

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

## Performance Results (Optimized)



### Small Message (3 fields: name, age, active)



#### Go



| Format      |    Encode |    Decode |     Size |

| :---------- | --------: | --------: | -------: |

| **XPB V2**  | **11 ns** |  **4 ns** | **19 B** |

| Protobuf    |     98 ns |    164 ns |     19 B |

| JSON        |    155 ns |    778 ns |     47 B |

| MessagePack |    288 ns |    346 ns |     33 B |



#### Node.js (JIT)



| Format      |    Encode |    Decode |     Size |

| :---------- | --------: | --------: | -------: |

| **XPB V2**  | **12 ns** | **60 ns** | **19 B** |

| Protobuf    |    162 ns |     84 ns |     19 B |

| JSON        |     83 ns |    216 ns |     47 B |

| MessagePack |  1,401 ns |    313 ns |     33 B |



### Large Message (7 fields: id, name, email, age, score, active, description)



#### Go



| Format      |     Encode |     Decode |      Size |

| :---------- | ---------: | ---------: | --------: |

| **XPB V2**  |  **18 ns** |   **8 ns** | **121 B** |

| Protobuf    |     225 ns |     331 ns |     128 B |

| JSON        |     469 ns |   1,916 ns |     192 B |

| MessagePack |     708 ns |     749 ns |     165 B |



#### Node.js (JIT)



| Format      |     Encode |     Decode |      Size |

| :---------- | ---------: | ---------: | --------: |

| **XPB V2**  | **156 ns** | **267 ns** | **121 B** |

| Protobuf    |     539 ns |     243 ns |     124 B |

| JSON        |     258 ns |     435 ns |     192 B |

| MessagePack |   1,522 ns |     887 ns |     165 B |



### Size Scaling (XPB vs JSON)



XPB provides greatest size savings for smaller messages:



| Message Size   | XPB (B) | JSON (B) | Size Savings | Encode Speedup | Decode Speedup |

| :------------- | ------: | -------: | -----------: | -------------: | -------------: |

| Tiny (1 bool)  |       1 |       11 |    **90.9%** |           -    |              - |

| Small (3 fld)  |      19 |       47 |    **59.6%** |          13.6x |           180x |

| Medium (8 fld) |     452 |      548 |    **17.5%** |           -    |              - |

| Large (10KB)   |  10,604 |   10,982 |     **3.4%** |          23.7x |           233x |

| XLarge (50KB)  |  52,407 |   53,434 |     **1.9%** |          18.8x |            53x |



### Collection Types (100 elements, Go)



| Collection   | XPB Encode | JSON Encode |  Speedup | XPB Decode | JSON Decode |  Speedup |

| :----------- | ---------: | ----------: | -------: | ---------: | ----------: | -------: |

| String Array |     1.6 µs |      3.9 µs | **2.4x** |     1.3 µs |     17.9 µs | **13.7x** |

| Int32 Array  |     462 ns |      1.8 µs | **3.9x** |     376 ns |      9.6 µs |  **25x** |

| String Map   |     4.2 µs |     24.7 µs | **5.8x** |     5.5 µs |     43.0 µs | **7.8x** |



## Key Insights







1.  **Go Encoding**: XPB is now **13-23x faster** than JSON thanks to `sync.Pool` (zero allocations).



2.  **Go Decoding**: XPB is **180-230x faster** than JSON thanks to `unsafe` zero-copy strings/bytes.



3.  **Browser**: XPB is **4.6x faster** than JSON for small message encoding.



4.  **Hybrid Runtime**: Automatically balances overhead vs throughput (JS < 256B < WASM).



5.  **Worker Threads**: Optimized workers provide **~3.3x speedup** for large arrays.



6.  **Size**: Consistent 37-90% reduction.

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

## Browser Bleeding Edge (2025)

XPB V2 implements advanced optimizations for modern browsers (Chrome 133+, Firefox Nightly) using Native Base64 and Zero-Copy Accessors.

### New Features

1.  **Native Base64**: Uses `Uint8Array.fromBase64` (C++ SIMD) for **160x faster** binary decoding vs `atob`.
2.  **Zero-Alloc Base64**: `Encoder.writeBase64AsBytes(str)` writes directly to the buffer using `setFromBase64`, avoiding intermediate allocations.
3.  **Zero-Copy Accessors**: `XPB.compileAccessor(schema)` creates a View class that reads memory on-demand.

### Benchmark Results (Browser)

| Metric | Specific Test | Speedup vs JSON |
| :--- | :--- | :--- |
| **Binary Data** | Base64 Decode (1MB) | **160x** 🚀 |
| **Zero-Alloc** | Base64 Write to Encoder | **3.2x** (vs Native) ⚡ |
| **Lazy Read** | 2 Field Access | **2.7x** ⚡ |

### Usage

```typescript
// 1. Binary Data (Fastest)
const imageBase64 = "iVBORw0KGgo...";
enc.writeBase64AsBytes(imageBase64); // Zero-Alloc, 160x faster than JSON

// 2. Zero-Copy Accessor (Lazy Read)
const UserAccessor = XPB.compileAccessor(userSchema);
const user = new UserAccessor(buffer);
console.log(user.id); // Reads 4 bytes from memory. No object allocation.
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

## Hybrid Runtime (TypeScript)

For optimal performance across all message sizes, use the Hybrid Runtime. It automatically selects:
- **Pure JS** for small messages (<256 bytes) to minimize overhead.
- **WASM** for large messages (>=256 bytes) to maximize throughput.

```typescript
import { createEncoder, createDecoder } from "@xpb/runtime/hybrid";

// Automatically uses JS or WASM based on size/availability
const enc = createEncoder(); 
const dec = createDecoder(data);
```

## Hyper-Speed Runtime (TypeScript)

For **maximum performance** in trusted environments, use the Hyper Runtime.
- **Zero Function Call Overhead**: Inline implementation.
- **Zero Allocation**: Reuse encoders/decoders.
- **Unsafe**: No validation checks.

```typescript
import { HyperEncoder, HyperDecoder, E, D } from "@xpb/runtime/hyper";

// Encode
const enc = new HyperEncoder();
E.str(enc, 1, "Alice");
E.i32(enc, 2, 30);
const data = enc.f();

// Decode
const dec = new HyperDecoder(data);
const name = D.str(dec); // tag is handled by caller or assumed in struct mode
```

## Advanced Go Usage (Zero-Copy)

For extreme performance, use unsafe zero-copy methods. These return slices/strings that alias the decoder's buffer.

```go
// Returns string pointing to decoder buffer memory
s, _ := dec.ReadString() 

// Returns byte slice pointing to decoder buffer memory
b, _ := dec.ReadBytesUnsafe()
```

## Key Files

- `pkg/wire/wire.go` - V2 wire format constants
- `runtime/go/xpb/xpb.go` - Go Encoder/Decoder (inc. Unsafe)
- `runtime/ts/src/index.ts` - TypeScript Encoder/Decoder
- `runtime/ts/src/jit.ts` - JIT Compiler
- `runtime/ts/src/hybrid.ts` - Hybrid JS/WASM Runtime
- `runtime/ts/src/hyper.ts` - Hyper-Speed Inline Runtime
- `benchmarks/browser/src/xpb-browser.ts` - Browser-optimized runtime
- `benchmarks/go/comparison_test.go` - Go vs JSON/Protobuf/Msgpack
- `benchmarks/go/collections_test.go` - Array and Map benchmarks
- `benchmarks/go/size_scaling_test.go` - Size scaling benchmarks

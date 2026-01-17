# XPB V2 Benchmark Results

Environment: Linux (Intel Core i9-13900H), Go 1.23.0, Node.js (V8 engine)

## Small Message (Tiny)

Single boolean: `{active: true}`

| Format | Encode | Decode | Size | Allocs |
|--------|--------|--------|------|--------|
| **XPB V2** | 9.5 ns | 0.16 ns | 1 B | 0 |
| JSON | 80 ns | 428 ns | 11 B | 1 |
| MessagePack | 234 ns | 171 ns | 9 B | 2 |
| Protobuf | - | - | - | - |

Encode speedup: 8.5x vs JSON
Decode speedup: 2735x vs JSON

## Small Message (3 fields: name, age, active)

### Go Runtime

| Format | Encode | Decode | Size | Allocs |
|--------|--------|--------|------|--------|
| **XPB V2** | 10.9 ns | 3.4 ns | 19 B | 0 |
| JSON | 144 ns | 788 ns | 47 B | 1 |
| MessagePack | 255 ns | 319 ns | 33 B | 2 |
| Protobuf | 90 ns | 148 ns | 19 B | 1 |

Encode speedup: 13.2x vs JSON
Decode speedup: 229x vs JSON
Size reduction: 60% vs JSON

### Node.js Runtime (JIT)

| Format | Encode | Decode | Size |
|--------|--------|--------|------|
| **XPB V2 (JIT)** | 12 ns | 57 ns | 19 B |
| **XPB V2 (Manual)** | 42 ns | 176 ns | 19 B |
| JSON | 80 ns | 217 ns | 47 B |
| MessagePack | 1213 ns | 326 ns | 33 B |
| Protobuf | 187 ns | 81 ns | 19 B |

Encode speedup: 6.5x vs JSON
Decode speedup: 3.8x vs JSON
Size reduction: 60% vs JSON

## Large Message (7 fields)

### Go Runtime

| Format | Encode | Decode | Size | Allocs |
|--------|--------|--------|------|--------|
| **XPB V2** | 17.5 ns | 7.8 ns | 53 B | 0 |
| JSON | 519 ns | 1867 ns | 192 B | 1 |
| MessagePack | 718 ns | 711 ns | 165 B | 4 |
| Protobuf | 217 ns | 397 ns | 53 B | 1 |

Encode speedup: 29.7x vs JSON
Decode speedup: 238x vs JSON

### Node.js Runtime (JIT)

| Format | Encode | Decode | Size |
|--------|--------|--------|------|
| **XPB V2 (JIT)** | 155 ns | 259 ns | 121 B |
| **XPB V2 (Manual)** | 252 ns | 399 ns | 121 B |
| JSON | 271 ns | 436 ns | 192 B |
| MessagePack | 1572 ns | 893 ns | 165 B |
| Protobuf | 563 ns | 222 ns | 124 B |

Encode speedup: 1.75x vs JSON
Decode speedup: 1.68x vs JSON

## Collections (Node.js Runtime)

### String Array (100 elements)

| Format | Encode | Decode | Size |
|--------|--------|--------|------|
| **XPB V2** | 2784 ns | 8326 ns | 1304 B |
| JSON | 1155 ns | 2384 ns | 1501 B |
| MessagePack | 5772 ns | 6952 ns | 1303 B |

⚠️ XPB is **0.41x** encode, **0.29x** decode (slower than JSON)

### Int32 Array (100 elements)

| Format | Encode | Decode | Size |
|--------|--------|--------|------|
| **XPB V2** | 112 ns | 134 ns | 404 B |
| JSON | 805 ns | 846 ns | 435 B |
| MessagePack | 1511 ns | 847 ns | 279 B |

Encode speedup: 7.2x vs JSON
Decode speedup: 6.3x vs JSON

### String Map (100 entries)

| Format | Encode | Decode | Size |
|--------|--------|--------|------|
| **XPB V2** | 5943 ns | 22056 ns | 2604 B |
| JSON | 4619 ns | 4829 ns | 3001 B |
| MessagePack | 16976 ns | 38419 ns | 2603 B |

⚠️ XPB is **0.78x** encode, **0.22x** decode (slower than JSON)

## Size Scaling

| Message Size | XPB (B) | JSON (B) | Savings |
|--------------|---------|----------|---------|
| Tiny (1 bool) | 1 | 11 | 90.9% |
| Small (3 fld) | 19 | 47 | 59.6% |
| Medium (8 fld) | 195 | 376 | 48.1% |
| Large (10 KB) | 10,604 | 10,982 | 3.4% |
| XLarge (50 KB) | 52,407 | 53,434 | 1.9% |

## Key Observations

### Go Runtime
1. **Small messages**: XPB excels (13-230x faster) due to no parsing overhead
2. **Large messages**: Speedup diminishes as data transfer dominates
3. **Collections**: Array/map decode shows 10-30x improvement
4. **Memory**: Zero allocations for encode, minimal for decode
5. **Size**: Best savings on small messages (up to 90%)

### Node.js Runtime
1. **Primitive types**: Fast (6.5x encode, 3.8x decode for small messages)
2. **Int32 arrays**: Excellent (7x faster than JSON)
3. **String collections**: Slower than JSON (0.2x-0.4x decode)
   - JIT compiler issue with string handling in collections
   - Int32 arrays work well, but string operations need optimization
4. **Large messages**: Moderate speedup (1.7x) as overhead diminishes

## Known Issues (Node.js)

| Issue | Impact | Workaround |
|-------|--------|------------|
| String Array decode | 0.29x vs JSON | Use JSON for string arrays |
| String Map decode | 0.22x vs JSON | Use JSON for string maps |
| String Array encode | 0.41x vs JSON | Use JSON for string arrays |

These issues are specific to the JIT compiler's handling of strings in collections. Primitive types and numeric arrays perform well.

## Running Benchmarks

```bash
# Go benchmarks
go test -bench=. -benchmem -count=1 ./benchmarks/go

# Node.js benchmarks
cd runtime/ts && npm run bench

# Unified benchmark tool
go run ./cmd/xpbench
```

## Files

### Go Benchmarks
- Benchmark code: `benchmarks/go/benchmark_test.go`
- Comparison tests: `benchmarks/go/comparison_test.go`
- Collection tests: `benchmarks/go/collections_test.go`
- Size scaling: `benchmarks/go/size_scaling_test.go`

### TypeScript Benchmarks
- Main benchmark: `benchmarks/ts/src/benchmark.ts`
- JIT compiler: `runtime/ts/src/jit.ts`
- Collections: `runtime/ts/src/collections.ts`

# XPB V2 Benchmark Results

Environment: Linux (Intel Core i9-13900H), Go 1.23.0

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

| Format | Encode | Decode | Size | Allocs |
|--------|--------|--------|------|--------|
| **XPB V2** | 10.9 ns | 3.4 ns | 19 B | 0 |
| JSON | 144 ns | 788 ns | 47 B | 1 |
| MessagePack | 255 ns | 319 ns | 33 B | 2 |
| Protobuf | 90 ns | 148 ns | 19 B | 1 |

Encode speedup: 13.2x vs JSON
Decode speedup: 229x vs JSON
Size reduction: 60% vs JSON

## Large Message (7 fields)

| Format | Encode | Decode | Size | Allocs |
|--------|--------|--------|------|--------|
| **XPB V2** | 17.5 ns | 7.8 ns | 53 B | 0 |
| JSON | 519 ns | 1867 ns | 192 B | 1 |
| MessagePack | 718 ns | 711 ns | 165 B | 4 |
| Protobuf | 217 ns | 397 ns | 53 B | 1 |

Encode speedup: 29.7x vs JSON
Decode speedup: 238x vs JSON

## Collections

### String Array (100 elements)

| Format | Encode | Decode | Allocs |
|--------|--------|--------|--------|
| **XPB V2** | 1.2 µs | 954 ns | 1 |
| JSON | 3.0 µs | 16.0 µs | 2 |
| MessagePack | 3.4 µs | 5.3 µs | 8 |

Encode speedup: 2.5x vs JSON
Decode speedup: 16.8x vs JSON

### Int32 Array (100 elements)

| Format | Encode | Decode | Allocs |
|--------|--------|--------|--------|
| **XPB V2** | 296 ns | 319 ns | 1 |
| JSON | 1.7 µs | 9.5 µs | 2 |
| MessagePack | 2.6 µs | 3.8 µs | 6 |

Encode speedup: 5.9x vs JSON
Decode speedup: 29.8x vs JSON

### String Map (100 entries)

| Format | Encode | Decode | Allocs |
|--------|--------|--------|--------|
| **XPB V2** | 2.8 µs | 4.1 µs | 1 |
| JSON | 23.3 µs | 37.2 µs | 202 |
| MessagePack | 6.6 µs | 10.4 µs | 8 |

Encode speedup: 8.2x vs JSON
Decode speedup: 9.0x vs JSON
Alloc reduction: 202x fewer allocations

## Size Scaling

| Message Size | XPB (B) | JSON (B) | Savings |
|--------------|---------|----------|---------|
| Tiny (1 bool) | 1 | 11 | 90.9% |
| Small (3 fld) | 19 | 47 | 59.6% |
| Medium (8 fld) | 195 | 376 | 48.1% |
| Large (10 KB) | 10,604 | 10,982 | 3.4% |
| XLarge (50 KB) | 52,407 | 53,434 | 1.9% |

## Key Observations

1. **Small messages**: XPB excels (13-230x faster) due to no parsing overhead
2. **Large messages**: Speedup diminishes as data transfer dominates
3. **Collections**: Array/map decode shows 10-30x improvement
4. **Memory**: Zero allocations for encode, minimal for decode
5. **Size**: Best savings on small messages (up to 90%)

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

- Benchmark code: `benchmarks/go/benchmark_test.go`
- Comparison tests: `benchmarks/go/comparison_test.go`
- Collection tests: `benchmarks/go/collections_test.go`
- Size scaling: `benchmarks/go/size_scaling_test.go`

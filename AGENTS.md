# XPB V2 - Agent Guidelines

XPB V2 is a high-performance binary serialization format with Go, TypeScript, C, Lua, and Java runtimes.

## Project Structure

```
xpb/
├── cmd/xpbc/           # CLI code generator
├── cmd/xpbench/        # Unified benchmark runner
├── pkg/                # Go packages (ast, parser, codegen, wire)
├── runtime/
│   ├── go/xpb/         # Go runtime
│   ├── ts/src/         # TypeScript runtime
│   ├── c/              # C runtime (xpb.h, xpb.c)
│   ├── lua/            # Lua runtime (xpb.lua)
│   └── java/           # Java runtime
├── benchmarks/         # Language-specific benchmarks
└── tests/              # E2E and runtime tests
```

## Build Commands

### Go

```bash
# Build CLI
go build -o xpbc ./cmd/xpbc

# Run all tests
go test ./...

# Run single test by name
go test -run TestName ./...
go test -run TestName ./pkg/parser

# Run benchmarks
go test -bench=. -benchmem ./benchmarks/go -count=1

# Lint
golangci-lint run ./...
```

### TypeScript

```bash
cd runtime/ts

# Build
npm run build

# Run all tests
npm test

# Run single test file
npx vitest run src/index.test.ts

# Run single test by name
npx vitest run -t "test name"

# Browser benchmarks
npm run test:browser           # Both browsers
npm run test:browser:chrome    # Chrome only
npm run test:browser:firefox   # Firefox only

# Feature benchmarks
npm run bench:features

# Lint
npx eslint src/
```

### C

```bash
# Compile & run tests
gcc -Wall -Wextra -I runtime/c/include tests/c/xpb_test.c runtime/c/xpb.c -o /tmp/xpb_test && /tmp/xpb_test

# Run benchmarks
gcc -Wall -Wextra -I runtime/c/include benchmarks/c/xpb_bench.c runtime/c/xpb.c -lm -o /tmp/bench && /tmp/bench
```

### Lua

```bash
# Run tests
lua5.4 -e "package.path='./runtime/lua/?.lua;'" tests/lua/xpb_test.lua

# Run benchmarks
lua5.4 -e "package.path='./runtime/lua/?.lua;'" benchmarks/lua/xpb_bench.lua
```

### Java

```bash
# Compile & run tests
javac -d /tmp/runtime_test runtime/java/src/main/java/xpb/*.java tests/java/XpbTest.java
java -cp /tmp/runtime_test xpb.XpbTest

# Run benchmarks
javac -d /tmp/bench runtime/java/src/main/java/xpb/*.java benchmarks/java/XpbBench.java
java -cp /tmp/bench xpb.XpbBench
```

## Code Style

### Go

- **Format**: `go fmt` before commit
- **Imports**: stdlib → third-party → internal
- **Naming**: PascalCase exported, camelCase unexported
- **Errors**: Use `fmt.Errorf("context: %w", err)` with context
- **Performance**: Use `sync.Pool` for pooling, `unsafe` for zero-copy

```go
import (
    "encoding/binary"
    
    "github.com/anthropic/xpb/pkg/wire"
)

var ErrBufferTooSmall = errors.New("xpb: buffer too small")

func (e *Encoder) WriteInt32(v int32) error {
    if e.pos+4 > len(e.buf) {
        return ErrBufferTooSmall
    }
    // implementation
    return nil
}
```

### TypeScript

- **Target**: ES2022, strict mode, ESNext modules
- **Imports**: Explicit paths, no barrel files
- **Naming**: PascalCase classes, camelCase functions
- **Errors**: Prefix with `xpb:`
- **JSDoc**: Required for public APIs

```typescript
import { Encoder, Decoder } from './index';

/**
 * V2 Encoder - tagless, fixed-width, compact lengths.
 */
export class Encoder {
  writeBool(v: boolean): void {
    if (this.pos >= this.buf.length) {
      throw new Error('xpb: buffer too small');
    }
    // implementation
  }
}
```

### C

- **Format**: Use `clang-format` or consistent indentation
- **Naming**: `snake_case`, `xpb_*` prefix for public API
- **Headers**: Use `#ifndef XPB_H` guards
- **Memory**: Use `xpb_free()` for decoder strings
- **Errors**: Return error codes, cleanup on failure

```c
#ifndef XPB_H
#define XPB_H

#include <stdint.h>

struct xpb_encoder* xpb_encoder_create(size_t capacity);
void xpb_encoder_write_int32(struct xpb_encoder* enc, int32_t v);
void xpb_free(void* ptr);

#endif
```

### Lua

- **Style**: `snake_case`, colon syntax for methods
- **Naming**: Module-level with `xpb.` prefix
- **Errors**: Use `error()` with descriptive messages
- **Performance**: Prefer bitwise operators

```lua
local xpb = {}

function xpb.Encoder(initial_size)
    local self = { buf = {}, pos = 0 }
    -- implementation
    return self
end

return xpb
```

### Java

- **Style**: Standard Java conventions
- **Naming**: PascalCase classes, camelCase methods
- **Package**: All classes in `xpb` package
- **Errors**: `IllegalArgumentException` with `xpb:` prefix

```java
package xpb;

public class Encoder {
    public void writeInt32(int v) {
        if (pos + 4 > buf.length) {
            throw new IllegalArgumentException("xpb: buffer too small");
        }
        // implementation
    }
}
```

## Wire Format

XPB V2 uses:
- **Struct Mode**: No field tags, declaration order
- **Fixed-Width**: 4 bytes int32, 8 bytes int64, little-endian
- **Compact Lengths**: 1 byte if <255, else 0xFF + 4 bytes

## Testing Guidelines

- Verify round-trip encoding/decoding
- Test edge cases: empty strings, max values, boundaries
- Use table-driven tests in Go
- Benchmark with `-count=1 -benchmem`
- E2E tests in `tests/e2e_test.go`

## Performance Tips

- **Go**: `sync.Pool` for encoder reuse
- **TypeScript**: Manual ASCII for strings <64 chars
- **C**: Stack allocation, minimize malloc/free
- **Lua**: Bitwise ops instead of multiplication
- **Java**: Reuse byte arrays where possible

## Key Files

- `runtime/go/xpb/xpb.go` - Go runtime
- `runtime/ts/src/index.ts` - TypeScript runtime
- `runtime/c/xpb.c` / `runtime/c/include/xpb/xpb.h` - C runtime
- `runtime/lua/xpb.lua` - Lua runtime
- `runtime/java/src/main/java/xpb/` - Java runtime
- `pkg/parser/parser.go` - Schema parser
- `pkg/wire/wire.go` - Wire format constants

# XPB V2 - Agent Guidelines

XPB V2 is a high-performance binary serialization format with Go, TypeScript, C, Lua, and Java runtimes. This document provides guidelines for agentic coding agents working in this repository.

## Project Structure

```
xpb/
├── cmd/xpbc/           # CLI code generator
├── cmd/xpbench/        # Unified benchmark runner
├── pkg/
│   ├── ast/            # AST definitions
│   ├── parser/         # Lexer and parser
│   ├── codegen/        # Go, TypeScript, C, Lua, Java code generators
│   └── wire/           # V2 wire format constants
├── runtime/
│   ├── go/xpb/         # Go runtime (Encoder/Decoder)
│   ├── ts/src/         # TypeScript runtime
│   ├── c/              # C runtime (xpb.h, xpb.c)
│   ├── lua/            # Lua runtime (xpb.lua)
│   └── java/           # Java runtime (Encoder.java, Decoder.java)
├── benchmarks/
│   ├── go/             # Go benchmarks
│   ├── c/              # C benchmarks
│   ├── lua/            # Lua benchmarks
│   └── java/           # Java benchmarks
└── tests/              # End-to-end and runtime tests
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
go test -bench=. -benchmem ./benchmarks/go -count=1
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

### C

```bash
# Compile runtime
gcc -c -Wall -Wextra -I runtime/c/include runtime/c/xpb.c -o xpb.o

# Compile and run tests
gcc -Wall -Wextra -I runtime/c/include tests/c/xpb_test.c runtime/c/xpb.c -o /tmp/xpb_test && /tmp/xpb_test

# Run benchmarks
gcc -Wall -Wextra -I runtime/c/include benchmarks/c/xpb_bench.c runtime/c/xpb.c -lm -o /tmp/bench && /tmp/bench
```

### Lua

```bash
# Run tests (requires lua5.4)
lua5.4 -e "package.path='./runtime/lua/?.lua;'" tests/lua/xpb_test.lua

# Run benchmarks
lua5.4 -e "package.path='./runtime/lua/?.lua;'" benchmarks/lua/xpb_bench.lua
```

### Java

```bash
# Compile runtime
javac -d /tmp/runtime runtime/java/src/main/java/xpb/*.java

# Compile and run tests
javac -d /tmp/runtime_test runtime/java/src/main/java/xpb/*.java tests/java/XpbTest.java
java -cp /tmp/runtime_test xpb.XpbTest

# Run benchmarks
javac -d /tmp/bench runtime/java/src/main/java/xpb/*.java benchmarks/java/XpbBench.java
java -cp /tmp/bench xpb.XpbBench
```

## Code Style Guidelines

### Go

- **Formatting**: Run `go fmt` on all files before committing
- **Error Handling**: Use early returns with descriptive errors; prefer `fmt.Errorf` with context
- **Naming**: PascalCase for exported, camelCase for unexported; avoid abbreviations
- **Package Comments**: Every package must have a package-level comment
- **Tests**: Use `*testing.T` with `t.Fatalf` for errors, `t.Logf` for debug info
- **Performance**: Use `sync.Pool` for object pooling; prefer `unsafe` for zero-copy

```go
// Package xpb provides the XPB V2 runtime library for encoding and decoding.
package xpb

var (
    ErrBufferTooSmall = errors.New("xpb: buffer too small")
    ErrInvalidData    = errors.New("xpb: invalid data")
)
```

### TypeScript

- **TypeScript Config**: Target ES2022, strict mode, module: ESNext
- **JSDoc**: Add JSDoc comments for public APIs
- **Error Handling**: Throw `Error` with messages prefixed with `xpb:`
- **Naming**: PascalCase for classes, camelCase for functions/variables
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

### C

- **Formatting**: Run `indent` or manually format with clang-format
- **Error Handling**: Return error codes; use `xpb_free()` for cleanup
- **Naming**: snake_case for functions, `xpb_*` prefix for public API
- **Header Guards**: Use `#ifndef XPB_H / #define XPB_H / #endif`
- **Memory**: Allocate with malloc/free; use `xpb_free()` for decoder strings

```c
#ifndef XPB_H
#define XPB_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

struct xpb_encoder* xpb_encoder_create(size_t initial_capacity);
void xpb_encoder_destroy(struct xpb_encoder* enc);
void xpb_encoder_write_int32(struct xpb_encoder* enc, int32_t v);
void xpb_free(void* ptr);

#ifdef __cplusplus
}
#endif
#endif
```

### Lua

- **Style**: Use snake_case, colon syntax for methods (`self:method()`)
- **Error Handling**: Use `error()` with descriptive messages
- **Naming**: lowercase for local functions, module-level with `xpb.` prefix
- **Performance**: Use bitwise operators (`<<`, `&`, `>>`) instead of multiplication

```lua
local xpb = {}

xpb.COMPACT_LENGTH_THRESHOLD = 254

function xpb.Encoder(initial_size)
    local self = { buf = {}, pos = 0 }
    -- implementation
    return self
end

return xpb
```

### Java

- **Formatting**: Use standard Java conventions
- **Error Handling**: Throw `IllegalArgumentException` with `xpb:` prefix
- **Naming**: PascalCase for classes, camelCase for methods
- **Package**: All runtime classes in `xpb` package

```java
package xpb;

public class Encoder {
    private byte[] buf;
    private int pos;

    public Encoder(int initialCapacity) {
        this.buf = new byte[initialCapacity];
        this.pos = 0;
    }

    public void writeInt32(int v) {
        buf[pos++] = (byte) (v & 0xFF);
        // ...
    }
}
```

## V2 Wire Format

XPB V2 uses:
- **Struct Mode**: No field tags, fields in declaration order
- **Fixed-Width Integers**: 4 bytes for int32, 8 bytes for int64, little-endian
- **Compact Lengths**: 1 byte if length < 255, else 0xFF + 4 bytes

## Import Conventions

### Go

Standard library first, then third-party, then internal:

```go
import (
    "encoding/binary"
    "io"

    "github.com/vmihailenco/msgpack/v5"

    "github.com/anthropic/xpb/pkg/wire"
)
```

### TypeScript

ES module imports with explicit paths:

```typescript
import { Encoder, Decoder } from './index';
```

### C

```c
#include <stdint.h>
#include <stdlib.h>
#include "xpb/xpb.h"
```

### Java

```java
package xpb;

import java.nio.charset.StandardCharsets;
```

## Testing Guidelines

- Tests should verify round-trip encoding/decoding
- Include edge cases (empty strings, large values, boundary conditions)
- Log encoded sizes for debugging serialization format
- E2E tests are in `tests/e2e_test.go`
- Runtime tests in `tests/c/`, `tests/lua/`, `tests/java/`

## Common Patterns

### Go Encoder/Decoder

```go
enc := xpb.NewEncoder(64)
enc.WriteString("Alice")
enc.WriteInt32(30)
data := enc.Bytes()

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

### C Encoder/Decoder

```c
struct xpb_encoder* enc = xpb_encoder_create(64);
xpb_encoder_write_string(enc, "Alice");
xpb_encoder_write_int32(enc, 30);
size_t len;
uint8_t* data = xpb_encoder_finish(enc, &len);

struct xpb_decoder* dec = xpb_decoder_create(data, len);
char* name = xpb_decoder_read_string(dec);
int32_t age = xpb_decoder_read_int32(dec);
xpb_free(name);
```

### Lua Encoder/Decoder

```lua
local enc = xpb.Encoder(64)
enc:write_string("Alice")
enc:write_int32(30)
local data = enc:finish()

local dec = xpb.Decoder(data)
local name = dec:read_string()
local age = dec:read_int32()
```

### Java Encoder/Decoder

```java
Encoder enc = new Encoder(64);
enc.writeString("Alice");
enc.writeInt32(30);
byte[] data = enc.finish();

Decoder dec = new Decoder(data);
String name = dec.readString();
int age = dec.readInt32();
```

## Key Files

- `runtime/go/xpb/xpb.go` - Go Encoder/Decoder implementation
- `runtime/ts/src/index.ts` - TypeScript Encoder/Decoder
- `runtime/c/xpb.c` / `runtime/c/include/xpb/xpb.h` - C runtime
- `runtime/lua/xpb.lua` - Lua runtime
- `runtime/java/src/main/java/xpb/Encoder.java` - Java Encoder
- `runtime/java/src/main/java/xpb/Decoder.java` - Java Decoder
- `pkg/parser/parser.go` - Schema parser
- `pkg/wire/wire.go` - Wire format constants
- `benchmarks/go/comparison_test.go` - Performance comparisons

## Performance Guidelines

- Go: Use `sync.Pool` for encoder/decoder reuse; prefer zero-copy methods
- TypeScript: Optimize for small messages (<256 bytes) with manual ASCII decoding
- C: Use stack allocation where possible; minimize malloc/free calls
- Lua: Use bitwise operators instead of multiplication/division
- Benchmarks should use `-count=1` and `-benchmem` for accurate measurements

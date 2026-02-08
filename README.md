# XPB V2 Binary Serialization

High-performance binary serialization for Go, TypeScript, C, Lua, and Java.

## Runtimes

| Platform | Location | Status |
|----------|----------|--------|
| Go | `runtime/go/xpb` | Production-ready |
| TypeScript | `runtime/ts/src` | Production-ready |
| C | `runtime/c` | Active |
| Lua | `runtime/lua` | Active |
| Java | `runtime/java` | Active |
| Browser | `runtime/ts/src` (browser exports) | Production-ready |

## Quick Start

```bash
# Build CLI
go build -o xpbc ./cmd/xpbc

# Generate code (all languages)
./xpbc --lang=go,ts,c,lua,java schema.xpb

# Or generate for specific languages
./xpbc --lang=go,ts schema.xpb
./xpbc --lang=c,lua schema.xpb
```

## Go API

```go
import "github.com/anthropic/xpb/runtime/go/xpb"

// Encode
enc := xpb.NewEncoder(64)
enc.WriteString("Alice")
enc.WriteInt32(30)
data := enc.Bytes()

// Decode
dec := xpb.NewDecoder(data)
name, _ := dec.ReadString()
age, _ := dec.ReadInt32()
```

## TypeScript API

```typescript
import { Encoder, Decoder } from '@xpb/runtime'

// Encode
const enc = new Encoder(64)
enc.writeString("Alice")
enc.writeInt32(30)
const data = enc.finish()

// Decode
const dec = new Decoder(data)
const name = dec.readString()
const age = dec.readInt32()
```

## C API

```c
#include "xpb/xpb.h"

// Encode
struct xpb_encoder* enc = xpb_encoder_create(64);
xpb_encoder_write_string(enc, "Alice");
xpb_encoder_write_int32(enc, 30);
size_t len;
uint8_t* data = xpb_encoder_finish(enc, &len);
xpb_encoder_destroy(enc);

// Decode
struct xpb_decoder* dec = xpb_decoder_create(data, len);
char* name = xpb_decoder_read_string(dec);
int32_t age = xpb_decoder_read_int32(dec);
xpb_free(name);
xpb_decoder_destroy(dec);
```

## Lua API

```lua
local xpb = require("xpb")

-- Encode
local enc = xpb.Encoder(64)
enc:write_string("Alice")
enc:write_int32(30)
local data = enc:finish()

-- Decode
local dec = xpb.Decoder(data)
local name = dec:read_string()
local age = dec:read_int32()
```

## Java API

```java
import xpb.Encoder;
import xpb.Decoder;

// Encode
Encoder enc = new Encoder(64);
enc.writeString("Alice");
enc.writeInt32(30);
byte[] data = enc.finish();

// Decode
Decoder dec = new Decoder(data);
String name = dec.readString();
int age = dec.readInt32();
```

## Wire Format

XPB V2 uses struct mode encoding:

- **int32**: 4 bytes, little-endian, two's complement
- **int64**: 8 bytes, little-endian, two's complement
- **uint32/uint64**: 4/8 bytes, little-endian
- **float32/float64**: 4/8 bytes, little-endian IEEE 754
- **string/bytes**: length prefix + data
  - Length < 255: 1 byte
  - Length >= 255: 0xFF marker + 4-byte length
- **bool**: 1 byte (0 or 1)

Fields are written/read in declaration order with no field tags.

## Schema Example

```xpb
package myapp

enum Status { ACTIVE = 1 }

message User {
    1: string name
    2: int32 age
    3: []string tags
    4: Status status
}
```

## Commands

```bash
# Run Go tests
go test ./...

# Run TypeScript tests
cd runtime/ts && npm test

# Run C tests
gcc -Wall -Wextra -I runtime/c/include tests/c/xpb_test.c runtime/c/xpb.c -o /tmp/xpb_test && /tmp/xpb_test

# Run Lua tests
lua5.4 -e "package.path='./runtime/lua/?.lua;'" tests/lua/xpb_test.lua

# Run Java tests
javac -d /tmp/runtime_test runtime/java/src/main/java/xpb/*.java tests/java/XpbTest.java
java -cp /tmp/runtime_test xpb.XpbTest

# Run benchmarks
go test -bench=. -benchmem ./benchmarks/go
cd runtime/ts && npm run bench

# Unified benchmark tool
go run ./cmd/xpbench
```

## Project Structure

```
xpb/
├── cmd/xpbc/           # CLI code generator
├── cmd/xpbench/        # Unified benchmark runner
├── pkg/
│   ├── ast/            # AST definitions
│   ├── parser/         # Lexer and parser
│   ├── codegen/        # Go, TypeScript, C, Lua, Java generators
│   └── wire/           # Wire format constants
├── runtime/
│   ├── go/xpb/         # Go runtime
│   ├── ts/src/         # TypeScript runtime
│   ├── c/              # C runtime
│   ├── lua/            # Lua runtime
│   └── java/           # Java runtime
├── benchmarks/
│   ├── go/             # Go benchmarks
│   ├── ts/             # Node.js benchmarks
│   ├── c/              # C benchmarks
│   ├── lua/            # Lua benchmarks
│   └── java/           # Java benchmarks
└── tests/              # E2E tests
```

## Documentation

- [Wire Format Spec](docs/WIRE_FORMAT.md)
- [Architecture Overview](docs/ARCHITECTURE.md)
- [Benchmark Results](docs/BENCHMARKS.md)
- [Agent Guidelines](AGENTS.md)

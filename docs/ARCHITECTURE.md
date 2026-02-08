# XPB V2 Architecture

XPB V2 is a high-performance binary serialization format with Go, TypeScript, C, Lua, and Java runtimes.

## Project Structure

```
xpb/
├── cmd/
│   ├── xpbc/              # CLI code generator
│   └── xpbench/           # Unified benchmark runner
├── pkg/
│   ├── ast/               # AST definitions for schema parsing
│   ├── parser/            # Lexer and parser for .xpb schema files
│   ├── codegen/           # Code generators
│   │   ├── golang/        # Go code generator
│   │   ├── typescript/    # TypeScript code generator
│   │   ├── c/             # C code generator
│   │   ├── lua/           # Lua code generator
│   │   └── java/          # Java code generator
│   └── wire/              # Wire format constants and utilities
├── runtime/
│   ├── go/xpb/            # Go runtime (Encoder/Decoder)
│   ├── ts/src/            # TypeScript runtime
│   │   ├── index.ts       # Core Encoder/Decoder
│   │   ├── node.ts        # Node.js optimized runtime
│   │   ├── browser.ts     # Browser optimized runtime
│   │   ├── hybrid.ts      # Auto-selects optimal strategy
│   │   ├── jit.ts         # JIT-compiled encoding/decoding
│   │   └── wasm.ts        # WebAssembly runtime
│   ├── c/                 # C runtime (xpb.h, xpb.c)
│   ├── lua/               # Lua runtime (xpb.lua)
│   └── java/              # Java runtime (Encoder.java, Decoder.java)
├── tests/                 # End-to-end and runtime tests
└── benchmarks/            # Performance benchmarks
    ├── go/                # Go benchmarks
    ├── ts/                # Node.js benchmarks
    ├── c/                 # C benchmarks
    ├── lua/               # Lua benchmarks
    └── java/              # Java benchmarks
```

## Wire Format (V2)

### Design Goals

1. **Tagless encoding** - No field tags, fields written in declaration order
2. **Fixed-width integers** - 4 bytes for int32/uint32, 8 bytes for int64/uint64
3. **Compact lengths** - 1 byte for lengths < 255, 5 bytes (0xFF + 4 bytes) for larger
4. **Little-endian** - Consistent byte order across platforms

### Data Types

| Type    | Size    | Encoding                          |
|---------|---------|-----------------------------------|
| bool    | 1 byte  | 0x00 = false, 0x01 = true         |
| int32   | 4 bytes | Little-endian two's complement    |
| int64   | 8 bytes | Little-endian two's complement    |
| uint32  | 4 bytes | Little-endian unsigned            |
| uint64  | 8 bytes | Little-endian unsigned            |
| float32 | 4 bytes | IEEE 754 little-endian            |
| float64 | 8 bytes | IEEE 754 little-endian            |
| string  | N+1/5   | Compact length + UTF-8 bytes      |
| bytes   | N+1/5   | Compact length + raw bytes        |
| message | N+1/5   | Compact length + nested content   |

### Compact Length Encoding

```
Length < 255:  [length byte] [data...]
Length >= 255: [0xFF] [4-byte length (LE)] [data...]
```

## Go Runtime

### Core Types

```go
// Encoder writes typed values to a buffer
type Encoder struct {
    buf []byte
    pos int
}

// Decoder reads typed values from a buffer
type Decoder struct {
    data []byte
    pos  int
}
```

### Performance Optimizations

1. **sync.Pool** - Reuse Encoder/Decoder instances
2. **unsafe.Pointer** - Zero-copy operations where possible
3. **Pre-allocated buffers** - Avoid allocations for small messages

### Usage

```go
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

## TypeScript Runtime

### Core Types

```typescript
export class Encoder {
  private buf: Uint8Array;
  private view: DataView;
  private pos = 0;
}

export class Decoder {
  private data: Uint8Array;
  private view: DataView;
  private pos = 0;
}
```

### Performance Optimizations

1. **Zero-copy reads** - Return Uint8Array slices instead of copies
2. **TextEncoder caching** - Reuse TextEncoder/TextDecoder instances
3. **ASCII fast path** - Manual encoding for short ASCII strings
4. **Buffer transfer** - Use ArrayBuffer.transfer() when available

### Runtimes

| Runtime    | Use Case                    | Performance       |
|------------|-----------------------------|-------------------|
| index.ts   | Universal (Node/Browser)    | Good              |
| node.ts    | Node.js                     | Better (Buffer)   |
| browser.ts | Browser                    | Better (Web APIs) |
| hybrid.ts  | Auto-select                 | Best              |
| wasm.ts    | Large messages              | Best              |

## C Runtime

### Core Types

```c
struct xpb_encoder {
    uint8_t* buf;
    size_t capacity;
    size_t pos;
};

struct xpb_decoder {
    const uint8_t* data;
    size_t len;
    size_t pos;
};
```

### Performance Optimizations

1. **Manual endianness handling** - Cross-platform support with fast paths for little-endian
2. **Minimal allocations** - Stack-friendly design with optional heap allocation
3. **Zero-copy string reads** - Returns pointers to decoder buffer (caller must copy if needed)

### Usage

```c
struct xpb_encoder* enc = xpb_encoder_create(64);
xpb_encoder_write_string(enc, "Alice");
xpb_encoder_write_int32(enc, 30);
size_t len;
uint8_t* data = xpb_encoder_finish(enc, &len);
xpb_encoder_destroy(enc);

struct xpb_decoder* dec = xpb_decoder_create(data, len);
char* name = xpb_decoder_read_string(dec);
int32_t age = xpb_decoder_read_int32(dec);
xpb_free(name);
xpb_decoder_destroy(dec);
```

## Lua Runtime

### Core Types

```lua
-- Encoder returns a table with methods
local enc = xpb.Encoder(initial_size)

-- Decoder returns a table with methods  
local dec = xpb.Decoder(data_string)
```

### Performance Optimizations

1. **Bitwise operations** - Uses `<<` and `&` instead of multiplication/division
2. **Direct table indexing** - Optimized buffer access with `buf[pos]`
3. **Chunk-based buffering** - Efficient table-based byte storage

### Usage

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

## Java Runtime

### Core Types

```java
public class Encoder {
    private byte[] buf;
    private int pos;
    private int capacity;
}

public class Decoder {
    private final byte[] data;
    private final int length;
    private int pos;
}
```

### Performance Optimizations

1. **Pre-allocated buffers** - Avoids repeated allocation during encoding
2. **System.arraycopy** - Native array copying for bytes
3. **Little-endian bit manipulation** - Manual byte ordering for speed

### Usage

```java
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

## Code Generation

### Schema Syntax

```
package name

message MessageName {
    field_number: type field_name
}

enum EnumName {
    VALUE_ONE = 0
    VALUE_TWO = 1
}
```

### Generated Go Code

```go
type User struct {
    Name string
    Age  int32
}

func (m *User) Marshal() ([]byte, error) { ... }
func (m *User) Unmarshal([]byte) error   { ... }
```

### Generated TypeScript Code

```typescript
export interface UserData {
    name: string;
    age: number;
}

export class User {
    constructor(public data: UserData) {}
    encode(): Uint8Array { ... }
    static decode(data: Uint8Array): User { ... }
}
```

### Generated C Code

```c
typedef struct {
    char* name;
    int32_t age;
} User;

void user_encode(const User* m, struct xpb_encoder* enc);
int user_decode(User* m, struct xpb_decoder* dec);
```

### Generated Lua Code

```lua
local User = {}

function User.encode(m)
    local enc = xpb.Encoder(64)
    -- ...
    return enc:finish()
end

function User.decode(data)
    local dec = xpb.Decoder(data)
    local m = {}
    -- ...
    return m
end

return User
```

### Generated Java Code

```java
public class User {
    public String name;
    public int age;

    public byte[] encode() {
        Encoder enc = new Encoder(64);
        // ...
        return enc.finish();
    }

    public static User decode(byte[] data) {
        Decoder dec = new Decoder(data);
        User m = new User();
        // ...
        return m;
    }
}
```

## Benchmarking

### Running Benchmarks

```bash
# Go benchmarks
go test -bench=. -benchmem ./benchmarks/go

# TypeScript benchmarks
cd runtime/ts && npm run bench

# C benchmarks
gcc -Wall -Wextra -I runtime/c/include benchmarks/c/xpb_bench.c runtime/c/xpb.c -lm -o /tmp/bench && /tmp/bench

# Lua benchmarks
lua5.4 -e "package.path='./runtime/lua/?.lua;'" benchmarks/lua/xpb_bench.lua

# Java benchmarks
javac -d /tmp/bench runtime/java/src/main/java/xpb/*.java benchmarks/java/XpbBench.java
java -cp /tmp/bench xpb.XpbBench

# Unified benchmarks
go run ./cmd/xpbench
```

### Key Metrics

- **Encode throughput** - MB/s for encoding
- **Decode throughput** - MB/s for decoding
- **Message size** - Encoded size vs JSON/Msgpack
- **Memory allocations** - Allocs/op for Go benchmarks

## CI/CD

### GitHub Actions

- **Test** - Runs Go and TypeScript tests with coverage
- **Lint** - Runs golangci-lint and ESLint

### Manual Testing

C, Lua, and Java runtimes are tested manually:

```bash
# C
gcc -Wall -Wextra -I runtime/c/include tests/c/xpb_test.c runtime/c/xpb.c -o /tmp/xpb_test && /tmp/xpb_test

# Lua
lua5.4 -e "package.path='./runtime/lua/?.lua;'" tests/lua/xpb_test.lua

# Java
javac -d /tmp/runtime_test runtime/java/src/main/java/xpb/*.java tests/java/XpbTest.java
java -cp /tmp/runtime_test xpb.XpbTest
```

### Coverage Targets

- Go: > 70% line coverage
- TypeScript: > 50% line coverage (for core runtime)

## Future Work

1. **Varint encoding** - For smaller integers (1-10 bytes)
2. **Zigzag encoding** - For negative integers
3. **Streaming API** - For large messages
4. **Schema evolution** - Backwards compatibility support
5. **Additional runtimes** - Rust, Python
6. **CI integration** - Add C, Lua, and Java to automated testing
7. **Array support** - Full collection support in C, Lua, Java runtimes
8. **Build system integration** - Makefiles for C, Maven/Gradle for Java, rockspec for Lua

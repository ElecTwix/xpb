# XPB - Compact Binary Serialization

A protobuf-like but smaller, C-style, compressible and streamable binary serialization format.

## Features

- **Compact** - Minimal wire overhead with varint encoding
- **Streamable** - Length-prefixed messages for incremental parsing
- **Simple** - Easy to implement in any language
- **Fast** - Zero-allocation decoding possible

## Installation

```bash
go install github.com/anthropic/xpb/cmd/xpbc@latest
```

## Quick Start

### 1. Define your schema (`user.xpb`)

```xpb
package myapp

message User {
    1: string name
    2: int32 age
    3: bool active
}
```

### 2. Generate code

```bash
# Generate Go code
xpbc --lang=go user.xpb

# Generate TypeScript code
xpbc --lang=ts user.xpb
```

### 3. Use generated code

**Go:**

```go
user := &User{Name: "Alice", Age: 30, Active: true}
data, _ := user.Marshal()
// data is a compact binary representation
```

**TypeScript:**

```typescript
const user = new User({ name: "Alice", age: 30, active: true });
const data = User.encode(user);
// data is a Uint8Array
```

## Wire Format

XPB uses a simple tag-length-value encoding:

| Component | Encoding                                 |
| --------- | ---------------------------------------- |
| Field tag | `(field_id << 3) \| wire_type` as varint |
| Length    | Varint (for strings/bytes/messages)      |
| Value     | Type-specific encoding                   |

### Wire Types

| Type            | ID  | Used For                           |
| --------------- | --- | ---------------------------------- |
| Varint          | 0   | int32, int64, uint32, uint64, bool |
| Fixed32         | 1   | float32                            |
| Fixed64         | 2   | float64                            |
| LengthDelimited | 3   | string, bytes, nested messages     |

## Benchmarks

See [benchmarks/](./benchmarks/) for performance comparisons against protobuf and msgpack.

## License

MIT

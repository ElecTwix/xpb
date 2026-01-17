# XPB V2 Wire Format Specification

This document describes the byte-level encoding for XPB V2 format.

## Overview

XPB V2 is a binary serialization format with these characteristics:

- **Struct Mode**: No field tags, fields encoded in declaration order
- **Fixed-Width Integers**: 4 bytes for int32, 8 bytes for int64, little **Compact Lengths**: 1 byte-endian
- if length < 255, else 5 bytes (0xFF + 4-byte length)
- **Little-Endian**: All multi-byte values are little-endian

## Basic Types

### Boolean

```
0x00 = false
0x01 = true
```

### Integers

All integers are signed two's complement, little-endian.

| Type | Size | Range |
|------|------|-------|
| int8 | 1 byte | -128 to 127 |
| int16 | 2 bytes | -32,768 to 32,767 |
| int32 | 4 bytes | -2^31 to 2^31-1 |
| int64 | 8 bytes | -2^63 to 2^63-1 |

Example: `int32(30)` encodes as `1E 00 00 00` (little-endian 30 = 0x0000001E)

### Unsigned Integers

| Type | Size | Range |
|------|------|-------|
| uint8 | 1 byte | 0 to 255 |
| uint16 | 2 bytes | 0 to 65,535 |
| uint32 | 4 bytes | 0 to 2^32-1 |
| uint64 | 8 bytes | 0 to 2^64-1 |

### Floating Point

| Type | Size | Format |
|------|------|--------|
| float32 | 4 bytes | IEEE 754 single precision |
| float64 | 8 bytes | IEEE 754 double precision |

### Compact Length Prefix

Used for strings, bytes, and variable-length data.

```
Length 0-254:  [length_byte]              // 1 byte
Length 255+:   [0xFF] [len_uint32_le]     // 5 bytes total
```

Examples:
- Length 5: `05`
- Length 255: `FF 00 01 00 00` (256 in little-endian)
- Length 1000: `FF E8 03 00 00` (1000 in little-endian = 0x000003E8)

### String

Length prefix followed by UTF-8 encoded bytes:

```
[length_prefix] [utf8_bytes...]
```

Example: `"Alice"` (5 bytes)
```
05 41 6C 69 63 65
|  |__________|
|       |
|       +-- "Alice" in UTF-8
+-- length = 5
```

### Bytes

Length prefix followed by raw bytes:

```
[length_prefix] [raw_bytes...]
```

### Array (Repeated)

Count (int32) followed by elements:

```
[int32 count] [element_0] [element_1] ... [element_n-1]
```

Example: `[1, 2, 3]`
```
03 00 00 00  01 00 00 00  02 00 00 00  03 00 00 00
|__________|  |__________|  |__________|  |__________|
     |            |            |            |
     +-- count=3  +-- 1        +-- 2        +-- 3
```

### Map

Count (int32) followed by key-value pairs:

```
[int32 count] [key_0] [value_0] [key_1] [value_1] ...
```

Example: `{"a": 1, "b": 2}`
```
02 00 00 00  01 61  01 00 00 00  01 62  02 00 00 00
|__________|  |__| |__|  |__________|  |__| |__|  |__________|
     |         |    |       |            |    |       |
     +-- count  + len + key  + value      + len + key  + value
```

## Message Structure

Messages encode fields in declaration order without field tags or numbers.

### Example Message

```xpb
message User {
    1: string name
    2: int32 age
    3: bool active
}
```

For `name="Bob", age=25, active=true`:

```
03 42 6F 62  19 00 00 00  01
|  |_______|  |________|  |__|
|      |          |         |
|      |          |         +-- bool = true
|      |          +-- int32 = 25 (0x00000019)
|      +-- length=3 + "Bob"
+-- length=3
```

Total: 10 bytes

### Complete Message Byte Breakdown

```
Offset  Size  Value      Description
------  ----  -----      -----------
0       1     0x03       length = 3
1       3     "Bob"      string data
4       4     0x19000000 int32 = 25 (LE)
8       1     0x01       bool = true
```

## Nested Messages

Nested messages are encoded as length-prefixed bytes:

```
[length_prefix] [nested_message_bytes...]
```

Example: Address nested in User

```xpb
message Address {
    1: string city
    2: string country
}

message User {
    1: string name
    2: Address addr
}
```

For `name="Alice", addr={city:"NYC", country:"USA"}`:

```
05 41 6C 69 63 65           // name = "Alice" (5 bytes)
03 03 4E 43 59  03 55 53 41 // Address: city="NYC" (3) + "NYC" + country="USA" (3) + "USA"
                            // Total nested: 3 + 3 + 3 + 3 = 12 bytes
```

## Enums

Enums are encoded as int32 values:

```xpb
enum Status {
    ACTIVE = 1
    INACTIVE = 2
}
```

`Status.ACTIVE` encodes as `01 00 00 00` (int32 1)

## Error Conditions

### Truncated Data

If decoder reads past end of buffer, returns `io.ErrUnexpectedEOF`.

### Invalid Compact Length

If 0xFF marker is followed by insufficient bytes for uint32, returns error.

### String Encoding

Strings are treated as UTF-8. Invalid UTF-8 sequences are decoded as-is (per Unicode replacement behavior).

## Byte Order Markers

None. All multi-byte values are little-endian by default.

## Reserved Values

- `0xFF` used as marker for extended length encoding

## Comparison with Other Formats

### Message: `{name: "A", age: 30, active: true}`

| Format | Bytes | Notes |
|--------|-------|-------|
| XPB V2 | 11 | No field tags |
| Protobuf | 19 | 1 byte tag per field |
| MessagePack | 33 | Type indicator per value |
| JSON | 47 | Key names included |

### XPB V2 Byte Layout

```
Offset  Size  Value    Field
------  ----  -----    -----
0       1     0x01     name length = 1
1       1     0x41     "A" (ASCII 65)
2       4     0x1E000000 age = 30
6       1     0x01     active = true
```

## Encoder/Decoder Interface

### Go

```go
type Encoder struct {
    buf []byte
}

func (e *Encoder) WriteBool(v bool)
func (e *Encoder) WriteInt32(v int32)
func (e *Encoder) WriteInt64(v int64)
func (e *Encoder) WriteUint32(v uint32)
func (e *Encoder) WriteUint64(v uint64)
func (e *Encoder) WriteFloat32(v float32)
func (e *Encoder) WriteFloat64(v float64)
func (e *Encoder) WriteString(v string)
func (e *Encoder) WriteBytes(v []byte)
func (e *Encoder) WriteMessage(data []byte)
func (e *Encoder) Bytes() []byte

type Decoder struct {
    buf []byte
    pos int
}

func (d *Decoder) ReadBool() (bool, error)
func (d *Decoder) ReadInt32() (int32, error)
func (d *Decoder) ReadInt64() (int64, error)
func (d *Decoder) ReadUint32() (uint32, error)
func (d *Decoder) ReadUint64() (uint64, error)
func (d *Decoder) ReadFloat32() (float32, error)
func (d *Decoder) ReadFloat64() (float64, error)
func (d *Decoder) ReadString() (string, error)
func (d *Decoder) ReadBytes() ([]byte, error)
func (d *Decoder) ReadMessageBytes() ([]byte, error)
```

### TypeScript

```typescript
class Encoder {
    writeBool(v: boolean): void
    writeInt32(v: number): void
    writeInt64(v: bigint): void
    writeUint32(v: number): void
    writeUint64(v: bigint): void
    writeFloat32(v: number): void
    writeFloat64(v: number): void
    writeString(v: string): void
    writeBytes(v: Uint8Array): void
    writeMessage(data: Uint8Array): void
    finish(): Uint8Array
}

class Decoder {
    readBool(): boolean
    readInt32(): number
    readInt64(): bigint
    readUint32(): number
    readUint64(): bigint
    readFloat32(): number
    readFloat64(): number
    readString(): string
    readBytes(): Uint8Array
    readMessageBytes(): Uint8Array
    eof(): boolean
    remaining(): number
}
```

## Related Files

- `pkg/wire/wire.go` - Wire format constants
- `runtime/go/xpb/xpb.go` - Go implementation
- `runtime/ts/src/index.ts` - TypeScript implementation

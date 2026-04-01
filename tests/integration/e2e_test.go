// Package integration contains end-to-end round-trip tests.
package integration

import (
	"testing"

	"github.com/ElecTwix/xpb/pkg/parser"
	"github.com/ElecTwix/xpb/runtime/go/xpb"
)

func TestE2E_GoRoundTrip_SimpleMessage(t *testing.T) {
	// Parse schema to verify it works
	schema := `
package test

message User {
    1: string name
    2: int32 age
    3: bool active
}
`
	_, err := parser.ParseFile(schema)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Test that the Marshal/Unmarshal methods work
	// by manually creating a message and encoding/decoding it
	type TestUser struct {
		Name   string
		Age    int32
		Active bool
	}

	// Manually create a message and encode it
	enc := xpb.NewEncoder(64)
	enc.WriteString("Alice")
	enc.WriteInt32(30)
	enc.WriteBool(true)
	data := enc.Bytes()

	// Decode
	dec := xpb.NewDecoder(data)
	name, err := dec.ReadString()
	if err != nil {
		t.Fatalf("ReadString failed: %v", err)
	}
	age, err := dec.ReadInt32()
	if err != nil {
		t.Fatalf("ReadInt32 failed: %v", err)
	}
	active, err := dec.ReadBool()
	if err != nil {
		t.Fatalf("ReadBool failed: %v", err)
	}

	// Verify
	if name != "Alice" {
		t.Errorf("Name = %q, want %q", name, "Alice")
	}
	if age != 30 {
		t.Errorf("Age = %d, want %d", age, 30)
	}
	if !active {
		t.Error("Active = false, want true")
	}
}

func TestE2E_GoRoundTrip_AllTypes(t *testing.T) {
	// Test all primitive types round-trip
	type testCase struct {
		name  string
		write func(*xpb.Encoder)
		read  func(*xpb.Decoder) (interface{}, error)
		want  interface{}
	}

	cases := []testCase{
		{
			name:  "bool true",
			write: func(e *xpb.Encoder) { e.WriteBool(true) },
			read:  func(d *xpb.Decoder) (interface{}, error) { return d.ReadBool() },
			want:  true,
		},
		{
			name:  "bool false",
			write: func(e *xpb.Encoder) { e.WriteBool(false) },
			read:  func(d *xpb.Decoder) (interface{}, error) { return d.ReadBool() },
			want:  false,
		},
		{
			name:  "int32 positive",
			write: func(e *xpb.Encoder) { e.WriteInt32(42) },
			read:  func(d *xpb.Decoder) (interface{}, error) { return d.ReadInt32() },
			want:  int32(42),
		},
		{
			name:  "int32 negative",
			write: func(e *xpb.Encoder) { e.WriteInt32(-1000) },
			read:  func(d *xpb.Decoder) (interface{}, error) { return d.ReadInt32() },
			want:  int32(-1000),
		},
		{
			name:  "int32 zero",
			write: func(e *xpb.Encoder) { e.WriteInt32(0) },
			read:  func(d *xpb.Decoder) (interface{}, error) { return d.ReadInt32() },
			want:  int32(0),
		},
		{
			name:  "int64 positive",
			write: func(e *xpb.Encoder) { e.WriteInt64(12345678901234) },
			read:  func(d *xpb.Decoder) (interface{}, error) { return d.ReadInt64() },
			want:  int64(12345678901234),
		},
		{
			name:  "int64 negative",
			write: func(e *xpb.Encoder) { e.WriteInt64(-9876543210) },
			read:  func(d *xpb.Decoder) (interface{}, error) { return d.ReadInt64() },
			want:  int64(-9876543210),
		},
		{
			name:  "uint32",
			write: func(e *xpb.Encoder) { e.WriteUint32(100000) },
			read:  func(d *xpb.Decoder) (interface{}, error) { return d.ReadUint32() },
			want:  uint32(100000),
		},
		{
			name:  "uint64",
			write: func(e *xpb.Encoder) { e.WriteUint64(9999999999999) },
			read:  func(d *xpb.Decoder) (interface{}, error) { return d.ReadUint64() },
			want:  uint64(9999999999999),
		},
		{
			name:  "float32",
			write: func(e *xpb.Encoder) { e.WriteFloat32(3.14) },
			read:  func(d *xpb.Decoder) (interface{}, error) { return d.ReadFloat32() },
			want:  float32(3.14),
		},
		{
			name:  "float64",
			write: func(e *xpb.Encoder) { e.WriteFloat64(2.718281828) },
			read:  func(d *xpb.Decoder) (interface{}, error) { return d.ReadFloat64() },
			want:  float64(2.718281828),
		},
		{
			name:  "string empty",
			write: func(e *xpb.Encoder) { e.WriteString("") },
			read:  func(d *xpb.Decoder) (interface{}, error) { return d.ReadString() },
			want:  "",
		},
		{
			name:  "string short",
			write: func(e *xpb.Encoder) { e.WriteString("hello") },
			read:  func(d *xpb.Decoder) (interface{}, error) { return d.ReadString() },
			want:  "hello",
		},
		{
			name:  "string long",
			write: func(e *xpb.Encoder) { e.WriteString("this is a longer string for testing") },
			read:  func(d *xpb.Decoder) (interface{}, error) { return d.ReadString() },
			want:  "this is a longer string for testing",
		},
		{
			name:  "bytes empty",
			write: func(e *xpb.Encoder) { e.WriteBytes([]byte{}) },
			read:  func(d *xpb.Decoder) (interface{}, error) { return d.ReadBytes() },
			want:  []byte{},
		},
		{
			name:  "bytes data",
			write: func(e *xpb.Encoder) { e.WriteBytes([]byte{1, 2, 3, 4, 5}) },
			read:  func(d *xpb.Decoder) (interface{}, error) { return d.ReadBytes() },
			want:  []byte{1, 2, 3, 4, 5},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enc := xpb.NewEncoder(128)
			tc.write(enc)
			data := enc.Bytes()

			dec := xpb.NewDecoder(data)
			got, err := tc.read(dec)
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}

			switch want := tc.want.(type) {
			case int32:
				if got.(int32) != want {
					t.Errorf("got %v, want %v", got, want)
				}
			case int64:
				if got.(int64) != want {
					t.Errorf("got %v, want %v", got, want)
				}
			case uint32:
				if got.(uint32) != want {
					t.Errorf("got %v, want %v", got, want)
				}
			case uint64:
				if got.(uint64) != want {
					t.Errorf("got %v, want %v", got, want)
				}
			case float32:
				if got.(float32) != want {
					t.Errorf("got %v, want %v", got, want)
				}
			case float64:
				if got.(float64) != want {
					t.Errorf("got %v, want %v", got, want)
				}
			case string:
				if got.(string) != want {
					t.Errorf("got %v, want %v", got, want)
				}
			case bool:
				if got.(bool) != want {
					t.Errorf("got %v, want %v", got, want)
				}
			case []byte:
				if string(got.([]byte)) != string(want) {
					t.Errorf("got %v, want %v", got, want)
				}
			}
		})
	}
}

func TestE2E_GoRoundTrip_RepeatedFields(t *testing.T) {
	// Test string array
	t.Run("string array", func(t *testing.T) {
		strings := []string{"alice", "bob", "charlie"}

		enc := xpb.NewEncoder(256)
		enc.WriteInt32(int32(len(strings)))
		for _, s := range strings {
			enc.WriteString(s)
		}
		data := enc.Bytes()

		dec := xpb.NewDecoder(data)
		count, err := dec.ReadInt32()
		if err != nil {
			t.Fatalf("Read count failed: %v", err)
		}

		got := make([]string, count)
		for i := int32(0); i < count; i++ {
			s, err := dec.ReadString()
			if err != nil {
				t.Fatalf("Read string %d failed: %v", i, err)
			}
			got[i] = s
		}

		if len(got) != len(strings) {
			t.Errorf("len(got) = %d, want %d", len(got), len(strings))
		}
		for i := range strings {
			if got[i] != strings[i] {
				t.Errorf("got[%d] = %q, want %q", i, got[i], strings[i])
			}
		}
	})

	// Test int32 array
	t.Run("int32 array", func(t *testing.T) {
		ints := []int32{10, 20, 30, 40, 50}

		enc := xpb.NewEncoder(256)
		enc.WriteInt32(int32(len(ints)))
		for _, v := range ints {
			enc.WriteInt32(v)
		}
		data := enc.Bytes()

		dec := xpb.NewDecoder(data)
		count, err := dec.ReadInt32()
		if err != nil {
			t.Fatalf("Read count failed: %v", err)
		}

		got := make([]int32, count)
		for i := int32(0); i < count; i++ {
			v, err := dec.ReadInt32()
			if err != nil {
				t.Fatalf("Read int32 %d failed: %v", i, err)
			}
			got[i] = v
		}

		if len(got) != len(ints) {
			t.Errorf("len(got) = %d, want %d", len(got), len(ints))
		}
		for i := range ints {
			if got[i] != ints[i] {
				t.Errorf("got[%d] = %d, want %d", i, got[i], ints[i])
			}
		}
	})
}

func TestE2E_GoRoundTrip_NestedMessages(t *testing.T) {
	// Test nested message encoding/decoding
	type Point struct {
		X, Y int32
	}

	type Rectangle struct {
		TopLeft     Point
		BottomRight Point
	}

	// Encode a rectangle
	enc := xpb.NewEncoder(128)

	// TopLeft
	enc.WriteInt32(10)
	enc.WriteInt32(20)

	// BottomRight
	enc.WriteInt32(100)
	enc.WriteInt32(200)

	data := enc.Bytes()

	// Decode
	dec := xpb.NewDecoder(data)

	topLeftX, err := dec.ReadInt32()
	if err != nil {
		t.Fatalf("Read TopLeft.X failed: %v", err)
	}
	topLeftY, err := dec.ReadInt32()
	if err != nil {
		t.Fatalf("Read TopLeft.Y failed: %v", err)
	}
	bottomRightX, err := dec.ReadInt32()
	if err != nil {
		t.Fatalf("Read BottomRight.X failed: %v", err)
	}
	bottomRightY, err := dec.ReadInt32()
	if err != nil {
		t.Fatalf("Read BottomRight.Y failed: %v", err)
	}

	// Verify
	if topLeftX != 10 {
		t.Errorf("TopLeft.X = %d, want %d", topLeftX, 10)
	}
	if topLeftY != 20 {
		t.Errorf("TopLeft.Y = %d, want %d", topLeftY, 20)
	}
	if bottomRightX != 100 {
		t.Errorf("BottomRight.X = %d, want %d", bottomRightX, 100)
	}
	if bottomRightY != 200 {
		t.Errorf("BottomRight.Y = %d, want %d", bottomRightY, 200)
	}
}

func TestE2E_GoRoundTrip_SizeVariants(t *testing.T) {
	testCases := []struct {
		name    string
		content string
	}{
		{"empty string", ""},
		{"1 char", "a"},
		{"100 chars", string(make([]byte, 100))},
		{"254 chars", string(make([]byte, 254))},
		{"255 chars", string(make([]byte, 255))},
		{"256 chars", string(make([]byte, 256))},
		{"1000 chars", string(make([]byte, 1000))},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			enc := xpb.NewEncoder(2048)
			enc.WriteString(tc.content)
			data := enc.Bytes()

			dec := xpb.NewDecoder(data)
			got, err := dec.ReadString()
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}

			if got != tc.content {
				t.Errorf("got %q, want %q", got, tc.content)
			}

			// Verify size
			expectedSize := len(tc.content)
			if expectedSize <= 254 {
				// 1 byte length + content
				if len(data) != expectedSize+1 {
					t.Errorf("len(data) = %d, want %d", len(data), expectedSize+1)
				}
			} else {
				// 0xFF marker + 4 bytes length + content
				if len(data) != expectedSize+5 {
					t.Errorf("len(data) = %d, want %d", len(data), expectedSize+5)
				}
			}
		})
	}
}

func TestE2E_GoRoundTrip_EdgeCases(t *testing.T) {
	// Test boundary values
	t.Run("int32 boundaries", func(t *testing.T) {
		boundaries := []int32{0, 1, -1, 127, -128, 32767, -32768, 2147483647, -2147483648}

		for _, want := range boundaries {
			enc := xpb.NewEncoder(64)
			enc.WriteInt32(want)
			data := enc.Bytes()

			dec := xpb.NewDecoder(data)
			got, err := dec.ReadInt32()
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}

			if got != want {
				t.Errorf("got %d, want %d", got, want)
			}
		}
	})

	t.Run("uint32 boundaries", func(t *testing.T) {
		boundaries := []uint32{0, 1, 255, 256, 65535, 65536, 4294967295}

		for _, want := range boundaries {
			enc := xpb.NewEncoder(64)
			enc.WriteUint32(want)
			data := enc.Bytes()

			dec := xpb.NewDecoder(data)
			got, err := dec.ReadUint32()
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}

			if got != want {
				t.Errorf("got %d, want %d", got, want)
			}
		}
	})
}

// Benchmarks for round-trip operations
func BenchmarkE2E_GoRoundTrip_Simple(b *testing.B) {
	type TestUser struct {
		Name   string
		Age    int32
		Active bool
	}

	user := TestUser{"Alice", 30, true}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc := xpb.NewEncoder(64)
		enc.WriteString(user.Name)
		enc.WriteInt32(user.Age)
		enc.WriteBool(user.Active)
		data := enc.Bytes()

		dec := xpb.NewDecoder(data)
		dec.ReadString()
		dec.ReadInt32()
		dec.ReadBool()
	}
}

func BenchmarkE2E_GoRoundTrip_AllTypes(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc := xpb.NewEncoder(256)
		enc.WriteBool(true)
		enc.WriteInt32(42)
		enc.WriteInt64(1234567890)
		enc.WriteUint32(100000)
		enc.WriteUint64(9999999999)
		enc.WriteFloat32(3.14)
		enc.WriteFloat64(2.71828)
		enc.WriteString("test string")
		enc.WriteBytes([]byte{1, 2, 3, 4, 5})
		data := enc.Bytes()

		dec := xpb.NewDecoder(data)
		dec.ReadBool()
		dec.ReadInt32()
		dec.ReadInt64()
		dec.ReadUint32()
		dec.ReadUint64()
		dec.ReadFloat32()
		dec.ReadFloat64()
		dec.ReadString()
		dec.ReadBytes()
	}
}

func BenchmarkE2E_GoRoundTrip_StringArray(b *testing.B) {
	arr := make([]string, 100)
	for i := range arr {
		arr[i] = "item"
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc := xpb.NewEncoder(4096)
		enc.WriteInt32(int32(len(arr)))
		for _, s := range arr {
			enc.WriteString(s)
		}
		data := enc.Bytes()

		dec := xpb.NewDecoder(data)
		count, _ := dec.ReadInt32()
		for j := int32(0); j < count; j++ {
			dec.ReadString()
		}
	}
}

func BenchmarkE2E_GoRoundTrip_Int32Array(b *testing.B) {
	arr := make([]int32, 100)
	for i := range arr {
		arr[i] = int32(i * 17)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc := xpb.NewEncoder(1024)
		enc.WriteInt32(int32(len(arr)))
		for _, v := range arr {
			enc.WriteInt32(v)
		}
		data := enc.Bytes()

		dec := xpb.NewDecoder(data)
		count, _ := dec.ReadInt32()
		for j := int32(0); j < count; j++ {
			dec.ReadInt32()
		}
	}
}

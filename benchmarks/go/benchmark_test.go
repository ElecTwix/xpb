// Package benchmark provides benchmarks comparing XPB to other serialization formats.
package benchmark

import (
	"testing"

	"github.com/anthropic/xpb/pkg/wire"
	"github.com/anthropic/xpb/runtime/go/xpb"
)

// SimpleUser represents a simple test message for benchmarking.
type SimpleUser struct {
	Name   string
	Age    int32
	Active bool
}

// Marshal encodes the SimpleUser to XPB format.
func (u *SimpleUser) Marshal() ([]byte, error) {
	enc := xpb.NewEncoder(64)
	enc.WriteString(1, u.Name)
	enc.WriteInt32(2, u.Age)
	enc.WriteBool(3, u.Active)
	return enc.Bytes(), nil
}

// Unmarshal decodes XPB format into SimpleUser.
func (u *SimpleUser) Unmarshal(data []byte) error {
	dec := xpb.NewDecoder(data)
	for !dec.EOF() {
		fieldNum, wireType, err := dec.ReadTag()
		if err != nil {
			return err
		}
		switch fieldNum {
		case 1:
			v, err := dec.ReadString()
			if err != nil {
				return err
			}
			u.Name = v
		case 2:
			v, err := dec.ReadInt32()
			if err != nil {
				return err
			}
			u.Age = v
		case 3:
			v, err := dec.ReadBool()
			if err != nil {
				return err
			}
			u.Active = v
		default:
			if err := dec.Skip(wireType); err != nil {
				return err
			}
		}
	}
	return nil
}

var _ = wire.WireVarint // suppress unused import warning

func BenchmarkXPB_Encode(b *testing.B) {
	user := &SimpleUser{
		Name:   "Alice Johnson",
		Age:    30,
		Active: true,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := user.Marshal()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkXPB_Decode(b *testing.B) {
	user := &SimpleUser{
		Name:   "Alice Johnson",
		Age:    30,
		Active: true,
	}
	data, err := user.Marshal()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		u := &SimpleUser{}
		if err := u.Unmarshal(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkXPB_RoundTrip(b *testing.B) {
	user := &SimpleUser{
		Name:   "Alice Johnson",
		Age:    30,
		Active: true,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		data, err := user.Marshal()
		if err != nil {
			b.Fatal(err)
		}
		u := &SimpleUser{}
		if err := u.Unmarshal(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkXPB_EncodeSize(b *testing.B) {
	user := &SimpleUser{
		Name:   "Alice Johnson",
		Age:    30,
		Active: true,
	}

	data, err := user.Marshal()
	if err != nil {
		b.Fatal(err)
	}

	b.ReportMetric(float64(len(data)), "bytes")
}

// LargerMessage tests with more fields
type LargerMessage struct {
	ID          uint64
	Name        string
	Email       string
	Age         int32
	Score       float64
	Active      bool
	Description string
	Tags        string // Would be repeated in full implementation
}

func (m *LargerMessage) Marshal() ([]byte, error) {
	enc := xpb.NewEncoder(256)
	enc.WriteUint64(1, m.ID)
	enc.WriteString(2, m.Name)
	enc.WriteString(3, m.Email)
	enc.WriteInt32(4, m.Age)
	enc.WriteFloat64(5, m.Score)
	enc.WriteBool(6, m.Active)
	enc.WriteString(7, m.Description)
	enc.WriteString(8, m.Tags)
	return enc.Bytes(), nil
}

func (m *LargerMessage) Unmarshal(data []byte) error {
	dec := xpb.NewDecoder(data)
	for !dec.EOF() {
		fieldNum, wireType, err := dec.ReadTag()
		if err != nil {
			return err
		}
		switch fieldNum {
		case 1:
			v, err := dec.ReadUint64()
			if err != nil {
				return err
			}
			m.ID = v
		case 2:
			v, err := dec.ReadString()
			if err != nil {
				return err
			}
			m.Name = v
		case 3:
			v, err := dec.ReadString()
			if err != nil {
				return err
			}
			m.Email = v
		case 4:
			v, err := dec.ReadInt32()
			if err != nil {
				return err
			}
			m.Age = v
		case 5:
			v, err := dec.ReadFloat64()
			if err != nil {
				return err
			}
			m.Score = v
		case 6:
			v, err := dec.ReadBool()
			if err != nil {
				return err
			}
			m.Active = v
		case 7:
			v, err := dec.ReadString()
			if err != nil {
				return err
			}
			m.Description = v
		case 8:
			v, err := dec.ReadString()
			if err != nil {
				return err
			}
			m.Tags = v
		default:
			if err := dec.Skip(wireType); err != nil {
				return err
			}
		}
	}
	return nil
}

func BenchmarkXPB_EncodeLarge(b *testing.B) {
	msg := &LargerMessage{
		ID:          12345678901234,
		Name:        "Alice Johnson",
		Email:       "alice.johnson@example.com",
		Age:         30,
		Score:       95.5,
		Active:      true,
		Description: "This is a longer description field that contains more text to test encoding performance with larger strings.",
		Tags:        "tag1,tag2,tag3,tag4,tag5",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := msg.Marshal()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkXPB_DecodeLarge(b *testing.B) {
	msg := &LargerMessage{
		ID:          12345678901234,
		Name:        "Alice Johnson",
		Email:       "alice.johnson@example.com",
		Age:         30,
		Score:       95.5,
		Active:      true,
		Description: "This is a longer description field that contains more text to test encoding performance with larger strings.",
		Tags:        "tag1,tag2,tag3,tag4,tag5",
	}
	data, err := msg.Marshal()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		m := &LargerMessage{}
		if err := m.Unmarshal(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkXPB_EncodeLargeSize(b *testing.B) {
	msg := &LargerMessage{
		ID:          12345678901234,
		Name:        "Alice Johnson",
		Email:       "alice.johnson@example.com",
		Age:         30,
		Score:       95.5,
		Active:      true,
		Description: "This is a longer description field that contains more text to test encoding performance with larger strings.",
		Tags:        "tag1,tag2,tag3,tag4,tag5",
	}

	data, err := msg.Marshal()
	if err != nil {
		b.Fatal(err)
	}

	b.ReportMetric(float64(len(data)), "bytes")
}

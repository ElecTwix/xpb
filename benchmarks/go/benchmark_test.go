// Package benchmark provides benchmarks comparing XPB V2 to other serialization formats.
package benchmark

import (
	"testing"

	"github.com/ElecTwix/xpb/runtime/go/xpb"
)

// SimpleUser represents a simple test message for benchmarking.
// V2: Fields are encoded/decoded in order without tags.
type SimpleUser struct {
	Name   string
	Age    int32
	Active bool
}

// Marshal encodes the SimpleUser to XPB V2 format (no tags).
func (u *SimpleUser) Marshal() ([]byte, error) {
	enc := xpb.NewEncoder(64)
	enc.WriteString(u.Name)
	enc.WriteInt32(u.Age)
	enc.WriteBool(u.Active)
	return enc.Bytes(), nil
}

// Unmarshal decodes XPB V2 format into SimpleUser (reads fields in order).
func (u *SimpleUser) Unmarshal(data []byte) error {
	dec := xpb.NewDecoder(data)

	name, err := dec.ReadString()
	if err != nil {
		return err
	}
	u.Name = name

	age, err := dec.ReadInt32()
	if err != nil {
		return err
	}
	u.Age = age

	active, err := dec.ReadBool()
	if err != nil {
		return err
	}
	u.Active = active

	return nil
}

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
	Tags        string
}

func (m *LargerMessage) Marshal() ([]byte, error) {
	enc := xpb.NewEncoder(256)
	enc.WriteUint64(m.ID)
	enc.WriteString(m.Name)
	enc.WriteString(m.Email)
	enc.WriteInt32(m.Age)
	enc.WriteFloat64(m.Score)
	enc.WriteBool(m.Active)
	enc.WriteString(m.Description)
	enc.WriteString(m.Tags)
	return enc.Bytes(), nil
}

func (m *LargerMessage) Unmarshal(data []byte) error {
	dec := xpb.NewDecoder(data)

	id, err := dec.ReadUint64()
	if err != nil {
		return err
	}
	m.ID = id

	name, err := dec.ReadString()
	if err != nil {
		return err
	}
	m.Name = name

	email, err := dec.ReadString()
	if err != nil {
		return err
	}
	m.Email = email

	age, err := dec.ReadInt32()
	if err != nil {
		return err
	}
	m.Age = age

	score, err := dec.ReadFloat64()
	if err != nil {
		return err
	}
	m.Score = score

	active, err := dec.ReadBool()
	if err != nil {
		return err
	}
	m.Active = active

	desc, err := dec.ReadString()
	if err != nil {
		return err
	}
	m.Description = desc

	tags, err := dec.ReadString()
	if err != nil {
		return err
	}
	m.Tags = tags

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

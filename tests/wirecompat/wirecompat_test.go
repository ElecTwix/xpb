// Package wirecompat proves the cross-version wire-compatibility guarantee for
// the 0.4 -> 0.5 breaking boundary as an EXECUTABLE test rather than prose:
//
//	"The wire format is unchanged. Data encoded by an older xpb still decodes
//	 under the current (0.5.0, value-optionals + zero-copy-bytes) Go codegen."
//
// 0.5.0 was a *codegen API* break (the Go default flipped to value-style
// optionals + zero-copy bytes), NOT a wire-format break. This test pins known
// wire bytes -- the exact bytes an older encoder produced -- and decodes them
// with the CURRENT 0.5.0 generated code, asserting the decoded values are
// correct. The symmetric arm encodes with 0.5.0 and asserts byte-identity with
// the pinned vectors (for non-map messages; maps are non-canonical per T-7, so
// the map arm compares decoded VALUES, not bytes).
//
// The "current generated code" is produced authentically: this test invokes the
// real Go codegen (golang.Generate, whose zero-value Options{} is the 0.5.0
// value+zero-copy default), writes it into a throwaway module that imports the
// xpb runtime from THIS checkout, and `go test`s a driver against it -- the same
// real-compile pattern tests/integration/go_codegen_test.go uses. A decode
// mismatch, a byte mismatch, or a compile failure all fail loudly. If a pinned
// vector ever stops matching, that is a genuine wire regression: the fix is to
// the runtime/codegen, never to the pinned bytes.
//
// This file is self-contained: it does not edit any existing test file, and it
// does not modify the testdata/conformance vectors -- it only reads them.
package wirecompat

import (
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ElecTwix/xpb/pkg/codegen/golang"
	"github.com/ElecTwix/xpb/pkg/parser"
	"github.com/ElecTwix/xpb/runtime/go/xpb"
)

// wireSchema is a deliberately small schema chosen to cover the wire paths most
// likely to regress across the 0.4 -> 0.5 boundary:
//
//   - Account.email is `?string`  -> the optional presence flag (0x00 absent /
//     0x01 + value present), with a trailing int32 field so a mis-consumed
//     presence byte would corrupt a following, asserted field.
//   - Account.name is `string`    -> drives the compact-length 0xFF path when
//     >= 255 bytes (and the 1-byte boundary at 254).
//   - Account.token is `bytes`    -> under 0.5.0 this decodes via the zero-copy
//     ReadBytesUnsafe path; we assert the exact byte content.
//   - Bag has `?string` + `map<int32,string>` -> a map message; per T-7 maps are
//     non-canonical so its arm compares decoded values, not bytes.
const wireSchema = `
package wirecompat

message Account {
    1: string name
    2: int32 age
    3: bool active
    4: ?string email
    5: bytes token
    6: int32 trailer
}

message Bag {
    1: ?string note
    2: map<int32, string> labels
}
`

// --- Pinned wire vectors -------------------------------------------------------
//
// These hex strings are the EXACT bytes an XPB encoder (0.4 or 0.5 -- the wire
// format is identical) produces for the values described in each comment. They
// are the version-independent contract under test. They were derived from the
// documented wire format (struct mode, fixed-width little-endian, compact
// lengths: 1 byte if <= 254, else 0xFF + 4-byte LE length; optional = 1-byte
// presence flag 0x00/0x01) and are independently re-verified every run by the
// symmetric encode arm, which marshals the same values with the CURRENT codegen
// and asserts byte-identity. If you change one of these, you are asserting the
// wire format changed -- do not do that to make a test pass.
const (
	// Account{Name:"Al", Age:7, Active:true, email ABSENT, Token:{0xCA,0xFE}, Trailer:99}
	//   02 41 6c            name "Al" (len 2)
	//   07 00 00 00         age 7 (int32 LE)
	//   01                  active true
	//   00                  email presence flag = ABSENT  <-- optional 0x00 path
	//   02 ca fe            token (len 2) {0xCA,0xFE}
	//   63 00 00 00         trailer 99 (int32 LE)
	pinnedAccountEmailAbsent = "02416c07000000010002cafe63000000"

	// Account{Name:"Al", Age:7, Active:true, Email:"x@y", Token:{0xCA,0xFE}, Trailer:99}
	//   02 41 6c            name "Al"
	//   07 00 00 00 01      age 7, active true
	//   01                  email presence flag = PRESENT  <-- optional 0x01 path
	//   03 78 40 79         email "x@y" (len 3)
	//   02 ca fe            token {0xCA,0xFE}
	//   63 00 00 00         trailer 99
	pinnedAccountEmailPresent = "02416c0700000001010378407902cafe63000000"

	// Account{Name: 255*"N", Age:1, Active:false, email ABSENT, Token:{}, Trailer:7}
	//   ff ff 00 00 00      name length 255 via the COMPACT-LENGTH 0xFF path
	//                       (0xFF marker + 4-byte LE 255)  <-- 0xFF length path
	//   <255 * 0x4e ("N")>
	//   01 00               age 1, active false
	//   00                  email ABSENT
	//   00                  token length 0
	//   07 00 00 00         trailer 7
	pinnedAccountName255 = "ffff0000004e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e4e0100000000000007000000"

	// Account{Name:"B", Age:0, Active:false, email ABSENT, Token:{0x00,0x01,0xFE,0xFF}, Trailer:-1}
	//   01 42               name "B"
	//   00 00 00 00 00      age 0, active false
	//   00                  email ABSENT
	//   04 00 01 fe ff      token (len 4) {0x00,0x01,0xFE,0xFF}  <-- bytes path
	//   ff ff ff ff         trailer -1 (int32 LE = 0xFFFFFFFF)
	pinnedAccountToken = "0142000000000000040001feffffffffff"

	// Bag{note ABSENT, Labels:{42:"answer"}} -- a single-entry map is the one
	// deterministic map case (one iteration order), so its bytes are stable:
	//   00                  note ABSENT
	//   01 00 00 00         map count 1 (int32 LE)
	//   2a 00 00 00         key 42 (int32 LE)
	//   06 61 6e 73 77 65 72 value "answer" (len 6)
	pinnedBagOneLabel = "00010000002a00000006616e73776572"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad pinned hex %q: %v", s, err)
	}
	return b
}

// --- Throwaway-module harness (generated 0.5.0 code, real compile) -------------

// repoRoot resolves the repository root from this file's own location, so it is
// independent of the test's working directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate repo root")
	}
	// This file lives at <root>/tests/wirecompat/wirecompat_test.go.
	root, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("computed repo root %q has no go.mod: %v", root, err)
	}
	return root
}

// generateCurrentGo parses wireSchema and generates Go with the DEFAULT 0.5.0
// options (golang.Generate == zero-value Options == value optionals + zero-copy
// bytes). This is exactly the "current 0.5.0 generated code" the wire-compat
// claim is about.
func generateCurrentGo(t *testing.T) []byte {
	t.Helper()
	file, err := parser.ParseFile(wireSchema)
	if err != nil {
		t.Fatalf("parse wireSchema: %v", err)
	}
	src, err := golang.Generate(file)
	if err != nil {
		t.Fatalf("generate (0.5.0 default): %v\n%s", err, src)
	}
	return src
}

// buildGenModule writes a throwaway module: the generated package (rewritten to
// `package gen`), the supplied driver, and a go.mod whose replace points the xpb
// import at this checkout. Returns the module directory.
func buildGenModule(t *testing.T, genSrc []byte, driverSrc string) string {
	t.Helper()
	root := repoRoot(t)
	dir := t.TempDir()

	src := rewritePackageClause(t, string(genSrc), "gen")
	if err := os.WriteFile(filepath.Join(dir, "generated.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write generated.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "driver_test.go"), []byte(driverSrc), 0o644); err != nil {
		t.Fatalf("write driver_test.go: %v", err)
	}
	goMod := "module wirecompatgen\n\ngo 1.23\n\n" +
		"require github.com/ElecTwix/xpb v0.0.0\n\n" +
		"replace github.com/ElecTwix/xpb => " + root + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	return dir
}

func rewritePackageClause(t *testing.T, src, pkg string) string {
	t.Helper()
	lines := strings.Split(src, "\n")
	for i, ln := range lines {
		if strings.HasPrefix(ln, "package ") {
			lines[i] = "package " + pkg
			return strings.Join(lines, "\n")
		}
	}
	t.Fatalf("generated source has no package clause:\n%s", src)
	return ""
}

// goTestModule runs `go test` in the throwaway module. `go test` compiles the
// package, so this is the authoritative compile + run check.
func goTestModule(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("go", "test", "-count=1", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated wire-compat module failed `go test`: %v\n--- output ---\n%s", err, out)
	}
}

// driver is the test program that runs INSIDE the throwaway module against the
// freshly generated 0.5.0 code. It receives the pinned vectors as hex constants
// and proves, for the generated Account/Bag types:
//
//   - decode of old-style pinned bytes yields the correct values (AC1,2,4,5);
//   - 0.5.0 encode of the same values is byte-identical to the pinned bytes for
//     the non-map Account cases (AC3,4);
//   - the map-bearing Bag decodes the pinned single-entry map correctly and
//     round-trips a multi-entry map by VALUE (not bytes), per T-7 (AC6).
//
// Any mismatch t.Fatalf's, which fails goTestModule loudly.
const driver = `package gen

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"
)

const (
	pinnedAccountEmailAbsent  = "` + pinnedAccountEmailAbsent + `"
	pinnedAccountEmailPresent = "` + pinnedAccountEmailPresent + `"
	pinnedAccountName255      = "` + pinnedAccountName255 + `"
	pinnedAccountToken        = "` + pinnedAccountToken + `"
	pinnedBagOneLabel         = "` + pinnedBagOneLabel + `"
)

func mh(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex: %v", err)
	}
	return b
}

func strN(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'N'
	}
	return string(b)
}

// AC1: optional ABSENT (presence flag 0x00). Old-style bytes decode under 0.5.0.
func TestDecode_OptionalAbsent(t *testing.T) {
	var a Account
	if err := a.Unmarshal(mh(t, pinnedAccountEmailAbsent)); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if a.Name != "Al" || a.Age != 7 || !a.Active {
		t.Fatalf("scalars: %+v", a)
	}
	if a.HasEmail {
		t.Fatalf("absent optional must decode to HasEmail=false, got Email=%q", a.Email)
	}
	if !bytes.Equal(a.Token, []byte{0xCA, 0xFE}) {
		t.Fatalf("token after absent optional corrupted: % x", a.Token)
	}
	// The trailer proves the 0x00 presence byte was consumed (exactly one byte,
	// no value): a mis-consumed flag would shift this int32.
	if a.Trailer != 99 {
		t.Fatalf("trailer after absent optional corrupted: got %d want 99", a.Trailer)
	}
}

// AC2: optional PRESENT (presence flag 0x01 + value).
func TestDecode_OptionalPresent(t *testing.T) {
	var a Account
	if err := a.Unmarshal(mh(t, pinnedAccountEmailPresent)); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !a.HasEmail || a.Email != "x@y" {
		t.Fatalf("present optional: has=%v email=%q", a.HasEmail, a.Email)
	}
	if a.Name != "Al" || a.Age != 7 || !a.Active || !bytes.Equal(a.Token, []byte{0xCA, 0xFE}) || a.Trailer != 99 {
		t.Fatalf("surrounding fields corrupted: %+v", a)
	}
}

// AC3: symmetric encode (non-map). 0.5.0 Marshal reproduces the pinned bytes
// byte-for-byte for both the absent and present optional cases.
func TestEncode_ByteIdentityNonMap(t *testing.T) {
	cases := []struct {
		name   string
		in     *Account
		pinned string
	}{
		{
			name:   "email_absent",
			in:     &Account{Name: "Al", Age: 7, Active: true, HasEmail: false, Token: []byte{0xCA, 0xFE}, Trailer: 99},
			pinned: pinnedAccountEmailAbsent,
		},
		{
			name:   "email_present",
			in:     &Account{Name: "Al", Age: 7, Active: true, Email: "x@y", HasEmail: true, Token: []byte{0xCA, 0xFE}, Trailer: 99},
			pinned: pinnedAccountEmailPresent,
		},
		{
			name:   "token_bytes",
			in:     &Account{Name: "B", Age: 0, Active: false, HasEmail: false, Token: []byte{0x00, 0x01, 0xFE, 0xFF}, Trailer: -1},
			pinned: pinnedAccountToken,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.in.Marshal()
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			want := mh(t, tc.pinned)
			if !bytes.Equal(got, want) {
				t.Fatalf("WIRE REGRESSION: 0.5.0 encode != pinned bytes\n got:  %x\n want: %x", got, want)
			}
		})
	}
}

// AC4: compact-length 0xFF path. A 255-byte string is length-prefixed with the
// 0xFF marker + 4-byte LE length. Decode of pinned bytes yields the value, and
// 0.5.0 encode reproduces the pinned bytes byte-for-byte. The 254-byte boundary
// (single-byte 0xFE length) is checked alongside it.
func TestWire_CompactLength0xFF(t *testing.T) {
	pinned := mh(t, pinnedAccountName255)
	// Sanity: the length prefix really is the 0xFF compact-length form.
	if pinned[0] != 0xFF || pinned[1] != 0xFF || pinned[2] != 0x00 || pinned[3] != 0x00 || pinned[4] != 0x00 {
		t.Fatalf("pinned vector does not start with the 0xFF+LE(255) length prefix: % x", pinned[:5])
	}

	var a Account
	if err := a.Unmarshal(pinned); err != nil {
		t.Fatalf("Unmarshal len255: %v", err)
	}
	if len(a.Name) != 255 || a.Name != strN(255) {
		t.Fatalf("len255 name decode wrong: len=%d", len(a.Name))
	}
	if a.Age != 1 || a.Active || a.HasEmail || len(a.Token) != 0 || a.Trailer != 7 {
		t.Fatalf("fields after 0xFF-length string corrupted: %+v", a)
	}

	// Symmetric encode arm.
	got, err := (&Account{Name: strN(255), Age: 1, Active: false, HasEmail: false, Token: []byte{}, Trailer: 7}).Marshal()
	if err != nil {
		t.Fatalf("Marshal len255: %v", err)
	}
	if !bytes.Equal(got, pinned) {
		t.Fatalf("WIRE REGRESSION: 0.5.0 encode of 255-byte string != pinned\n got:  %x\n want: %x", got, pinned)
	}

	// Boundary: 254 bytes is the last single-byte length (0xFE), not the 0xFF path.
	got254, err := (&Account{Name: strN(254), Age: 0, Active: false, HasEmail: false, Token: []byte{}, Trailer: 0}).Marshal()
	if err != nil {
		t.Fatalf("Marshal len254: %v", err)
	}
	if got254[0] != 0xFE {
		t.Fatalf("254-byte string must use the single-byte length 0xFE, got 0x%02x", got254[0])
	}
	// The 254 prefix must be a SINGLE byte: 1 length byte + 254 payload, then the
	// remaining fixed fields (5 run + 1 presence + 1 token-len + 4 trailer = 11),
	// so the full message is 1+254+11 = 266 bytes. Decode it back and assert the
	// value to prove the single-byte length is consumed correctly (not just that
	// byte 0 happens to be 0xFE).
	if len(got254) != 266 {
		t.Fatalf("254-byte message length = %d, want 266 (1 len + 254 payload + 11 fixed)", len(got254))
	}
	var a254 Account
	if err := a254.Unmarshal(got254); err != nil {
		t.Fatalf("Unmarshal len254: %v", err)
	}
	if len(a254.Name) != 254 || a254.Name != strN(254) || a254.Trailer != 0 {
		t.Fatalf("254-byte round-trip wrong: nameLen=%d trailer=%d", len(a254.Name), a254.Trailer)
	}
}

// TestDecode_TruncatedFailsLoudly proves the message decoder rejects malformed
// (truncated) old-format bytes rather than silently succeeding -- the error path
// of the "old bytes decode under 0.5.0 code" guarantee. Truncating a pinned
// vector at several points (mid-length-prefix, mid-string, mid-trailer) must each
// return a non-nil error, never a bogus partial decode that looks successful.
func TestDecode_TruncatedFailsLoudly(t *testing.T) {
	full := mh(t, pinnedAccountEmailPresent)
	// Truncate at every length from 1 to full-1; each must error (the full buffer
	// is the only valid length). This covers a cut inside the name length/payload,
	// inside the email length/payload, inside the token, and inside the trailer.
	for cut := 1; cut < len(full); cut++ {
		var a Account
		if err := a.Unmarshal(full[:cut]); err == nil {
			t.Fatalf("truncated decode at %d/%d returned no error (silent partial decode)", cut, len(full))
		}
	}
	// The full buffer still decodes cleanly.
	var ok Account
	if err := ok.Unmarshal(full); err != nil {
		t.Fatalf("full buffer must decode: %v", err)
	}
}

// AC5: bytes field. The pinned token bytes decode to the exact content under the
// current (zero-copy) bytes decode path.
func TestDecode_BytesField(t *testing.T) {
	var a Account
	if err := a.Unmarshal(mh(t, pinnedAccountToken)); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !bytes.Equal(a.Token, []byte{0x00, 0x01, 0xFE, 0xFF}) {
		t.Fatalf("bytes field decode wrong: % x", a.Token)
	}
	if a.Name != "B" || a.Trailer != -1 {
		t.Fatalf("surrounding fields corrupted: %+v", a)
	}
}

// AC6: map message. Per T-7, map encoding is non-canonical, so this arm compares
// decoded VALUES, not bytes. The single-entry pinned vector is deterministic and
// is decoded directly; a multi-entry map is round-tripped by value.
func TestMapMessage_ValuesNotBytes(t *testing.T) {
	var b Bag
	if err := b.Unmarshal(mh(t, pinnedBagOneLabel)); err != nil {
		t.Fatalf("Unmarshal one-label: %v", err)
	}
	if b.HasNote {
		t.Fatalf("note should be absent, got %q", b.Note)
	}
	if !reflect.DeepEqual(b.Labels, map[int32]string{42: "answer"}) {
		t.Fatalf("single-entry map decode wrong: %+v", b.Labels)
	}

	// Multi-entry: round-trip by VALUE (bytes are non-canonical for >1 entry).
	in := &Bag{Note: "hi", HasNote: true, Labels: map[int32]string{1: "a", 2: "bb", 3: "ccc"}}
	data, err := in.Marshal()
	if err != nil {
		t.Fatalf("Marshal multi: %v", err)
	}
	var out Bag
	if err := out.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal multi: %v", err)
	}
	if !out.HasNote || out.Note != "hi" || !reflect.DeepEqual(out.Labels, in.Labels) {
		t.Fatalf("map message value round-trip mismatch:\n got  %+v\n want %+v", out, *in)
	}
}
`

// TestWireCompat_GeneratedCode generates the current 0.5.0 Go code and runs the
// driver (decode of old-style pinned bytes + symmetric encode byte-identity +
// map value round-trip) against it in a throwaway module that really compiles
// and runs. Covers AC1-AC6.
func TestWireCompat_GeneratedCode(t *testing.T) {
	src := generateCurrentGo(t)
	dir := buildGenModule(t, src, driver)
	goTestModule(t, dir)
}

// --- Conformance golden-vector arm (AC7) ---------------------------------------
//
// The committed testdata/conformance/*.bin files are version-independent wire
// bytes (the Go runtime is the reference encoder for cross-language conformance;
// they predate and are unchanged across the 0.5.0 codegen flip). Decoding them
// directly with the CURRENT runtime Decoder and asserting values proves the same
// "old bytes still decode" guarantee at the runtime level, and specifically
// re-covers the 1-byte vs 0xFF compact-length boundary and the bytes path with
// bytes nobody on this branch generated. These files are READ ONLY here.

// conformanceDir resolves testdata/conformance relative to this file.
func conformanceDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "testdata", "conformance")
}

func readVector(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join(conformanceDir(t), name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read conformance vector %s: %v", name, err)
	}
	return b
}

// TestWireCompat_ConformanceGoldenVectors decodes a selection of committed
// golden vectors with the current runtime and asserts exact values, including
// the compact-length boundary (254 single-byte / 255 via 0xFF) and a bytes
// payload. A decode error or value mismatch is a real wire regression and fails
// loudly; the vectors are never rewritten to make the test pass.
func TestWireCompat_ConformanceGoldenVectors(t *testing.T) {
	// string_len254: last single-byte length (0xFE). Decode -> 254*"a".
	t.Run("string_len254_singlebyte", func(t *testing.T) {
		data := readVector(t, "string_len254.bin")
		if data[0] != 0xFE {
			t.Fatalf("string_len254 must start with single-byte length 0xFE, got 0x%02x", data[0])
		}
		d := xpb.NewDecoder(data)
		s, err := d.CloneString()
		if err != nil {
			t.Fatalf("decode string_len254: %v", err)
		}
		if len(s) != 254 || s != strings.Repeat("a", 254) {
			t.Fatalf("string_len254 decode wrong: len=%d", len(s))
		}
		if !d.EOF() {
			t.Fatalf("trailing bytes after string_len254: %d", d.Remaining())
		}
	})

	// string_len255: first 0xFF compact-length form (0xFF + LE(255)). Decode -> 255*"b".
	t.Run("string_len255_compact0xFF", func(t *testing.T) {
		data := readVector(t, "string_len255.bin")
		if data[0] != 0xFF || data[1] != 0xFF || data[2] != 0x00 || data[3] != 0x00 || data[4] != 0x00 {
			t.Fatalf("string_len255 must start with 0xFF+LE(255), got % x", data[:5])
		}
		d := xpb.NewDecoder(data)
		s, err := d.CloneString()
		if err != nil {
			t.Fatalf("decode string_len255: %v", err)
		}
		if len(s) != 255 || s != strings.Repeat("b", 255) {
			t.Fatalf("string_len255 decode wrong: len=%d", len(s))
		}
		if !d.EOF() {
			t.Fatalf("trailing bytes after string_len255: %d", d.Remaining())
		}
	})

	// bytes_nonempty: a bytes payload {0x00,0x01,0xFE,0xFF,0x7F,0x80}.
	t.Run("bytes_nonempty", func(t *testing.T) {
		data := readVector(t, "bytes_nonempty.bin")
		d := xpb.NewDecoder(data)
		got, err := d.ReadBytes()
		if err != nil {
			t.Fatalf("decode bytes_nonempty: %v", err)
		}
		want := []byte{0x00, 0x01, 0xFE, 0xFF, 0x7F, 0x80}
		if len(got) != len(want) {
			t.Fatalf("bytes_nonempty len: got %d want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("bytes_nonempty[%d]: got 0x%02x want 0x%02x", i, got[i], want[i])
			}
		}
		if !d.EOF() {
			t.Fatalf("trailing bytes after bytes_nonempty: %d", d.Remaining())
		}
	})

	// int32_sample: 30 (0x1E) little-endian.
	t.Run("int32_sample", func(t *testing.T) {
		data := readVector(t, "int32_sample.bin")
		d := xpb.NewDecoder(data)
		v, err := d.ReadInt32()
		if err != nil {
			t.Fatalf("decode int32_sample: %v", err)
		}
		if v != 30 {
			t.Fatalf("int32_sample: got %d want 30", v)
		}
		if !d.EOF() {
			t.Fatalf("trailing bytes after int32_sample: %d", d.Remaining())
		}
	})

	// bool_true: single byte 0x01.
	t.Run("bool_true", func(t *testing.T) {
		data := readVector(t, "bool_true.bin")
		d := xpb.NewDecoder(data)
		v, err := d.ReadBool()
		if err != nil {
			t.Fatalf("decode bool_true: %v", err)
		}
		if !v {
			t.Fatalf("bool_true decoded false")
		}
		if !d.EOF() {
			t.Fatalf("trailing bytes after bool_true: %d", d.Remaining())
		}
	})
}

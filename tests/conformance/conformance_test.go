package conformance

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// testdataDir returns the absolute path to testdata/conformance, resolved
// relative to this test file's package directory (tests/conformance).
func testdataDir(t testing.TB) string {
	t.Helper()
	// This file lives at <repo>/tests/conformance; testdata is at
	// <repo>/testdata/conformance.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// wd == <repo>/tests/conformance
	return filepath.Join(wd, "..", "..", "testdata", "conformance")
}

func manifestPath(t testing.TB) string {
	return filepath.Join(testdataDir(t), "vectors.json")
}

// TestGenerateVectors writes the .bin files and manifest from the Go reference
// encoder. It only runs when XPB_GEN=1 so that the normal test run verifies the
// committed bytes rather than silently regenerating them.
//
//	XPB_GEN=1 go test ./tests/conformance/ -run TestGenerateVectors
func TestGenerateVectors(t *testing.T) {
	if os.Getenv("XPB_GEN") != "1" {
		t.Skip("set XPB_GEN=1 to regenerate conformance vectors")
	}
	dir := testdataDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	vecs := Vectors()
	for i := range vecs {
		v := &vecs[i]
		data := Encode(v.Ops)
		v.File = v.Name + ".bin"
		v.Hex = hex.EncodeToString(data)
		if err := os.WriteFile(filepath.Join(dir, v.File), data, 0o644); err != nil {
			t.Fatalf("write %s: %v", v.File, err)
		}
	}

	m := Manifest{
		Format:  "int32/uint32=number; int64/uint64=decimal string; float32/float64=hex bit pattern string; bytes=hex string; array=count+elems; map=count+k/v; message=length-prefixed nested ops",
		Vectors: vecs,
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(manifestPath(t), b, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	t.Logf("generated %d vectors into %s", len(vecs), dir)
}

// loadManifest reads the committed manifest.
func loadManifest(t testing.TB) Manifest {
	t.Helper()
	b, err := os.ReadFile(manifestPath(t))
	if err != nil {
		t.Fatalf("read manifest (run XPB_GEN=1 go test -run TestGenerateVectors first): %v", err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	return m
}

// TestConformance reads every committed .bin vector, decodes with the Go
// runtime and asserts decoded values match the manifest, then re-encodes and
// asserts byte-identity with both the .bin file and the manifest hex.
func TestConformance(t *testing.T) {
	dir := testdataDir(t)
	m := loadManifest(t)
	if len(m.Vectors) == 0 {
		t.Fatal("manifest has no vectors")
	}

	for _, v := range m.Vectors {
		v := v
		t.Run(v.Name, func(t *testing.T) {
			binPath := filepath.Join(dir, v.File)
			fileBytes, err := os.ReadFile(binPath)
			if err != nil {
				t.Fatalf("read %s: %v", v.File, err)
			}

			// Manifest hex must match the .bin file exactly.
			wantHex, err := hex.DecodeString(v.Hex)
			if err != nil {
				t.Fatalf("bad manifest hex: %v", err)
			}
			if !bytesEqual(fileBytes, wantHex) {
				t.Fatalf("manifest hex != .bin bytes\n hex:  %x\n file: %x", wantHex, fileBytes)
			}

			// Decode the .bin and verify values bit-exactly.
			if err := DecodeAndVerify(fileBytes, v.Ops); err != nil {
				t.Fatalf("decode/verify: %v", err)
			}

			// Re-encode from the manifest ops and assert byte-identity.
			reencoded := Encode(v.Ops)
			if !bytesEqual(reencoded, fileBytes) {
				t.Fatalf("re-encode mismatch\n got:  %x\n want: %x", reencoded, fileBytes)
			}
		})
	}

	t.Logf("verified %d vectors", len(m.Vectors))
}

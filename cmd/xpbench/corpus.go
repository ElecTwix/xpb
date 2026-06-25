package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ElecTwix/xpb/tests/conformance"
)

// shapeMeta is the per-shape bookkeeping the driver keeps after writing the
// corpus: the wire size (identical across every runtime, since the Go reference
// encoder produced the bytes) and the iteration count each harness loops.
type shapeMeta struct {
	Name     string `json:"name"`
	WireSize int    `json:"wireSize"`
	Iters    int    `json:"iters"`
}

// benchVector is one entry in the benchmark manifest. It is the conformance
// vector shape (name/file/hex/ops -- so every existing per-runtime JSON+op
// parser reads it unchanged) plus an additive "iters" field the harnesses use
// to size their timing loops. Reusing conformance.Op keeps the op JSON encoding
// (field names, int64/uint64-as-string, float-bits-as-hex) identical to what
// the runtime harnesses already expect.
type benchVector struct {
	Name  string           `json:"name"`
	File  string           `json:"file"`
	Hex   string           `json:"hex"`
	Iters int              `json:"iters"`
	Ops   []conformance.Op `json:"ops"`
}

// benchManifest is the top-level vectors.json the harnesses parse.
type benchManifest struct {
	Format  string        `json:"format"`
	Vectors []benchVector `json:"vectors"`
}

// itersFor picks a per-shape iteration count from the wire size, targeting a
// roughly constant amount of byte-work per shape so small shapes get enough
// iterations to be stable and large shapes do not dominate wall time. Clamped
// to keep a full cross-runtime run fast (seconds, not minutes).
func itersFor(wireSize int) int {
	const targetBytes = 8_000_000
	w := wireSize
	if w < 8 {
		w = 8
	}
	n := targetBytes / w
	if n < 5_000 {
		n = 5_000
	}
	if n > 200_000 {
		n = 200_000
	}
	return n
}

// writeCorpus encodes every canonical shape with the Go reference encoder and
// writes a manifest (vectors.json) plus one .bin per shape into dir. It returns
// the per-shape metadata (wire size + iteration count) in shape order. The
// .bin bytes are the cross-runtime input: every runtime decodes exactly these.
func writeCorpus(dir string, shapes []shape) ([]shapeMeta, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	man := benchManifest{
		Format: "xpbench cross-runtime corpus (Go reference encoder); conformance value model + per-vector iters",
	}
	metas := make([]shapeMeta, 0, len(shapes))
	for _, s := range shapes {
		data := conformance.Encode(s.ops)
		iters := itersFor(len(data))
		file := s.name + ".bin"
		if err := os.WriteFile(filepath.Join(dir, file), data, 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", file, err)
		}
		man.Vectors = append(man.Vectors, benchVector{
			Name:  s.name,
			File:  file,
			Hex:   hex.EncodeToString(data),
			Iters: iters,
			Ops:   s.ops,
		})
		metas = append(metas, shapeMeta{Name: s.name, WireSize: len(data), Iters: iters})
	}
	b, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return nil, err
	}
	b = append(b, '\n')
	if err := os.WriteFile(filepath.Join(dir, "vectors.json"), b, 0o644); err != nil {
		return nil, err
	}
	return metas, nil
}

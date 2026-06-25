package differential

import (
	"os"
	"sort"
	"testing"

	"github.com/ElecTwix/xpb/tests/conformance"
)

// splitCorpus partitions generated vectors into the map-free set and the
// map-containing set. Both sets are driven cross-language, but with different
// assertions: the map-free set goes through the byte-identity arm (modeBytes);
// the map-containing set goes through the decoded-VALUES arm (modeValues),
// because map entry order is non-canonical across runtimes per T-7. Splitting
// this way means EVERY runtime still exercises map decode/re-encode (closing the
// gap the package exists for) without depending on a canonical map byte order.
func splitCorpus(vecs []conformance.Vector) (mapFree, withMaps []conformance.Vector) {
	for _, v := range vecs {
		if containsMap(v.Ops) {
			withMaps = append(withMaps, v)
		} else {
			mapFree = append(mapFree, v)
		}
	}
	return
}

// driveCorpus writes the given vectors to a temp dir (Go reference encoder) and
// drives every AVAILABLE runtime over it in the given mode. A toolchain that is
// absent is skipped cleanly; a runner that runs and reports a mismatch fails the
// test. It returns the exercised and skipped runtime names.
func driveCorpus(t *testing.T, root string, vecs []conformance.Vector, mode diffMode) (exercised, skipped []string) {
	t.Helper()
	if len(vecs) == 0 {
		return nil, nil
	}
	dir := t.TempDir()
	if _, err := writeCorpus(dir, vecs); err != nil {
		t.Fatalf("write corpus (%s): %v", mode, err)
	}

	for _, runner := range allRunners() {
		res := runner(root, dir, mode)
		switch {
		case res.skipped != "":
			skipped = append(skipped, res.name)
			t.Logf("[SKIP] %s (%s): %s", res.name, mode, res.skipped)
		case res.err != nil:
			t.Errorf("[FAIL] %s differential mismatch (%s) over %d vectors: %v", res.name, mode, len(vecs), res.err)
		default:
			exercised = append(exercised, res.name)
			t.Logf("[PASS] %s (%s): %d vectors verified", res.name, mode, len(vecs))
		}
	}
	return
}

// uniqueSorted dedupes and sorts a slice of names.
func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// reportRuntimes prints the exercised-vs-skipped summary required by the ticket.
// An all-skipped run (no non-Go toolchain present) is surfaced distinctly so a
// misconfigured CI image cannot pass off a Go-only run as a real cross-language
// differential -- but it is NOT a hard failure, because the ticket requires the
// harness to skip cleanly in minimal environments.
func reportRuntimes(t *testing.T, exercised, skipped []string) {
	t.Helper()
	ex := uniqueSorted(exercised)
	sk := uniqueSorted(skipped)
	t.Logf("cross-language differential: exercised=%v skipped=%v", ex, sk)
	if len(ex) == 0 {
		t.Log("NOTE: NO non-Go runtime toolchain was available, so the " +
			"cross-language oracle ran against zero runtimes; only the Go " +
			"reference round-trip validated the corpus. Install cargo / node / " +
			"a C compiler / lua / a JDK to exercise the differential.")
	}
}

// goReferenceRoundTrip asserts the whole corpus survives a Go encode -> decode
// + value verify, independent of any other toolchain. This is the always-on
// baseline; the cross-language arms are the real T-9 oracle on top of it.
func goReferenceRoundTrip(t testing.TB, vecs []conformance.Vector) {
	t.Helper()
	for _, v := range vecs {
		data := conformance.Encode(v.Ops)
		if err := conformance.DecodeAndVerify(data, v.Ops); err != nil {
			t.Fatalf("Go reference round-trip failed for %s: %v", v.Name, err)
		}
	}
}

// TestDifferential is the deterministic entrypoint that runs under `make verify`
// (plain `go test`, no -fuzz). It generates a fixed-seed random corpus and drives
// every available runtime over BOTH the map-free corpus (byte-identity arm) and
// the map-containing corpus (decoded-values arm). It never hard-fails on a
// missing toolchain.
func TestDifferential(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("repoRoot: %v", err)
	}

	const seed = 0xC0FFEE
	const n = 64
	vecs := genVectors(seed, n)

	goReferenceRoundTrip(t, vecs)

	mapFree, withMaps := splitCorpus(vecs)

	var exercised, skipped []string
	ex, sk := driveCorpus(t, root, mapFree, modeBytes)
	exercised, skipped = append(exercised, ex...), append(skipped, sk...)
	ex, sk = driveCorpus(t, root, withMaps, modeValues)
	exercised, skipped = append(exercised, ex...), append(skipped, sk...)

	reportRuntimes(t, exercised, skipped)
}

// FuzzDifferential is the testing.F target. Each fuzz input is reduced to a
// 64-bit seed that deterministically reproduces a corpus, so a crashing input in
// testdata/fuzz reproduces the exact failing vectors.
//
// By default the fuzz body drives only the Go reference round-trip + decode/verify
// over the generated corpus: this is fast enough for `go test -fuzz` to mutate
// seeds continuously (each cross-language drive forks five heavyweight toolchain
// subprocesses, which is far too slow for the fuzzing engine's per-input budget
// and would make `-fuzz` time out gathering baseline coverage). The full
// cross-language differential over the SEED corpus runs under TestDifferential
// (and under plain `go test`, which is what `make verify` invokes). To also fan
// the FUZZ corpus out to the other runtimes, set XPB_DIFF_CROSS=1 (accepting that
// continuous fuzzing then runs at a few execs/sec).
func FuzzDifferential(f *testing.F) {
	root, err := repoRoot()
	if err != nil {
		f.Fatalf("repoRoot: %v", err)
	}
	cross := os.Getenv("XPB_DIFF_CROSS") == "1"

	// Seed corpus: a spread of deterministic seeds plus the documented edges.
	for _, s := range []int64{0, 1, 2, 7, 42, 0xC0FFEE, 0x7FFFFFFF} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, seed int64) {
		vecs := genVectors(seed, 24)
		// Always: the corpus must survive a Go encode -> decode + value verify.
		goReferenceRoundTrip(t, vecs)
		for _, v := range vecs {
			if err := conformance.DecodeAndVerify(conformance.Encode(v.Ops), v.Ops); err != nil {
				t.Fatalf("Go decode/verify failed (seed=%d) for %s: %v", seed, v.Name, err)
			}
		}
		if !cross {
			return
		}
		mapFree, withMaps := splitCorpus(vecs)
		driveFuzzCorpus(t, root, mapFree, modeBytes, seed)
		driveFuzzCorpus(t, root, withMaps, modeValues, seed)
	})
}

// driveFuzzCorpus is the fuzz-path equivalent of driveCorpus: it writes the
// corpus and drives every available runtime, failing only on a genuine mismatch
// (never on a missing toolchain).
func driveFuzzCorpus(t *testing.T, root string, vecs []conformance.Vector, mode diffMode, seed int64) {
	t.Helper()
	if len(vecs) == 0 {
		return
	}
	dir := t.TempDir()
	if _, err := writeCorpus(dir, vecs); err != nil {
		t.Fatalf("write corpus (seed=%d, %s): %v", seed, mode, err)
	}
	for _, runner := range allRunners() {
		res := runner(root, dir, mode)
		if res.skipped == "" && res.err != nil {
			t.Errorf("[FAIL] %s differential mismatch (seed=%d, %s): %v", res.name, seed, mode, res.err)
		}
	}
}

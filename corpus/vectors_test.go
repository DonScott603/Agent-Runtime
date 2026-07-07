// Vector harness (WP-02, minimal): loads every docs/vectors/*.json and
// asserts the implementation against it. The vectors DEFINE correct
// behavior (docs/vectors/README.md); this harness conforms to them,
// never the other way around. Unknown vector files fail loudly so a
// new golden file cannot land silently unasserted.
package corpus

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

const vectorsDir = "../docs/vectors"

// Registered vector files. canon.json is asserted (WP-01); the rest are
// skipped-but-registered until their work packages land.
var vectorFiles = map[string]string{
	"canon.json":      "", // asserted below
	"chain.json":      "WP-04: log writer + per-run chains",
	"resolution.json": "WP-06: gate resolution",
	"derivation.json": "WP-06: manifest scope derivation",
	"upcaster.json":   "fold-time payload migration (versioning.md M3/M4)",
}

func TestVectors(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join(vectorsDir, "*.json"))
	if err != nil {
		t.Fatalf("globbing %s: %v", vectorsDir, err)
	}
	if len(matches) == 0 {
		t.Fatalf("no vector files found under %s — harness miswired", vectorsDir)
	}
	for _, path := range matches {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			skip, known := vectorFiles[name]
			if !known {
				t.Fatalf("unknown vector file %s: extend the harness to assert it (WP-02 rule: unknown vector files fail loudly)", name)
			}
			if name == "canon.json" {
				runCanonVectors(t, path)
				return
			}
			t.Skip("registered, not yet asserted: " + skip)
		})
	}
}

type canonCase struct {
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	Canonical string          `json:"canonical"`
	SHA256    string          `json:"sha256"`
}

type canonFile struct {
	Rules map[string]string `json:"_rules"`
	Cases []canonCase       `json:"cases"`
}

func runCanonVectors(t *testing.T, path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var f canonFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	if len(f.Cases) == 0 {
		t.Fatal("canon.json has no cases — harness misparse")
	}
	for _, tc := range f.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			// UseNumber so integers survive intact (int64 max would be
			// mangled by float64) and float-form tokens reach Canonical
			// undisguised for the hard-error check.
			dec := json.NewDecoder(bytes.NewReader(tc.Input))
			dec.UseNumber()
			var v any
			if err := dec.Decode(&v); err != nil {
				t.Fatalf("decoding input: %v", err)
			}
			got, err := kernel.Canonical(v)
			if err != nil {
				t.Fatalf("Canonical: %v", err)
			}
			if want := []byte(tc.Canonical); !bytes.Equal(got, want) {
				t.Errorf("canonical bytes mismatch\n got: %q\nwant: %q", got, want)
			}
			sum := sha256.Sum256(got)
			if gotHex := hex.EncodeToString(sum[:]); gotHex != tc.SHA256 {
				t.Errorf("sha256 mismatch\n got: %s\nwant: %s", gotHex, tc.SHA256)
			}
		})
	}
}

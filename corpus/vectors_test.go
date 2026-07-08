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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
	klog "github.com/DonScott603/Agent-Runtime/kernel/log"
	"github.com/DonScott603/Agent-Runtime/vault"
)

const vectorsDir = "../docs/vectors"

// Registered vector files. canon.json (WP-01), blob.json (WP-03) and
// chain.json (WP-04a) are asserted; the rest are skipped-but-registered
// until their work packages land.
var vectorFiles = map[string]string{
	"canon.json":      "", // asserted below
	"blob.json":       "", // asserted below
	"chain.json":      "", // asserted below
	"resolution.json": "WP-06a: matchers + resolve",
	"derivation.json": "WP-06b: derive (manifest scope derivation)",
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
			switch name {
			case "canon.json":
				runCanonVectors(t, path)
				return
			case "blob.json":
				runBlobVectors(t, path)
				return
			case "chain.json":
				runChainVectors(t, path)
				return
			}
			t.Skip("registered, not yet asserted: " + skip)
		})
	}
}

type chainRun struct {
	Name         string         `json:"name"`
	Note         string         `json:"note"`
	Events       []kernel.Event `json:"events"`
	ExpectedHead string         `json:"expected_head"`
}

type chainVerifyCase struct {
	Name     string         `json:"name"`
	Note     string         `json:"note"`
	Events   []kernel.Event `json:"events"`
	Expected struct {
		Error string       `json:"error"`
		Seq   kernel.Seq   `json:"seq"`
		RunID kernel.RunID `json:"run_id"`
	} `json:"expected"`
}

type chainSealCase struct {
	Name            string       `json:"name"`
	Note            string       `json:"note"`
	Event           kernel.Event `json:"event"`
	ExpectedEventID string       `json:"expected_event_id"`
}

type chainFile struct {
	Rules        map[string]string `json:"_rules"`
	Events       []kernel.Event    `json:"events"`
	VerifyCases  []chainVerifyCase `json:"verify_cases"`
	SealCases    []chainSealCase   `json:"seal_cases"`
	Runs         []chainRun        `json:"runs"`
	ExpectedHead string            `json:"expected_head"`
}

// assertChainRun asserts one sealed run: each recorded event_id must
// re-derive via kernel.SealEvent (which zeroes event_id and sig
// internally, so passing the event as-recorded also exercises the
// re-seal path), a signed event must seal identically with its sig
// stripped (_rules "runs"; RFC-0002 s5), prev_hash must link
// genesis-onward, the last event_id must equal expectedHead, and
// VerifyChain must agree.
func assertChainRun(t *testing.T, events []kernel.Event, expectedHead string) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("run has no events — harness misparse")
	}
	prev := kernel.ZeroHash
	for _, ev := range events {
		sealed, err := kernel.SealEvent(ev)
		if err != nil {
			t.Fatalf("seq %d: SealEvent: %v", ev.Seq, err)
		}
		if sealed.EventID != ev.EventID {
			t.Errorf("seq %d: event_id does not re-derive\n got: %s\nwant: %s", ev.Seq, sealed.EventID, ev.EventID)
		}
		if ev.Sig != nil {
			stripped := ev
			stripped.Sig = nil
			resealed, err := kernel.SealEvent(stripped)
			if err != nil {
				t.Fatalf("seq %d: SealEvent with sig stripped: %v", ev.Seq, err)
			}
			if resealed.EventID != ev.EventID {
				t.Errorf("seq %d: sig leaked into the hash (RFC-0002 s5)\n stripped: %s\n recorded: %s", ev.Seq, resealed.EventID, ev.EventID)
			}
		}
		if ev.PrevHash != prev {
			t.Errorf("seq %d: prev_hash linkage\n got: %s\nwant: %s", ev.Seq, ev.PrevHash, prev)
		}
		prev = ev.EventID
	}
	if prev != expectedHead {
		t.Errorf("chain head mismatch\n got: %s\nwant: %s", prev, expectedHead)
	}
	if err := klog.VerifyChain(events); err != nil {
		t.Errorf("VerifyChain over vector events: %v", err)
	}
}

// runChainVectors asserts seal + chain (WP-04a/WP-04a.1) against
// chain.json: the top-level run, each mini-run in "runs", the
// "seal_cases" identities, and the behavioral "verify_cases"
// (expected OUTCOME, resolution.json precedent).
func runChainVectors(t *testing.T, path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var f chainFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	assertChainRun(t, f.Events, f.ExpectedHead)
	for _, run := range f.Runs {
		t.Run(run.Name, func(t *testing.T) { assertChainRun(t, run.Events, run.ExpectedHead) })
	}
	for _, tc := range f.SealCases {
		t.Run(tc.Name, func(t *testing.T) {
			sealed, err := kernel.SealEvent(tc.Event)
			if err != nil {
				t.Fatalf("SealEvent: %v", err)
			}
			if sealed.EventID != tc.ExpectedEventID {
				t.Errorf("event_id mismatch\n got: %s\nwant: %s", sealed.EventID, tc.ExpectedEventID)
			}
		})
	}
	for _, tc := range f.VerifyCases {
		t.Run(tc.Name, func(t *testing.T) {
			err := klog.VerifyChain(tc.Events)
			var cbe *klog.ChainBrokenError
			if !errors.As(err, &cbe) {
				t.Fatalf("want *ChainBrokenError, got %T: %v", err, err)
			}
			if cbe.Seq != tc.Expected.Seq || cbe.RunID != tc.Expected.RunID {
				t.Errorf("first failure at run %q seq %d, want run %q seq %d", cbe.RunID, cbe.Seq, tc.Expected.RunID, tc.Expected.Seq)
			}
			if !strings.Contains(err.Error(), tc.Expected.Error) {
				t.Errorf("error %q does not carry code %q", err, tc.Expected.Error)
			}
		})
	}
}

type blobCase struct {
	Name         string `json:"name"`
	Note         string `json:"note"`
	DEK          string `json:"dek"`
	KEK          string `json:"kek"`
	Nonce        string `json:"nonce"`
	Key          string `json:"key"`
	PlaintextHex string `json:"plaintext_hex"`
	AADUTF8      string `json:"aad_utf8"`
	File         string `json:"file"`
	Address      string `json:"address"`
	Wrapped      string `json:"wrapped"`
}

type blobFile struct {
	Rules map[string]string `json:"_rules"`
	Cases []blobCase        `json:"cases"`
}

func hexBytes(t *testing.T, field, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("decoding %s: %v", field, err)
	}
	return b
}

func hexKey32(t *testing.T, field, s string) (k [32]byte) {
	t.Helper()
	b := hexBytes(t, field, s)
	if len(b) != 32 {
		t.Fatalf("%s: want 32 bytes, got %d", field, len(b))
	}
	copy(k[:], b)
	return k
}

func hexNonce12(t *testing.T, field, s string) (n [12]byte) {
	t.Helper()
	b := hexBytes(t, field, s)
	if len(b) != 12 {
		t.Fatalf("%s: want 12 bytes, got %d", field, len(b))
	}
	copy(n[:], b)
	return n
}

// runBlobVectors asserts the vault-lite pure format layer (WP-03,
// ADR-0021) against goldens computed by an independent implementation
// (blob.json _rules.provenance).
func runBlobVectors(t *testing.T, path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var f blobFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	if len(f.Cases) == 0 {
		t.Fatal("blob.json has no cases — harness misparse")
	}
	for _, tc := range f.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			switch {
			case tc.DEK != "": // blob encoding + addressing case
				dek := hexKey32(t, "dek", tc.DEK)
				nonce := hexNonce12(t, "nonce", tc.Nonce)
				pt := hexBytes(t, "plaintext_hex", tc.PlaintextHex)
				got := vault.EncodeBlobV1(dek, nonce, pt)
				if gotHex := hex.EncodeToString(got); gotHex != tc.File {
					t.Errorf("blob file mismatch\n got: %s\nwant: %s", gotHex, tc.File)
				}
				if addr := vault.BlobAddress(got); addr != tc.Address {
					t.Errorf("address mismatch\n got: %s\nwant: %s", addr, tc.Address)
				}
			case tc.KEK != "": // key wrap case
				kek := hexKey32(t, "kek", tc.KEK)
				nonce := hexNonce12(t, "nonce", tc.Nonce)
				key := hexKey32(t, "key", tc.Key)
				got := vault.WrapKey(kek, nonce, key, []byte(tc.AADUTF8))
				if gotHex := hex.EncodeToString(got); gotHex != tc.Wrapped {
					t.Errorf("wrapped key mismatch\n got: %s\nwant: %s", gotHex, tc.Wrapped)
				}
			default:
				t.Fatalf("case %s is neither a blob case (dek) nor a wrap case (kek)", tc.Name)
			}
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

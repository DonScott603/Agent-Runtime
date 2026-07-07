// T10 — property test: random op sequences against an in-memory model
// (repo convention: rapid for property suites, CLAUDE.md).
package vault_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"pgregory.net/rapid"

	"github.com/DonScott603/Agent-Runtime/vault"
)

func TestStoreModelProperty(t *testing.T) {
	base := t.TempDir()
	rapid.Check(t, func(rt *rapid.T) {
		dir, err := os.MkdirTemp(base, "v")
		if err != nil {
			rt.Fatal(err)
		}
		s, err := vault.Open(dir, &fakeRand{})
		if err != nil {
			rt.Fatal(err)
		}
		defer s.Close()

		runs := []string{"run-a", "run-b", "run-c"}
		// Model: readable plaintext per (run, hash); every blob file
		// ever written, with its bytes at write time (immutability).
		stored := map[string]map[string][]byte{}
		written := map[string][]byte{}

		anyHash := func() string {
			if len(written) > 0 && rapid.Bool().Draw(rt, "pickKnown") {
				keys := make([]string, 0, len(written))
				for h := range written {
					keys = append(keys, h)
				}
				return rapid.SampledFrom(keys).Draw(rt, "knownHash")
			}
			raw := rapid.SliceOfN(rapid.Byte(), 1, 8).Draw(rt, "rnd")
			sum := sha256.Sum256(raw)
			return hex.EncodeToString(sum[:])
		}

		nOps := rapid.IntRange(1, 40).Draw(rt, "nOps")
		for i := 0; i < nOps; i++ {
			run := rapid.SampledFrom(runs).Draw(rt, "run")
			switch rapid.SampledFrom([]string{"put", "get", "eraseBlob", "eraseRun"}).Draw(rt, "op") {
			case "put":
				pt := rapid.SliceOfN(rapid.Byte(), 0, 64).Draw(rt, "pt")
				h, err := s.Put(run, pt)
				if err != nil {
					rt.Fatalf("Put: %v", err)
				}
				if stored[run] == nil {
					stored[run] = map[string][]byte{}
				}
				stored[run][h] = bytes.Clone(pt)
				b, err := os.ReadFile(dir + "/blobs/" + h)
				if err != nil {
					rt.Fatalf("blob file missing after Put: %v", err)
				}
				written[h] = b
			case "get":
				h := anyHash()
				got, err := s.Get(run, h)
				wantPt, isStored := stored[run][h]
				switch {
				case isStored:
					if err != nil {
						rt.Fatalf("Get(stored): %v", err)
					}
					if !bytes.Equal(got, wantPt) {
						rt.Fatal("Get(stored) plaintext mismatch")
					}
				case written[h] != nil:
					if !errors.Is(err, vault.ErrErased) {
						rt.Fatalf("Get(erased/other-run): want ErrErased, got %v", err)
					}
				default:
					if !errors.Is(err, vault.ErrNotFound) {
						rt.Fatalf("Get(never stored): want ErrNotFound, got %v", err)
					}
				}
			case "eraseBlob":
				h := anyHash()
				err := s.EraseBlob(run, h)
				_, isStored := stored[run][h]
				switch {
				case isStored:
					if err != nil {
						rt.Fatalf("EraseBlob(stored): %v", err)
					}
					delete(stored[run], h)
				case written[h] != nil:
					if err != nil {
						rt.Fatalf("EraseBlob(idempotent): want nil, got %v", err)
					}
				default:
					if !errors.Is(err, vault.ErrNotFound) {
						rt.Fatalf("EraseBlob(never stored): want ErrNotFound, got %v", err)
					}
				}
			case "eraseRun":
				if err := s.EraseRun(run); err != nil {
					rt.Fatalf("EraseRun: %v", err)
				}
				delete(stored, run)
			}
		}

		// Blob files are immutable once written, whatever was erased.
		for h, want := range written {
			b, err := os.ReadFile(dir + "/blobs/" + h)
			if err != nil {
				rt.Fatalf("blob file %s vanished: %v", h, err)
			}
			if !bytes.Equal(b, want) {
				rt.Fatalf("blob file %s mutated", h)
			}
		}
	})
}

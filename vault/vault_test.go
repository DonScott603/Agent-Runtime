// Tests for the vault-lite blob store (WP-03, RFC-0002 §6, D9/D10).
//
// All key material is produced by fakeRand — an obviously synthetic
// counter pattern, never real material (vault/CLAUDE.md). The fake
// records every chunk it serves so tests can assert raw key bytes
// never land on disk outside master.key.
package vault_test

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

	"github.com/DonScott603/Agent-Runtime/vault"
)

// fakeRand serves a deterministic counter pattern and records every
// chunk handed out. Chunks of 32 bytes are key material; 12-byte
// chunks are nonces (public by design).
type fakeRand struct {
	n      byte
	chunks [][]byte
}

func (f *fakeRand) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = f.n
		f.n++
	}
	c := make([]byte, len(p))
	copy(c, p)
	f.chunks = append(f.chunks, c)
	return len(p), nil
}

func newStore(t *testing.T) (*vault.Store, string, *fakeRand) {
	t.Helper()
	dir := t.TempDir()
	r := &fakeRand{}
	s, err := vault.Open(dir, r)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dir, r
}

func blobPath(dir, h string) string {
	return filepath.Join(dir, "blobs", h)
}

func keystorePath(dir, run string) string {
	return filepath.Join(dir, "keys", hex.EncodeToString([]byte(run))+".json")
}

type ksFile struct {
	Version       int               `json:"version"`
	RunID         string            `json:"run_id"`
	WrappedRunKey string            `json:"wrapped_run_key"`
	Blobs         map[string]string `json:"blobs"`
}

func readKeystore(t *testing.T, dir, run string) ksFile {
	t.Helper()
	b, err := os.ReadFile(keystorePath(dir, run))
	if err != nil {
		t.Fatalf("read keystore: %v", err)
	}
	var ks ksFile
	if err := json.Unmarshal(b, &ks); err != nil {
		t.Fatalf("parse keystore: %v", err)
	}
	return ks
}

func readBlobFile(t *testing.T, dir, h string) []byte {
	t.Helper()
	b, err := os.ReadFile(blobPath(dir, h))
	if err != nil {
		t.Fatalf("read blob %s: %v", h, err)
	}
	return b
}

func isLowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// T1 — round trip; returned hash is the whole-file sha256 (ADR-0021).
func TestPutGetRoundTrip(t *testing.T) {
	cases := map[string][]byte{
		"empty":   {},
		"one":     {0x2a},
		"unicode": []byte("héllo, vàult — ünïcode ✓"),
		"large":   bytes.Repeat([]byte{0xab, 0xcd, 0xef}, 350000),
	}
	s, dir, _ := newStore(t)
	for name, pt := range cases {
		t.Run(name, func(t *testing.T) {
			h, err := s.Put("run-1", pt)
			if err != nil {
				t.Fatalf("Put: %v", err)
			}
			if len(h) != 64 || !isLowerHex(h) {
				t.Fatalf("hash %q is not 64 lowercase hex chars", h)
			}
			file := readBlobFile(t, dir, h)
			sum := sha256.Sum256(file)
			if hex.EncodeToString(sum[:]) != h {
				t.Fatal("address is not sha256 over the whole stored file")
			}
			got, err := s.Get("run-1", h)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if !bytes.Equal(got, pt) {
				t.Fatal("round trip mismatch")
			}
		})
	}
}

// T2 — THE NAMED VERIFICATION (workplan WP-03): destroy the key,
// content unreadable, references intact.
func TestEraseRunNamedVerification(t *testing.T) {
	s, dir, _ := newStore(t)

	putN := func(run string, n int) []string {
		var hs []string
		for i := 0; i < n; i++ {
			h, err := s.Put(run, []byte(run+"-blob-"+strings.Repeat("x", i+1)))
			if err != nil {
				t.Fatalf("Put: %v", err)
			}
			hs = append(hs, h)
		}
		return hs
	}
	hsA := putN("run-a", 2)
	hsB := putN("run-b", 2)

	before := map[string][]byte{}
	for _, h := range hsA {
		before[h] = readBlobFile(t, dir, h)
	}

	if err := s.EraseRun("run-a"); err != nil {
		t.Fatalf("EraseRun: %v", err)
	}

	for _, h := range hsA {
		// Content unreadable.
		if _, err := s.Get("run-a", h); !errors.Is(err, vault.ErrErased) {
			t.Fatalf("Get after EraseRun: want ErrErased, got %v", err)
		}
		// References intact: blob file byte-identical, address still verifies.
		after := readBlobFile(t, dir, h)
		if !bytes.Equal(before[h], after) {
			t.Fatal("blob file changed by erasure")
		}
		sum := sha256.Sum256(after)
		if hex.EncodeToString(sum[:]) != h {
			t.Fatal("reference hash no longer matches stored file")
		}
	}

	// Other runs unaffected.
	for _, h := range hsB {
		if _, err := s.Get("run-b", h); err != nil {
			t.Fatalf("run-b blob unreadable after erasing run-a: %v", err)
		}
	}

	// Erasure survives reopen.
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	s2, err := vault.Open(dir, &fakeRand{n: 128})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	for _, h := range hsA {
		if _, err := s2.Get("run-a", h); !errors.Is(err, vault.ErrErased) {
			t.Fatalf("after reopen: want ErrErased, got %v", err)
		}
	}
	for _, h := range hsB {
		if _, err := s2.Get("run-b", h); err != nil {
			t.Fatalf("after reopen: run-b unreadable: %v", err)
		}
	}
}

// T3 — per-blob erasure rotates the run key and excludes the target
// (ADR-0009 "re-wraps the run key excluding the target").
func TestEraseBlobRotation(t *testing.T) {
	s, dir, _ := newStore(t)
	const run = "run-rotate"
	h1, _ := s.Put(run, []byte("blob one"))
	h2, _ := s.Put(run, []byte("blob two"))
	h3, _ := s.Put(run, []byte("blob three"))

	ksBefore := readKeystore(t, dir, run)
	filesBefore := map[string][]byte{}
	for _, h := range []string{h1, h2, h3} {
		filesBefore[h] = readBlobFile(t, dir, h)
	}

	if err := s.EraseBlob(run, h2); err != nil {
		t.Fatalf("EraseBlob: %v", err)
	}

	if _, err := s.Get(run, h2); !errors.Is(err, vault.ErrErased) {
		t.Fatalf("erased blob: want ErrErased, got %v", err)
	}
	for _, h := range []string{h1, h3} {
		got, err := s.Get(run, h)
		if err != nil {
			t.Fatalf("survivor %s unreadable: %v", h, err)
		}
		if len(got) == 0 {
			t.Fatal("survivor plaintext empty")
		}
	}

	ksAfter := readKeystore(t, dir, run)
	if ksAfter.WrappedRunKey == ksBefore.WrappedRunKey {
		t.Fatal("run key was not rotated on per-blob erasure")
	}
	if _, ok := ksAfter.Blobs[h2]; ok {
		t.Fatal("erased blob's wrapped key still present")
	}
	if len(ksAfter.Blobs) != 2 {
		t.Fatalf("want 2 surviving wrapped keys, got %d", len(ksAfter.Blobs))
	}
	for _, h := range []string{h1, h2, h3} {
		if !bytes.Equal(filesBefore[h], readBlobFile(t, dir, h)) {
			t.Fatal("blob file changed by per-blob erasure")
		}
	}
}

// T4 — erasure idempotency and Put-after-EraseRun.
func TestErasureIdempotency(t *testing.T) {
	s, _, _ := newStore(t)
	const run = "run-idem"
	h1, _ := s.Put(run, []byte("first"))
	h2, _ := s.Put(run, []byte("second"))

	if err := s.EraseBlob(run, h2); err != nil {
		t.Fatalf("EraseBlob: %v", err)
	}
	if err := s.EraseBlob(run, h2); err != nil {
		t.Fatalf("second EraseBlob: want nil, got %v", err)
	}
	never := strings.Repeat("ab", 32)
	if err := s.EraseBlob(run, never); !errors.Is(err, vault.ErrNotFound) {
		t.Fatalf("EraseBlob(never stored): want ErrNotFound, got %v", err)
	}

	if err := s.EraseRun(run); err != nil {
		t.Fatalf("EraseRun: %v", err)
	}
	if err := s.EraseRun(run); err != nil {
		t.Fatalf("second EraseRun: want nil, got %v", err)
	}
	if err := s.EraseRun("run-never-existed"); err != nil {
		t.Fatalf("EraseRun(unknown run): want nil, got %v", err)
	}

	// Put after EraseRun starts a fresh run key; old blobs stay erased.
	h3, err := s.Put(run, []byte("post-erasure"))
	if err != nil {
		t.Fatalf("Put after EraseRun: %v", err)
	}
	if got, err := s.Get(run, h3); err != nil || !bytes.Equal(got, []byte("post-erasure")) {
		t.Fatalf("new blob after EraseRun: %v", err)
	}
	if _, err := s.Get(run, h1); !errors.Is(err, vault.ErrErased) {
		t.Fatalf("old blob after EraseRun+Put: want ErrErased, got %v", err)
	}
}

// Amendment 4 — cross-run semantics: a hash stored under run A is
// ErrErased under run B (no key there), and erasing it "from" run B is
// an idempotent no-op that must not affect run A.
func TestCrossRunIsolation(t *testing.T) {
	s, _, _ := newStore(t)
	ptA := []byte("belongs to run-a")
	hA, err := s.Put("run-a", ptA)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	hB, err := s.Put("run-b", []byte("belongs to run-b"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if _, err := s.Get("run-b", hA); !errors.Is(err, vault.ErrErased) {
		t.Fatalf("Get(run-b, hash of run-a): want ErrErased, got %v", err)
	}
	if err := s.EraseBlob("run-b", hA); err != nil {
		t.Fatalf("EraseBlob(run-b, hash of run-a): want nil no-op, got %v", err)
	}

	got, err := s.Get("run-a", hA)
	if err != nil || !bytes.Equal(got, ptA) {
		t.Fatalf("run-a blob affected by run-b erase: %v", err)
	}
	if _, err := s.Get("run-b", hB); err != nil {
		t.Fatalf("run-b's own blob affected: %v", err)
	}
}

// T5 — tamper detection and malformed input handling.
func TestTamperDetection(t *testing.T) {
	const run = "run-tamper"

	t.Run("blob-file-regions", func(t *testing.T) {
		s, dir, _ := newStore(t)
		h, _ := s.Put(run, []byte("tamper target payload"))
		orig := readBlobFile(t, dir, h)
		regions := map[string]int{
			"version":    0,
			"nonce":      5,
			"ciphertext": 14,
			"tag":        len(orig) - 1,
		}
		for name, idx := range regions {
			t.Run(name, func(t *testing.T) {
				bad := bytes.Clone(orig)
				bad[idx] ^= 0x01
				if err := os.WriteFile(blobPath(dir, h), bad, 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := s.Get(run, h); !errors.Is(err, vault.ErrCorrupt) {
					t.Fatalf("want ErrCorrupt, got %v", err)
				}
				if err := os.WriteFile(blobPath(dir, h), orig, 0o600); err != nil {
					t.Fatal(err)
				}
			})
		}
		t.Run("truncated", func(t *testing.T) {
			if err := os.WriteFile(blobPath(dir, h), orig[:10], 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Get(run, h); !errors.Is(err, vault.ErrCorrupt) {
				t.Fatalf("want ErrCorrupt, got %v", err)
			}
		})
	})

	t.Run("keystore", func(t *testing.T) {
		flip := func(t *testing.T, wrappedHex string) string {
			t.Helper()
			raw, err := hex.DecodeString(wrappedHex)
			if err != nil {
				t.Fatal(err)
			}
			raw[20] ^= 0x01
			return hex.EncodeToString(raw)
		}
		write := func(t *testing.T, dir string, ks ksFile) {
			t.Helper()
			b, err := json.Marshal(ks)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(keystorePath(dir, run), b, 0o600); err != nil {
				t.Fatal(err)
			}
		}

		cases := map[string]func(t *testing.T, dir string, h1, h2 string){
			"garbage-bytes": func(t *testing.T, dir, _, _ string) {
				os.WriteFile(keystorePath(dir, run), []byte("\x00\xffnot json"), 0o600)
			},
			"truncated-json": func(t *testing.T, dir, _, _ string) {
				b, _ := os.ReadFile(keystorePath(dir, run))
				os.WriteFile(keystorePath(dir, run), b[:len(b)/2], 0o600)
			},
			"wrong-version": func(t *testing.T, dir, _, _ string) {
				ks := readKeystore(t, dir, run)
				ks.Version = 2
				write(t, dir, ks)
			},
			"non-hex-run-key": func(t *testing.T, dir, _, _ string) {
				ks := readKeystore(t, dir, run)
				ks.WrappedRunKey = "zz" + ks.WrappedRunKey[2:]
				write(t, dir, ks)
			},
			"bitflipped-run-key": func(t *testing.T, dir, _, _ string) {
				ks := readKeystore(t, dir, run)
				ks.WrappedRunKey = flip(t, ks.WrappedRunKey)
				write(t, dir, ks)
			},
			"bitflipped-dek": func(t *testing.T, dir, h1, _ string) {
				ks := readKeystore(t, dir, run)
				ks.Blobs[h1] = flip(t, ks.Blobs[h1])
				write(t, dir, ks)
			},
			"spliced-dek-entries": func(t *testing.T, dir, h1, h2 string) {
				ks := readKeystore(t, dir, run)
				ks.Blobs[h1], ks.Blobs[h2] = ks.Blobs[h2], ks.Blobs[h1]
				write(t, dir, ks)
			},
		}
		for name, corrupt := range cases {
			t.Run(name, func(t *testing.T) {
				s, dir, _ := newStore(t)
				h1, _ := s.Put(run, []byte("keystore tamper one"))
				h2, _ := s.Put(run, []byte("keystore tamper two"))
				corrupt(t, dir, h1, h2)
				if _, err := s.Get(run, h1); !errors.Is(err, vault.ErrCorrupt) {
					t.Fatalf("want ErrCorrupt, got %v", err)
				}
			})
		}
	})

	t.Run("hash-handling", func(t *testing.T) {
		s, _, _ := newStore(t)
		if _, err := s.Put(run, []byte("exists")); err != nil {
			t.Fatal(err)
		}
		unknown := strings.Repeat("cd", 32)
		if _, err := s.Get(run, unknown); !errors.Is(err, vault.ErrNotFound) {
			t.Fatalf("unknown hash: want ErrNotFound, got %v", err)
		}
		malformed := []string{
			"../x",
			"..\\x",
			strings.Repeat("a", 63),
			strings.Repeat("a", 65),
			strings.ToUpper(strings.Repeat("ab", 32)),
			"",
		}
		for _, h := range malformed {
			if _, err := s.Get(run, h); !errors.Is(err, vault.ErrNotFound) {
				t.Fatalf("malformed hash %q: want ErrNotFound, got %v", h, err)
			}
			if err := s.EraseBlob(run, h); !errors.Is(err, vault.ErrNotFound) {
				t.Fatalf("EraseBlob malformed hash %q: want ErrNotFound, got %v", h, err)
			}
		}
	})
}

// T6 — master key discipline.
func TestMasterKeyDiscipline(t *testing.T) {
	t.Run("created-on-fresh-open", func(t *testing.T) {
		_, dir, _ := newStore(t)
		b, err := os.ReadFile(filepath.Join(dir, "master.key"))
		if err != nil {
			t.Fatalf("master.key not created: %v", err)
		}
		if len(b) != 32 {
			t.Fatalf("master.key is %d bytes, want 32", len(b))
		}
	})

	t.Run("missing-with-keystores", func(t *testing.T) {
		s, dir, _ := newStore(t)
		if _, err := s.Put("run-x", []byte("data")); err != nil {
			t.Fatal(err)
		}
		s.Close()
		if err := os.Remove(filepath.Join(dir, "master.key")); err != nil {
			t.Fatal(err)
		}
		if _, err := vault.Open(dir, &fakeRand{}); err == nil {
			t.Fatal("Open must refuse to mint a new master key over existing keystores")
		}
	})

	t.Run("wrong-size", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "master.key"), bytes.Repeat([]byte{1}, 16), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := vault.Open(dir, &fakeRand{}); err == nil {
			t.Fatal("Open must reject a wrong-size master key")
		}
	})
}

// T7 — no plaintext and no raw key material on disk. The fake entropy
// source records every chunk it served; 32-byte chunks are keys
// (master, run keys, DEKs) and must appear nowhere except the master
// key file itself. Nonces (12-byte chunks) are public by design.
func TestNoKeyOrPlaintextMaterialOnDisk(t *testing.T) {
	s, dir, r := newStore(t)
	marker := []byte("VAULT-MARKER-plaintext-sentinel-0123456789")

	h1, _ := s.Put("run-a", marker)
	s.Put("run-a", append([]byte("second "), marker...))
	s.Put("run-b", append([]byte("other run "), marker...))
	// Force a rotation so re-wrapped material is on disk too.
	if err := s.EraseBlob("run-a", h1); err != nil {
		t.Fatalf("EraseBlob: %v", err)
	}

	masterPath := filepath.Join(dir, "master.key")
	master := r.chunks[0]
	if len(master) != 32 {
		t.Fatalf("first entropy chunk is %d bytes, want the 32-byte master key", len(master))
	}

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(b, marker) {
			t.Errorf("plaintext marker found in %s", path)
		}
		for i, chunk := range r.chunks {
			if len(chunk) != 32 {
				continue // nonces are public
			}
			if path == masterPath && i == 0 {
				if !bytes.Equal(b, chunk) {
					t.Errorf("master.key does not hold the first served key chunk")
				}
				continue
			}
			if bytes.Contains(b, chunk) {
				t.Errorf("raw key material (chunk %d) found in %s", i, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// T8 — the repo's invoke-twice/byte-compare determinism law, applied
// to the store: identical entropy + identical ops => identical trees.
func TestDeterministicByteLayout(t *testing.T) {
	runOps := func(t *testing.T, dir string) {
		t.Helper()
		s, err := vault.Open(dir, &fakeRand{})
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		s.Put("run-a", []byte("alpha"))
		h2, _ := s.Put("run-a", []byte("beta"))
		s.Put("run-b", []byte("gamma"))
		if err := s.EraseBlob("run-a", h2); err != nil {
			t.Fatal(err)
		}
		if err := s.EraseRun("run-b"); err != nil {
			t.Fatal(err)
		}
	}

	snapshot := func(t *testing.T, dir string) map[string][]byte {
		t.Helper()
		files := map[string][]byte{}
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files[rel] = b
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return files
	}

	dirA, dirB := t.TempDir(), t.TempDir()
	runOps(t, dirA)
	runOps(t, dirB)
	a, b := snapshot(t, dirA), snapshot(t, dirB)
	if len(a) != len(b) {
		t.Fatalf("tree size differs: %d vs %d files", len(a), len(b))
	}
	for rel, ab := range a {
		bb, ok := b[rel]
		if !ok {
			t.Fatalf("file %s missing from second tree", rel)
		}
		if !bytes.Equal(ab, bb) {
			t.Fatalf("file %s differs between identical op sequences", rel)
		}
	}
}

// T9 — lifecycle: persistence across reopen, closed-store guards,
// constructor guards.
func TestLifecycle(t *testing.T) {
	t.Run("reopen-persistence", func(t *testing.T) {
		dir := t.TempDir()
		s, err := vault.Open(dir, &fakeRand{})
		if err != nil {
			t.Fatal(err)
		}
		pt := []byte("survives reopen")
		h, err := s.Put("run-p", pt)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		s2, err := vault.Open(dir, &fakeRand{n: 200})
		if err != nil {
			t.Fatal(err)
		}
		defer s2.Close()
		got, err := s2.Get("run-p", h)
		if err != nil || !bytes.Equal(got, pt) {
			t.Fatalf("blob lost across reopen: %v", err)
		}
	})

	t.Run("closed-store", func(t *testing.T) {
		s, _, _ := newStore(t)
		h, _ := s.Put("run-c", []byte("x"))
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("Close must be idempotent, got %v", err)
		}
		if _, err := s.Put("run-c", []byte("y")); err == nil {
			t.Fatal("Put on closed store must error")
		}
		if _, err := s.Get("run-c", h); err == nil {
			t.Fatal("Get on closed store must error")
		}
		if err := s.EraseBlob("run-c", h); err == nil {
			t.Fatal("EraseBlob on closed store must error")
		}
		if err := s.EraseRun("run-c"); err == nil {
			t.Fatal("EraseRun on closed store must error")
		}
	})

	t.Run("nil-entropy", func(t *testing.T) {
		if _, err := vault.Open(t.TempDir(), nil); err == nil {
			t.Fatal("Open(nil entropy) must error, never default to ambient rand")
		}
	})

	t.Run("empty-run-id", func(t *testing.T) {
		s, _, _ := newStore(t)
		if _, err := s.Put("", []byte("x")); err == nil {
			t.Fatal("Put with empty run id must error")
		}
		h := strings.Repeat("ef", 32)
		if _, err := s.Get("", h); err == nil {
			t.Fatal("Get with empty run id must error")
		}
		if err := s.EraseBlob("", h); err == nil {
			t.Fatal("EraseBlob with empty run id must error")
		}
		if err := s.EraseRun(""); err == nil {
			t.Fatal("EraseRun with empty run id must error")
		}
	})

	t.Run("tmp-cleanup", func(t *testing.T) {
		dir := t.TempDir()
		s, err := vault.Open(dir, &fakeRand{})
		if err != nil {
			t.Fatal(err)
		}
		h, _ := s.Put("run-t", []byte("data"))
		s.Close()
		// A leftover pre-rename temp never holds the only copy of
		// anything; Open must remove it and proceed.
		stray := filepath.Join(dir, "keys", ".tmp-stray")
		if err := os.WriteFile(stray, []byte("partial write"), 0o600); err != nil {
			t.Fatal(err)
		}
		s2, err := vault.Open(dir, &fakeRand{n: 99})
		if err != nil {
			t.Fatalf("Open with stray temp: %v", err)
		}
		defer s2.Close()
		if _, err := os.Stat(stray); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("stray temp file not cleaned up on Open")
		}
		if _, err := s2.Get("run-t", h); err != nil {
			t.Fatalf("store unusable after temp cleanup: %v", err)
		}
	})
}

// Pure format layer: invoke twice, byte-compare (CLAUDE.md determinism
// law for pure components).
func TestPureFormatDeterminism(t *testing.T) {
	var dek, kek, key [32]byte
	var nonce [12]byte
	for i := range dek {
		dek[i] = byte(i)
		kek[i] = byte(0x40 + i)
		key[i] = byte(0x80 + i)
	}
	for i := range nonce {
		nonce[i] = byte(0xf0 + i)
	}
	pt := []byte("determinism probe")
	aad := []byte("run-determinism")

	f1 := vault.EncodeBlobV1(dek, nonce, pt)
	f2 := vault.EncodeBlobV1(dek, nonce, pt)
	if !bytes.Equal(f1, f2) {
		t.Fatal("EncodeBlobV1 is not deterministic")
	}
	if vault.BlobAddress(f1) != vault.BlobAddress(f2) {
		t.Fatal("BlobAddress is not deterministic")
	}
	w1 := vault.WrapKey(kek, nonce, key, aad)
	w2 := vault.WrapKey(kek, nonce, key, aad)
	if !bytes.Equal(w1, w2) {
		t.Fatal("WrapKey is not deterministic")
	}
}

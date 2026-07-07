// SECURITY-CRITICAL (vault/CLAUDE.md). Per-run keystore files: the
// mutable key store whose destruction IS erasure (D9). The keystore
// is as durability-critical as the log — its corruption is
// equivalent to erasure of the entire run (ADR-0021) — so every
// write fsyncs a temp file and renames it into place atomically.
package vault

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

const keystoreVersion = 1

// keystore is serialized with plain encoding/json, NOT kernel/canon:
// it is mutable, never hashed, and never enters the log. json.Marshal
// sorts map keys, so the bytes stay deterministic.
type keystore struct {
	Version       int               `json:"version"`
	RunID         string            `json:"run_id"` // debug aid; the wrap AAD is authoritative
	WrappedRunKey string            `json:"wrapped_run_key"`
	Blobs         map[string]string `json:"blobs"` // blob address -> wrapped DEK (hex)
}

func (s *Store) blobPath(h kernel.Hash) string {
	return filepath.Join(s.dir, "blobs", h)
}

// keystorePath: hex neutralizes path separators and case-insensitive
// filename collisions. Local layout, not a data contract (ADR-0021).
func (s *Store) keystorePath(run kernel.RunID) string {
	return filepath.Join(s.dir, "keys", hex.EncodeToString([]byte(run))+".json")
}

// loadKeystore returns (nil, nil) when the run has no keystore —
// which, for an address whose blob file exists, is the erased state.
func (s *Store) loadKeystore(run kernel.RunID) (*keystore, error) {
	b, err := os.ReadFile(s.keystorePath(run))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("vault: keystore run=%q: %w", run, err)
	}
	var ks keystore
	if err := json.Unmarshal(b, &ks); err != nil {
		return nil, fmt.Errorf("vault: keystore run=%q: %w", run, ErrCorrupt)
	}
	if ks.Version != keystoreVersion || len(ks.WrappedRunKey) != 2*wrappedSize {
		return nil, fmt.Errorf("vault: keystore run=%q: %w", run, ErrCorrupt)
	}
	if ks.Blobs == nil {
		ks.Blobs = map[string]string{}
	}
	return &ks, nil
}

func (s *Store) saveKeystore(run kernel.RunID, ks *keystore) error {
	b, err := json.Marshal(ks)
	if err != nil {
		return fmt.Errorf("vault: keystore run=%q: %w", run, err)
	}
	return writeFileAtomic(s.keystorePath(run), b)
}

// unwrapRunKey recovers the run key under the master key. The AAD is
// the run id from the REQUEST, never from the file, so a keystore
// copied between runs fails authentication.
func (s *Store) unwrapRunKey(ks *keystore, run kernel.RunID) ([keySize]byte, error) {
	var key [keySize]byte
	raw, err := hex.DecodeString(ks.WrappedRunKey)
	if err != nil {
		return key, fmt.Errorf("vault: keystore run=%q: %w", run, ErrCorrupt)
	}
	key, err = unwrapKey(s.master, raw, []byte(run))
	if err != nil {
		return key, fmt.Errorf("vault: keystore run=%q: %w", run, ErrCorrupt)
	}
	return key, nil
}

// validHash: exactly 64 lowercase hex chars. Anything else is treated
// as not-found, which closes path traversal through caller-supplied
// addresses outright.
func validHash(h kernel.Hash) bool {
	if len(h) != 64 {
		return false
	}
	for i := 0; i < len(h); i++ {
		c := h[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

const tmpPattern = ".tmp-*"

// writeFileAtomic: temp file in the target's directory, write, fsync,
// close, rename over the target (os.Rename replaces on Windows). The
// parent-directory fsync after rename is best-effort — it is not
// portable to Windows, so its failure is ignored.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, tmpPattern)
	if err != nil {
		return fmt.Errorf("vault: write %s: %w", filepath.Base(path), err)
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	if werr == nil {
		werr = f.Sync()
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp, path)
	}
	if werr != nil {
		os.Remove(tmp)
		return fmt.Errorf("vault: write %s: %w", filepath.Base(path), werr)
	}
	if d, err := os.Open(dir); err == nil {
		d.Sync() // best-effort (see above)
		d.Close()
	}
	return nil
}

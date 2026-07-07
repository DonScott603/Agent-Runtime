// Package vault is the Stage-1 blob store (vault-lite): content
// addressing over ciphertext with crypto-erasure (WP-03; RFC-0002 §6,
// D9/D10; ADR-0021).
//
// SECURITY-CRITICAL (vault/CLAUDE.md): every diff here needs human
// review; key material never appears in events, payloads, errors, or
// logs.
//
// Key hierarchy: each blob is sealed under its own random DEK; a
// per-run key wraps the run's DEKs; the master key (file-based at
// Stage 1) wraps each run key. Blob files are immutable — the log
// references their addresses forever — so erasure operates purely on
// keys: EraseRun destroys the run's keystore; EraseBlob rotates the
// run key, re-wrapping survivors and excluding the target (D9). The
// signed redaction.applied event that accompanies erasure lands with
// the log writer in a later work package.
//
// Erased content (blob file present, key destroyed → ErrErased) is
// distinguishable from never-stored (no blob file → ErrNotFound).
// Orphaned blobs — a crash between blob write and keystore save —
// are indistinguishable from erased, harmless, and never collected.
//
// The store assumes a single process and single writer (Stage 1); a
// mutex serializes all operations. Entropy is injected: pass
// crypto/rand.Reader at the composition root. kernel.Entropy is NOT
// used here — its answers are recorded as rng.seed events, which
// would write key material into the log.
package vault

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

var (
	// ErrErased: the blob file exists but no key is available for it
	// under the requested run — the content is crypto-erased.
	ErrErased = errors.New("vault: content erased (key destroyed)")
	// ErrNotFound: no blob file under that address, or the address is
	// malformed.
	ErrNotFound = errors.New("vault: blob not found")
	// ErrCorrupt: stored bytes fail hash or AEAD verification.
	ErrCorrupt = errors.New("vault: content failed verification")
)

type Store struct {
	mu     sync.Mutex
	dir    string
	rand   io.Reader
	master [keySize]byte
	closed bool
}

// Open opens (or initializes) the vault at dir. A master key is
// minted only for a fresh vault; if keystores exist without a master
// key, Open refuses — silently minting a new master would present
// every existing run as corruption instead of what it is.
func Open(dir string, rand io.Reader) (*Store, error) {
	if rand == nil {
		return nil, errors.New("vault: nil entropy source (pass crypto/rand.Reader at the composition root)")
	}
	blobsDir := filepath.Join(dir, "blobs")
	keysDir := filepath.Join(dir, "keys")
	for _, d := range []string{blobsDir, keysDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, fmt.Errorf("vault: open: %w", err)
		}
	}
	// Leftover pre-rename temps never hold the only copy of anything;
	// removing them is always safe.
	for _, d := range []string{dir, blobsDir, keysDir} {
		if tmps, err := filepath.Glob(filepath.Join(d, tmpPattern)); err == nil {
			for _, t := range tmps {
				os.Remove(t)
			}
		}
	}

	s := &Store{dir: dir, rand: rand}
	masterPath := filepath.Join(dir, "master.key")
	b, err := os.ReadFile(masterPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		existing, _ := filepath.Glob(filepath.Join(keysDir, "*.json"))
		if len(existing) > 0 {
			return nil, errors.New("vault: keystores present but master key missing — refusing to mint a new master key")
		}
		k, kerr := newKey(rand)
		if kerr != nil {
			return nil, kerr
		}
		if werr := writeFileAtomic(masterPath, k[:]); werr != nil {
			wipe(k[:])
			return nil, werr
		}
		s.master = k
		wipe(k[:])
	case err != nil:
		return nil, fmt.Errorf("vault: open master key: %w", err)
	case len(b) != keySize:
		wipe(b)
		return nil, errors.New("vault: master key has wrong size")
	default:
		copy(s.master[:], b)
		wipe(b)
	}
	return s, nil
}

func (s *Store) guard(run kernel.RunID) error {
	if s.closed {
		return errors.New("vault: store is closed")
	}
	if run == "" {
		return errors.New("vault: empty run id")
	}
	return nil
}

// Put seals plaintext into a new blob under run and returns its
// content address. Ordering: encrypt → address → blob write →
// keystore save; a crash between the writes leaves an orphaned blob
// (harmless, indistinguishable from erased).
func (s *Store) Put(run kernel.RunID, plaintext []byte) (kernel.Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.guard(run); err != nil {
		return "", err
	}
	ks, err := s.loadKeystore(run)
	if err != nil {
		return "", err
	}
	var runKey [keySize]byte
	defer wipe(runKey[:])
	if ks == nil {
		runKey, err = newKey(s.rand)
		if err != nil {
			return "", err
		}
		wnonce, werr := newNonce(s.rand)
		if werr != nil {
			return "", werr
		}
		ks = &keystore{
			Version:       keystoreVersion,
			RunID:         run,
			WrappedRunKey: hex.EncodeToString(WrapKey(s.master, wnonce, runKey, []byte(run))),
			Blobs:         map[string]string{},
		}
	} else {
		runKey, err = s.unwrapRunKey(ks, run)
		if err != nil {
			return "", err
		}
	}

	dek, err := newKey(s.rand)
	if err != nil {
		return "", err
	}
	defer wipe(dek[:])
	bnonce, err := newNonce(s.rand)
	if err != nil {
		return "", err
	}
	file := EncodeBlobV1(dek, bnonce, plaintext)
	h := BlobAddress(file)
	if err := writeFileAtomic(s.blobPath(h), file); err != nil {
		return "", err
	}
	dnonce, err := newNonce(s.rand)
	if err != nil {
		return "", err
	}
	ks.Blobs[h] = hex.EncodeToString(WrapKey(runKey, dnonce, dek, []byte(h)))
	if err := s.saveKeystore(run, ks); err != nil {
		return "", err
	}
	return h, nil
}

// Get returns the plaintext for h under run. Order of judgment:
// malformed or absent address → ErrNotFound; stored bytes that fail
// the address check → ErrCorrupt (tamper beats erasure); no key for
// (run, h) → ErrErased; any AEAD or parse failure → ErrCorrupt.
func (s *Store) Get(run kernel.RunID, h kernel.Hash) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.guard(run); err != nil {
		return nil, err
	}
	if !validHash(h) {
		return nil, fmt.Errorf("vault: get run=%q: malformed address: %w", run, ErrNotFound)
	}
	file, err := os.ReadFile(s.blobPath(h))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("vault: get run=%q blob=%s: %w", run, h, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("vault: get run=%q blob=%s: %w", run, h, err)
	}
	if BlobAddress(file) != h {
		return nil, fmt.Errorf("vault: get run=%q blob=%s: %w", run, h, ErrCorrupt)
	}
	ks, err := s.loadKeystore(run)
	if err != nil {
		return nil, err
	}
	var wrappedDEK string
	if ks != nil {
		wrappedDEK = ks.Blobs[h]
	}
	if wrappedDEK == "" {
		return nil, fmt.Errorf("vault: get run=%q blob=%s: %w", run, h, ErrErased)
	}
	runKey, err := s.unwrapRunKey(ks, run)
	if err != nil {
		return nil, err
	}
	defer wipe(runKey[:])
	raw, err := hex.DecodeString(wrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("vault: get run=%q blob=%s: %w", run, h, ErrCorrupt)
	}
	dek, err := unwrapKey(runKey, raw, []byte(h))
	if err != nil {
		return nil, fmt.Errorf("vault: get run=%q blob=%s: %w", run, h, ErrCorrupt)
	}
	defer wipe(dek[:])
	pt, err := decodeBlobV1(dek, file)
	if err != nil {
		return nil, fmt.Errorf("vault: get run=%q blob=%s: %w", run, h, ErrCorrupt)
	}
	return pt, nil
}

// EraseBlob destroys the key for h under run by rotating the run key:
// survivors are re-wrapped under a fresh run key, the target is
// excluded, and the keystore is replaced atomically (D9, ADR-0021).
// Idempotent: erasing an already-erased blob (or one held by another
// run) is a no-op; an address never stored at all is ErrNotFound. The
// blob file itself is never touched.
func (s *Store) EraseBlob(run kernel.RunID, h kernel.Hash) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.guard(run); err != nil {
		return err
	}
	if !validHash(h) {
		return fmt.Errorf("vault: erase run=%q: malformed address: %w", run, ErrNotFound)
	}
	ks, err := s.loadKeystore(run)
	if err != nil {
		return err
	}
	if ks == nil || ks.Blobs[h] == "" {
		if _, serr := os.Stat(s.blobPath(h)); errors.Is(serr, fs.ErrNotExist) {
			return fmt.Errorf("vault: erase run=%q blob=%s: %w", run, h, ErrNotFound)
		} else if serr != nil {
			return fmt.Errorf("vault: erase run=%q blob=%s: %w", run, h, serr)
		}
		return nil
	}

	oldKey, err := s.unwrapRunKey(ks, run)
	if err != nil {
		return err
	}
	defer wipe(oldKey[:])

	// Unwrap every surviving DEK first; any failure aborts with the
	// old keystore untouched. Sorted order keeps entropy consumption
	// deterministic (CLAUDE.md: sort keys when map iteration feeds
	// serialized output).
	survivors := make([]string, 0, len(ks.Blobs)-1)
	for bh := range ks.Blobs {
		if bh != h {
			survivors = append(survivors, bh)
		}
	}
	sort.Strings(survivors)
	deks := make([][keySize]byte, len(survivors))
	defer func() {
		for i := range deks {
			wipe(deks[i][:])
		}
	}()
	for i, bh := range survivors {
		raw, derr := hex.DecodeString(ks.Blobs[bh])
		if derr != nil {
			return fmt.Errorf("vault: erase run=%q blob=%s: %w", run, bh, ErrCorrupt)
		}
		deks[i], derr = unwrapKey(oldKey, raw, []byte(bh))
		if derr != nil {
			return fmt.Errorf("vault: erase run=%q blob=%s: %w", run, bh, ErrCorrupt)
		}
	}

	freshKey, err := newKey(s.rand)
	if err != nil {
		return err
	}
	defer wipe(freshKey[:])
	wnonce, err := newNonce(s.rand)
	if err != nil {
		return err
	}
	next := &keystore{
		Version:       keystoreVersion,
		RunID:         run,
		WrappedRunKey: hex.EncodeToString(WrapKey(s.master, wnonce, freshKey, []byte(run))),
		Blobs:         make(map[string]string, len(survivors)),
	}
	for i, bh := range survivors {
		dnonce, nerr := newNonce(s.rand)
		if nerr != nil {
			return nerr
		}
		next.Blobs[bh] = hex.EncodeToString(WrapKey(freshKey, dnonce, deks[i], []byte(bh)))
	}
	// The highest-exposure write in the store (ADR-0021): the rename
	// either lands the rotated keystore or leaves the old one intact.
	return s.saveKeystore(run, next)
}

// EraseRun destroys the run's entire keystore: every blob of the run
// becomes unreadable in one operation (D9). Blob files and the
// addresses referencing them remain intact. Idempotent.
func (s *Store) EraseRun(run kernel.RunID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.guard(run); err != nil {
		return err
	}
	if err := os.Remove(s.keystorePath(run)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("vault: erase run=%q: %w", run, err)
	}
	return nil
}

// Close zeroes the in-memory master key (best-effort, ADR-0021) and
// refuses further operations. Idempotent.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	wipe(s.master[:])
	s.closed = true
	return nil
}

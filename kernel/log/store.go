// Durable single-writer event store (WP-04b; RFC-0002 §2, D5;
// ADR-0022). This is the systems half of the package — the one file
// allowed to import os (kernel/log local law: the pure layer never
// does). Append code never rewrites stored bytes (constitution #4);
// recovery truncates only torn tails, which never committed.
package log

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

// ErrStorePoisoned marks a store whose append handle failed mid-write
// or mid-sync: the on-disk tail may be torn, so every later call
// refuses until the store is reopened and recovery has run
// (ADR-0022). The returned errors wrap both this sentinel and the
// original failure.
var ErrStorePoisoned = errors.New("log: store poisoned by a failed append; reopen to recover (ADR-0022)")

// ErrStoreClosed is returned by calls on a closed store.
var ErrStoreClosed = errors.New("log: store is closed")

// SeqGapError reports a gap between valid records found at recovery:
// refuse to open the store and alarm (docs/errors.md SEQ_GAP —
// "should be impossible (D5 gapless); treat as corruption").
type SeqGapError struct {
	Want   kernel.Seq
	Got    kernel.Seq
	Offset int64 // file offset of the offending record
}

func (e *SeqGapError) Error() string {
	return fmt.Sprintf("SEQ_GAP: record at offset %d has seq %d, want %d (refusing to open; docs/errors.md)", e.Offset, e.Got, e.Want)
}

// CorruptError reports corruption of committed bytes found at
// recovery (docs/errors.md LOG_CORRUPT; ADR-0022 C1-C3): refuse to
// open, alarm, and leave the file bytes untouched. Torn tails are NOT
// corruption — they are truncated by recovery. Chain-level failures
// wrap *ChainBrokenError (errors.As-able).
type CorruptError struct {
	Offset int64
	Detail string
	Err    error // underlying cause, if any
}

func (e *CorruptError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("LOG_CORRUPT: offset %d: %s: %v (refusing to open; docs/errors.md)", e.Offset, e.Detail, e.Err)
	}
	return fmt.Sprintf("LOG_CORRUPT: offset %d: %s (refusing to open; docs/errors.md)", e.Offset, e.Detail)
}

func (e *CorruptError) Unwrap() error { return e.Err }

// writeSyncer is the injectable seam over the *os.File subset the
// store uses, so crash-fault tests can exercise every point of the
// append and recovery paths deterministically (fail after N bytes,
// before Sync, after Sync before return) without killing a process.
// Production always uses the *os.File directly.
type writeSyncer interface {
	Write(p []byte) (n int, err error)
	Sync() error
	Truncate(size int64) error
	Close() error
}

// Store is the durable event log: the single log-writer of RFC-0002
// D5. It owns seq (gapless, assigned on commit) and per-run prev_hash
// threading; an append is committed when its record has been written
// and the file synced — Append does not return success before then.
type Store struct {
	mu         sync.Mutex
	f          writeSyncer
	path       string
	lastSeq    kernel.Seq
	heads      map[kernel.RunID]kernel.Hash
	hasRecords bool  // distinguishes an empty store from base != 1
	poison     error // non-nil after a failed append; see ErrStorePoisoned
	closed     bool

	// Container base (ADR-0022 A1; anchored by WriteAnchor, ADR-0024):
	// the first committed record's seq and event_id, captured at
	// recovery or on the first append to a virgin store. Zero values
	// iff hasRecords is false.
	baseSeq      kernel.Seq
	firstEventID kernel.Hash
}

// Open opens or creates the single-file event log under dir
// (<dir>/events.log; local layout, ADR-0022) and runs recovery: scan,
// truncate a torn tail (then Sync — recovery decisions are themselves
// durable), refuse to open on SEQ_GAP or LOG_CORRUPT leaving the file
// bytes untouched.
func Open(dir string) (*Store, error) { return openStore(dir, nil) }

// openStore is Open plus the fault seam: wrap, when non-nil, wraps
// the real file handle before recovery and appends use it. Tests
// only; production passes nil.
func openStore(dir string, wrap func(*os.File) writeSyncer) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("log: open: %w", err)
	}
	path := filepath.Join(dir, "events.log")
	data, err := os.ReadFile(path)
	created := false
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("log: open %s: %w", path, err)
		}
		created = true
		data = nil
	}
	res, err := scanLog(data)
	if err != nil {
		return nil, err // refusal: the file bytes stay untouched
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("log: open %s: %w", path, err)
	}
	var h writeSyncer = f
	if wrap != nil {
		h = wrap(f)
	}
	fail := func(err error) (*Store, error) {
		h.Close()
		return nil, err
	}
	if res.validLen < int64(len(logMagic)) {
		// Fresh store or torn creation: Open never returned, so
		// nothing was ever committed. Reset = Truncate(0) + rewrite
		// magic + Sync — never an in-place patch (ADR-0022, owner A2).
		if err := h.Truncate(0); err != nil {
			return fail(fmt.Errorf("log: initialize %s: %w", path, err))
		}
		if _, err := h.Write([]byte(logMagic)); err != nil {
			return fail(fmt.Errorf("log: initialize %s: %w", path, err))
		}
		if err := h.Sync(); err != nil {
			return fail(fmt.Errorf("log: initialize %s: %w", path, err))
		}
		if created {
			syncDir(dir)
		}
		res.validLen = int64(len(logMagic))
	} else if res.torn {
		// WAL-truncate (ADR-0022): [validLen, EOF) belongs to the one
		// append whose Sync never returned — never committed, never
		// acknowledged. Truncation is synced before Open returns:
		// recovery decisions are themselves durable (owner A3).
		if err := h.Truncate(res.validLen); err != nil {
			return fail(fmt.Errorf("log: recovery truncate %s: %w", path, err))
		}
		if err := h.Sync(); err != nil {
			return fail(fmt.Errorf("log: recovery sync %s: %w", path, err))
		}
	}
	if _, err := f.Seek(res.validLen, io.SeekStart); err != nil {
		return fail(fmt.Errorf("log: open %s: %w", path, err))
	}
	st := &Store{
		f:          h,
		path:       path,
		lastSeq:    res.lastSeq,
		heads:      res.heads,
		hasRecords: len(res.events) > 0,
	}
	if st.hasRecords {
		st.baseSeq = res.events[0].Seq
		st.firstEventID = res.events[0].EventID
	}
	return st, nil
}

// syncDir flushes the directory entry after file creation.
// Best-effort: a directory flush is not portable to Windows, so its
// failure is ignored (vault/keystore.go precedent; the per-platform
// durability statement is ADR-0022's).
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
}

// usable is the common precondition of every store call.
func (s *Store) usable() error {
	if s.closed {
		return ErrStoreClosed
	}
	if s.poison != nil {
		return fmt.Errorf("%w (cause: %w)", ErrStorePoisoned, s.poison)
	}
	return nil
}

// Append commits one event: assign seq = last+1 (1 on a virgin
// store), thread prev_hash from the run's head (genesis for an unseen
// run — each distinct run_id, including "" owner-scope, keys its own
// chain), seal via kernel.SealEvent (which normalizes first,
// ADR-0020), then persist: one framed record, one Write, one Sync —
// the fsync-at-event-boundary of RFC-0002 §9.3. It returns the sealed
// event only after the Sync returns; any caller-supplied seq,
// prev_hash or event_id is overwritten unconditionally.
//
// The caller supplies ts, mono, principal, type, type_version,
// payload, blobs and sig: the writer never reads time or entropy
// (kernel Clock/Entropy handles are WP-09).
func (s *Store) Append(e kernel.Event) (kernel.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.usable(); err != nil {
		return kernel.Event{}, err
	}
	return s.appendLocked(e)
}

// appendLocked is Append's body: seq assignment, chain threading,
// seal, persist. WriteAnchor (kernel/log/anchor.go) shares it so an
// anchor commits through exactly the normal append path.
func (s *Store) appendLocked(e kernel.Event) (kernel.Event, error) {
	e.Seq = s.lastSeq + 1
	e.EventID = ""
	if h, ok := s.heads[e.RunID]; ok {
		// NextInChain threading with the head hash directly: heads
		// only ever hold sealed event_ids, so its unsealed-predecessor
		// guard is satisfied by construction.
		e.PrevHash = h
	} else {
		e = Genesis(e)
	}
	sealed, err := kernel.SealEvent(e)
	if err != nil {
		// Nothing was written: the store is not poisoned.
		return kernel.Event{}, fmt.Errorf("log: append: %w", err)
	}
	if err := s.appendSealedLocked(sealed); err != nil {
		return kernel.Event{}, err
	}
	return sealed, nil
}

// appendSealed persists an ALREADY-sealed event verbatim, enforcing
// on write exactly what recovery enforces on read: consecutive seq
// when records exist (any base on an empty store — the file-local
// projection of D5 gaplessness, ADR-0022), prev_hash == run head (or
// ZeroHash for an unseen run), and event_id re-derivation. In-memory
// state advances only after Sync returns nil; any write or sync
// failure poisons the store (ErrStorePoisoned) because the tail may
// be torn — reopen to recover. White-box tests use it to persist
// fixture events whose seq base is not 1 (docs/trace-annotated.md).
func (s *Store) appendSealed(sealed kernel.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.usable(); err != nil {
		return err
	}
	return s.appendSealedLocked(sealed)
}

func (s *Store) appendSealedLocked(sealed kernel.Event) error {
	// Enforce on write exactly what recovery enforces on read
	// (ADR-0022): consecutive seq when records exist (any base on an
	// empty store), run-head linkage, event identity.
	if s.hasRecords && sealed.Seq != s.lastSeq+1 {
		return fmt.Errorf("log: append: seq %d, want %d (gapless, RFC-0002 D5)", sealed.Seq, s.lastSeq+1)
	}
	want := kernel.ZeroHash
	if h, ok := s.heads[sealed.RunID]; ok {
		want = h
	}
	if sealed.PrevHash != want {
		return fmt.Errorf("log: append: prev_hash %q does not link to run %q head %q", sealed.PrevHash, sealed.RunID, want)
	}
	resealed, err := kernel.SealEvent(sealed)
	if err != nil {
		return fmt.Errorf("log: append: %w", err)
	}
	if resealed.EventID != sealed.EventID {
		return fmt.Errorf("log: append: event_id %q does not re-derive (derived %q)", sealed.EventID, resealed.EventID)
	}
	body, err := kernel.Canonical(sealed)
	if err != nil {
		return fmt.Errorf("log: append: %w", err)
	}
	rec, err := encodeRecord(body)
	if err != nil {
		return err // nothing written: not poisoned
	}
	// ONE Write, ONE Sync; commit is the Sync returning nil (fsync at
	// the event boundary, RFC-0002 §9.3). In-memory state advances
	// only after commit; a failure of either poisons the store — the
	// on-disk tail may be torn and only reopening (recovery) may
	// resolve it.
	if _, err := s.f.Write(rec); err != nil {
		s.poison = err
		return fmt.Errorf("log: append write: %w", err)
	}
	if err := s.f.Sync(); err != nil {
		s.poison = err
		return fmt.Errorf("log: append sync: %w", err)
	}
	if !s.hasRecords {
		// First record of a virgin store: it IS the container base
		// (ADR-0022 A1; attested by WriteAnchor, ADR-0024).
		s.baseSeq = sealed.Seq
		s.firstEventID = sealed.EventID
	}
	s.lastSeq = sealed.Seq
	s.hasRecords = true
	s.heads[sealed.RunID] = sealed.EventID
	return nil
}

// ReadAll re-reads the file through the same validator recovery uses
// (scanLog) and returns every committed event in seq order. Minimal
// read API for tests and WP-05a; an iterator may supersede it pre-1.0.
func (s *Store) ReadAll() ([]kernel.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.usable(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("log: read %s: %w", s.path, err)
	}
	res, err := scanLog(data)
	if err != nil {
		return nil, err
	}
	// A healthy open store holds the single writer; a torn tail or a
	// diverged last seq means someone else touched the file.
	if res.torn || res.lastSeq != s.lastSeq {
		return nil, &CorruptError{Offset: res.validLen, Detail: "file diverged from the open store (external modification?)"}
	}
	return res.events, nil
}

// LastSeq returns the seq of the last committed event (0 if none).
func (s *Store) LastSeq() kernel.Seq {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastSeq
}

// Close closes the append handle. Idempotent; it never writes.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if err := s.f.Close(); err != nil {
		return fmt.Errorf("log: close: %w", err)
	}
	return nil
}

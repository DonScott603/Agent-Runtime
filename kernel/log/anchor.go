// WriteAnchor: the anchor event writer (WP-04c; RFC-0002 §4, D4;
// ADR-0022 A1; ADR-0024). Append-path code — on the same
// human-review footing as store.go (kernel/log local law). The Merkle
// construction itself lives in kernel/anchor.go (import topology:
// the fold-side verifier shares it and must never import kernel/log).
package log

import (
	"errors"
	"fmt"
	"maps"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

// WriteAnchor snapshots the store's current run heads and container
// base attestation, builds the anchor.appended v1 payload, and
// commits it through the normal append path — one Write, one Sync; an
// anchor is just an event. It refuses on an empty store (nothing to
// attest; plain error, never poisoning — ADR-0024). ts and mono are
// caller-supplied like every append: the writer never reads time.
// Cadence/scheduling is out of scope at Stage 1 (manual call); the
// returned sealed event is the Stage-1 export surface.
func (s *Store) WriteAnchor(ts int64, mono uint64) (kernel.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.usable(); err != nil {
		return kernel.Event{}, err
	}
	if !s.hasRecords {
		// Plain refusal, never poisoning: an anchor attests committed
		// history and an empty container has none — the base
		// attestation would be self-referential (ADR-0024).
		return kernel.Event{}, errors.New("log: WriteAnchor: empty store, nothing to anchor (ADR-0024)")
	}
	// Snapshot under the same lock hold that appends, so the payload
	// is exactly the pre-anchor state: heads (including the "" entry —
	// RFC-0002 D4's "latest owner-scope event") and the container base
	// attestation (ADR-0022 A1).
	heads := make(map[kernel.RunID]kernel.Hash, len(s.heads))
	maps.Copy(heads, s.heads)
	root, err := kernel.AnchorRoot(heads)
	if err != nil {
		return kernel.Event{}, fmt.Errorf("log: WriteAnchor: %w", err)
	}
	payload, err := kernel.Canonical(kernel.AnchorPayload{
		Container:  kernel.AnchorContainer{BaseSeq: s.baseSeq, FirstEventID: s.firstEventID},
		Heads:      heads,
		MerkleRoot: root,
	})
	if err != nil {
		return kernel.Event{}, fmt.Errorf("log: WriteAnchor: %w", err)
	}
	return s.appendLocked(kernel.Event{
		RunID:       "",
		TS:          ts,
		Mono:        mono,
		Principal:   "service:kernel",
		Type:        kernel.AnchorEventType,
		TypeVersion: 1,
		Payload:     payload,
	})
}

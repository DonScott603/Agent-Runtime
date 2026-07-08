// SealEvent implementation (WP-04a). It lives beside the frozen shapes
// rather than in kernel/log because kernel/log imports this package for
// the Event type, so the delegation Canonical-style (types.go one-liner
// into a leaf package) can only point at a sibling file here. The chain
// threading helpers and verifier live in kernel/log.
package kernel

import (
	"crypto/sha256"
	"encoding/hex"
)

// sealEvent computes the event identity per RFC-0002 §4 and
// docs/vectors/chain.json _rules: NormalizeEnvelope first (ADR-0020
// Consequences — one logical event, one byte form), then hash the
// canonical envelope with event_id zeroed to "" and sig to null.
// Zeroing is unconditional, so an input carrying a non-empty (possibly
// stale) EventID is re-sealed from scratch, and sealing an already-
// sealed event is a fixpoint. The returned event is the NORMALIZED
// input with EventID set; Sig is preserved — it is zeroed only on the
// hashing copy, because signature validity is the fold layer's job
// (RFC-0002 §5), never the chain's.
func sealEvent(e Event) (Event, error) {
	e = NormalizeEnvelope(e)
	zeroed := e
	zeroed.EventID = ""
	zeroed.Sig = nil
	b, err := Canonical(zeroed)
	if err != nil {
		return Event{}, err
	}
	sum := sha256.Sum256(b)
	e.EventID = hex.EncodeToString(sum[:])
	return e, nil
}

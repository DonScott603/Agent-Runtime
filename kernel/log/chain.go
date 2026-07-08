// Package log implements the event log: the pure seal/chain layer
// (WP-04a; RFC-0002 §2, §4, D4 — per-run prev_hash threading and the
// chain verifier, this file) and the durable single-writer store
// (WP-04b; D5, ADR-0022 — seq assignment, persistence, fsync,
// recovery; store.go). Sealing itself is kernel.SealEvent
// (kernel/seal.go), which lives beside the Event type to keep the
// types.go delegation acyclic; this package builds on it.
//
// Nothing in this file touches disk; the pure layer never imports os
// (local law, CLAUDE.md).
package log

import (
	"fmt"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

// ChainBrokenError reports the FIRST broken position in a run's chain
// (docs/errors.md CHAIN_BROKEN). It is a tamper-evidence tripwire:
// report, never repair. Every VerifyChain failure is of this type.
// Fields describe the event at the broken position — its seq, its
// run_id (in uniformity failures the run_id is the divergent event's,
// not the chain base's).
type ChainBrokenError struct {
	RunID  kernel.RunID
	Seq    kernel.Seq
	Detail string
}

func (e *ChainBrokenError) Error() string {
	return fmt.Sprintf("CHAIN_BROKEN: run %q seq %d: %s", e.RunID, e.Seq, e.Detail)
}

func brokenf(ev kernel.Event, format string, args ...any) error {
	return &ChainBrokenError{RunID: ev.RunID, Seq: ev.Seq, Detail: fmt.Sprintf(format, args...)}
}

// Genesis threads e as the first event of its run: prev_hash is the
// zero hash (RFC-0002 §4; chain.json _rules "genesis").
func Genesis(e kernel.Event) kernel.Event {
	e.PrevHash = kernel.ZeroHash
	return e
}

// NextInChain threads e after prev: prev_hash = the predecessor's
// event_id. The predecessor must already be sealed — threading after
// an unsealed event would otherwise need a placeholder prev_hash and
// silently break the chain, so it is an error instead.
func NextInChain(prev, e kernel.Event) (kernel.Event, error) {
	if prev.EventID == "" {
		return kernel.Event{}, fmt.Errorf("log: NextInChain: predecessor (run %q seq %d) is unsealed", prev.RunID, prev.Seq)
	}
	e.PrevHash = prev.EventID
	return e, nil
}

// VerifyChain walks a single run's events in the given order and
// returns nil iff the chain verifies: genesis prev_hash, per-event
// linkage, uniform run_id (per-run chains, RFC-0002 D4), and every
// event_id re-deriving from its zeroed envelope (kernel.SealEvent).
// The first failure returns a *ChainBrokenError naming that event's
// run and seq — it never repairs, truncates, or continues past it
// (docs/errors.md). Signature VALIDITY is not checked here: sig is
// zeroed out of the hash (chain.json _rules) and consent-event
// signatures are validated at fold time (RFC-0002 §5). An empty input
// verifies vacuously.
func VerifyChain(events []kernel.Event) error {
	if len(events) == 0 {
		return nil
	}
	prev := kernel.ZeroHash
	for i, ev := range events {
		if ev.RunID != events[0].RunID {
			return brokenf(ev, "run_id diverges from chain run_id %q (per-run chains, RFC-0002 D4)", events[0].RunID)
		}
		if ev.PrevHash != prev {
			if i == 0 {
				return brokenf(ev, "genesis prev_hash %q is not the zero hash", ev.PrevHash)
			}
			return brokenf(ev, "prev_hash %q does not link to predecessor event_id %q", ev.PrevHash, prev)
		}
		sealed, err := kernel.SealEvent(ev)
		if err != nil {
			return brokenf(ev, "event identity underivable: %v", err)
		}
		if sealed.EventID != ev.EventID {
			return brokenf(ev, "event_id %q does not re-derive from the zeroed envelope (derived %q)", ev.EventID, sealed.EventID)
		}
		prev = ev.EventID
	}
	return nil
}

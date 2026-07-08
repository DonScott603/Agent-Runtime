// Package chainverify is the standing chain-verification reducer
// (WP-05a; RFC-0002 §9.2): it re-derives per-run heads incrementally —
// the fold independently re-deriving what the store enforces — and
// surfaces CHAIN_BROKEN as an ALARM VALUE in its state, never an
// error that halts the fold (docs/errors.md: report, never repair).
// On a break it alarms ONCE, marks the run broken, freezes its head
// at the last good event and skips the run's later events — mirroring
// VerifyChain's never-continue-past-first-failure semantics per run
// while every other run keeps verifying.
//
// It claims payload understanding of NOTHING (no Handles): it reads
// only envelope fields, so no version gate ever applies and every
// event — unknown types, future anchor events (WP-04c), a
// type_version far beyond any reducer — threads the chain it must
// keep verifying. Totality eating its own dogfood.
//
// Alarm strings are a FIXED VOCABULARY with structured fields (owner
// A1, WP-05a): alarms are hashed state, so no err.Error() passthrough
// or library text ever enters them.
//
// Imports kernel only — never kernel/log (fold/plugins must not;
// kernel/log's round-trip test imports this package).
package chainverify

import (
	"encoding/json"

	"github.com/DonScott603/Agent-Runtime/kernel"
	"github.com/DonScott603/Agent-Runtime/kernel/fold"
)

const (
	PluginID = "chain-verify"
	Semver   = "0.1.0" // bump on ANY behavior change (ADR-0023)
)

// Alarm detail vocabulary (fixed; owner A1). The specifics ride in
// the alarm's structured fields, never in prose.
const (
	DetailGenesis     = "genesis"     // first event of a run: prev_hash != ZeroHash
	DetailLinkage     = "linkage"     // prev_hash does not link to the held head
	DetailIdentity    = "identity"    // event_id does not re-derive from the zeroed envelope
	DetailUnderivable = "underivable" // the envelope cannot be canonicalized/sealed at all
)

// state: all keys always present from Init (nil-vs-empty discipline —
// one logical state, one byte form).
type state struct {
	Alarms []alarm                      `json:"alarms"`
	Broken map[string]bool              `json:"broken"`
	Heads  map[kernel.RunID]kernel.Hash `json:"heads"`
}

// alarm is the docs/errors.md wire shape plus structured facts. Only
// vocabulary and hashes — never error text (owner A1).
type alarm struct {
	Code         string      `json:"code"` // always "CHAIN_BROKEN"
	RunID        string      `json:"run_id"`
	Seq          kernel.Seq  `json:"seq"`
	Detail       string      `json:"detail"`
	ExpectedPrev kernel.Hash `json:"expected_prev,omitempty"`
	GotPrev      kernel.Hash `json:"got_prev,omitempty"`
	ExpectedID   kernel.Hash `json:"expected_id,omitempty"`
	GotID        kernel.Hash `json:"got_id,omitempty"`
}

type reducer struct{}

// New returns the reducer. It is stateless: state is threaded by the
// fold engine.
func New() kernel.Reducer { return reducer{} }

// Registration is the canonical registration: owner scope (it holds
// per-run heads across the whole stream, including the run_id ""
// owner chain), no Handles.
func Registration() fold.Registration {
	return fold.Registration{
		ID:      PluginID,
		Semver:  Semver,
		Scope:   fold.ScopeOwner,
		Reducer: reducer{},
	}
}

func (reducer) Init() json.RawMessage {
	return json.RawMessage(`{"alarms":[],"broken":{},"heads":{}}`)
}

func (reducer) Apply(st json.RawMessage, e kernel.Event) (json.RawMessage, error) {
	var s state
	if err := json.Unmarshal(st, &s); err != nil {
		return nil, err
	}
	run := string(e.RunID)
	if s.Broken[run] {
		// Report once, never repair (docs/errors.md): a broken run's
		// later linkage is meaningless. Byte-identical pass-through.
		return st, nil
	}
	broke := func(a alarm) (json.RawMessage, error) {
		a.Code = "CHAIN_BROKEN"
		a.RunID = run
		a.Seq = e.Seq
		s.Alarms = append(s.Alarms, a)
		s.Broken[run] = true
		return json.Marshal(s)
	}
	want, seenRun := s.Heads[e.RunID]
	if !seenRun {
		want = kernel.ZeroHash
	}
	if e.PrevHash != want {
		detail := DetailLinkage
		if !seenRun {
			detail = DetailGenesis
		}
		return broke(alarm{Detail: detail, ExpectedPrev: want, GotPrev: e.PrevHash})
	}
	sealed, err := kernel.SealEvent(e)
	if err != nil {
		// A1: the cause is structural (unsealable envelope); the
		// error text never enters hashed state.
		return broke(alarm{Detail: DetailUnderivable})
	}
	if sealed.EventID != e.EventID {
		return broke(alarm{Detail: DetailIdentity, ExpectedID: sealed.EventID, GotID: e.EventID})
	}
	s.Heads[e.RunID] = e.EventID
	return json.Marshal(s)
}

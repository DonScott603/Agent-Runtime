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
// Since 0.2.0 (WP-04c; ADR-0024) it also verifies anchor.appended
// events: the Merkle root must recompute from the verifier's OWN held
// heads at that point, and the payload's container attestation must
// match its own observed base (ADR-0022 A1). A failed anchor check is
// ANCHOR_MISMATCH — it impeaches the ANCHOR, not the chain: linkage
// evidence stands, Broken is untouched, and heads keep advancing
// (including the anchor event itself). An anchor is a claim about the
// past, not a gate on the future.
//
// Envelope checks still read only envelope fields and apply to every
// event (totality: unknown types thread the chain). The one payload
// this reducer now understands is anchor.appended v1 — declared in
// Handles as a version gate, so a future v2 anchor makes the view
// honestly unavailable (SCHEMA_UNKNOWN_VERSION) instead of raising
// false anchor alarms from a misparsed payload (ADR-0024, owner R2).
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
	Semver   = "0.2.0" // bump on ANY behavior change (ADR-0023)
)

// Alarm codes (docs/errors.md). CHAIN_BROKEN freezes the run;
// ANCHOR_MISMATCH freezes nothing (ADR-0024).
const (
	CodeChainBroken    = "CHAIN_BROKEN"
	CodeAnchorMismatch = "ANCHOR_MISMATCH"
)

// Alarm detail vocabulary (fixed; owner A1). The specifics ride in
// the alarm's structured fields, never in prose.
const (
	DetailGenesis     = "genesis"     // first event of a run: prev_hash != ZeroHash
	DetailLinkage     = "linkage"     // prev_hash does not link to the held head
	DetailIdentity    = "identity"    // event_id does not re-derive from the zeroed envelope
	DetailUnderivable = "underivable" // the envelope cannot be canonicalized/sealed at all

	// ANCHOR_MISMATCH details (ADR-0024). anchor_payload is the
	// schema-required gate: it fires FIRST (unparseable payload,
	// nil/empty heads, empty root, zero container fields, or a root
	// recomputation failure), so a malformed payload never masquerades
	// as a root or base mismatch.
	DetailAnchorPayload = "anchor_payload"
	DetailAnchorRoot    = "anchor_root" // root does not recompute from the verifier's own heads
	DetailAnchorBase    = "anchor_base" // container attestation does not match the observed base
)

// state: all keys always present from Init (nil-vs-empty discipline —
// one logical state, one byte form). base is the container base as
// OBSERVED: the first event this view ever folded, recorded as stored
// before any checks (ADR-0024; owner R1) — seq 0 / empty id means no
// event seen yet.
type state struct {
	Alarms []alarm                      `json:"alarms"`
	Base   containerBase                `json:"base"`
	Broken map[string]bool              `json:"broken"`
	Heads  map[kernel.RunID]kernel.Hash `json:"heads"`
}

type containerBase struct {
	EventID kernel.Hash `json:"event_id"`
	Seq     kernel.Seq  `json:"seq"`
}

// alarm is the docs/errors.md wire shape plus structured facts. Only
// vocabulary and hashes — never error text (owner A1).
type alarm struct {
	Code         string      `json:"code"` // CHAIN_BROKEN | ANCHOR_MISMATCH
	RunID        string      `json:"run_id"`
	Seq          kernel.Seq  `json:"seq"`
	Detail       string      `json:"detail"`
	ExpectedPrev kernel.Hash `json:"expected_prev,omitempty"`
	GotPrev      kernel.Hash `json:"got_prev,omitempty"`
	ExpectedID   kernel.Hash `json:"expected_id,omitempty"`
	GotID        kernel.Hash `json:"got_id,omitempty"`
	// ANCHOR_MISMATCH facts (expected = derived from own state, got =
	// the anchor's claim).
	ExpectedRoot    kernel.Hash `json:"expected_root,omitempty"`
	GotRoot         kernel.Hash `json:"got_root,omitempty"`
	ExpectedBaseSeq kernel.Seq  `json:"expected_base_seq,omitempty"`
	GotBaseSeq      kernel.Seq  `json:"got_base_seq,omitempty"`
	ExpectedFirstID kernel.Hash `json:"expected_first_id,omitempty"`
	GotFirstID      kernel.Hash `json:"got_first_id,omitempty"`
}

type reducer struct{}

// New returns the reducer. It is stateless: state is threaded by the
// fold engine.
func New() kernel.Reducer { return reducer{} }

// Registration is the canonical registration: owner scope (it holds
// per-run heads across the whole stream, including the run_id ""
// owner chain). Handles declares the ONE payload this reducer parses
// — anchor.appended v1 — as a version gate (never a filter): every
// other type is still delivered and threads the chain untouched.
func Registration() fold.Registration {
	return fold.Registration{
		ID:      PluginID,
		Semver:  Semver,
		Scope:   fold.ScopeOwner,
		Handles: map[string]uint16{kernel.AnchorEventType: 1},
		Reducer: reducer{},
	}
}

func (reducer) Init() json.RawMessage {
	return json.RawMessage(`{"alarms":[],"base":{"event_id":"","seq":0},"broken":{},"heads":{}}`)
}

func (reducer) Apply(st json.RawMessage, e kernel.Event) (json.RawMessage, error) {
	var s state
	if err := json.Unmarshal(st, &s); err != nil {
		return nil, err
	}
	// Container base, recorded AS-OBSERVED (the stored event_id) from
	// the first event this view ever folds, BEFORE any checks
	// (ADR-0024, owner R1). A broken-run pass-through cannot lose the
	// recording: Broken non-empty implies an earlier event already
	// recorded the base.
	if s.Base.Seq == 0 && s.Base.EventID == "" {
		s.Base = containerBase{EventID: e.EventID, Seq: e.Seq}
	}
	run := string(e.RunID)
	if s.Broken[run] {
		// Report once, never repair (docs/errors.md): a broken run's
		// later linkage is meaningless. Byte-identical pass-through.
		return st, nil
	}
	broke := func(a alarm) (json.RawMessage, error) {
		a.Code = CodeChainBroken
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
	if e.Type == kernel.AnchorEventType {
		// Content check runs only after the envelope verified, against
		// the verifier's OWN pre-anchor state (heads not yet threaded
		// with the anchor itself — mirroring the store's snapshot).
		s.verifyAnchor(e)
	}
	s.Heads[e.RunID] = e.EventID
	return json.Marshal(s)
}

// verifyAnchor checks an anchor.appended v1 claim against the
// verifier's own derived state (ADR-0024): schema-required gate
// first (anchor_payload), then the Merkle root recomputed from OWN
// heads (anchor_root), then the ADR-0022 A1 base attestation against
// the OBSERVED base (anchor_base). Each failed check appends one
// ANCHOR_MISMATCH alarm; nothing freezes — the anchor is a claim
// about the past, not a gate on the future, so Broken is untouched
// and the caller threads the anchor into heads regardless.
func (s *state) verifyAnchor(e kernel.Event) {
	mismatch := func(a alarm) {
		a.Code = CodeAnchorMismatch
		a.RunID = string(e.RunID)
		a.Seq = e.Seq
		s.Alarms = append(s.Alarms, a)
	}
	var p kernel.AnchorPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		// A1: structural failure; error text never enters hashed state.
		mismatch(alarm{Detail: DetailAnchorPayload})
		return
	}
	if len(p.Heads) == 0 || p.MerkleRoot == "" || p.Container.BaseSeq == 0 || p.Container.FirstEventID == "" {
		mismatch(alarm{Detail: DetailAnchorPayload})
		return
	}
	derived, err := kernel.AnchorRoot(s.Heads)
	if err != nil {
		mismatch(alarm{Detail: DetailAnchorPayload})
		return
	}
	if derived != p.MerkleRoot {
		mismatch(alarm{Detail: DetailAnchorRoot, ExpectedRoot: derived, GotRoot: p.MerkleRoot})
	}
	if p.Container.BaseSeq != s.Base.Seq || p.Container.FirstEventID != s.Base.EventID {
		mismatch(alarm{Detail: DetailAnchorBase,
			ExpectedBaseSeq: s.Base.Seq, GotBaseSeq: p.Container.BaseSeq,
			ExpectedFirstID: s.Base.EventID, GotFirstID: p.Container.FirstEventID})
	}
}

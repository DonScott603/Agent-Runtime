// chainverify reducer tests (WP-05a): per-run heads, CHAIN_BROKEN as
// an alarm value (report once, freeze the run, never repair, never
// halt the fold), the A1 fixed-vocabulary alarm shape, and totality
// dogfood (unknown types thread the chain).
package chainverify_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
	"github.com/DonScott603/Agent-Runtime/kernel/fold"
	"github.com/DonScott603/Agent-Runtime/plugins/chainverify"
)

// cvState mirrors the reducer's state shape for assertions.
type cvState struct {
	Alarms []cvAlarm              `json:"alarms"`
	Base   cvBase                 `json:"base"`
	Broken map[string]bool        `json:"broken"`
	Heads  map[string]kernel.Hash `json:"heads"`
}

type cvBase struct {
	EventID kernel.Hash `json:"event_id"`
	Seq     kernel.Seq  `json:"seq"`
}

type cvAlarm struct {
	Code         string      `json:"code"`
	RunID        string      `json:"run_id"`
	Seq          kernel.Seq  `json:"seq"`
	Detail       string      `json:"detail"`
	ExpectedPrev kernel.Hash `json:"expected_prev"`
	GotPrev      kernel.Hash `json:"got_prev"`
	ExpectedID   kernel.Hash `json:"expected_id"`
	GotID        kernel.Hash `json:"got_id"`

	ExpectedRoot    kernel.Hash `json:"expected_root"`
	GotRoot         kernel.Hash `json:"got_root"`
	ExpectedBaseSeq kernel.Seq  `json:"expected_base_seq"`
	GotBaseSeq      kernel.Seq  `json:"got_base_seq"`
	ExpectedFirstID kernel.Hash `json:"expected_first_id"`
	GotFirstID      kernel.Hash `json:"got_first_id"`
}

// sealedChain builds a properly threaded, sealed chain for one run:
// n events with the given types (padded with "t.step"), seq starting
// at base.
func sealedChain(t testing.TB, run kernel.RunID, base kernel.Seq, n int, types ...string) []kernel.Event {
	t.Helper()
	events := make([]kernel.Event, 0, n)
	prev := kernel.ZeroHash
	for i := range n {
		typ := "t.step"
		if i < len(types) {
			typ = types[i]
		}
		e := kernel.Event{
			Seq: base + kernel.Seq(i), RunID: run, Principal: "owner",
			Type: typ, TypeVersion: 1,
			TS: 1751790000 + int64(i), Mono: uint64(i + 1),
			Payload:  json.RawMessage(fmt.Sprintf(`{"i":%d}`, i)),
			PrevHash: prev,
		}
		sealed, err := kernel.SealEvent(e)
		if err != nil {
			t.Fatalf("SealEvent: %v", err)
		}
		events = append(events, sealed)
		prev = sealed.EventID
	}
	return events
}

// interleave merges chains in seq order (assumes disjoint seqs).
func interleave(chains ...[]kernel.Event) []kernel.Event {
	var all []kernel.Event
	for _, c := range chains {
		all = append(all, c...)
	}
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].Seq < all[j-1].Seq; j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
	return all
}

func applyAll(t testing.TB, events []kernel.Event) json.RawMessage {
	t.Helper()
	r := chainverify.New()
	state := r.Init()
	for _, e := range events {
		next, err := r.Apply(state, e)
		if err != nil {
			t.Fatalf("Apply(seq %d): %v", e.Seq, err)
		}
		state = next
	}
	return state
}

func parse(t testing.TB, state json.RawMessage) cvState {
	t.Helper()
	var s cvState
	if err := json.Unmarshal(state, &s); err != nil {
		t.Fatalf("state does not parse: %v (%s)", err, state)
	}
	return s
}

func TestHappyInterleavedChains(t *testing.T) {
	a := sealedChain(t, "run_a", 1, 3)
	b := sealedChain(t, "run_b", 4, 2)
	s := parse(t, applyAll(t, interleave(a, b)))
	if len(s.Alarms) != 0 || len(s.Broken) != 0 {
		t.Fatalf("healthy chains raised alarms: %+v", s)
	}
	if s.Heads["run_a"] != a[len(a)-1].EventID {
		t.Errorf("run_a head %s, want %s", s.Heads["run_a"], a[len(a)-1].EventID)
	}
	if s.Heads["run_b"] != b[len(b)-1].EventID {
		t.Errorf("run_b head %s, want %s", s.Heads["run_b"], b[len(b)-1].EventID)
	}
}

// The run_id "" owner-scope chain is its own chain, exactly as the
// store threads it (store.go: each distinct run_id keys its own
// chain).
func TestOwnerScopeChain(t *testing.T) {
	c := sealedChain(t, "", 1, 2)
	s := parse(t, applyAll(t, c))
	if len(s.Alarms) != 0 {
		t.Fatalf("owner chain raised alarms: %+v", s.Alarms)
	}
	if s.Heads[""] != c[1].EventID {
		t.Errorf(`heads[""] = %s, want %s`, s.Heads[""], c[1].EventID)
	}
}

// Unknown event types thread the chain normally — the totality
// dogfood. (anchor.appended left this club in 0.2.0: it is now the
// one type whose payload this reducer parses; a malformed anchor
// still THREADS — see TestAnchorPayloadMalformed — it just also
// alarms on content.)
func TestUnknownTypesThreadChain(t *testing.T) {
	c := sealedChain(t, "run_a", 1, 3, "run.created", "utterly.unknown", "ext.vendor.custom")
	s := parse(t, applyAll(t, c))
	if len(s.Alarms) != 0 {
		t.Fatalf("unknown types raised alarms: %+v", s.Alarms)
	}
	if s.Heads["run_a"] != c[2].EventID {
		t.Errorf("head did not advance over unknown types: %s, want %s", s.Heads["run_a"], c[2].EventID)
	}
}

func TestTamperedEventID(t *testing.T) {
	a := sealedChain(t, "run_a", 1, 4)
	b := sealedChain(t, "run_b", 5, 2)
	tampered := flipHex(a[2].EventID)
	a[2].EventID = tampered
	s := parse(t, applyAll(t, interleave(a, b)))

	if len(s.Alarms) != 1 {
		t.Fatalf("want exactly one alarm (report once, then freeze), got %d: %+v", len(s.Alarms), s.Alarms)
	}
	al := s.Alarms[0]
	if al.Code != "CHAIN_BROKEN" || al.RunID != "run_a" || al.Seq != a[2].Seq {
		t.Errorf("alarm = %+v, want CHAIN_BROKEN run_a seq %d", al, a[2].Seq)
	}
	if al.Detail != chainverify.DetailIdentity {
		t.Errorf("detail %q, want %q", al.Detail, chainverify.DetailIdentity)
	}
	if al.GotID != tampered || al.ExpectedID == "" || al.ExpectedID == tampered {
		t.Errorf("structured fields wrong: expected_id=%s got_id=%s", al.ExpectedID, al.GotID)
	}
	if !s.Broken["run_a"] {
		t.Error("run_a not marked broken")
	}
	// Head frozen at the last GOOD event; later run_a events skipped.
	if s.Heads["run_a"] != a[1].EventID {
		t.Errorf("head not frozen at last good: %s, want %s", s.Heads["run_a"], a[1].EventID)
	}
	// The sibling run keeps verifying.
	if s.Heads["run_b"] != b[1].EventID {
		t.Errorf("run_b affected by run_a's break: head %s, want %s", s.Heads["run_b"], b[1].EventID)
	}
}

func TestBrokenLinkage(t *testing.T) {
	c := sealedChain(t, "run_a", 1, 3)
	goodPrev := c[1].PrevHash
	c[1].PrevHash = flipHex(goodPrev)
	// Re-seal so the tampered event's identity is self-consistent:
	// the break is pure linkage.
	resealed, err := kernel.SealEvent(c[1])
	if err != nil {
		t.Fatalf("SealEvent: %v", err)
	}
	c[1] = resealed
	s := parse(t, applyAll(t, c))
	if len(s.Alarms) != 1 {
		t.Fatalf("want one alarm, got %+v", s.Alarms)
	}
	al := s.Alarms[0]
	if al.Detail != chainverify.DetailLinkage || al.Seq != c[1].Seq {
		t.Errorf("alarm %+v, want linkage at seq %d", al, c[1].Seq)
	}
	if al.ExpectedPrev != goodPrev || al.GotPrev != c[1].PrevHash {
		t.Errorf("structured fields wrong: expected_prev=%s got_prev=%s", al.ExpectedPrev, al.GotPrev)
	}
}

func TestGenesisViolation(t *testing.T) {
	c := sealedChain(t, "run_a", 1, 1)
	c[0].PrevHash = flipHex(kernel.ZeroHash)
	resealed, err := kernel.SealEvent(c[0])
	if err != nil {
		t.Fatalf("SealEvent: %v", err)
	}
	c[0] = resealed
	s := parse(t, applyAll(t, c))
	if len(s.Alarms) != 1 || s.Alarms[0].Detail != chainverify.DetailGenesis {
		t.Fatalf("want one genesis alarm, got %+v", s.Alarms)
	}
	if s.Alarms[0].ExpectedPrev != kernel.ZeroHash {
		t.Errorf("expected_prev %s, want the zero hash", s.Alarms[0].ExpectedPrev)
	}
}

// A1 (owner amendment): alarm strings are a fixed vocabulary with
// structured fields — never err.Error() passthrough or library text.
// The underivable case is where an error string would be tempting;
// assert the FULL state bytes so nothing can sneak in.
func TestUnderivableAlarmBytesAreStructuredFactsOnly(t *testing.T) {
	e := kernel.Event{
		Seq: 7, RunID: "run_x", Principal: "owner",
		Type: "t.bad", TypeVersion: 1,
		PrevHash: kernel.ZeroHash,
		Payload:  json.RawMessage(`{"spend":1.5}`), // float: unsealable (RFC-0001 D2)
	}
	r := chainverify.New()
	state, err := r.Apply(r.Init(), e)
	if err != nil {
		t.Fatalf("Apply must alarm, not error: %v", err)
	}
	// base is recorded AS-OBSERVED from the first event seen — here the
	// unsealable event itself, whose stored event_id is empty (ADR-0024,
	// owner R1).
	want := `{"alarms":[{"code":"CHAIN_BROKEN","run_id":"run_x","seq":7,"detail":"underivable"}],"base":{"event_id":"","seq":7},"broken":{"run_x":true},"heads":{}}`
	if !bytes.Equal(state, []byte(want)) {
		t.Errorf("alarm bytes are not the fixed-vocabulary shape\n got: %s\nwant: %s", state, want)
	}
}

func TestDeterminism(t *testing.T) {
	a := sealedChain(t, "run_a", 1, 3)
	a[1].EventID = flipHex(a[1].EventID) // include a failure path
	events := interleave(a, sealedChain(t, "run_b", 4, 2))
	x := applyAll(t, events)
	y := applyAll(t, events)
	if !bytes.Equal(x, y) {
		t.Fatalf("two identical folds produced different bytes\n a: %s\n b: %s", x, y)
	}
}

func TestRegistrationShape(t *testing.T) {
	r := chainverify.Registration()
	if r.ID != chainverify.PluginID || r.Semver != chainverify.Semver {
		t.Errorf("registration identity %s@%s, want %s@%s", r.ID, r.Semver, chainverify.PluginID, chainverify.Semver)
	}
	if r.Scope != fold.ScopeOwner {
		t.Errorf("scope %q, want owner", r.Scope)
	}
	if len(r.Handles) != 1 || r.Handles[kernel.AnchorEventType] != 1 {
		t.Errorf("Handles must declare exactly anchor.appended v1 (the one payload this reducer parses; ADR-0024), got %v", r.Handles)
	}
	if _, err := fold.NewRegistry(r); err != nil {
		t.Errorf("registration rejected by NewRegistry: %v", err)
	}
}

// flipHex returns h with its first character replaced by a different
// hex digit.
func flipHex(h kernel.Hash) kernel.Hash {
	if len(h) == 0 {
		return "0"
	}
	c := byte('0')
	if h[0] == '0' {
		c = 'f'
	}
	return kernel.Hash(string(c) + h[1:])
}

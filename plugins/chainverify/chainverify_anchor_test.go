// Anchor verification tests (WP-04c; ADR-0024): the reducer
// recomputes the Merkle root from its OWN held heads, checks the
// ADR-0022 A1 base attestation against its own observed base, and on
// mismatch raises ANCHOR_MISMATCH — which impeaches the anchor, not
// the chain: Broken stays untouched and heads keep advancing.
package chainverify_test

import (
	"encoding/json"
	"errors"
	"maps"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
	"github.com/DonScott603/Agent-Runtime/kernel/fold"
	"github.com/DonScott603/Agent-Runtime/plugins/chainverify"
)

// buildAnchor seals an anchor.appended v1 event threaded after prev
// on the given run, with an owner-shaped envelope. mutate edits the
// payload before sealing (nil for a faithful anchor).
func buildAnchor(t testing.TB, run kernel.RunID, seq kernel.Seq, prev kernel.Hash,
	heads map[kernel.RunID]kernel.Hash, baseSeq kernel.Seq, baseID kernel.Hash,
	mutate func(*kernel.AnchorPayload)) kernel.Event {
	t.Helper()
	root, err := kernel.AnchorRoot(heads)
	if err != nil {
		t.Fatalf("AnchorRoot: %v", err)
	}
	p := kernel.AnchorPayload{
		Container:  kernel.AnchorContainer{BaseSeq: baseSeq, FirstEventID: baseID},
		Heads:      heads,
		MerkleRoot: root,
	}
	if mutate != nil {
		mutate(&p)
	}
	b, err := kernel.Canonical(p)
	if err != nil {
		t.Fatalf("Canonical(payload): %v", err)
	}
	e := kernel.Event{
		Seq: seq, RunID: run, PrevHash: prev,
		TS: 1751795000 + int64(seq), Mono: uint64(seq),
		Principal: "service:kernel", Type: kernel.AnchorEventType, TypeVersion: 1,
		Payload: json.RawMessage(b),
	}
	sealed, err := kernel.SealEvent(e)
	if err != nil {
		t.Fatalf("SealEvent(anchor): %v", err)
	}
	return sealed
}

// anchorScenario: run_a (2 events), run_b (2), one owner-scope event,
// an anchor built by mutate over the true state at that point, and
// run_a's post-anchor continuation at seq 7. Global seqs 1..7 in feed
// order.
func anchorScenario(t testing.TB, mutate func(*kernel.AnchorPayload)) (pre []kernel.Event, anchor, after kernel.Event) {
	t.Helper()
	a := sealedChain(t, "run_a", 1, 2)
	b := sealedChain(t, "run_b", 3, 2)
	o := sealedChain(t, "", 5, 1)
	pre = append(pre, a[0], a[1], b[0], b[1], o[0])
	heads := map[kernel.RunID]kernel.Hash{
		"run_a": a[1].EventID,
		"run_b": b[1].EventID,
		"":      o[0].EventID,
	}
	anchor = buildAnchor(t, "", 6, o[0].EventID, heads, 1, a[0].EventID, mutate)
	cont := kernel.Event{
		Seq: 7, RunID: "run_a", PrevHash: a[1].EventID,
		TS: 1751795300, Mono: 7,
		Principal: "owner", Type: "t.step", TypeVersion: 1,
		Payload: json.RawMessage(`{"i":2}`),
	}
	sealed, err := kernel.SealEvent(cont)
	if err != nil {
		t.Fatalf("SealEvent(continuation): %v", err)
	}
	return pre, anchor, sealed
}

func applyScenario(t testing.TB, pre []kernel.Event, anchor, after kernel.Event) cvState {
	t.Helper()
	events := append(append([]kernel.Event{}, pre...), anchor, after)
	return parse(t, applyAll(t, events))
}

// A faithful anchor raises nothing; heads advance over and past it,
// and the "" head becomes the anchor itself.
func TestAnchorHappyPath(t *testing.T) {
	pre, anchor, after := anchorScenario(t, nil)
	s := applyScenario(t, pre, anchor, after)
	if len(s.Alarms) != 0 || len(s.Broken) != 0 {
		t.Fatalf("faithful anchor raised alarms: %+v", s)
	}
	if s.Heads[""] != anchor.EventID {
		t.Errorf(`heads[""] = %s, want the anchor %s`, s.Heads[""], anchor.EventID)
	}
	if s.Heads["run_a"] != after.EventID {
		t.Errorf("run_a head did not advance past the anchor: %s, want %s", s.Heads["run_a"], after.EventID)
	}
	if s.Base.Seq != 1 {
		t.Errorf("observed base seq %d, want 1", s.Base.Seq)
	}
}

// A root claim over tampered heads: exactly one ANCHOR_MISMATCH/
// anchor_root with expected (derived) and got (claimed) roots; Broken
// untouched; heads keep advancing — the anchor is impeached, not the
// chain.
func TestAnchorRootMismatchFreezesNothing(t *testing.T) {
	var claimed kernel.Hash
	pre, anchor, after := anchorScenario(t, func(p *kernel.AnchorPayload) {
		tampered := make(map[kernel.RunID]kernel.Hash, len(p.Heads))
		maps.Copy(tampered, p.Heads)
		tampered["run_a"] = flipHex(tampered["run_a"])
		root, err := kernel.AnchorRoot(tampered)
		if err != nil {
			t.Fatalf("AnchorRoot(tampered): %v", err)
		}
		p.Heads = tampered
		p.MerkleRoot = root
		claimed = root
	})
	trueRoot, err := kernel.AnchorRoot(map[kernel.RunID]kernel.Hash{
		"run_a": pre[1].EventID, "run_b": pre[3].EventID, "": pre[4].EventID,
	})
	if err != nil {
		t.Fatalf("AnchorRoot: %v", err)
	}
	s := applyScenario(t, pre, anchor, after)
	if len(s.Alarms) != 1 {
		t.Fatalf("want exactly one alarm, got %+v", s.Alarms)
	}
	al := s.Alarms[0]
	if al.Code != chainverify.CodeAnchorMismatch || al.Detail != chainverify.DetailAnchorRoot ||
		al.RunID != "" || al.Seq != anchor.Seq {
		t.Errorf("alarm = %+v, want ANCHOR_MISMATCH/anchor_root at the anchor", al)
	}
	if al.ExpectedRoot != trueRoot || al.GotRoot != claimed {
		t.Errorf("structured fields: expected_root=%s got_root=%s, want %s / %s",
			al.ExpectedRoot, al.GotRoot, trueRoot, claimed)
	}
	if len(s.Broken) != 0 {
		t.Fatalf("anchor alarm froze a run: %v", s.Broken)
	}
	if s.Heads[""] != anchor.EventID || s.Heads["run_a"] != after.EventID {
		t.Errorf("heads stopped advancing after an anchor alarm: %+v", s.Heads)
	}
}

// Wrong base seq and wrong first event_id each raise exactly one
// anchor_base alarm carrying the full expected/got attestation.
func TestAnchorBaseMismatch(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*kernel.AnchorPayload)
	}{
		{"wrong-base-seq", func(p *kernel.AnchorPayload) { p.Container.BaseSeq = 2 }},
		{"wrong-first-event-id", func(p *kernel.AnchorPayload) {
			p.Container.FirstEventID = flipHex(p.Container.FirstEventID)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pre, anchor, after := anchorScenario(t, tc.mutate)
			s := applyScenario(t, pre, anchor, after)
			if len(s.Alarms) != 1 {
				t.Fatalf("want exactly one alarm, got %+v", s.Alarms)
			}
			al := s.Alarms[0]
			if al.Code != chainverify.CodeAnchorMismatch || al.Detail != chainverify.DetailAnchorBase {
				t.Fatalf("alarm = %+v, want ANCHOR_MISMATCH/anchor_base", al)
			}
			if al.ExpectedBaseSeq != 1 || al.ExpectedFirstID != pre[0].EventID {
				t.Errorf("expected side wrong: %+v (want seq 1, id %s)", al, pre[0].EventID)
			}
			if tc.name == "wrong-base-seq" && al.GotBaseSeq != 2 {
				t.Errorf("got_base_seq = %d, want 2", al.GotBaseSeq)
			}
			if len(s.Broken) != 0 || s.Heads["run_a"] != after.EventID {
				t.Errorf("anchor_base froze or stalled the fold: broken=%v heads=%v", s.Broken, s.Heads)
			}
		})
	}
}

// An anchor as the container's FIRST event: base is observed from the
// anchor itself, and the payload cannot attest the anchor's own
// event_id (a sha256 fixpoint), so anchor_base always fires — the
// ADR-0024 corollary of the empty-store refusal. The claimed root is
// the empty-set root so the root check passes and the alarm is
// exactly the base one.
func TestAnchorAsFirstEventAlarmsBase(t *testing.T) {
	emptyRoot, err := kernel.AnchorRoot(map[kernel.RunID]kernel.Hash{})
	if err != nil {
		t.Fatalf("AnchorRoot({}): %v", err)
	}
	anchor := buildAnchor(t, "", 1, kernel.ZeroHash,
		map[kernel.RunID]kernel.Hash{"run_x": "1111111111111111111111111111111111111111111111111111111111111111"},
		1, "2222222222222222222222222222222222222222222222222222222222222222",
		func(p *kernel.AnchorPayload) { p.MerkleRoot = emptyRoot })
	s := parse(t, applyAll(t, []kernel.Event{anchor}))
	if len(s.Alarms) != 1 {
		t.Fatalf("want exactly one alarm, got %+v", s.Alarms)
	}
	al := s.Alarms[0]
	if al.Code != chainverify.CodeAnchorMismatch || al.Detail != chainverify.DetailAnchorBase {
		t.Fatalf("alarm = %+v, want anchor_base", al)
	}
	if al.ExpectedFirstID != anchor.EventID || al.ExpectedBaseSeq != 1 {
		t.Errorf("expected side must be the anchor itself (as-observed base): %+v", al)
	}
	if s.Heads[""] != anchor.EventID {
		t.Errorf("anchor did not thread its chain: %+v", s.Heads)
	}
}

// The schema-required gate: malformed payloads are anchor_payload,
// never anchor_root/anchor_base (ADR-0024 — {"heads":null} must not
// masquerade as a root mismatch via the empty-set root).
func TestAnchorPayloadMalformed(t *testing.T) {
	payloads := []struct {
		name    string
		payload string
	}{
		{"heads-null", `{"heads":null,"merkle_root":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","container":{"base_seq":1,"first_event_id":"1111111111111111111111111111111111111111111111111111111111111111"}}`},
		{"empty-object", `{}`},
		{"zero-container", `{"heads":{"run_a":"1111111111111111111111111111111111111111111111111111111111111111"},"merkle_root":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","container":{"base_seq":0,"first_event_id":""}}`},
		{"missing-root", `{"heads":{"run_a":"1111111111111111111111111111111111111111111111111111111111111111"},"container":{"base_seq":1,"first_event_id":"1111111111111111111111111111111111111111111111111111111111111111"}}`},
	}
	for _, tc := range payloads {
		t.Run(tc.name, func(t *testing.T) {
			base := sealedChain(t, "run_a", 1, 1)
			e := kernel.Event{
				Seq: 2, RunID: "", PrevHash: kernel.ZeroHash,
				TS: 1751795100, Mono: 2,
				Principal: "service:kernel", Type: kernel.AnchorEventType, TypeVersion: 1,
				Payload: json.RawMessage(tc.payload),
			}
			anchor, err := kernel.SealEvent(e)
			if err != nil {
				t.Fatalf("SealEvent: %v", err)
			}
			s := parse(t, applyAll(t, []kernel.Event{base[0], anchor}))
			if len(s.Alarms) != 1 {
				t.Fatalf("want exactly one alarm, got %+v", s.Alarms)
			}
			if s.Alarms[0].Detail != chainverify.DetailAnchorPayload {
				t.Errorf("detail = %q, want anchor_payload (gate fires first)", s.Alarms[0].Detail)
			}
			if s.Heads[""] != anchor.EventID {
				t.Errorf("malformed anchor did not thread its chain: %+v", s.Heads)
			}
		})
	}
}

// A hand-placed anchor INSIDE a run is content-checked identically —
// the check is uniform over run_id (ADR-0024).
func TestAnchorInsideRunCheckedUniformly(t *testing.T) {
	a := sealedChain(t, "run_a", 1, 2)
	heads := map[kernel.RunID]kernel.Hash{"run_a": a[1].EventID}
	anchor := buildAnchor(t, "run_a", 3, a[1].EventID, heads, 1, a[0].EventID,
		func(p *kernel.AnchorPayload) { p.MerkleRoot = flipHex(p.MerkleRoot) })
	s := parse(t, applyAll(t, []kernel.Event{a[0], a[1], anchor}))
	if len(s.Alarms) != 1 || s.Alarms[0].Detail != chainverify.DetailAnchorRoot {
		t.Fatalf("want one anchor_root alarm, got %+v", s.Alarms)
	}
	if s.Alarms[0].RunID != "run_a" {
		t.Errorf("alarm run_id %q, want run_a", s.Alarms[0].RunID)
	}
	if s.Heads["run_a"] != anchor.EventID {
		t.Errorf("anchor did not thread run_a's chain: %+v", s.Heads)
	}
}

// Determinism through the anchor paths (root CLAUDE.md law): a fold
// containing a faithful anchor AND an impeached one produces
// byte-identical state twice.
func TestAnchorDeterminism(t *testing.T) {
	pre, good, after := anchorScenario(t, nil)
	badHeads := map[kernel.RunID]kernel.Hash{
		"run_a": after.EventID, "run_b": pre[3].EventID, "": good.EventID,
	}
	bad := buildAnchor(t, "", 8, good.EventID, badHeads, 1, pre[0].EventID,
		func(p *kernel.AnchorPayload) { p.MerkleRoot = flipHex(p.MerkleRoot) })
	events := append(append([]kernel.Event{}, pre...), good, after, bad)
	x := applyAll(t, events)
	y := applyAll(t, events)
	if string(x) != string(y) {
		t.Fatalf("two identical folds produced different bytes\n a: %s\n b: %s", x, y)
	}
}

// A future anchor version hits the Handles version gate: the whole
// view goes honestly unavailable (SCHEMA_UNKNOWN_VERSION) instead of
// raising false anchor alarms from a misparsed payload (ADR-0024,
// owner R2).
func TestAnchorTypeVersionGate(t *testing.T) {
	reg, err := fold.NewRegistry(chainverify.Registration())
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	f := fold.New(reg)
	e := kernel.Event{
		Seq: 1, RunID: "", PrevHash: kernel.ZeroHash,
		TS: 1751795200, Mono: 1,
		Principal: "service:kernel", Type: kernel.AnchorEventType, TypeVersion: 2,
		Payload: json.RawMessage(`{"a_v2_shape":true}`),
	}
	sealed, err := kernel.SealEvent(e)
	if err != nil {
		t.Fatalf("SealEvent: %v", err)
	}
	if err := f.Step(sealed); err != nil {
		t.Fatalf("Step: %v", err)
	}
	_, err = f.State(chainverify.PluginID, "")
	var ve *fold.ViewError
	if !errors.As(err, &ve) {
		t.Fatalf("want *ViewError, got %v", err)
	}
	if ve.Code != fold.CodeSchemaUnknownVersion {
		t.Errorf("code %q, want SCHEMA_UNKNOWN_VERSION", ve.Code)
	}
}

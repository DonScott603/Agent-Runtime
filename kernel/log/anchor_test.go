// WriteAnchor suite (WP-04c; ADR-0022 A1; ADR-0024): envelope shape,
// payload correctness against the store's own state, base attestation
// through recovery (trace fixtures at base 101), tamper sensitivity
// (the workplan's named verify), empty-store refusal without
// poisoning, and anchors surviving reopen as ordinary events.
package log

import (
	"encoding/json"
	"maps"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

func anchorPayload(t testing.TB, e kernel.Event) kernel.AnchorPayload {
	t.Helper()
	var p kernel.AnchorPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		t.Fatalf("anchor payload does not parse: %v", err)
	}
	return p
}

// Refusal on an empty store is a plain error: nothing written, store
// NOT poisoned, and a subsequent Append succeeds (the oversize-record
// precedent; owner ruling at plan approval).
func TestWriteAnchorEmptyStoreRefuses(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if _, err := s.WriteAnchor(1751780000, 1); err == nil {
		t.Fatal("WriteAnchor on an empty store must refuse")
	}
	if got := readStoreFile(t, dir); len(got) != len(logMagic) {
		t.Fatalf("refusal wrote bytes: file is %d bytes, want magic only", len(got))
	}
	sealed, err := s.Append(protoEvent("run_a", 0))
	if err != nil {
		t.Fatalf("Append after refused WriteAnchor: %v (store poisoned?)", err)
	}
	if sealed.Seq != 1 {
		t.Fatalf("first append after refusal got seq %d, want 1", sealed.Seq)
	}
}

// The full envelope + payload contract on a live store with run and
// owner-scope events, then across a reopen (an anchor is just an
// event to recovery).
func TestWriteAnchorEnvelopeAndPayload(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	var first kernel.Event
	appended := map[kernel.RunID]kernel.Hash{}
	for i, run := range []kernel.RunID{"run_a", "run_b", "", "run_a"} {
		sealed, err := s.Append(protoEvent(run, i))
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
		if i == 0 {
			first = sealed
		}
		appended[run] = sealed.EventID
	}
	ownerHead := appended[""]

	anchor, err := s.WriteAnchor(1751780100, 99)
	if err != nil {
		t.Fatalf("WriteAnchor: %v", err)
	}

	// Envelope (ADR-0024 D-4).
	if anchor.Seq != 5 || anchor.RunID != "" || anchor.Type != kernel.AnchorEventType ||
		anchor.TypeVersion != 1 || anchor.Principal != "service:kernel" || anchor.Sig != nil {
		t.Fatalf("anchor envelope wrong: %+v", anchor)
	}
	if anchor.TS != 1751780100 || anchor.Mono != 99 {
		t.Fatalf("caller-supplied ts/mono not honored: ts=%d mono=%d", anchor.TS, anchor.Mono)
	}
	if len(anchor.Blobs) != 0 {
		t.Fatalf("anchor blobs = %v, want empty", anchor.Blobs)
	}
	if anchor.PrevHash != ownerHead {
		t.Fatalf("anchor prev_hash %s does not thread the \"\" chain head %s", anchor.PrevHash, ownerHead)
	}

	// Payload: heads snapshot excludes the anchor itself; the root
	// recomputes; the base attestation names the first event.
	p := anchorPayload(t, anchor)
	if len(p.Heads) != len(appended) {
		t.Fatalf("payload heads %v, want the pre-anchor snapshot %v", p.Heads, appended)
	}
	for run, head := range appended {
		if p.Heads[run] != head {
			t.Errorf("payload heads[%q] = %s, want %s", run, p.Heads[run], head)
		}
	}
	root, err := kernel.AnchorRoot(p.Heads)
	if err != nil {
		t.Fatalf("AnchorRoot over payload heads: %v", err)
	}
	if root != p.MerkleRoot {
		t.Fatalf("merkle_root does not recompute from payload heads\n got: %s\nwant: %s", root, p.MerkleRoot)
	}
	if p.Container.BaseSeq != 1 || p.Container.FirstEventID != first.EventID {
		t.Fatalf("base attestation = %+v, want seq 1 / %s", p.Container, first.EventID)
	}

	// The workplan's named verify: tamper any single head, root flips.
	for run := range p.Heads {
		mutated := make(map[kernel.RunID]kernel.Hash, len(p.Heads))
		maps.Copy(mutated, p.Heads)
		mutated[run] = flipHexTest(mutated[run])
		flipped, err := kernel.AnchorRoot(mutated)
		if err != nil {
			t.Fatalf("AnchorRoot(mutated): %v", err)
		}
		if flipped == p.MerkleRoot {
			t.Errorf("tampering head of run %q did not flip the root", run)
		}
	}

	// An anchor is just an event: reopen recovers it byte-identically
	// and the next append threads after it on the "" chain.
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen with anchor: %v", err)
	}
	defer s2.Close()
	got, err := s2.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("recovered %d events, want 5", len(got))
	}
	assertSameEvents(t, got[4:], []kernel.Event{anchor})

	second, err := s2.WriteAnchor(1751780200, 100)
	if err != nil {
		t.Fatalf("second WriteAnchor: %v", err)
	}
	if second.PrevHash != anchor.EventID {
		t.Fatalf("second anchor prev_hash %s, want the first anchor %s (anchors chain through \"\")", second.PrevHash, anchor.EventID)
	}
	p2 := anchorPayload(t, second)
	if p2.Heads[""] != anchor.EventID {
		t.Fatalf("second anchor heads[\"\"] = %s, want the first anchor %s", p2.Heads[""], anchor.EventID)
	}
}

// A store with no owner-scope events anchors with a genesis "" chain
// and no "" entry in heads.
func TestWriteAnchorGenesisOwnerChain(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if _, err := s.Append(protoEvent("run_a", 0)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	anchor, err := s.WriteAnchor(1751780100, 2)
	if err != nil {
		t.Fatalf("WriteAnchor: %v", err)
	}
	if anchor.PrevHash != kernel.ZeroHash {
		t.Fatalf("anchor prev_hash %s, want genesis (no prior owner-scope event)", anchor.PrevHash)
	}
	p := anchorPayload(t, anchor)
	if _, ok := p.Heads[""]; ok {
		t.Fatal(`payload heads contain "" though no owner-scope event precedes the anchor`)
	}
}

// The ADR-0022 A1 attestation through RECOVERY: the trace fixtures at
// base 101 reopen, and the anchor attests the recovered base, not 1.
func TestWriteAnchorBaseAttestationFromRecovery(t *testing.T) {
	events := loadTraceEvents(t)
	_, store := persistReopen(t, events)
	anchor, err := store.WriteAnchor(1751793800, 50)
	if err != nil {
		t.Fatalf("WriteAnchor: %v", err)
	}
	if anchor.Seq != 121 {
		t.Fatalf("anchor seq %d, want 121 (after the 20 trace events)", anchor.Seq)
	}
	p := anchorPayload(t, anchor)
	if p.Container.BaseSeq != 101 || p.Container.FirstEventID != events[0].EventID {
		t.Fatalf("base attestation %+v, want seq 101 / %s", p.Container, events[0].EventID)
	}
	if len(p.Heads) != 1 || p.Heads["run_0042"] != traceHead {
		t.Fatalf("payload heads %v, want run_0042 at the documented trace head", p.Heads)
	}
}

// flipHexTest returns h with its first character replaced by a
// different hex digit (chainverify_test precedent).
func flipHexTest(h kernel.Hash) kernel.Hash {
	if len(h) == 0 {
		return "0"
	}
	c := byte('0')
	if h[0] == '0' {
		c = 'f'
	}
	return kernel.Hash(string(c) + h[1:])
}

// Round-trip suites over the frozen fixtures (WP-04b): chain.json's
// top-level run, the signed-event-chain mini-run, and the 20-event
// annotated trace all persist and re-read byte-identically, with
// SealEvent re-derivation green on everything read back. Fixtures are
// read from the frozen files, never invented here; a mismatch is a
// finding, not a golden to edit.
package log

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

const (
	chainVectorPath = "../../docs/vectors/chain.json"
	traceFixture    = "../../docs/trace-annotated.md"
	// "Chain head after seq 120" as documented at the end of the trace
	// (same pin as corpus/trace_test.go).
	traceHead   = "36283240ba955ea123c4eb7fe941f07106c027890c019d26d521c50e9529b59a"
	traceEvents = 20 // seq 101..120
)

type chainFixtureRun struct {
	Name         string         `json:"name"`
	Events       []kernel.Event `json:"events"`
	ExpectedHead string         `json:"expected_head"`
}

type chainFixture struct {
	Events       []kernel.Event    `json:"events"`
	Runs         []chainFixtureRun `json:"runs"`
	ExpectedHead string            `json:"expected_head"`
}

func loadChainFixture(t testing.TB) chainFixture {
	t.Helper()
	raw, err := os.ReadFile(chainVectorPath)
	if err != nil {
		t.Fatalf("reading %s: %v", chainVectorPath, err)
	}
	var fx chainFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("parsing %s: %v", chainVectorPath, err)
	}
	if len(fx.Events) == 0 || fx.ExpectedHead == "" {
		t.Fatalf("%s has no top-level run — fixture shape changed?", chainVectorPath)
	}
	return fx
}

var jsonFence = regexp.MustCompile("(?s)```json\\s*(.*?)```")

func loadTraceEvents(t testing.TB) []kernel.Event {
	t.Helper()
	raw, err := os.ReadFile(traceFixture)
	if err != nil {
		t.Fatalf("reading %s: %v", traceFixture, err)
	}
	blocks := jsonFence.FindAllSubmatch(raw, -1)
	if len(blocks) != traceEvents {
		t.Fatalf("extracted %d json blocks, want %d — extraction brittle or trace changed", len(blocks), traceEvents)
	}
	events := make([]kernel.Event, len(blocks))
	for i, m := range blocks {
		if err := json.Unmarshal(m[1], &events[i]); err != nil {
			t.Fatalf("trace block %d does not parse as an Event: %v", i, err)
		}
	}
	return events
}

// persistReopen appendSealeds the fixture events into a fresh store,
// closes it, reopens (recovery runs over the persisted bytes) and
// returns the re-read events.
func persistReopen(t *testing.T, events []kernel.Event) ([]kernel.Event, *Store) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for i, e := range events {
		if err := s.appendSealed(e); err != nil {
			t.Fatalf("appendSealed event %d (seq %d): %v", i, e.Seq, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { s2.Close() })
	got, err := s2.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return got, s2
}

// resealGreen: SealEvent re-derivation green on everything read back.
func resealGreen(t *testing.T, events []kernel.Event) {
	t.Helper()
	for i, e := range events {
		sealed, err := kernel.SealEvent(e)
		if err != nil {
			t.Fatalf("re-seal event %d: %v", i, err)
		}
		if sealed.EventID != e.EventID {
			t.Fatalf("event %d (seq %d): event_id %s does not re-derive (got %s)", i, e.Seq, e.EventID, sealed.EventID)
		}
	}
}

func TestRoundTripChainJSONEvents(t *testing.T) {
	fx := loadChainFixture(t)
	got, _ := persistReopen(t, fx.Events)
	assertSameEvents(t, got, fx.Events)
	resealGreen(t, got)
	if head := got[len(got)-1].EventID; head != fx.ExpectedHead {
		t.Errorf("head after round-trip %s, want %s", head, fx.ExpectedHead)
	}
}

func TestRoundTripSignedRun(t *testing.T) {
	fx := loadChainFixture(t)
	var run *chainFixtureRun
	for i := range fx.Runs {
		if fx.Runs[i].Name == "signed-event-chain" {
			run = &fx.Runs[i]
		}
	}
	if run == nil {
		t.Fatalf("%s runs[] missing signed-event-chain — fixture shape changed?", chainVectorPath)
	}
	got, _ := persistReopen(t, run.Events)
	assertSameEvents(t, got, run.Events)
	resealGreen(t, got)
	// The sig is STORED; zeroing is virtual, for hashing only. It must
	// survive persistence byte-identically.
	sawSig := false
	for i, e := range got {
		if w := run.Events[i]; w.Sig != nil {
			sawSig = true
			if e.Sig == nil || *e.Sig != *w.Sig {
				t.Errorf("event %d: sig did not survive persistence: got %+v, want %+v", i, e.Sig, w.Sig)
			}
		}
	}
	if !sawSig {
		t.Fatal("fixture run has no signed event — wrong fixture?")
	}
	if head := got[len(got)-1].EventID; head != run.ExpectedHead {
		t.Errorf("head after round-trip %s, want %s", head, run.ExpectedHead)
	}
}

// The trace starts at global seq 101: the file-local
// consecutive-from-any-base rule (ADR-0022) is what makes a container
// that begins mid-stream recoverable. A subsequent Append continues
// the global sequence.
func TestRoundTripTraceBase101(t *testing.T) {
	events := loadTraceEvents(t)
	got, s := persistReopen(t, events)
	assertSameEvents(t, got, events)
	resealGreen(t, got)
	if err := VerifyChain(got); err != nil {
		t.Fatalf("trace chain does not verify after round-trip: %v", err)
	}
	if head := got[len(got)-1].EventID; head != traceHead {
		t.Errorf("trace head %s, want %s", head, traceHead)
	}
	next, err := s.Append(protoEvent(events[0].RunID, 21))
	if err != nil {
		t.Fatalf("Append after trace: %v", err)
	}
	if next.Seq != 121 {
		t.Errorf("continuation seq %d, want 121", next.Seq)
	}
	if next.PrevHash != traceHead {
		t.Errorf("continuation prev_hash %s, want trace head", next.PrevHash)
	}
}

// Feeding the fixture events' FIELDS to the exported Append on a
// virgin store must reproduce the vector event_ids and head
// end-to-end: the store's assignment discipline (seq, threading,
// seal) is pinned by the same goldens as the pure layer.
func TestAppendReproducesChainJSONHead(t *testing.T) {
	fx := loadChainFixture(t)
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	for i, w := range fx.Events {
		in := kernel.Event{
			RunID: w.RunID, TS: w.TS, Mono: w.Mono,
			Principal: w.Principal, Type: w.Type, TypeVersion: w.TypeVersion,
			Payload: w.Payload, Blobs: w.Blobs, Sig: w.Sig,
		}
		got, err := s.Append(in)
		if err != nil {
			t.Fatalf("Append event %d: %v", i, err)
		}
		if got.Seq != w.Seq {
			t.Fatalf("event %d: assigned seq %d, want %d", i, got.Seq, w.Seq)
		}
		if got.EventID != w.EventID {
			t.Fatalf("event %d: sealed event_id %s, want %s — assignment discipline diverges from the vectors", i, got.EventID, w.EventID)
		}
	}
}

package kernel_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

// One logical event, one canonical byte form (ADR-0020 Consequences
// note): an Event built with nil Blobs/Payload and one built with
// empty values must canonicalize identically after NormalizeEnvelope.
func TestNormalizeEnvelopeNilVsEmpty(t *testing.T) {
	base := kernel.Event{
		Seq: 1, RunID: "run_0001", PrevHash: kernel.ZeroHash,
		TS: 1751780000, Mono: 1, Principal: "owner",
		Type: "run.created", TypeVersion: 1,
	}
	nilBuilt := base // Blobs and Payload left nil
	emptyBuilt := base
	emptyBuilt.Blobs = []kernel.Hash{}
	emptyBuilt.Payload = json.RawMessage("{}")

	a, err := kernel.Canonical(kernel.NormalizeEnvelope(nilBuilt))
	if err != nil {
		t.Fatal(err)
	}
	b, err := kernel.Canonical(kernel.NormalizeEnvelope(emptyBuilt))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("nil-built and empty-built diverge after normalization:\n  nil: %s\nempty: %s", a, b)
	}

	// Non-vacuousness: without normalization the nil-built form MUST
	// differ (nil serializes as null), or this test proves nothing.
	raw, err := kernel.Canonical(nilBuilt)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(raw, a) {
		t.Fatal("normalization was a no-op on a nil-built event; test is vacuous")
	}
}

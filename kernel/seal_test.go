package kernel_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

func baseEvent() kernel.Event {
	return kernel.Event{
		Seq: 1, RunID: "run_0001", PrevHash: kernel.ZeroHash,
		TS: 1751780000, Mono: 1, Principal: "owner",
		Type: "run.created", TypeVersion: 1,
	}
}

// Determinism across constructions (ADR-0020 Consequences note): an
// event built with nil Blobs/Payload and one built with empty values
// are the same logical event and MUST seal to the same EventID.
func TestSealEventDeterminism(t *testing.T) {
	nilBuilt := baseEvent() // Blobs and Payload left nil
	emptyBuilt := baseEvent()
	emptyBuilt.Blobs = []kernel.Hash{}
	emptyBuilt.Payload = json.RawMessage("{}")

	a, err := kernel.SealEvent(nilBuilt)
	if err != nil {
		t.Fatal(err)
	}
	b, err := kernel.SealEvent(emptyBuilt)
	if err != nil {
		t.Fatal(err)
	}
	if a.EventID == "" {
		t.Fatal("sealed EventID is empty")
	}
	if a.EventID != b.EventID {
		t.Fatalf("nil-built and empty-built seal to different EventIDs:\n  nil: %s\nempty: %s", a.EventID, b.EventID)
	}

	// Invoke twice, compare the whole result (repo determinism rule).
	a2, err := kernel.SealEvent(nilBuilt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, a2) {
		t.Fatalf("sealing the same event twice diverges:\n first: %+v\nsecond: %+v", a, a2)
	}
}

// A non-empty input EventID is ignored: sealing re-derives from
// scratch, and sealing an already-sealed event is a fixpoint.
func TestSealEventReseal(t *testing.T) {
	fresh, err := kernel.SealEvent(baseEvent())
	if err != nil {
		t.Fatal(err)
	}

	stale := baseEvent()
	stale.EventID = "not-a-real-hash"
	resealed, err := kernel.SealEvent(stale)
	if err != nil {
		t.Fatal(err)
	}
	if resealed.EventID != fresh.EventID {
		t.Fatalf("re-seal did not recompute from scratch:\n got: %s\nwant: %s", resealed.EventID, fresh.EventID)
	}

	again, err := kernel.SealEvent(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, fresh) {
		t.Fatalf("sealing a sealed event is not a fixpoint:\n got: %+v\nwant: %+v", again, fresh)
	}
}

// sig is zeroed for hashing (chain.json _rules): a signature never
// changes the EventID, and the returned event keeps it. Validity is
// the fold layer's job (RFC-0002 §5), not the chain's.
func TestSealEventSigOutsideHash(t *testing.T) {
	unsigned, err := kernel.SealEvent(baseEvent())
	if err != nil {
		t.Fatal(err)
	}

	signedIn := baseEvent()
	signedIn.Sig = &kernel.Signature{Alg: "ed25519", KeyID: "owner-k1", Value: "ILLUSTRATIVE"}
	signed, err := kernel.SealEvent(signedIn)
	if err != nil {
		t.Fatal(err)
	}
	if signed.EventID != unsigned.EventID {
		t.Fatalf("sig leaked into the hash:\n  signed: %s\nunsigned: %s", signed.EventID, unsigned.EventID)
	}
	if !reflect.DeepEqual(signed.Sig, signedIn.Sig) {
		t.Fatalf("SealEvent dropped or altered Sig: %+v", signed.Sig)
	}
}

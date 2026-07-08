package log_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
	klog "github.com/DonScott603/Agent-Runtime/kernel/log"
)

// mkChain builds a sealed, correctly threaded n-event chain.
func mkChain(t testing.TB, run kernel.RunID, n int) []kernel.Event {
	t.Helper()
	events := make([]kernel.Event, n)
	for i := range events {
		e := kernel.Event{
			Seq: kernel.Seq(i + 1), RunID: run,
			TS: 1751780000 + int64(i), Mono: uint64(i + 1),
			Principal: "agent:demo", Type: "msg.appended", TypeVersion: 1,
			Payload: json.RawMessage(fmt.Sprintf(`{"n":%d}`, i)),
		}
		if i == 0 {
			e = klog.Genesis(e)
		} else {
			var err error
			e, err = klog.NextInChain(events[i-1], e)
			if err != nil {
				t.Fatalf("NextInChain: %v", err)
			}
		}
		sealed, err := kernel.SealEvent(e)
		if err != nil {
			t.Fatalf("SealEvent: %v", err)
		}
		events[i] = sealed
	}
	return events
}

func TestVerifyChain(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func([]kernel.Event) []kernel.Event
		wantSeq kernel.Seq // 0 = chain must verify
	}{
		{"valid", nil, 0},
		{"empty", func([]kernel.Event) []kernel.Event { return nil }, 0},
		{"single-genesis", func(evs []kernel.Event) []kernel.Event { return evs[:1] }, 0},
		// sig is outside the hash by design: chain verification never
		// judges signature validity — a consent event with a missing or
		// invalid signature is the signature-validation layer's problem
		// (treated as absent by fold, RFC-0002 §5), not a chain break.
		{"sig-mutation-does-not-break-chain", func(evs []kernel.Event) []kernel.Event {
			evs[1].Sig = &kernel.Signature{Alg: "ed25519", KeyID: "owner-k1", Value: "FORGED"}
			return evs
		}, 0},
		{"genesis-prev-hash-not-zero", func(evs []kernel.Event) []kernel.Event {
			evs[0].PrevHash = strings.Repeat("11", 32)
			return evs
		}, 1},
		{"missing-genesis", func(evs []kernel.Event) []kernel.Event { return evs[1:] }, 2},
		{"middle-linkage-broken", func(evs []kernel.Event) []kernel.Event {
			evs[2].PrevHash = strings.Repeat("22", 32)
			return evs
		}, 3},
		{"tampered-event-id", func(evs []kernel.Event) []kernel.Event {
			evs[1].EventID = strings.Repeat("33", 32)
			return evs
		}, 2},
		{"tampered-payload", func(evs []kernel.Event) []kernel.Event {
			evs[1].Payload = json.RawMessage(`{"n":999}`)
			return evs
		}, 2},
		{"run-id-diverges-mid-chain", func(evs []kernel.Event) []kernel.Event {
			evs[2].RunID = "run_other"
			return evs
		}, 3},
		// Underivable identity (amendment: one error type, no raw-error
		// second path): corrupt payload bytes make SealEvent fail, and
		// VerifyChain reports that as CHAIN_BROKEN at the event's seq.
		{"corrupt-payload-bytes", func(evs []kernel.Event) []kernel.Event {
			evs[1].Payload = json.RawMessage(`{"unterminated`)
			return evs
		}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evs := mkChain(t, "run_t", 4)
			if tc.mutate != nil {
				evs = tc.mutate(evs)
			}
			err := klog.VerifyChain(evs)
			if tc.wantSeq == 0 {
				if err != nil {
					t.Fatalf("chain should verify, got: %v", err)
				}
				return
			}
			var cbe *klog.ChainBrokenError
			if !errors.As(err, &cbe) {
				t.Fatalf("want *ChainBrokenError, got %T: %v", err, err)
			}
			if cbe.Seq != tc.wantSeq {
				t.Errorf("first broken seq = %d, want %d (%v)", cbe.Seq, tc.wantSeq, err)
			}
			if !strings.Contains(err.Error(), "CHAIN_BROKEN") {
				t.Errorf("error does not carry the CHAIN_BROKEN code (docs/errors.md): %v", err)
			}
		})
	}
}

func TestGenesisSetsZeroHash(t *testing.T) {
	e := klog.Genesis(kernel.Event{Seq: 1, RunID: "run_t", PrevHash: "leftover"})
	if e.PrevHash != kernel.ZeroHash {
		t.Fatalf("Genesis prev_hash = %q, want kernel.ZeroHash", e.PrevHash)
	}
}

func TestNextInChainRejectsUnsealedPredecessor(t *testing.T) {
	prev := kernel.Event{Seq: 1, RunID: "run_t"} // never sealed: EventID ""
	if _, err := klog.NextInChain(prev, kernel.Event{Seq: 2, RunID: "run_t"}); err == nil {
		t.Fatal("NextInChain accepted an unsealed predecessor")
	}
}

func TestNextInChainThreadsPrevHash(t *testing.T) {
	evs := mkChain(t, "run_t", 2)
	if evs[1].PrevHash != evs[0].EventID {
		t.Fatalf("prev_hash %q, want predecessor event_id %q", evs[1].PrevHash, evs[0].EventID)
	}
}

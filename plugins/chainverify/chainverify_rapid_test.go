// Consistency property (WP-05a): the incremental verifier and the
// batch verifier are the same law. For any single-run chain with at
// most one random mutation, chainverify alarms IFF klog.VerifyChain
// errors, at the same first (run, seq). This test-only kernel/log
// import welds the two verifiers together permanently (no import
// cycle: the plugin itself never imports kernel/log).
package chainverify_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"pgregory.net/rapid"

	"github.com/DonScott603/Agent-Runtime/kernel"
	klog "github.com/DonScott603/Agent-Runtime/kernel/log"
	"github.com/DonScott603/Agent-Runtime/plugins/chainverify"
)

func TestConsistencyWithVerifyChain(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 12).Draw(t, "n")
		events := make([]kernel.Event, 0, n)
		prev := kernel.ZeroHash
		for i := range n {
			e := kernel.Event{
				Seq: kernel.Seq(i + 1), RunID: "run_r", Principal: "owner",
				Type: "t.step", TypeVersion: 1,
				TS: int64(1751790000 + i), Mono: uint64(i + 1),
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

		mutation := rapid.SampledFrom([]string{"none", "event_id", "prev_hash", "payload"}).Draw(t, "mutation")
		idx := rapid.IntRange(0, n-1).Draw(t, "idx")
		switch mutation {
		case "event_id":
			events[idx].EventID = flipHex(events[idx].EventID)
		case "prev_hash":
			events[idx].PrevHash = flipHex(events[idx].PrevHash)
		case "payload":
			events[idx].Payload = json.RawMessage(`{"tampered":true}`)
		}

		batchErr := klog.VerifyChain(events)

		r := chainverify.New()
		state := r.Init()
		for _, e := range events {
			next, err := r.Apply(state, e)
			if err != nil {
				t.Fatalf("Apply seq %d: %v", e.Seq, err)
			}
			state = next
		}
		var s cvState
		if err := json.Unmarshal(state, &s); err != nil {
			t.Fatalf("state does not parse: %v", err)
		}

		if batchErr == nil {
			if len(s.Alarms) != 0 {
				t.Fatalf("VerifyChain green but reducer alarmed: %+v", s.Alarms)
			}
			if s.Heads["run_r"] != events[n-1].EventID {
				t.Fatalf("head %s, want %s", s.Heads["run_r"], events[n-1].EventID)
			}
			return
		}
		var cbe *klog.ChainBrokenError
		if !errors.As(batchErr, &cbe) {
			t.Fatalf("VerifyChain returned non-ChainBrokenError: %v", batchErr)
		}
		if len(s.Alarms) != 1 {
			t.Fatalf("VerifyChain broke at seq %d but reducer raised %d alarms: %+v", cbe.Seq, len(s.Alarms), s.Alarms)
		}
		if s.Alarms[0].Seq != cbe.Seq || kernel.RunID(s.Alarms[0].RunID) != cbe.RunID {
			t.Fatalf("first failure disagrees: reducer (run %q seq %d) vs VerifyChain (run %q seq %d)",
				s.Alarms[0].RunID, s.Alarms[0].Seq, cbe.RunID, cbe.Seq)
		}
	})
}

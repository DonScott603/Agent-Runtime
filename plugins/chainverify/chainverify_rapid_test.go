// Consistency property (WP-05a; anchors WP-04c): the incremental
// verifier and the batch verifier are the same law on chain
// integrity. For any single-run chain — now optionally carrying an
// anchor.appended event (faithful or root-impeached) — with at most
// one random mutation:
//
//   - chainverify raises a CHAIN_BROKEN alarm IFF klog.VerifyChain
//     errors, at the same first (run, seq). Anchor-CONTENT alarms are
//     excluded from the iff: VerifyChain reads only envelopes, so a
//     validly sealed anchor carrying a false claim is invisible to it
//     (totality both directions — the anchor is just another link).
//   - NEGATIVE (owner rider, WP-04c plan approval): an anchor
//     mismatch never surfaces as CHAIN_BROKEN — the two vocabularies
//     never cross codes.
//   - A root-impeached anchor alarms exactly once, IFF the chain was
//     intact up to the anchor (a break at or before it freezes the
//     run first and the content is never checked).
//   - incremental == rebuild through the fold engine stays hash-equal
//     with anchors interleaved.
//
// This test-only kernel/log import welds the two verifiers together
// permanently (no import cycle: the plugin itself never imports
// kernel/log).
package chainverify_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"pgregory.net/rapid"

	"github.com/DonScott603/Agent-Runtime/kernel"
	"github.com/DonScott603/Agent-Runtime/kernel/fold"
	klog "github.com/DonScott603/Agent-Runtime/kernel/log"
	"github.com/DonScott603/Agent-Runtime/plugins/chainverify"
)

var chainDetails = map[string]bool{
	chainverify.DetailGenesis:     true,
	chainverify.DetailLinkage:     true,
	chainverify.DetailIdentity:    true,
	chainverify.DetailUnderivable: true,
}

var anchorDetails = map[string]bool{
	chainverify.DetailAnchorPayload: true,
	chainverify.DetailAnchorRoot:    true,
	chainverify.DetailAnchorBase:    true,
}

func TestConsistencyWithVerifyChain(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 12).Draw(t, "n")
		anchorMode := rapid.SampledFrom([]string{"none", "valid", "wrongRoot"}).Draw(t, "anchorMode")
		anchorIdx := -1
		total := n
		if anchorMode != "none" {
			anchorIdx = rapid.IntRange(1, n).Draw(t, "anchorIdx")
			total = n + 1
		}

		events := make([]kernel.Event, 0, total)
		prev := kernel.ZeroHash
		anchorSeq := kernel.Seq(0)
		for i := range total {
			var e kernel.Event
			if i == anchorIdx {
				heads := map[kernel.RunID]kernel.Hash{"run_r": prev}
				root, err := kernel.AnchorRoot(heads)
				if err != nil {
					t.Fatalf("AnchorRoot: %v", err)
				}
				if anchorMode == "wrongRoot" {
					root = flipHex(root)
				}
				payload, err := kernel.Canonical(kernel.AnchorPayload{
					Container:  kernel.AnchorContainer{BaseSeq: events[0].Seq, FirstEventID: events[0].EventID},
					Heads:      heads,
					MerkleRoot: root,
				})
				if err != nil {
					t.Fatalf("Canonical(payload): %v", err)
				}
				e = kernel.Event{
					Seq: kernel.Seq(i + 1), RunID: "run_r", Principal: "service:kernel",
					Type: kernel.AnchorEventType, TypeVersion: 1,
					TS: int64(1751790000 + i), Mono: uint64(i + 1),
					Payload:  json.RawMessage(payload),
					PrevHash: prev,
				}
				anchorSeq = e.Seq
			} else {
				e = kernel.Event{
					Seq: kernel.Seq(i + 1), RunID: "run_r", Principal: "owner",
					Type: "t.step", TypeVersion: 1,
					TS: int64(1751790000 + i), Mono: uint64(i + 1),
					Payload:  json.RawMessage(fmt.Sprintf(`{"i":%d}`, i)),
					PrevHash: prev,
				}
			}
			sealed, err := kernel.SealEvent(e)
			if err != nil {
				t.Fatalf("SealEvent: %v", err)
			}
			events = append(events, sealed)
			prev = sealed.EventID
		}

		mutation := rapid.SampledFrom([]string{"none", "event_id", "prev_hash", "payload"}).Draw(t, "mutation")
		idx := rapid.IntRange(0, total-1).Draw(t, "idx")
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

		// Split the alarm stream by code; assert the vocabularies
		// never cross (owner rider: anchor mismatches never surface
		// as CHAIN_BROKEN).
		var chainAlarms, anchorAlarms []cvAlarm
		for _, al := range s.Alarms {
			switch al.Code {
			case chainverify.CodeChainBroken:
				if !chainDetails[al.Detail] {
					t.Fatalf("CHAIN_BROKEN carries non-chain detail %q", al.Detail)
				}
				chainAlarms = append(chainAlarms, al)
			case chainverify.CodeAnchorMismatch:
				if !anchorDetails[al.Detail] {
					t.Fatalf("ANCHOR_MISMATCH carries non-anchor detail %q", al.Detail)
				}
				anchorAlarms = append(anchorAlarms, al)
			default:
				t.Fatalf("unknown alarm code %q", al.Code)
			}
		}

		// The chain-integrity iff, unchanged from WP-05a.
		if batchErr == nil {
			if len(chainAlarms) != 0 {
				t.Fatalf("VerifyChain green but reducer raised CHAIN_BROKEN: %+v", chainAlarms)
			}
			if s.Heads["run_r"] != events[total-1].EventID {
				t.Fatalf("head %s, want %s", s.Heads["run_r"], events[total-1].EventID)
			}
		} else {
			var cbe *klog.ChainBrokenError
			if !errors.As(batchErr, &cbe) {
				t.Fatalf("VerifyChain returned non-ChainBrokenError: %v", batchErr)
			}
			if len(chainAlarms) != 1 {
				t.Fatalf("VerifyChain broke at seq %d but reducer raised %d CHAIN_BROKEN alarms: %+v", cbe.Seq, len(chainAlarms), chainAlarms)
			}
			if chainAlarms[0].Seq != cbe.Seq || kernel.RunID(chainAlarms[0].RunID) != cbe.RunID {
				t.Fatalf("first failure disagrees: reducer (run %q seq %d) vs VerifyChain (run %q seq %d)",
					chainAlarms[0].RunID, chainAlarms[0].Seq, cbe.RunID, cbe.Seq)
			}
		}

		// Anchor-content alarms: exactly one anchor_root IFF the
		// anchor was root-impeached AND the chain was intact strictly
		// before it (a break at or before the anchor freezes the run
		// first; the content check never runs).
		expectAnchorAlarm := 0
		if anchorMode == "wrongRoot" {
			reached := batchErr == nil
			if !reached {
				var cbe *klog.ChainBrokenError
				errors.As(batchErr, &cbe)
				reached = anchorSeq < cbe.Seq
			}
			if reached {
				expectAnchorAlarm = 1
			}
		}
		if len(anchorAlarms) != expectAnchorAlarm {
			t.Fatalf("anchorMode=%s mutation=%s idx=%d: %d anchor alarms, want %d: %+v",
				anchorMode, mutation, idx, len(anchorAlarms), expectAnchorAlarm, anchorAlarms)
		}
		if expectAnchorAlarm == 1 && anchorAlarms[0].Detail != chainverify.DetailAnchorRoot {
			t.Fatalf("anchor alarm detail %q, want anchor_root", anchorAlarms[0].Detail)
		}

		// incremental == rebuild through the engine, anchors included.
		reg, err := fold.NewRegistry(chainverify.Registration())
		if err != nil {
			t.Fatalf("NewRegistry: %v", err)
		}
		rebuilt, err := fold.Rebuild(reg, events)
		if err != nil {
			t.Fatalf("Rebuild: %v", err)
		}
		stepped := fold.New(reg)
		for _, e := range events {
			if err := stepped.Step(e); err != nil {
				t.Fatalf("Step seq %d: %v", e.Seq, err)
			}
		}
		h1, err := rebuilt.StateHash(chainverify.PluginID, "")
		if err != nil {
			t.Fatalf("StateHash(rebuild): %v", err)
		}
		h2, err := stepped.StateHash(chainverify.PluginID, "")
		if err != nil {
			t.Fatalf("StateHash(stepped): %v", err)
		}
		if h1 != h2 {
			t.Fatalf("incremental != rebuild with anchors interleaved:\n rebuild:     %s\n incremental: %s", h1, h2)
		}
	})
}

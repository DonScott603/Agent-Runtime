// Property suite (WP-04a; repo convention: rapid): a correctly
// threaded chain verifies, and any single-field mutation at any
// position is rejected with the first broken seq.
package log_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/DonScott603/Agent-Runtime/kernel"
	klog "github.com/DonScott603/Agent-Runtime/kernel/log"
)

func TestChainProperty(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		const run = "run_prop"
		n := rapid.IntRange(1, 12).Draw(rt, "n")

		// Payloads stay in canon-safe territory: ASCII keys/strings,
		// integers only (floats are a canon hard error by design).
		payloadGen := rapid.MapOfN(
			rapid.StringMatching(`[a-z]{1,8}`),
			rapid.OneOf(
				rapid.Int64().AsAny(),
				rapid.StringMatching(`[ -~]{0,12}`).AsAny(),
			),
			0, 4,
		)

		payloads := make([]map[string]any, n)
		events := make([]kernel.Event, n)
		for i := range events {
			payloads[i] = payloadGen.Draw(rt, "payload")
			raw, err := json.Marshal(payloads[i])
			if err != nil {
				rt.Fatalf("marshal payload: %v", err)
			}
			e := kernel.Event{
				Seq:   kernel.Seq(i + 1), // assignment discipline is WP-04b; any values work here
				RunID: run,
				TS:    rapid.Int64Range(0, 1<<40).Draw(rt, "ts"),
				Mono:  uint64(i + 1),
				Principal: rapid.SampledFrom([]kernel.PrincipalID{
					"owner", "agent:demo", "service:kernel",
				}).Draw(rt, "principal"),
				Type: rapid.SampledFrom([]string{
					"run.created", "msg.appended", "effect.proposed",
				}).Draw(rt, "type"),
				TypeVersion: 1,
				Payload:     raw,
			}
			if i == 0 {
				e = klog.Genesis(e)
			} else {
				e, err = klog.NextInChain(events[i-1], e)
				if err != nil {
					rt.Fatalf("NextInChain: %v", err)
				}
			}
			if events[i], err = kernel.SealEvent(e); err != nil {
				rt.Fatalf("SealEvent: %v", err)
			}
		}

		if err := klog.VerifyChain(events); err != nil {
			rt.Fatalf("correctly threaded chain rejected: %v", err)
		}

		// Single-field mutation at a random position. Payload mutation
		// edits the generated map and re-marshals: valid JSON, wrong
		// content — the primary tamper case. (Invalid payload BYTES are
		// the corrupt-payload-bytes table case in chain_test.go.)
		pos := rapid.IntRange(0, n-1).Draw(rt, "pos")
		mutated := append([]kernel.Event(nil), events...)
		ev := mutated[pos]
		switch rapid.SampledFrom([]string{"payload", "ts", "principal", "type", "prev_hash"}).Draw(rt, "field") {
		case "payload":
			m := payloads[pos]
			if m["tamper"] == "x" {
				m["tamper"] = "y"
			} else {
				m["tamper"] = "x"
			}
			raw, err := json.Marshal(m)
			if err != nil {
				rt.Fatalf("marshal mutated payload: %v", err)
			}
			ev.Payload = raw
		case "ts":
			ev.TS++
		case "principal":
			ev.Principal += "x"
		case "type":
			ev.Type += "x"
		case "prev_hash":
			other := strings.Repeat("11", 32)
			if ev.PrevHash == other {
				other = strings.Repeat("22", 32)
			}
			ev.PrevHash = other
		}
		mutated[pos] = ev

		err := klog.VerifyChain(mutated)
		var cbe *klog.ChainBrokenError
		if !errors.As(err, &cbe) {
			rt.Fatalf("mutation at pos %d not rejected as *ChainBrokenError: %v", pos, err)
		}
		if cbe.Seq != events[pos].Seq {
			rt.Fatalf("first broken seq = %d, want %d (%v)", cbe.Seq, events[pos].Seq, err)
		}
		if cbe.RunID != run {
			rt.Fatalf("ChainBrokenError run_id = %q, want %q", cbe.RunID, run)
		}
	})
}

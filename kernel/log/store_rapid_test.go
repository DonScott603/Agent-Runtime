// Model-based property suite (WP-04b; repo convention: rapid): random
// append/crash/recover sequences against an in-memory model built on
// the vector-verified pure layer. Recovery must agree with the model
// exactly — zero committed-event loss, no resurrection of torn ones.
package log

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"pgregory.net/rapid"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

func TestStoreRapid(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dir := t.TempDir()
		var tf *tornFile
		openIt := func() *Store {
			s, err := openStore(dir, func(f *os.File) writeSyncer { tf = &tornFile{f: f}; return tf })
			if err != nil {
				rt.Fatalf("openStore: %v", err)
			}
			return s
		}
		s := openIt()
		defer func() { _ = s.Close() }()

		model := struct {
			lastSeq kernel.Seq
			heads   map[kernel.RunID]kernel.Hash
			events  []kernel.Event
		}{heads: map[kernel.RunID]kernel.Hash{}}

		// Canon-safe payloads (ASCII keys, integers only — floats are
		// a canon hard error by design; same discipline as
		// rapid_test.go).
		payloadGen := rapid.MapOfN(
			rapid.StringMatching(`[a-z]{1,8}`),
			rapid.OneOf(
				rapid.Int64().AsAny(),
				rapid.StringMatching(`[ -~]{0,12}`).AsAny(),
			),
			0, 4,
		)
		genProto := func(i int) kernel.Event {
			raw, err := json.Marshal(payloadGen.Draw(rt, "payload"))
			if err != nil {
				rt.Fatalf("marshal payload: %v", err)
			}
			return kernel.Event{
				RunID: rapid.SampledFrom([]kernel.RunID{"run_a", "run_b", ""}).Draw(rt, "run"),
				TS:    rapid.Int64Range(0, 1<<40).Draw(rt, "ts"),
				Mono:  uint64(i + 1), Principal: "agent:demo",
				Type: "msg.appended", TypeVersion: 1, Payload: raw,
			}
		}
		checkAll := func() {
			got, err := s.ReadAll()
			if err != nil {
				rt.Fatalf("ReadAll: %v", err)
			}
			if len(got) != len(model.events) {
				rt.Fatalf("ReadAll returned %d events, model has %d", len(got), len(model.events))
			}
			for i := range got {
				g, err := kernel.Canonical(got[i])
				if err != nil {
					rt.Fatalf("Canonical(got[%d]): %v", i, err)
				}
				w, err := kernel.Canonical(model.events[i])
				if err != nil {
					rt.Fatalf("Canonical(model[%d]): %v", i, err)
				}
				if !bytes.Equal(g, w) {
					rt.Fatalf("event %d diverges from model\n got: %s\nwant: %s", i, g, w)
				}
			}
		}

		n := rapid.IntRange(1, 20).Draw(rt, "ops")
		for i := 0; i < n; i++ {
			switch rapid.SampledFrom([]string{"append", "append", "append", "crash", "reopen", "readall"}).Draw(rt, "op") {
			case "append":
				proto := genProto(i)
				want, err := threadSeal(model.lastSeq, model.heads, proto)
				if err != nil {
					rt.Fatalf("threadSeal: %v", err)
				}
				got, err := s.Append(proto)
				if err != nil {
					rt.Fatalf("Append: %v", err)
				}
				if got.Seq != want.Seq || got.EventID != want.EventID {
					rt.Fatalf("Append diverges from model: got seq %d id %s, want seq %d id %s",
						got.Seq, got.EventID, want.Seq, want.EventID)
				}
				model.lastSeq = want.Seq
				model.heads[want.RunID] = want.EventID
				model.events = append(model.events, want)
			case "crash":
				proto := genProto(i)
				want, err := threadSeal(model.lastSeq, model.heads, proto)
				if err != nil {
					rt.Fatalf("threadSeal: %v", err)
				}
				mode := rapid.SampledFrom([]string{"zero", "partial", "full-unsynced"}).Draw(rt, "crashmode")
				switch mode {
				case "zero":
					tf.mode, tf.durable = "torn", func(int) int { return 0 }
				case "partial":
					pct := rapid.IntRange(1, 99).Draw(rt, "pct")
					tf.mode, tf.durable = "torn", func(n int) int {
						k := n * pct / 100
						if k >= n {
							k = n - 1
						}
						return k
					}
				case "full-unsynced":
					tf.mode = "sync-fail"
				}
				if _, err := s.Append(proto); err == nil {
					rt.Fatalf("injected crash (%s) did not surface", mode)
				}
				if mode == "full-unsynced" {
					// ADR-0022 accepted-unacked: fully written, valid at
					// EOF — recovery keeps it.
					model.lastSeq = want.Seq
					model.heads[want.RunID] = want.EventID
					model.events = append(model.events, want)
				}
				_ = s.Close()
				s = openIt()
				if s.LastSeq() != model.lastSeq {
					rt.Fatalf("recovered LastSeq %d, want %d (crash mode %s)", s.LastSeq(), model.lastSeq, mode)
				}
			case "reopen":
				_ = s.Close()
				s = openIt()
				if s.LastSeq() != model.lastSeq {
					rt.Fatalf("reopened LastSeq %d, want %d", s.LastSeq(), model.lastSeq)
				}
			case "readall":
				checkAll()
			}
		}
		checkAll()
	})
}

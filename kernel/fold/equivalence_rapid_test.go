// Incremental == rebuild, always (WP-05a): the property that makes
// "one code path" testable. Random event sequences — mixed run_ids
// including "", unknown types interleaved, versions that trip the
// gate, reducer errors — split at a random point: folding the whole
// slice must equal folding a prefix then Stepping the rest, on every
// (view, run) state hash and every failure code.
package fold_test

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"

	"github.com/DonScott603/Agent-Runtime/kernel"
	"github.com/DonScott603/Agent-Runtime/kernel/fold"
)

var (
	rapidRunIDs = []kernel.RunID{"", "run_a", "run_b", "run_c"}
	// known.a is gated at 2, x.y at 1; boom.now trips errReducer;
	// the rest are unknown to every fixture.
	rapidTypes = []string{"known.a", "x.y", "boom.now", "utterly.unknown", "future.anchor"}
)

func rapidRegistry(t tb) *fold.Registry {
	t.Helper()
	return mustRegistry(t,
		reg("owner-count", fold.ScopeOwner, nil, countingReducer{}),
		reg("run-count", fold.ScopeRun, nil, countingReducer{}),
		reg("gated-owner", fold.ScopeOwner, map[string]uint16{"known.a": 2, "x.y": 1}, countingReducer{}),
		reg("gated-run", fold.ScopeRun, map[string]uint16{"known.a": 2, "x.y": 1}, countingReducer{}),
		reg("erring", fold.ScopeOwner, nil, errReducer{}),
	)
}

// genEvents draws a canon-safe event sequence with strictly
// increasing seq from a random base (any-base rule, ADR-0022).
func genEvents(t *rapid.T) []kernel.Event {
	n := rapid.IntRange(0, 40).Draw(t, "n")
	seq := kernel.Seq(rapid.Uint64Range(1, 1_000).Draw(t, "base"))
	events := make([]kernel.Event, 0, n)
	for range n {
		payload := fmt.Sprintf(`{"n":%d,"note":"%s"}`,
			rapid.Int64Range(-1000, 1000).Draw(t, "n_val"),
			rapid.StringMatching(`[a-z0-9 ]{0,12}`).Draw(t, "note"))
		events = append(events, kernel.Event{
			Seq:         seq,
			RunID:       rapid.SampledFrom(rapidRunIDs).Draw(t, "run"),
			Principal:   "owner",
			Type:        rapid.SampledFrom(rapidTypes).Draw(t, "type"),
			TypeVersion: uint16(rapid.IntRange(1, 3).Draw(t, "ver")),
			Payload:     []byte(payload),
		})
		seq += kernel.Seq(rapid.Uint64Range(1, 3).Draw(t, "gap"))
	}
	return events
}

func assertSameViews(t *rapid.T, full, inc *fold.Fold) {
	views := []struct {
		id    string
		scope fold.Scope
	}{
		{"owner-count", fold.ScopeOwner},
		{"run-count", fold.ScopeRun},
		{"gated-owner", fold.ScopeOwner},
		{"gated-run", fold.ScopeRun},
		{"erring", fold.ScopeOwner},
	}
	for _, v := range views {
		runs := []kernel.RunID{""}
		if v.scope == fold.ScopeRun {
			runs = []kernel.RunID{"run_a", "run_b", "run_c"}
		}
		for _, run := range runs {
			hFull, errFull := full.StateHash(v.id, run)
			hInc, errInc := inc.StateHash(v.id, run)
			if (errFull == nil) != (errInc == nil) {
				t.Fatalf("%s/%s: rebuild and incremental disagree on failure: %v vs %v", v.id, run, errFull, errInc)
			}
			if errFull != nil {
				if errFull.Error() != errInc.Error() {
					t.Fatalf("%s/%s: failure differs:\n rebuild:     %v\n incremental: %v", v.id, run, errFull, errInc)
				}
				continue
			}
			if hFull != hInc {
				t.Fatalf("%s/%s: state hash differs:\n rebuild:     %s\n incremental: %s", v.id, run, hFull, hInc)
			}
		}
	}
}

func TestIncrementalEqualsRebuild(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		events := genEvents(t)
		reg := rapidRegistry(t)

		full, err := fold.Rebuild(reg, events)
		if err != nil {
			t.Fatalf("Rebuild(all): %v", err)
		}

		k := rapid.IntRange(0, len(events)).Draw(t, "split")
		inc, err := fold.Rebuild(reg, events[:k])
		if err != nil {
			t.Fatalf("Rebuild(prefix): %v", err)
		}
		for _, e := range events[k:] {
			if err := inc.Step(e); err != nil {
				t.Fatalf("Step seq %d: %v", e.Seq, err)
			}
		}

		assertSameViews(t, full, inc)
		if full.LastSeq() != inc.LastSeq() {
			t.Fatalf("LastSeq differs: %d vs %d", full.LastSeq(), inc.LastSeq())
		}
	})
}

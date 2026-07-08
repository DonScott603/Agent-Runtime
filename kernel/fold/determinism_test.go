// Fold determinism (WP-05a; RFC-0002 §9.1; root CLAUDE.md: every pure
// component ships its determinism test in the same commit).
package fold_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
	"github.com/DonScott603/Agent-Runtime/kernel/canon"
	"github.com/DonScott603/Agent-Runtime/kernel/fold"
)

// mixedRegistry covers both scopes, a version gate, an erroring
// reducer and a healthy counter — the shapes a real fold holds.
func mixedRegistry(t testing.TB) *fold.Registry {
	t.Helper()
	return mustRegistry(t,
		reg("owner-count", fold.ScopeOwner, nil, countingReducer{}),
		reg("run-count", fold.ScopeRun, nil, countingReducer{}),
		reg("gated", fold.ScopeOwner, map[string]uint16{"x.y": 1}, countingReducer{}),
		reg("erring", fold.ScopeOwner, nil, errReducer{}),
	)
}

// mixedEvents exercises unknown types, both scopes, a version-gate
// trip and a reducer error — all deterministic failures included.
func mixedEvents() []kernel.Event {
	return []kernel.Event{
		ev(1, "run_a", "run.created", 1),
		ev(2, "run_a", "utterly.unknown", 1),
		ev(3, "run_b", "t.other", 1),
		ev(4, "", "x.y", 99),     // gated view fails here
		ev(5, "", "boom.now", 1), // erring view fails here
		ev(6, "run_a", "t.more", 1),
	}
}

func viewKeys() [][2]string {
	return [][2]string{
		{"owner-count", ""},
		{"run-count", "run_a"},
		{"run-count", "run_b"},
		{"gated", ""},
		{"erring", ""},
	}
}

func TestFoldTwiceStateHashesEqual(t *testing.T) {
	r := mixedRegistry(t)
	a, err := fold.Rebuild(r, mixedEvents())
	if err != nil {
		t.Fatalf("first Rebuild: %v", err)
	}
	b, err := fold.Rebuild(r, mixedEvents())
	if err != nil {
		t.Fatalf("second Rebuild: %v", err)
	}
	for _, key := range viewKeys() {
		view, run := key[0], kernel.RunID(key[1])
		ha, errA := a.StateHash(view, run)
		hb, errB := b.StateHash(view, run)
		if (errA == nil) != (errB == nil) {
			t.Fatalf("%s/%s: one fold failed, the other did not: %v vs %v", view, run, errA, errB)
		}
		if errA != nil {
			var va, vb *fold.ViewError
			if !errors.As(errA, &va) || !errors.As(errB, &vb) {
				t.Fatalf("%s/%s: non-ViewError failures: %v vs %v", view, run, errA, errB)
			}
			if va.Code != vb.Code || va.Seq != vb.Seq {
				t.Errorf("%s/%s: failure differs across identical folds: %+v vs %+v", view, run, va, vb)
			}
			continue
		}
		if ha != hb {
			t.Errorf("%s/%s: state hash differs across identical folds:\n %s\n %s", view, run, ha, hb)
		}
	}
}

// CanonicalStateHash must be insensitive to the reducer's marshaling
// quirks: key order and whitespace differences of the SAME logical
// state hash identically (canon re-parses RawMessage).
func TestCanonicalStateHashEquivalence(t *testing.T) {
	a := json.RawMessage(`{"alpha":1,"beta":"x"}`)
	b := json.RawMessage("{ \"beta\": \"x\", \"alpha\": 1 }")
	ha, err := fold.CanonicalStateHash(a)
	if err != nil {
		t.Fatalf("CanonicalStateHash: %v", err)
	}
	hb, err := fold.CanonicalStateHash(b)
	if err != nil {
		t.Fatalf("CanonicalStateHash: %v", err)
	}
	if ha != hb {
		t.Errorf("equivalent states hash differently:\n %s\n %s", ha, hb)
	}
	if len(ha) != 64 {
		t.Errorf("hash %q is not lowercase-hex sha256 shaped", ha)
	}
}

// Floats in reducer state fail LOUD at hash time (RFC-0001 D2).
func TestCanonicalStateHashRejectsFloats(t *testing.T) {
	_, err := fold.CanonicalStateHash(json.RawMessage(`{"spend":1.5}`))
	if !errors.Is(err, canon.ErrFloat) {
		t.Fatalf("want canon.ErrFloat, got %v", err)
	}
}

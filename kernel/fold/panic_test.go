// Panic containment (WP-05a): RFC-0004 P3 discipline applied to
// reducers on the read path — a panic is a reducer bug, caught as
// PLUGIN_CONTRACT, never an engine crash.
package fold_test

import (
	"strings"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel/fold"
)

func TestPanicContainment(t *testing.T) {
	r := mustRegistry(t,
		reg("panicky", fold.ScopeOwner, nil, panicReducer{}),
		reg("count", fold.ScopeOwner, nil, countingReducer{}),
	)
	f := fold.New(r)
	mustStep(t, f, ev(1, "", "known.a", 1))
	mustStep(t, f, ev(2, "", "utterly.unknown", 1)) // panics inside Apply; engine must survive

	ve := viewErr(t, f, "panicky", "")
	if ve.Code != fold.CodePluginContract {
		t.Errorf("code %q, want %q", ve.Code, fold.CodePluginContract)
	}
	if ve.Seq != 2 {
		t.Errorf("failed at seq %d, want 2", ve.Seq)
	}
	if !strings.Contains(ve.Detail, "unknown type") {
		t.Errorf("Detail %q does not carry the panic text", ve.Detail)
	}

	// Other views complete over the same events; the engine keeps
	// accepting Steps after the contained panic.
	if got := seen(t, f, "count", ""); len(got) != 2 {
		t.Errorf("sibling view saw %d events, want 2", len(got))
	}
	mustStep(t, f, ev(3, "", "t.later", 1))
	if got := seen(t, f, "count", ""); len(got) != 3 {
		t.Errorf("engine did not keep folding after a contained panic: %d events, want 3", len(got))
	}
}

// Run-scoped panic downs only the panicking run's instance.
func TestPanicPerRunInstance(t *testing.T) {
	r := mustRegistry(t, reg("panicky", fold.ScopeRun, nil, panicReducer{}))
	f := fold.New(r)
	mustStep(t, f, ev(1, "run_a", "utterly.unknown", 1)) // run_a down
	mustStep(t, f, ev(2, "run_b", "known.a", 1))         // run_b healthy

	ve := viewErr(t, f, "panicky", "run_a")
	if ve.Code != fold.CodePluginContract || ve.Run != "run_a" {
		t.Errorf("run_a failure = %+v, want PLUGIN_CONTRACT for run_a", ve)
	}
	if _, err := f.State("panicky", "run_b"); err != nil {
		t.Errorf("run_b instance must be healthy, got %v", err)
	}
}

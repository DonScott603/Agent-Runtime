// Round-trip integration (WP-05a): real bytes through the durable
// store's recovery, then folded — the fold independently re-deriving
// what the store verified. Internal package ON PURPOSE: it reuses
// persistReopen/loadTraceEvents (store_roundtrip_test.go) and needs
// appendSealed for the trace's seq base 101; legal only because
// kernel/fold and plugins/* never import kernel/log, so this file
// doubles as the import-topology tripwire — if fold ever grows a
// kernel/log import, this fails to compile with an import cycle.
package log

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
	"github.com/DonScott603/Agent-Runtime/kernel/fold"
	"github.com/DonScott603/Agent-Runtime/plugins/chainverify"
	"github.com/DonScott603/Agent-Runtime/plugins/runstatus"
)

func foldRegistry(t testing.TB) *fold.Registry {
	t.Helper()
	reg, err := fold.NewRegistry(runstatus.Registration(), chainverify.Registration())
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

type cvState struct {
	Alarms []json.RawMessage      `json:"alarms"`
	Broken map[string]bool        `json:"broken"`
	Heads  map[string]kernel.Hash `json:"heads"`
}

func chainState(t testing.TB, f *fold.Fold) cvState {
	t.Helper()
	raw, err := f.State(chainverify.PluginID, "")
	if err != nil {
		t.Fatalf("chainverify state: %v", err)
	}
	var s cvState
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("chainverify state does not parse: %v", err)
	}
	return s
}

// The 20 trace events (docs/trace-annotated.md, base seq 101) persist
// through the store, recover, and fold: run-status lands on
// "completed" having passed through suspended(awaiting_approval) and
// resumed; the chain-verifier's head for run_0042 re-derives the
// documented trace head. Then a run.suspended at type_version 99
// appended THROUGH the store (type_version is opaque to the writer)
// makes the run-status view unavailable with SCHEMA_UNKNOWN_VERSION
// while the chain verifier verifies the same event and advances —
// one reducer's version failure is invisible to another.
func TestFoldRoundTripTrace(t *testing.T) {
	events := loadTraceEvents(t)
	_, store := persistReopen(t, events)
	reg := foldRegistry(t)

	f, err := fold.FromSource(reg, store)
	if err != nil {
		t.Fatalf("FromSource: %v", err)
	}

	// Run-status: terminal state via the suspension waypoint.
	state, err := f.State(runstatus.PluginID, "run_0042")
	if err != nil {
		t.Fatalf("run-status state: %v", err)
	}
	if want := `{"status":"completed"}`; !bytes.Equal(state, []byte(want)) {
		t.Errorf("run-status state %s, want %s", state, want)
	}

	// The waypoint, asserted by folding the prefix through seq 113 —
	// then Stepping the remainder to completion, which is itself an
	// incremental-equals-rebuild check on real recovered data.
	prefix, err := fold.Rebuild(reg, events[:13]) // seq 101..113
	if err != nil {
		t.Fatalf("Rebuild(prefix): %v", err)
	}
	waypoint, err := prefix.State(runstatus.PluginID, "run_0042")
	if err != nil {
		t.Fatalf("run-status state at seq 113: %v", err)
	}
	if want := `{"status":"suspended","reason":"awaiting_approval"}`; !bytes.Equal(waypoint, []byte(want)) {
		t.Errorf("state at seq 113 = %s, want %s", waypoint, want)
	}
	for _, e := range events[13:] {
		if err := prefix.Step(e); err != nil {
			t.Fatalf("Step seq %d: %v", e.Seq, err)
		}
	}
	fullHash, err := f.StateHash(runstatus.PluginID, "run_0042")
	if err != nil {
		t.Fatalf("StateHash: %v", err)
	}
	steppedHash, err := prefix.StateHash(runstatus.PluginID, "run_0042")
	if err != nil {
		t.Fatalf("StateHash: %v", err)
	}
	if fullHash != steppedHash {
		t.Errorf("incremental != rebuild on real data:\n rebuild:     %s\n incremental: %s", fullHash, steppedHash)
	}

	// Chain-verifier: the fold re-derives the documented head.
	cs := chainState(t, f)
	if len(cs.Alarms) != 0 || len(cs.Broken) != 0 {
		t.Fatalf("trace chain raised alarms: %+v", cs)
	}
	if cs.Heads["run_0042"] != traceHead {
		t.Errorf("chain-verifier head %s, want %s (docs/trace-annotated.md)", cs.Heads["run_0042"], traceHead)
	}

	// Fold determinism over the recovered bytes (RFC-0002 §9.1).
	again, err := fold.FromSource(reg, store)
	if err != nil {
		t.Fatalf("second FromSource: %v", err)
	}
	for _, key := range []struct {
		view string
		run  kernel.RunID
	}{
		{runstatus.PluginID, "run_0042"},
		{chainverify.PluginID, ""},
	} {
		h1, err := f.StateHash(key.view, key.run)
		if err != nil {
			t.Fatalf("StateHash: %v", err)
		}
		h2, err := again.StateHash(key.view, key.run)
		if err != nil {
			t.Fatalf("StateHash: %v", err)
		}
		if h1 != h2 {
			t.Errorf("%s/%s: fold twice, different hashes:\n %s\n %s", key.view, key.run, h1, h2)
		}
	}

	// Continuation: a future-versioned run.suspended goes THROUGH the
	// store (type_version is opaque to the writer) and into the live
	// fold. The run-status view goes SCHEMA_UNKNOWN_VERSION-unavailable
	// for run_0042; the chain verifier verifies the same event and
	// advances its head.
	sealed, err := store.Append(kernel.Event{
		RunID: "run_0042", TS: 1751793700, Mono: 31,
		Principal: "service:kernel", Type: "run.suspended", TypeVersion: 99,
		Payload: json.RawMessage(`{"reason":"a_shape_from_the_future"}`),
	})
	if err != nil {
		t.Fatalf("Append v99: %v", err)
	}
	if sealed.Seq != 121 || sealed.PrevHash != traceHead {
		t.Fatalf("continuation event seq %d prev %s, want 121 after trace head", sealed.Seq, sealed.PrevHash)
	}
	if err := f.Step(sealed); err != nil {
		t.Fatalf("Step v99: %v", err)
	}
	_, err = f.State(runstatus.PluginID, "run_0042")
	var ve *fold.ViewError
	if !errors.As(err, &ve) {
		t.Fatalf("run-status after v99: want *ViewError, got %v", err)
	}
	if ve.Code != fold.CodeSchemaUnknownVersion || ve.Seq != 121 {
		t.Errorf("view error = %+v, want SCHEMA_UNKNOWN_VERSION at seq 121", ve)
	}
	cs = chainState(t, f)
	if len(cs.Alarms) != 0 {
		t.Fatalf("chain verifier alarmed on a validly chained v99 event: %+v", cs.Alarms)
	}
	if cs.Heads["run_0042"] != sealed.EventID {
		t.Errorf("chain head did not advance over the v99 event: %s, want %s", cs.Heads["run_0042"], sealed.EventID)
	}
}

// The anchored end-to-end weld (WP-04c): the store WRITES an anchor
// over the recovered trace (base 101), and the fold-side verifier
// independently re-derives and confirms every claim in it — Merkle
// root from its own heads, base attestation from its own observed
// base — with zero alarms. The two sides never share state; agreeing
// green is the whole point of the anchor.
func TestFoldRoundTripAnchoredTrace(t *testing.T) {
	events := loadTraceEvents(t)
	_, store := persistReopen(t, events)

	anchor, err := store.WriteAnchor(1751793800, 40)
	if err != nil {
		t.Fatalf("WriteAnchor: %v", err)
	}
	if anchor.Seq != 121 {
		t.Fatalf("anchor seq %d, want 121", anchor.Seq)
	}

	f, err := fold.FromSource(foldRegistry(t), store)
	if err != nil {
		t.Fatalf("FromSource: %v", err)
	}
	cs := chainState(t, f)
	if len(cs.Alarms) != 0 || len(cs.Broken) != 0 {
		t.Fatalf("anchored trace raised alarms: %+v", cs)
	}
	if cs.Heads["run_0042"] != traceHead {
		t.Errorf("run_0042 head %s, want the documented trace head", cs.Heads["run_0042"])
	}
	if cs.Heads[""] != anchor.EventID {
		t.Errorf(`heads[""] = %s, want the anchor %s`, cs.Heads[""], anchor.EventID)
	}
}

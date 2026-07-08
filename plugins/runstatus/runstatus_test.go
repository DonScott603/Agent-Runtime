// runstatus reducer tests (WP-05a): the transition projection, the
// must-ignore obligations (unknown types, unknown payload fields),
// and the determinism law (invoke twice, byte-compare — root
// CLAUDE.md same-commit rule).
package runstatus_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
	"github.com/DonScott603/Agent-Runtime/kernel/fold"
	"github.com/DonScott603/Agent-Runtime/plugins/runstatus"
)

func ev(seq kernel.Seq, typ, payload string) kernel.Event {
	return kernel.Event{
		Seq: seq, RunID: "run_x", Principal: "service:kernel",
		Type: typ, TypeVersion: 1,
		Payload: json.RawMessage(payload),
	}
}

// applyAll threads events through Apply directly (unit level; the
// engine path is covered by kernel/fold and the round-trip test).
func applyAll(t testing.TB, events ...kernel.Event) json.RawMessage {
	t.Helper()
	r := runstatus.New()
	state := r.Init()
	for _, e := range events {
		next, err := r.Apply(state, e)
		if err != nil {
			t.Fatalf("Apply(%s): %v", e.Type, err)
		}
		state = next
	}
	return state
}

func assertState(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(got, &a); err != nil {
		t.Fatalf("state does not parse: %v (%s)", err, got)
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatalf("bad want literal: %v", err)
	}
	gotN, _ := json.Marshal(a)
	wantN, _ := json.Marshal(b)
	if !bytes.Equal(gotN, wantN) {
		t.Errorf("state mismatch\n got: %s\nwant: %s", gotN, wantN)
	}
}

func TestProjection(t *testing.T) {
	cases := []struct {
		name   string
		events []kernel.Event
		want   string
	}{
		{"init is empty object", nil, `{}`},
		{"created", []kernel.Event{ev(1, "run.created", `{"agent":"research"}`)}, `{"status":"created"}`},
		{"created-started", []kernel.Event{
			ev(1, "run.created", `{}`), ev(2, "run.started", `{}`),
		}, `{"status":"running"}`},
		{"suspended carries reason", []kernel.Event{
			ev(1, "run.created", `{}`), ev(2, "run.started", `{}`),
			ev(3, "run.suspended", `{"reason":"awaiting_approval"}`),
		}, `{"status":"suspended","reason":"awaiting_approval"}`},
		{"resume clears reason", []kernel.Event{
			ev(1, "run.created", `{}`), ev(2, "run.started", `{}`),
			ev(3, "run.suspended", `{"reason":"awaiting_approval"}`),
			ev(4, "run.resumed", `{}`),
		}, `{"status":"running"}`},
		{"completed", []kernel.Event{
			ev(1, "run.created", `{}`), ev(2, "run.started", `{}`),
			ev(3, "run.completed", `{}`),
		}, `{"status":"completed"}`},
		{"failed", []kernel.Event{
			ev(1, "run.created", `{}`), ev(2, "run.failed", `{"error_ref":"e1"}`),
		}, `{"status":"failed"}`},
		{"cancelled", []kernel.Event{
			ev(1, "run.created", `{}`), ev(2, "run.cancelled", `{}`),
		}, `{"status":"cancelled"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertState(t, applyAll(t, tc.events...), tc.want)
		})
	}
}

// Unknown payload FIELDS are the reducer's must-ignore obligation
// (RFC-0002 §2; versioning.md M1): an additive field changes nothing.
func TestUnknownPayloadFieldsIgnored(t *testing.T) {
	plain := applyAll(t, ev(1, "run.suspended", `{"reason":"awaiting_approval"}`))
	extra := applyAll(t, ev(1, "run.suspended", `{"reason":"awaiting_approval","future_field":123,"more":"stuff"}`))
	if !bytes.Equal(plain, extra) {
		t.Errorf("unknown payload fields leaked into state\n plain: %s\n extra: %s", plain, extra)
	}
}

// Unknown event TYPES leave state byte-identical (totality): a fold
// with interleaved unknown types equals one without them.
func TestUnknownTypesInvariant(t *testing.T) {
	with := applyAll(t,
		ev(1, "run.created", `{}`),
		ev(2, "utterly.unknown", `{"payload":"whatever"}`),
		ev(3, "run.started", `{}`),
		ev(4, "anchor.appended", `{"root":"abc"}`), // the 04c future, ignored today
	)
	without := applyAll(t,
		ev(1, "run.created", `{}`),
		ev(3, "run.started", `{}`),
	)
	if !bytes.Equal(with, without) {
		t.Errorf("unknown types affected state\n with:    %s\n without: %s", with, without)
	}
}

// Determinism: invoke twice, byte-compare (root CLAUDE.md).
func TestDeterminism(t *testing.T) {
	events := []kernel.Event{
		ev(1, "run.created", `{}`),
		ev(2, "run.started", `{}`),
		ev(3, "run.suspended", `{"reason":"awaiting_approval"}`),
		ev(4, "run.resumed", `{}`),
		ev(5, "run.completed", `{}`),
	}
	a := applyAll(t, events...)
	b := applyAll(t, events...)
	if !bytes.Equal(a, b) {
		t.Fatalf("two identical folds produced different bytes\n a: %s\n b: %s", a, b)
	}
}

func TestRegistrationShape(t *testing.T) {
	r := runstatus.Registration()
	if r.ID != runstatus.PluginID || r.Semver != runstatus.Semver {
		t.Errorf("registration identity %s@%s, want %s@%s", r.ID, r.Semver, runstatus.PluginID, runstatus.Semver)
	}
	if r.Scope != fold.ScopeRun {
		t.Errorf("scope %q, want run", r.Scope)
	}
	want := []string{
		"run.created", "run.started", "run.suspended", "run.resumed",
		"run.completed", "run.failed", "run.cancelled",
	}
	if len(r.Handles) != len(want) {
		t.Errorf("Handles has %d entries, want %d", len(r.Handles), len(want))
	}
	for _, typ := range want {
		if v, ok := r.Handles[typ]; !ok || v != 1 {
			t.Errorf("Handles[%q] = %d,%v; want 1,true", typ, v, ok)
		}
	}
	if r.Reducer == nil {
		t.Error("nil reducer in registration")
	}
	// The registration must be engine-valid.
	if _, err := fold.NewRegistry(r); err != nil {
		t.Errorf("registration rejected by NewRegistry: %v", err)
	}
}

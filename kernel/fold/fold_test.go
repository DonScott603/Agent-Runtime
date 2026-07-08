// Engine contract tests (WP-05a): totality (no pre-filtering), scope
// routing, the version gate, containment classification, and the A2
// rejection-atomicity law. Fixture reducers live here and are shared
// by the sibling test files (same package).
package fold_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
	"github.com/DonScott603/Agent-Runtime/kernel/fold"
)

// ---------------------------------------------------------------------
// Fixture reducers
// ---------------------------------------------------------------------

// seenState records every event a counting fixture received, in
// order — the engine-does-not-pre-filter probe.
type seenState struct {
	Events []seenEvent `json:"events"`
}

type seenEvent struct {
	Seq  kernel.Seq `json:"seq"`
	Type string     `json:"type"`
}

// countingReducer appends every event it receives to its state. With
// empty Handles it must still see everything (totality: Handles is a
// version gate, never a filter).
type countingReducer struct{}

func (countingReducer) Init() json.RawMessage { return json.RawMessage(`{"events":[]}`) }

func (countingReducer) Apply(state json.RawMessage, e kernel.Event) (json.RawMessage, error) {
	var s seenState
	if err := json.Unmarshal(state, &s); err != nil {
		return nil, err
	}
	s.Events = append(s.Events, seenEvent{Seq: e.Seq, Type: e.Type})
	return json.Marshal(s)
}

// errReducer returns a deterministic error VALUE on "boom.now"
// (RFC-0004 P3: errors are values; docs/errors.md PLUGIN_ERROR).
type errReducer struct{}

var errBoom = errors.New("deterministic boom")

func (errReducer) Init() json.RawMessage { return json.RawMessage(`{}`) }

func (errReducer) Apply(state json.RawMessage, e kernel.Event) (json.RawMessage, error) {
	if e.Type == "boom.now" {
		return nil, errBoom
	}
	return state, nil
}

// nilReducer returns (nil, nil) on "nil.now" — the explicit
// PLUGIN_CONTRACT law (no silent normalizing).
type nilReducer struct{}

func (nilReducer) Init() json.RawMessage { return json.RawMessage(`{}`) }

func (nilReducer) Apply(state json.RawMessage, e kernel.Event) (json.RawMessage, error) {
	if e.Type == "nil.now" {
		return nil, nil
	}
	return state, nil
}

// panicReducer panics on any type it does not know — the reducer
// contract violation under test (a reducer MUST ignore unknown types,
// RFC-0002 §2; a panic is a bug by contract, RFC-0004 P3).
type panicReducer struct{}

func (panicReducer) Init() json.RawMessage { return json.RawMessage(`{"n":0}`) }

func (panicReducer) Apply(state json.RawMessage, e kernel.Event) (json.RawMessage, error) {
	if e.Type != "known.a" {
		panic(fmt.Sprintf("panicReducer: unknown type %s", e.Type))
	}
	var s struct {
		N int `json:"n"`
	}
	if err := json.Unmarshal(state, &s); err != nil {
		return nil, err
	}
	s.N++
	return json.Marshal(s)
}

// ---------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------

func ev(seq kernel.Seq, run kernel.RunID, typ string, ver uint16) kernel.Event {
	return kernel.Event{
		Seq: seq, RunID: run, Principal: "owner",
		Type: typ, TypeVersion: ver,
		Payload: json.RawMessage(`{}`),
	}
}

func reg(id string, scope fold.Scope, handles map[string]uint16, r kernel.Reducer) fold.Registration {
	return fold.Registration{ID: id, Semver: "0.1.0", Scope: scope, Handles: handles, Reducer: r}
}

// tb is the subset of testing.TB the helpers need; *rapid.T satisfies
// it too (rapid's T does not implement the full testing.TB).
type tb interface {
	Helper()
	Fatalf(format string, args ...any)
}

func mustRegistry(t tb, regs ...fold.Registration) *fold.Registry {
	t.Helper()
	r, err := fold.NewRegistry(regs...)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return r
}

func mustStep(t tb, f *fold.Fold, e kernel.Event) {
	t.Helper()
	if err := f.Step(e); err != nil {
		t.Fatalf("Step seq %d: %v", e.Seq, err)
	}
}

func seen(t tb, f *fold.Fold, view string, run kernel.RunID) []seenEvent {
	t.Helper()
	state, err := f.State(view, run)
	if err != nil {
		t.Fatalf("State(%q, %q): %v", view, run, err)
	}
	var s seenState
	if err := json.Unmarshal(state, &s); err != nil {
		t.Fatalf("state does not parse: %v", err)
	}
	return s.Events
}

func viewErr(t tb, f *fold.Fold, view string, run kernel.RunID) *fold.ViewError {
	t.Helper()
	_, err := f.State(view, run)
	if err == nil {
		t.Fatalf("State(%q, %q): want a view failure, got healthy state", view, run)
	}
	var ve *fold.ViewError
	if !errors.As(err, &ve) {
		t.Fatalf("State(%q, %q): want *ViewError, got %T: %v", view, run, err, err)
	}
	return ve
}

// ---------------------------------------------------------------------
// Totality: the engine never pre-filters
// ---------------------------------------------------------------------

func TestEngineDoesNotPreFilter(t *testing.T) {
	r := mustRegistry(t,
		reg("count", fold.ScopeOwner, nil, countingReducer{}),
	)
	f := fold.New(r)
	events := []kernel.Event{
		ev(1, "run_a", "run.created", 1),
		ev(2, "run_a", "utterly.unknown", 1),
		ev(3, "", "another.mystery", 99), // unknown type at a wild version: no gate, still delivered
	}
	if err := f.StepAll(events); err != nil {
		t.Fatalf("StepAll: %v", err)
	}
	got := seen(t, f, "count", "")
	if len(got) != 3 {
		t.Fatalf("counting reducer saw %d events, want all 3 (engine must not pre-filter): %+v", len(got), got)
	}
	for i, e := range events {
		if got[i].Seq != e.Seq || got[i].Type != e.Type {
			t.Errorf("event %d: saw {%d %s}, want {%d %s}", i, got[i].Seq, got[i].Type, e.Seq, e.Type)
		}
	}
}

// A gated reducer still RECEIVES types absent from Handles — the gate
// applies only to declared types (version gate, never a filter).
func TestUnknownTypeBypassesGate(t *testing.T) {
	r := mustRegistry(t,
		reg("gated", fold.ScopeOwner, map[string]uint16{"x.y": 1}, countingReducer{}),
	)
	f := fold.New(r)
	mustStep(t, f, ev(1, "", "unknown.z", 99)) // v99 on an UNKNOWN type: un-gated, delivered
	got := seen(t, f, "gated", "")
	if len(got) != 1 || got[0].Type != "unknown.z" {
		t.Fatalf("gated reducer did not receive the unknown type: %+v", got)
	}
}

// ---------------------------------------------------------------------
// Scope routing
// ---------------------------------------------------------------------

func TestScopeRouting(t *testing.T) {
	r := mustRegistry(t,
		reg("owner-count", fold.ScopeOwner, nil, countingReducer{}),
		reg("run-count", fold.ScopeRun, nil, countingReducer{}),
	)
	f := fold.New(r)
	if err := f.StepAll([]kernel.Event{
		ev(1, "run_a", "t.a", 1),
		ev(2, "run_b", "t.b", 1),
		ev(3, "", "owner.scoped", 1), // run_id "" never reaches run instances
		ev(4, "run_a", "t.c", 1),
	}); err != nil {
		t.Fatalf("StepAll: %v", err)
	}
	if got := seen(t, f, "owner-count", ""); len(got) != 4 {
		t.Errorf("owner-scoped view saw %d events, want 4", len(got))
	}
	if got := seen(t, f, "run-count", "run_a"); len(got) != 2 {
		t.Errorf("run_a instance saw %d events, want 2: %+v", len(got), got)
	}
	if got := seen(t, f, "run-count", "run_b"); len(got) != 1 {
		t.Errorf("run_b instance saw %d events, want 1: %+v", len(got), got)
	}
}

func TestUnseenRunReturnsInit(t *testing.T) {
	r := mustRegistry(t, reg("run-count", fold.ScopeRun, nil, countingReducer{}))
	f := fold.New(r)
	if got := seen(t, f, "run-count", "ghost"); len(got) != 0 {
		t.Fatalf("unseen run: want Init state (fold of zero events), got %+v", got)
	}
}

func TestQueryErrors(t *testing.T) {
	r := mustRegistry(t,
		reg("owner-count", fold.ScopeOwner, nil, countingReducer{}),
		reg("run-count", fold.ScopeRun, nil, countingReducer{}),
	)
	f := fold.New(r)
	cases := []struct {
		name string
		view string
		run  kernel.RunID
	}{
		{"unknown view", "nope", ""},
		{"owner view queried with a run", "owner-count", "run_a"},
		{"run view queried without a run", "run-count", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.State(tc.view, tc.run)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			var ve *fold.ViewError
			if errors.As(err, &ve) {
				t.Fatalf("query error must not be a ViewError (no view failed): %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------
// Version gate (SCHEMA_UNKNOWN_VERSION)
// ---------------------------------------------------------------------

func TestVersionGate(t *testing.T) {
	r := mustRegistry(t,
		reg("gated", fold.ScopeOwner, map[string]uint16{"x.y": 1}, countingReducer{}),
		reg("count", fold.ScopeOwner, nil, countingReducer{}),
	)
	f := fold.New(r)
	if err := f.StepAll([]kernel.Event{
		ev(1, "", "x.y", 1),
		ev(2, "", "x.y", 99), // above declared max: view unavailable
		ev(3, "", "x.y", 1),  // sticky: never applied
	}); err != nil {
		t.Fatalf("StepAll: %v", err)
	}
	ve := viewErr(t, f, "gated", "")
	if ve.Code != fold.CodeSchemaUnknownVersion {
		t.Errorf("code %q, want %q", ve.Code, fold.CodeSchemaUnknownVersion)
	}
	if ve.Seq != 2 {
		t.Errorf("failed at seq %d, want 2 (the first over-version event)", ve.Seq)
	}
	if ve.View != "gated" {
		t.Errorf("view %q, want gated", ve.View)
	}
	// The sibling view is untouched by the neighbor's failure.
	if got := seen(t, f, "count", ""); len(got) != 3 {
		t.Errorf("sibling view saw %d events, want 3", len(got))
	}
}

// Run-scope granularity: a v99 event in run A fails ONLY run A's
// instance of the gated view; run B keeps folding.
func TestVersionGatePerRunInstance(t *testing.T) {
	r := mustRegistry(t,
		reg("gated", fold.ScopeRun, map[string]uint16{"x.y": 1}, countingReducer{}),
	)
	f := fold.New(r)
	if err := f.StepAll([]kernel.Event{
		ev(1, "run_a", "x.y", 99),
		ev(2, "run_b", "x.y", 1),
		ev(3, "run_a", "t.z", 1), // sticky for run_a: not applied
		ev(4, "run_b", "t.z", 1),
	}); err != nil {
		t.Fatalf("StepAll: %v", err)
	}
	ve := viewErr(t, f, "gated", "run_a")
	if ve.Code != fold.CodeSchemaUnknownVersion || ve.Seq != 1 || ve.Run != "run_a" {
		t.Errorf("run_a failure = %+v, want SCHEMA_UNKNOWN_VERSION at seq 1 run run_a", ve)
	}
	if got := seen(t, f, "gated", "run_b"); len(got) != 2 {
		t.Errorf("run_b instance saw %d events, want 2 (unaffected by run_a's failure)", len(got))
	}
}

// ---------------------------------------------------------------------
// Containment classification
// ---------------------------------------------------------------------

func TestReducerErrorValueIsPluginError(t *testing.T) {
	r := mustRegistry(t,
		reg("erring", fold.ScopeOwner, nil, errReducer{}),
		reg("count", fold.ScopeOwner, nil, countingReducer{}),
	)
	f := fold.New(r)
	mustStep(t, f, ev(1, "", "boom.now", 1))
	ve := viewErr(t, f, "erring", "")
	if ve.Code != fold.CodePluginError {
		t.Errorf("code %q, want %q", ve.Code, fold.CodePluginError)
	}
	if !errors.Is(ve, errBoom) {
		t.Errorf("ViewError does not unwrap to the reducer's error: %v", ve)
	}
	if got := seen(t, f, "count", ""); len(got) != 1 {
		t.Errorf("sibling view saw %d events, want 1", len(got))
	}
}

func TestNilStateNilErrorIsContractViolation(t *testing.T) {
	r := mustRegistry(t, reg("nils", fold.ScopeOwner, nil, nilReducer{}))
	f := fold.New(r)
	mustStep(t, f, ev(1, "", "nil.now", 1))
	ve := viewErr(t, f, "nils", "")
	if ve.Code != fold.CodePluginContract {
		t.Errorf("code %q, want %q (nil state with nil error is a contract breach)", ve.Code, fold.CodePluginContract)
	}
}

// ---------------------------------------------------------------------
// A2: engine-level rejection atomicity
// ---------------------------------------------------------------------

func TestRejectionAtomicity(t *testing.T) {
	r := mustRegistry(t,
		reg("owner-count", fold.ScopeOwner, nil, countingReducer{}),
		reg("run-count", fold.ScopeRun, nil, countingReducer{}),
	)
	f := fold.New(r)
	mustStep(t, f, ev(5, "run_a", "t.a", 1))

	snapOwner, err := f.State("owner-count", "")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	snapRun, err := f.State("run-count", "run_a")
	if err != nil {
		t.Fatalf("State: %v", err)
	}

	for _, bad := range []kernel.Seq{5, 4} { // equal and lower: both out of order
		if err := f.Step(ev(bad, "run_a", "t.b", 1)); !errors.Is(err, fold.ErrOutOfOrder) {
			t.Fatalf("Step seq %d after 5: want ErrOutOfOrder, got %v", bad, err)
		}
	}

	// Byte-unchanged states, unchanged position, fold still usable.
	afterOwner, _ := f.State("owner-count", "")
	afterRun, _ := f.State("run-count", "run_a")
	if !bytes.Equal(snapOwner, afterOwner) || !bytes.Equal(snapRun, afterRun) {
		t.Fatal("rejected event leaked into view state (A2 atomicity)")
	}
	if got := f.LastSeq(); got != 5 {
		t.Fatalf("LastSeq %d after rejection, want 5", got)
	}
	mustStep(t, f, ev(6, "run_a", "t.c", 1))
	if got := seen(t, f, "owner-count", ""); len(got) != 2 {
		t.Fatalf("corrected Step did not apply: saw %d events, want 2", len(got))
	}
}

// ---------------------------------------------------------------------
// Rebuild == New + StepAll; misc engine surface
// ---------------------------------------------------------------------

func TestRebuildIsStepLoop(t *testing.T) {
	r := mustRegistry(t, reg("count", fold.ScopeOwner, nil, countingReducer{}))
	events := []kernel.Event{
		ev(101, "run_a", "t.a", 1), // any base, strictly increasing after
		ev(102, "", "t.b", 1),
	}
	rebuilt, err := fold.Rebuild(r, events)
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	stepped := fold.New(r)
	for _, e := range events {
		mustStep(t, stepped, e)
	}
	rh, err := rebuilt.StateHash("count", "")
	if err != nil {
		t.Fatalf("StateHash: %v", err)
	}
	sh, err := stepped.StateHash("count", "")
	if err != nil {
		t.Fatalf("StateHash: %v", err)
	}
	if rh != sh {
		t.Fatalf("rebuild hash %s != stepped hash %s", rh, sh)
	}
	if rebuilt.LastSeq() != 102 {
		t.Errorf("LastSeq %d, want 102", rebuilt.LastSeq())
	}
}

type sliceSource []kernel.Event

func (s sliceSource) ReadAll() ([]kernel.Event, error) { return s, nil }

func TestFromSource(t *testing.T) {
	r := mustRegistry(t, reg("count", fold.ScopeOwner, nil, countingReducer{}))
	f, err := fold.FromSource(r, sliceSource{ev(1, "", "t.a", 1)})
	if err != nil {
		t.Fatalf("FromSource: %v", err)
	}
	if got := seen(t, f, "count", ""); len(got) != 1 {
		t.Fatalf("saw %d events, want 1", len(got))
	}
}

func TestStateIsDefensiveCopy(t *testing.T) {
	r := mustRegistry(t, reg("count", fold.ScopeOwner, nil, countingReducer{}))
	f := fold.New(r)
	mustStep(t, f, ev(1, "", "t.a", 1))
	state, err := f.State("count", "")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	for i := range state {
		state[i] = 'X'
	}
	again, err := f.State("count", "")
	if err != nil {
		t.Fatalf("State after mutation: %v", err)
	}
	if bytes.Contains(again, []byte("XXX")) {
		t.Fatal("caller mutation corrupted the fold's state (defensive copy law)")
	}
}

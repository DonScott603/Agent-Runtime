// Package fold is the fold engine (WP-05a): state is a reducer folded
// over the event log (architecture §2). The engine enforces the
// totality contract of RFC-0002 §2 — it NEVER pre-filters event types
// (a reducer like the chain verifier wants everything); unknown
// payload fields are each reducer's must-ignore obligation.
//
// ONE Apply loop, two entry points: Rebuild is New + Step, literally.
// There is no way for live and rebuilt state to disagree by
// construction; the equivalence tests guard against regression, not
// construction.
//
// Pure package (local law): no os, no clock, no entropy, and never an
// import of kernel/log — kernel/log's round-trip test imports this
// package, so a reverse import is a compile-breaking cycle by design.
package fold

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

// ErrOutOfOrder rejects an event whose seq is not strictly greater
// than the last folded seq (RFC-0002 D5: seq order is THE order;
// reducers MUST fold in it). Strictly increasing, NOT gapless —
// gaplessness is the store's owned property (ADR-0022); re-owning it
// here would false-alarm on future filtered or segmented reads.
// Rejection is atomic (owner A2): no view sees the rejected event.
var ErrOutOfOrder = errors.New("fold: event seq not strictly increasing (RFC-0002 D5: fold in seq order)")

// Source is anything that can hand the engine a committed event
// sequence in seq order. *log.Store satisfies it structurally; the
// interface exists here so this package never imports kernel/log.
type Source interface {
	ReadAll() ([]kernel.Event, error)
}

// Fold holds the live state of every registered view. Not safe for
// concurrent use: one fold, one goroutine (the read-path analog of
// the single log-writer).
type Fold struct {
	views   []*view // registration order — deterministic iteration
	index   map[string]int
	lastSeq kernel.Seq
	started bool
}

// view is one registration's state: a single instance for owner
// scope, per-run instances for run scope.
type view struct {
	reg   Registration
	owner *instance
	runs  map[kernel.RunID]*instance
}

// instance is one independent fold: state bytes (immutable once
// committed — internal law; queries return clones) or a sticky
// failure.
type instance struct {
	state json.RawMessage
	err   *ViewError
}

// instanceFor routes an event to the view's instance, creating a
// run instance on first sight. nil means the view's scope does not
// cover this event (run-scoped view, run_id "" owner-scope event) —
// scope semantics, never type pre-filtering.
func (v *view) instanceFor(e kernel.Event) *instance {
	if v.reg.Scope == ScopeOwner {
		return v.owner
	}
	if e.RunID == "" {
		return nil
	}
	inst, ok := v.runs[e.RunID]
	if !ok {
		inst = &instance{state: v.reg.Reducer.Init()}
		v.runs[e.RunID] = inst
	}
	return inst
}

// New returns a fresh fold over the registry: every owner-scoped view
// at Init, no run instances yet.
func New(reg *Registry) *Fold {
	f := &Fold{
		views: make([]*view, 0, len(reg.regs)),
		index: reg.index,
	}
	for _, r := range reg.regs {
		v := &view{reg: r}
		if r.Scope == ScopeOwner {
			v.owner = &instance{state: r.Reducer.Init()}
		} else {
			v.runs = make(map[kernel.RunID]*instance)
		}
		f.views = append(f.views, v)
	}
	return f
}

// Step feeds ONE event through every registered view — THE apply
// loop; both entry points end here. The returned error is engine
// integrity only (ErrOutOfOrder); view failures are values held by
// the views (the fold never halts). Rejection is atomic (owner A2):
// the seq check runs before any view sees the event, so on error
// every state is byte-unchanged and the fold remains usable.
func (f *Fold) Step(e kernel.Event) error {
	if f.started && e.Seq <= f.lastSeq {
		return fmt.Errorf("%w: seq %d after %d", ErrOutOfOrder, e.Seq, f.lastSeq)
	}
	// One logical event, one byte form (ADR-0020; process.md §7):
	// reducers that canonicalize or re-seal must see the hashed form.
	e = kernel.NormalizeEnvelope(e)
	for _, v := range f.views {
		inst := v.instanceFor(e)
		if inst == nil || inst.err != nil { // out of scope, or sticky failure
			continue
		}
		applyOne(v.reg, inst, e)
	}
	f.lastSeq = e.Seq
	f.started = true
	return nil
}

// applyOne is the per-instance pipeline: version gate, upcast seam,
// Apply with containment. Failures are sticky values on the instance
// (view unavailable) — never engine errors, never other views'
// problem.
func applyOne(reg Registration, inst *instance, e kernel.Event) {
	run := kernel.RunID("")
	if reg.Scope == ScopeRun {
		run = e.RunID
	}
	// Version gate, on declared types only: a type_version above the
	// declared max is the missing-upcaster case (docs/errors.md
	// SCHEMA_UNKNOWN_VERSION — release-blocking). Types absent from
	// Handles bypass the gate and are still delivered (totality).
	if max, declared := reg.Handles[e.Type]; declared && e.TypeVersion > max {
		inst.err = &ViewError{
			View: reg.ID, Run: run, Seq: e.Seq, Code: CodeSchemaUnknownVersion,
			Detail: fmt.Sprintf("event type %s type_version %d above declared max %d (missing upcaster)", e.Type, e.TypeVersion, max),
		}
		return
	}
	next, panicDetail, err := runApply(reg.Reducer, inst.state, upcast(e))
	switch {
	case panicDetail != "":
		inst.err = &ViewError{
			View: reg.ID, Run: run, Seq: e.Seq, Code: CodePluginContract,
			Detail: panicDetail,
		}
	case err != nil:
		inst.err = &ViewError{
			View: reg.ID, Run: run, Seq: e.Seq, Code: CodePluginError,
			Detail: "reducer returned an error value", Err: err,
		}
	case next == nil:
		inst.err = &ViewError{
			View: reg.ID, Run: run, Seq: e.Seq, Code: CodePluginContract,
			Detail: "reducer returned nil state with nil error",
		}
	default:
		inst.state = next
	}
}

// runApply invokes the reducer with panic containment (RFC-0004 P3
// applied to reducers: a panic is a plugin bug, caught, never an
// engine crash).
func runApply(r kernel.Reducer, state json.RawMessage, e kernel.Event) (next json.RawMessage, panicDetail string, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			next, err = nil, nil
			panicDetail = fmt.Sprintf("reducer panic: %v", rec)
		}
	}()
	next, err = r.Apply(state, e)
	return next, "", err
}

// StepAll folds events in order, stopping at the first engine error.
func (f *Fold) StepAll(events []kernel.Event) error {
	for _, e := range events {
		if err := f.Step(e); err != nil {
			return err
		}
	}
	return nil
}

// Rebuild folds a complete event slice from scratch: New + StepAll,
// literally — the same code path as incremental folding.
func Rebuild(reg *Registry, events []kernel.Event) (*Fold, error) {
	f := New(reg)
	if err := f.StepAll(events); err != nil {
		return nil, err
	}
	return f, nil
}

// FromSource rebuilds from a Source's ReadAll (seq order).
func FromSource(reg *Registry, src Source) (*Fold, error) {
	events, err := src.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("fold: reading source: %w", err)
	}
	return Rebuild(reg, events)
}

// LastSeq returns the seq of the last folded event (0 if none).
func (f *Fold) LastSeq() kernel.Seq {
	return f.lastSeq
}

// State returns the current state of a view. run must be "" for an
// owner-scoped view and a run id for a run-scoped view (wrong-scope
// queries error). A failed instance returns its *ViewError
// (errors.As-able); an unseen run returns the reducer's Init state —
// the fold of zero events. The returned bytes are a defensive copy.
func (f *Fold) State(viewID string, run kernel.RunID) (json.RawMessage, error) {
	i, ok := f.index[viewID]
	if !ok {
		return nil, fmt.Errorf("fold: unknown view %q", viewID)
	}
	v := f.views[i]
	var inst *instance
	if v.reg.Scope == ScopeOwner {
		if run != "" {
			return nil, fmt.Errorf("fold: view %q is owner-scoped; query with run \"\"", viewID)
		}
		inst = v.owner
	} else {
		if run == "" {
			return nil, fmt.Errorf("fold: view %q is run-scoped; query with a run id", viewID)
		}
		inst = v.runs[run]
		if inst == nil {
			return bytes.Clone(v.reg.Reducer.Init()), nil
		}
	}
	if inst.err != nil {
		return nil, inst.err
	}
	return bytes.Clone(inst.state), nil
}

// StateHash is CanonicalStateHash over State.
func (f *Fold) StateHash(viewID string, run kernel.RunID) (kernel.Hash, error) {
	state, err := f.State(viewID, run)
	if err != nil {
		return "", err
	}
	return CanonicalStateHash(state)
}

// CanonicalStateHash is the determinism law's comparator and the
// future ratchet pin (WP-10/17): sha256 over kernel.Canonical(state).
// Canon re-parses and re-canonicalizes RawMessage, so reducer
// marshaling quirks (key order, whitespace) cannot leak into the
// hash; floats in state fail loud (RFC-0001 D2).
func CanonicalStateHash(state json.RawMessage) (kernel.Hash, error) {
	b, err := kernel.Canonical(state)
	if err != nil {
		return "", fmt.Errorf("fold: state hash: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

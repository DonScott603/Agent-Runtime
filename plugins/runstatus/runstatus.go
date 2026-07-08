// Package runstatus is the run-status projection reducer (WP-05a):
// the K3 state machine's states as folded state — created / running /
// suspended(reason) / completed / failed / cancelled — from run.*
// events (RFC-0002 §3). PROJECTION ONLY: transition legality is the
// run state machine's job (WP-07); this reducer records what the log
// says happened.
//
// Totality (RFC-0002 §2): non-run.* and unknown types leave state
// byte-identical; unknown payload FIELDS are ignored by construction
// (unmarshal into a struct — the reader's must-ignore obligation).
package runstatus

import (
	"encoding/json"

	"github.com/DonScott603/Agent-Runtime/kernel"
	"github.com/DonScott603/Agent-Runtime/kernel/fold"
)

const (
	PluginID = "run-status"
	Semver   = "0.1.0" // bump on ANY behavior change (ADR-0023)
)

// state is the projection: reason is present only while suspended.
// No floats, additive-only evolution (versioning.md M1).
type state struct {
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type reducer struct{}

// New returns the reducer. It is stateless: state is threaded by the
// fold engine.
func New() kernel.Reducer { return reducer{} }

// Registration is the canonical registration: run scope, the seven
// run.* types declared at type_version 1 (declaring 1 claims Apply
// parses the v1 payload shapes as stored).
func Registration() fold.Registration {
	return fold.Registration{
		ID:     PluginID,
		Semver: Semver,
		Scope:  fold.ScopeRun,
		Handles: map[string]uint16{
			"run.created":   1,
			"run.started":   1,
			"run.suspended": 1,
			"run.resumed":   1,
			"run.completed": 1,
			"run.failed":    1,
			"run.cancelled": 1,
		},
		Reducer: reducer{},
	}
}

func (reducer) Init() json.RawMessage { return json.RawMessage(`{}`) }

func (reducer) Apply(st json.RawMessage, e kernel.Event) (json.RawMessage, error) {
	var s state
	if err := json.Unmarshal(st, &s); err != nil {
		return nil, err
	}
	switch e.Type {
	case "run.created":
		s.Status, s.Reason = "created", ""
	case "run.started":
		s.Status, s.Reason = "running", ""
	case "run.suspended":
		// Unknown payload fields fall away here by construction.
		var p struct {
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return nil, err
		}
		s.Status, s.Reason = "suspended", p.Reason
	case "run.resumed":
		s.Status, s.Reason = "running", ""
	case "run.completed":
		s.Status, s.Reason = "completed", ""
	case "run.failed":
		s.Status, s.Reason = "failed", ""
	case "run.cancelled":
		s.Status, s.Reason = "cancelled", ""
	default:
		// Unknown type: byte-identical pass-through (totality).
		return st, nil
	}
	return json.Marshal(s)
}

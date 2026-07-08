// Reducer registration (WP-05a; RFC-0004 §6 for delivery class D11a).
package fold

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

// Scope is where a reducer's state lives. Run-scoped views hold one
// independent fold per run_id; owner-scoped views hold one fold over
// the full stream in seq order. Workspace scope (RFC-0004 §4) is
// deferred past Stage 1.
type Scope string

const (
	ScopeRun   Scope = "run"
	ScopeOwner Scope = "owner"
)

// Registration declares a reducer to the engine.
type Registration struct {
	ID     string
	Semver string // manually bumped on ANY behavior change (ADR-0023)
	Scope  Scope

	// Handles maps event type -> the highest type_version this
	// reducer's Apply understands for that type. It is a VERSION
	// GATE, never a subscription filter (local law): types absent
	// from the map are unknown to the reducer and are DELIVERED
	// anyway (totality, RFC-0002 §2) with no gate applied; a known
	// type above its declared version fails that view instance with
	// SCHEMA_UNKNOWN_VERSION (docs/errors.md — missing upcaster is
	// release-blocking). Declaring version N claims Apply parses
	// payload shapes v1..vN as stored.
	Handles map[string]uint16

	Reducer kernel.Reducer
}

// IdentityHash is the reducer identity of ADR-0023: sha256 over the
// canonical bytes of {"plugin_id": id, "semver": semver}. Scope,
// Handles and configuration are NOT identity (config is
// per-invocation data, RFC-0004 §7); behavior changes are semver
// bumps. WP-05b snapshot keys and plugin.invoked provenance hang off
// this value.
func IdentityHash(id, semver string) (kernel.Hash, error) {
	b, err := kernel.Canonical(struct {
		PluginID string `json:"plugin_id"`
		Semver   string `json:"semver"`
	}{PluginID: id, Semver: semver})
	if err != nil {
		return "", fmt.Errorf("fold: identity hash: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Registry is an ordered, validated set of registrations. Views are
// held in registration order — no map iteration ever feeds
// serialized or hashed output (root CLAUDE.md).
type Registry struct {
	regs  []Registration
	index map[string]int
}

// NewRegistry validates fail-fast: unique non-empty IDs, non-empty
// semver, known scope, non-nil reducer, and one Init() probe per
// reducer asserting non-nil valid JSON (a broken Init would otherwise
// surface event-by-event later).
func NewRegistry(regs ...Registration) (*Registry, error) {
	r := &Registry{index: make(map[string]int, len(regs))}
	for _, g := range regs {
		if g.ID == "" {
			return nil, fmt.Errorf("fold: registration with empty id")
		}
		if _, dup := r.index[g.ID]; dup {
			return nil, fmt.Errorf("fold: duplicate registration id %q", g.ID)
		}
		if g.Semver == "" {
			return nil, fmt.Errorf("fold: registration %q: empty semver (identity, ADR-0023)", g.ID)
		}
		if g.Scope != ScopeRun && g.Scope != ScopeOwner {
			return nil, fmt.Errorf("fold: registration %q: unknown scope %q", g.ID, g.Scope)
		}
		if g.Reducer == nil {
			return nil, fmt.Errorf("fold: registration %q: nil reducer", g.ID)
		}
		if init := g.Reducer.Init(); init == nil || !json.Valid(init) {
			return nil, fmt.Errorf("fold: registration %q: Init() is not valid JSON", g.ID)
		}
		r.index[g.ID] = len(r.regs)
		r.regs = append(r.regs, g)
	}
	return r, nil
}

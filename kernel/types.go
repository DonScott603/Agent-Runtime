// Package kernel defines the frozen data contracts of the runtime.
//
// TYPES-FIRST COMMIT. These types are the Go projection of the ACCEPTED
// RFCs; JSON tags match the golden vectors in docs/vectors/ exactly.
// Do not "improve" field names or shapes here — a change to this file
// that alters serialization is an ABI change (versioning.md S1–S4) and
// requires an ADR. Behavior stubs panic on purpose; implement them in
// work packages (docs/workplan.md), never by editing shapes here.
//
// Constitution reminders (CLAUDE.md): no floats in serialized types;
// no time.Now()/math/rand outside kernel handle implementations.
package kernel

import (
	"encoding/json"

	"agentruntime/kernel/canon"
)

// ---------------------------------------------------------------------
// Identifiers and primitives
// ---------------------------------------------------------------------

type (
	Hash        = string // lowercase hex sha256 (docs/vectors/canon.json)
	RunID       = string
	MessageID   = string
	BlockID     = string
	PrincipalID = string // "owner" | "agent:<id>" | "capability:<id>" | "service:<id>" — kernel-assigned (RFC-0003 §2)
	PluginID    = string
	RuleID      = string
	Seq         = uint64 // global, per-owner, gapless (RFC-0002 D5)
)

// ZeroHash is the prev_hash of a run's first event (RFC-0002 §4).
const ZeroHash = "0000000000000000000000000000000000000000000000000000000000000000"

// ---------------------------------------------------------------------
// Canonical message schema (RFC-0001)
// ---------------------------------------------------------------------

// Role per D1: tool results are blocks, not a role.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Core block types every consumer MUST understand (RFC-0001 §3).
// Extension types are namespaced ext.<vendor>.<name> / x.<user>.<name>.
const (
	BlockText       = "core.text"
	BlockToolUse    = "core.tool_use"
	BlockToolResult = "core.tool_result"
	BlockBlob       = "core.blob"
)

// ContentBlock. Body is retained as raw canonical bytes so unknown
// block types survive byte-for-byte (passthrough law, RFC-0001 §3).
type ContentBlock struct {
	ID   BlockID         `json:"id"`
	Type string          `json:"type"`
	Body json.RawMessage `json:"body,omitempty"`
	// Known-type projections; exactly one is populated for core types.
	// Text bodies live in blobs per D10 (BodyBlob on the wire).
	BodyBlob   Hash   `json:"body_blob,omitempty"`
	ToolUseID  string `json:"tool_use_id,omitempty"`
	Capability string `json:"capability,omitempty"`
	Operation  string `json:"operation,omitempty"`
	InputBlob  Hash   `json:"input_blob,omitempty"`
	OutputBlob Hash   `json:"output_blob,omitempty"`
}

type Provenance struct {
	Principal PrincipalID `json:"principal"`
	Source    string      `json:"source"` // "owner" | "model" | "capability:<id>" — taint labels
}

type Message struct {
	ID            MessageID      `json:"message_id"`
	Role          Role           `json:"role"`
	Blocks        []ContentBlock `json:"blocks"` // order is produced-order, never reordered (I1)
	Provenance    *Provenance    `json:"provenance,omitempty"`
	SchemaVersion string         `json:"schema_version,omitempty"`
}

// ---------------------------------------------------------------------
// Event envelope (RFC-0002 §2) — field names match docs/vectors/chain.json
// ---------------------------------------------------------------------

type Signature struct {
	Alg   string `json:"alg"`
	KeyID string `json:"key_id"`
	Value string `json:"value"`
}

type Event struct {
	Seq         Seq             `json:"seq"`
	RunID       RunID           `json:"run_id"` // "" for owner-scope events
	EventID     Hash            `json:"event_id"`
	PrevHash    Hash            `json:"prev_hash"`
	TS          int64           `json:"ts"`   // wall clock, informational (D5)
	Mono        uint64          `json:"mono"` // kernel monotonic counter
	Principal   PrincipalID     `json:"principal"`
	Type        string          `json:"type"` // "<domain>.<name>" (RFC-0002 §3)
	TypeVersion uint16          `json:"type_version"`
	Payload     json.RawMessage `json:"payload"`
	Blobs       []Hash          `json:"blobs"`
	Sig         *Signature      `json:"sig"` // required for consent/policy types (RFC-0002 §5)
}

// Canonical serializes per D2 (see docs/vectors/canon.json _rules).
func Canonical(v any) ([]byte, error) { return canon.Canonical(v) }

// SealEvent computes EventID: zero event_id (empty string) and sig
// (null), canonicalize, sha256 (docs/vectors/chain.json _rules).
func SealEvent(e Event) (Event, error) { panic("WP-04: implement log writer") }

// ---------------------------------------------------------------------
// Scopes, rules, resolution (RFC-0003) — shapes match resolution.json
// ---------------------------------------------------------------------

type MatcherKind string

const (
	MatchExact        MatcherKind = "Exact"
	MatchPrefix       MatcherKind = "Prefix"
	MatchSuffix       MatcherKind = "Suffix"
	MatchOneOf        MatcherKind = "OneOf"
	MatchNumericRange MatcherKind = "NumericRange"
	// CLOSED SET (D6). Adding a kind is an RFC-level event. Do not.
)

type Matcher struct {
	Kind   MatcherKind `json:"kind"`
	Value  string      `json:"value,omitempty"`
	Values []string    `json:"values,omitempty"`
	Min    *int64      `json:"min,omitempty"`
	Max    *int64      `json:"max,omitempty"`
}

// Scope: a derived scope's qualifier values are DATA, never patterns
// (derivation.json "hostile-glob-value-is-inert-data").
type Scope struct {
	Capability string            `json:"capability"`
	Operation  string            `json:"operation"`
	Qualifiers map[string]string `json:"qualifiers"`
}

type ScopeSelector struct {
	Capability string             `json:"capability"`
	Operation  string             `json:"operation"`
	Qualifiers map[string]Matcher `json:"qualifiers"`
}

type Action string

const (
	ActionAllow Action = "allow"
	ActionAsk   Action = "ask"
	ActionDeny  Action = "deny"
)

type Level string

const ( // specificity order: run > agent > profile > workspace > owner (D7)
	LevelRun       Level = "run"
	LevelAgent     Level = "agent"
	LevelProfile   Level = "profile"
	LevelWorkspace Level = "workspace"
	LevelOwner     Level = "owner"
)

type TaintCondition struct {
	IfRunUsed ScopeSelector `json:"if_run_used"` // may only TIGHTEN (RFC-0003 §7)
}

type Rule struct {
	ID         RuleID           `json:"id"`
	Level      Level            `json:"level"`
	Action     Action           `json:"action"`
	ScopeSel   ScopeSelector    `json:"scope_sel"`
	Conditions []TaintCondition `json:"conditions,omitempty"`
	Expiry     int64            `json:"expiry,omitempty"` // epoch seconds; 0 = none
	Sig        *Signature       `json:"sig,omitempty"`    // owner-signed (RFC-0002 §5)
}

type Decision struct {
	Action      Action   `json:"action"`
	WinningRule RuleID   `json:"winning_rule"` // or "default-ask"/"default-deny"
	Candidates  []RuleID `json:"candidates"`
}

// Resolve is a PURE function: (granted, rules, scope, runUsed) only.
// Algorithm: resolution.json _rules. Property suite: RFC-0003 §9.
func Resolve(granted bool, rules []Rule, scope Scope, runUsed []Scope) Decision {
	panic("WP-06: implement kernel/gate — make docs/vectors/resolution.json pass")
}

// ---------------------------------------------------------------------
// Manifests and derivation (RFC-0006) — shapes match derivation.json
// ---------------------------------------------------------------------

type Transform string

const (
	TransformIdentity Transform = "identity"
	TransformDomainOf Transform = "domain_of"
	TransformSuffix   Transform = "suffix"
	TransformPrefix   Transform = "prefix"
	TransformLowerNFC Transform = "lowercase_nfc"
	TransformCount    Transform = "count"
	TransformByteLen  Transform = "byte_len"
	// CLOSED SET (D15). Needing more means the operation is too coarse.
)

type Derivation struct {
	Qualifier string    `json:"qualifier"`
	Path      string    `json:"path"` // dot, [i], [*] ONLY (RFC-0006 §4)
	Transform Transform `json:"transform"`
}

type EffectClass string

const (
	EffectSafelyRepeatable   EffectClass = "safely-repeatable"
	EffectSuspendOnUncertain EffectClass = "suspend-on-uncertain" // the default (D13)
)

type Operation struct {
	Name        string       `json:"name"`
	EffectClass EffectClass  `json:"effect_class"`
	Derives     []Derivation `json:"derives"`
}

type TrustLevel string

const (
	TrustBundled    TrustLevel = "bundled" // in-repo only; validator rejects third-party requests (RFC-0006 §6)
	TrustVerified   TrustLevel = "verified"
	TrustThirdParty TrustLevel = "third-party"
	TrustUntrusted  TrustLevel = "untrusted"
)

// Derive is a PURE kernel computation over payload bytes + manifest.
// Underivable => deny, never guess. Vectors: derivation.json.
func Derive(payload json.RawMessage, op Operation) ([]Scope, error) {
	panic("WP-06: implement kernel/derive — make docs/vectors/derivation.json pass")
}

// ---------------------------------------------------------------------
// Kernel handles (RFC-0004 P2) — the ONLY sources of time and entropy
// ---------------------------------------------------------------------

type Clock interface{ Now() int64 }       // answers are recorded as clock.read events
type Entropy interface{ Seed() [32]byte } // answers are recorded as rng.seed events

// ---------------------------------------------------------------------
// Pure plugin interfaces (RFC-0004 §4) — signatures may churn until 1.0
// ---------------------------------------------------------------------

type FoldedState struct { // minimal v0 shape; extend additively
	Run      RunID
	Status   string
	Messages []Message
}

type ContextProvider interface {
	Assemble(state FoldedState, clock Clock) ([]Message, error)
}

type RoutingPolicy interface {
	Route(inbound Event, agents []string) (string, error)
}

type Reducer interface {
	Init() json.RawMessage
	Apply(state json.RawMessage, e Event) (json.RawMessage, error) // TOTAL: unknown types pass through
}

type Evaluator interface {
	Score(events []Event) (json.RawMessage, error)
}

# Glossary and Naming Map

Concept -> meaning -> canonical Go identifier. Synonym drift across
sessions becomes refactoring debt; use these names and no others.

| Concept | Meaning | Go identifier |
|---|---|---|
| Event | one envelope in the append-only store | `kernel.Event` |
| Envelope | the fixed fields around a payload | (fields of `Event`) |
| Payload | type-specific canonical bytes inside an event | `Event.Payload` |
| Blob | content-addressed encrypted bytes outside the store | `kernel.Hash` ref |
| Effect | a frozen proposal to touch the world (NOT "tool call") | `effect.proposed` payload |
| Operation | a capability's named action (`email.send`) | `kernel.Operation` |
| Tool call | a `core.tool_use` block in a message; becomes an Effect when proposed | `BlockToolUse` |
| Scope | derived classification of an effect (data, not pattern) | `kernel.Scope` |
| Matcher | typed comparator inside selectors (never a pattern language) | `kernel.Matcher` |
| Rule | owner-signed policy statement | `kernel.Rule` |
| Grant | informal term for an allow-rule or capability install; avoid in code | — |
| Decision | resolution output with provenance | `kernel.Decision` |
| Gate | the resolution + derivation choke point | `kernel/gate` |
| Broker | the single doorway executing authorized effects | `broker/` |
| Worker | an isolated process executing one effect class | `broker.Worker` |
| Fold | computing state by reducing events in seq order | `kernel/fold` |
| Reducer | pure (state, event) -> state view definition | `kernel.Reducer` |
| Snapshot | disposable cached fold; NEVER "checkpoint" | snapshot sidecar |
| Checkpoint | a log offset (implicitly every event boundary) | offset / `Seq` |
| Replay | re-folding recorded execution; never re-executing effects | `kernel/replay` |
| Verify mode | replay that re-executes pure invocations and diffs | — |
| Fork | new run borrowing a log prefix by reference | — |
| Taint | provenance label marking model/capability-derived data | `Provenance.Source` |
| Principal | kernel-assigned identity; never self-declared | `kernel.PrincipalID` |
| Manifest | signed capability contract (MCP wrapper) | RFC-0006 §2 |
| Derivation | declarative payload-field -> qualifier mapping | `kernel.Derivation` |
| Anchor | signed Merkle root of run heads, exported off-machine | anchor event |
| Ratchet | CI gate: historical corpora must fold under HEAD | `make conformance` |
| Vault slot | named credential reference in a manifest; never a value | RFC-0006 §5 |
| Uncertain window | executed-without-result crash gap | `effect.uncertain` |

Naming rules: event types are `<domain>.<name>` lowercase; JSON keys
are snake_case; Go exported identifiers per stubs/kernel/types.go.

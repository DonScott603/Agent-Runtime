# Architecture

Status: ACCEPTED — 2026-07-06. This document is the design RATIONALE:
why the system has the shape it has. The RFCs (docs/rfc-0001..0006)
are the CONTRACTS. When this document and an RFC disagree, the RFC
wins and this document has a bug. When neither settles a question,
docs/threat-model.md is the tiebreaker.

## 1. Thesis

A microkernel runtime for hosting AI agents, redesigned around one
inversion: instead of a kernel with six coordinated subsystems (state,
events, checkpoints, artifacts, replay, audit), **the append-only
event log is the kernel**, and everything else is a read strategy
over it. The guiding principle survives from the original proposal —
*the runtime owns WHEN; plugins own HOW* — but the log is what gives
it teeth.

Five invariants define the kernel. Everything not on this list is a
plugin, a capability, or a derived view:

  K1  The per-owner append-only event log (RFC-0002): content-
      addressed blobs, per-run hash chains, gapless global seq.
  K2  The canonical message schema (RFC-0001): the runtime's true
      ABI — the one data structure every component holds.
  K3  The run state machine (created / running / suspended(reason) /
      completed / failed / cancelled), whose transitions are events.
  K4  The permission gate (RFC-0003): derivation + resolution, whose
      inputs never include model-influenced content except as the
      payload under judgment.
  K5  The plugin/capability registry, itself folded state.

## 2. The log unifies state, checkpoints, replay, and audit

**State is a fold.** Current state of anything — run status, budgets,
the approval inbox, spend per capability — is a reducer folded over
events. Reducers are pure, total (unknown event types pass through),
and independent; new views are retroactive because reducers are code
and the log is data. Fixing a reducer bug retroactively corrects every
"historical" figure, because historical figures were never stored
facts, only cached computations.

**A checkpoint is an offset.** Since state is a deterministic fold
over a prefix, the prefix boundary — one integer — is a complete
resumable description. Every event boundary is implicitly a
checkpoint; creating one costs nothing and coordinates nothing.
Snapshots are memoized folds, keyed by (reducer hash, schema version,
offset), and therefore DISPOSABLE: corruption is a cache miss, not
data loss; reducer upgrades are re-folds, not migrations.

**Replay is a re-fold, in three grades.** Effects are never
re-executed — their recorded results are read back (you cannot
re-send an email, and a model call would not repeat). That is the
honest guarantee: deterministic RECONSTRUCTION of recorded execution,
not deterministic re-execution. Grade 1, reconstruction: fold to any
offset and inspect. Grade 2, verification: re-execute PURE plugin
invocations against recorded inputs and diff against recorded outputs
— recorded code hashes distinguish hidden nondeterminism (hashes
match, outputs differ) from version drift (hashes differ) from
corruption (chain fails). Grade 3, forking: borrow a log prefix by
reference and continue as a new run — with the gate fully re-applied;
recorded one-shot approvals never port across timelines.

**Audit is the log wearing a different hat.** Conventional audit
trails drift because they are a second write path narrating the
first. Here the records ARE the mechanism: the broker executes only
what the log authorized, so authorization, consent, and execution
share one write path and cannot disagree. Tamper-evidence layers:
per-run hash chains (cheap local detection), an anchored Merkle head
exported off-machine (defeats whole-chain recomputation), signatures
on consent events (an attacker can delete consent but never fabricate
it). Crypto-erasure composes cleanly because events hash blob
REFERENCES: destroying a key leaves the chain valid and the redaction
itself is a signed event — erasure is distinguishable from tampering.

## 3. Plugins split by purity, not by function

The kernel cares about one property: does it perform effects?

**Pure plugins** (context assembly, routing, memory selection,
reducers, evaluators) are deterministic functions of kernel-provided
inputs. No ambient state: time and entropy arrive through kernel
handles whose answers are recorded (clock.read / rng.seed), so replay
reproduces them. Purity is a VERIFIED contract — determinism harness,
ambient-state detector, continuous verify-replay — not a promise. At
v1 pure plugins compile into the binary (D11) and are therefore
kernel-trust: no third-party code in-process until a sandboxed
delivery exists (threat-model R6).

**Effectful plugins** (model adapters, tools, communication) live
behind the effect broker. Their outputs are recorded and never
re-executed.

**The broker is the single doorway** (RFC-0005). Pipeline: freeze the
payload in the log; gate decides; (approval if ask); execute the
frozen bytes in an isolated worker; record the result with taint
provenance. Credentials bind at worker-request assembly — post-freeze,
post-gate — from vault slots named in manifests, so secrets are
structurally absent from payloads, events, context, and approval
cards. Isolation is a dial set by the capability's trust level:
platform-native primitives (D12 a′: Linux full; Windows/macOS
documented tiers) with opportunistic container escalation, and the
recorded isolation descriptor names what was ACTUALLY applied — the
audit trail never overstates confinement. Crash recovery follows the
effect class (D13): safely-repeatable re-runs; everything else
suspends with an owner card, because a forgotten annotation must fail
toward a human question, never a double-send.

## 4. The influence boundary

Design axiom: prompt injection means any agent may be adversarial at
any moment, so the agent is an untrusted principal (threat-model §2).
The gate's rule: model-influenced content is JUDGED, never JUDGING.
Its decision inputs are owner-signed policy (agents cannot emit
policy events — signature-rejected writes), kernel-assigned identity
(never self-declared), and manifest-declared scope derivation (a pure
computation over frozen payload bytes; the agent's stated intent and
all context content are excluded — note that assembled context is
INSIDE the boundary even though the context plugin is pure, because
tainted inputs through a pure function yield tainted output). Any
future semantic screening may only TIGHTEN outcomes, so controlling
its input gains an attacker nothing.

The approval channel is part of the boundary: approval cards render
kernel-attested facts from the frozen payload; agent-authored
justification is displayed but visibly quarantined; and approval
binds to the recorded bytes — the broker later executes those bytes
exactly, closing the show-one-send-another TOCTOU hole.

Residual, named, accepted: the confused deputy (permitted scope,
hostile content) is narrowed by taint escalation — rules conditioned
on kernel-attested provenance ("anything that read secrets asks
before anything leaves") — plus narrow-by-default grants; and
approval fatigue is the long game the default-ask + narrowest-offer
design exists to win.

## 5. Suspension is the signature interaction

Because suspension is a durable log position (no thread, no memory),
"ask" is not an interruption the architecture tolerates but its
central UX: a headless run hits ask at 3am, suspends for free, and
any client — CLI, phone, a reply to an email — resolves it later with
a signed event. The approval inbox is not a feature; it is a fold
(requested-without-resolved). Denial is fed back to the agent as a
tool result so the run adapts rather than dies. This suspend/resume-
on-consent flow is the runtime's differentiator: the pause between
intent and effect is where trust lives.

## 6. Ecosystem position

A capability is an MCP server plus a signed manifest (RFC-0006):
MCP supplies the tool protocol; the manifest supplies what MCP does
not — scope derivation, effect classes, trust level, vault slots,
owner consent. Model adapters are capabilities under the same format.
The project's durable position is NOT the abstract kernel (the
industry is converging there) but the local-first, single-owner trust
layer: profiles, a keychain-backed credential broker, centralized
permissioning, the approval inbox — systemd-plus-keychain for agents.

## 7. Design tests for future decisions

Most decisions in the register (docs/adr/) reduce to two principles;
test new questions against them first:

  P1  Determinism must be total. If a proposal introduces an input
      the log does not capture, it breaks replay, checkpoints, and
      verification simultaneously.
  P2  The security boundary must not parse. Expressiveness inside the
      gate — pattern languages, embedded code, semantic judgment on
      tainted content — is attack surface, whatever it is called.

And the standing question that keeps the kernel small: *does this
have to be an invariant, or can it be an event producer/consumer?*
Almost everything is the latter.

## 8. Map: concept -> contract

  message schema, blocks, passthrough, adapters ......... RFC-0001
  envelope, taxonomy, chains, blobs, erasure ............ RFC-0002
  principals, scopes, rules, resolution, taint .......... RFC-0003
  plugin classes, purity contract, delivery ............. RFC-0004
  broker pipeline, idempotency, isolation, workers ...... RFC-0005
  manifests, derivation, install ceremony, trust ........ RFC-0006
  adversaries, boundaries, residual risk ................ threat-model.md
  what may change and how ............................... versioning.md
  stages and falsifiable exits .......................... roadmap.md
  decisions D1-D18 with rationale ....................... adr/

## 9. Provenance

Distilled from the design dialogue that produced RFC-0001..0006; the
pre-redesign proposal is preserved unmodified at
docs/history/proposal-original.md for the record of what changed and
why (chiefly: six subsystems became one log; capabilities became MCP
wrappers; Step and Profile left the kernel; "deterministic and
replayable" became a precise event-sourcing guarantee).

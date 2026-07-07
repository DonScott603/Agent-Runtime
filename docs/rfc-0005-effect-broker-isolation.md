# RFC-0005: Effect Broker and Isolation

Status: ACCEPTED — 2026-07-06. Resolved: D12=(a′) platform-tiered
native primitives with opportunistic container escalation;
D13=default suspend-on-uncertain
Change policy: the execution pipeline (§2) and idempotency rules (§4)
are kernel law, frozen after v1. Isolation mechanisms (§6) may be
strengthened at any time; they may never be weakened for an installed
capability without a re-consent ceremony (RFC-0006 §7).

## 1. Purpose and position

The broker is the single doorway between the recorded world and the
real one. It executes exactly the frozen payloads the log has
authorized, binds credentials at the last moment, confines workers to
the resources their granted scopes name, and records every outcome.
The permission gate decides; the broker makes decisions physically
binding. No component other than the broker may perform I/O on behalf
of a run.

## 2. Execution pipeline (frozen)

    effect.proposed        payload frozen in the log (RFC-0002 §3)
    gate.decision          allow -> proceed | ask -> suspend | deny ->
                           result(error: denied, rule_id) fed back to
                           the agent as a tool result
    [approval.resolved]    if ask; signed; bound to this payload only
    effect.executed        broker claims the effect: idempotency key,
                           worker id, isolation descriptor
    effect.result          output or error, with taint provenance
    effect.uncertain       crash window: executed without result (§5)

The broker executes the recorded payload byte-for-byte. It never
accepts a payload from a caller; it discovers work by folding the log
(authorized effects without effect.executed). Approvals are not
portable across runs or forks; a forked timeline re-enters the gate
(architecture doc: replay).

## 3. Credential binding

Payloads never contain secrets, by construction: manifests reference
vault slots (RFC-0006 §5), and the broker resolves slots at worker-
request assembly, after freezing, after the gate, after any approval.
Resolution is keyed (principal, profile, capability, scope); a slot the
scope does not warrant does not resolve. Injection is by the manifest's
declared method (env var, header template, file mount) into the worker
only. Secrets therefore never appear in events, payloads, approval
cards, or model context. The conformance suite scans every recorded
corpus for vault material as a standing check (§10).

## 4. Idempotency

    idempotency_key = H(run_id, effect_proposed.event_id)

The broker persists claim state through effect.executed. Exactly-once
recording is guaranteed; exactly-once world-effect cannot be (no one
can guarantee it), which is why §5 exists. Duplicate resolution events
for one approval are no-ops after the first (log order decides).

## 5. Crash recovery and the uncertain window

### D13 — Re-run classification [RESOLVED: default suspend-on-uncertain]

On broker restart, fold: effects with effect.executed but no
effect.result are in the uncertain window — the world may or may not
have changed. Per-operation manifest field:

    effect_class: safely-repeatable | suspend-on-uncertain

  safely-repeatable      re-execute (reads, searches, idempotent PUTs
                         the capability vouches for)
  suspend-on-uncertain   append effect.uncertain, suspend the run
                         (reason: awaiting_input) with an owner card:
                         "this may or may not have happened — check
                         and resolve"

DECISION REQUIRED: the default for operations that do not declare a
class. Options: default safely-repeatable (convenient, wrong for
send-like operations someone forgot to annotate) vs default
suspend-on-uncertain (annoying for forgotten reads, harmless).

DECISION (accepted): default suspend-on-uncertain. A forgotten annotation
must fail toward a human question, never toward a double-send. The
manifest linter (RFC-0006 §9) flags undeclared classes so defaults are
rare in practice.

## 6. Isolation

### D12 — v1 isolation baseline per platform [RESOLVED: (a′)]

Options:
  (a) Linux-first: Landlock (filesystem), seccomp (syscall), network
      namespace + broker-side egress proxy (network). macOS: subprocess
      with dropped privileges, all network forced through the broker
      proxy, filesystem best-effort — documented as a weaker guarantee
      in the threat model.
  (b) Containers everywhere (uniform guarantees; heavy dependency,
      slow worker start, awkward for a local-first daemon).
  (c) Subprocess-only everywhere at v1 (rejected: makes the gate
      advisory, which the architecture explicitly forbids).

DECISION (accepted): (a′) — option (a) amended with Windows tiering
and opportunistic container escalation:

  Linux    (full)        Landlock (filesystem) + seccomp (syscall) +
                         network namespace with allowlist derived from
                         granted scopes; egress via broker proxy.
  Windows  (v1)          restricted token + Job Objects (rlimits,
                         kill-on-close) + all egress forced through the
                         broker proxy. Documented best-effort.
           (post-v1)     AppContainer confinement — network denied by
                         default (no capabilities granted), filesystem
                         limited to paths ACL'd to the container SID —
                         is the FIRST scheduled hardening milestone,
                         bringing Windows to third-party-tier parity.
           (power path)  running the runtime inside WSL2 yields the
                         full Linux tier today (WSL2 kernels include
                         Landlock); documented with its costs (9P
                         file-bridge performance, Linux-side vault).
  macOS    (v1)          subprocess with dropped privileges, egress via
                         broker proxy only, filesystem best-effort.
                         Documented best-effort.

  Containers (all platforms): opportunistic, never a dependency. If a
  container runtime is detected, the broker MAY escalate workers at
  trust level `untrusted` (or where profile policy demands) into
  containers. Absence of a runtime never blocks operation; presence
  never weakens native confinement — container escalation composes
  with, and is recorded alongside, the native mechanisms.

Rejected: (b) containers-everywhere — per-effect latency is solvable
with warm pools, but the operational identity is not: a hard runtime
dependency (Docker Desktop licensing, 1–4GB idle VM on Desktop
platforms, virtiofs I/O penalties on fs-heavy tools, the root-
equivalent daemon socket, image supply chain) contradicts the
single-binary local-first positioning. Rejected: (c) as before.

Honest asymmetry beats uniform heaviness for a single-owner local
runtime; every platform gets a documented guarantee, never a silent
one. The isolation descriptor recorded in effect.executed names the
mechanisms actually applied, so the audit trail never overstates
confinement.

Trust-level mapping (levels defined in RFC-0006 §6):

    bundled       subprocess, scoped FS, proxied network
    verified      + Landlock/seccomp (Linux), rlimits everywhere
    third-party   + network namespace, empty by default, allowlist
                  derived from granted scopes only
    untrusted     + no network except broker RPC; ephemeral FS;
                  strict rlimits; container-escalated when a runtime
                  is detected (a′)

Resource scoping is derived, never advisory: an fs.read grant on a path
becomes a Landlock rule / mount view; a domain scope becomes a proxy
allowlist entry. The worker cannot reach what the grant did not name.

## 7. Worker protocol

Broker <-> worker over a private channel (unix socket; JSON-RPC framing,
canonical serialization rules of RFC-0001 §5). Request: frozen payload,
resolved credential material, resource descriptors, deadline. Response:
result | error | stream.

Streaming: workers may stream deltas (model tokens, long tool output).
Deltas are ephemeral, fanned out to subscribers (UI, the run loop);
only the final assembled result is the event of record. Timeout or
cancellation (owner action, run cancelled) kills the worker; the effect
lands as effect.result(error: cancelled) if the class is
safely-repeatable, else effect.uncertain.

Limits: per-trust-level rlimits (cpu, memory, disk, wall clock) in the
isolation descriptor; breach = kill + recorded error.

## 8. Model adapters under the broker

Adapters are effectful workers with three broker-side extras: spend
metering (estimated pre-call against model.call budget scopes; actual
recorded post-call), failover policy (retry/fallback chains are broker
configuration; every attempt is recorded so replay attributes the
answering provider), and the raw escape hatch (provider-native
request/response attached as blobs, RFC-0001 §7).

## 9. Inbound communication workers

Watchers (mail, chat, webhooks) are long-lived workers under broker
supervision with one privilege: appending inbound events. They hold no
run context, cannot propose effects, cannot resolve approvals. What an
inbound event wakes or resumes is routing-plugin and policy territory.
Supervision: crash-looping watchers back off exponentially and surface
an owner notification rather than silently dying.

## 10. Conformance suite

  1. kill -9 matrix: broker killed before/during/after worker
     execution; recovery must honor D13 semantics exactly; zero
     double-sends across 10k trials with a sentinel send-like tool.
  2. Idempotency: duplicate approvals, duplicate broker instances
     against one log — exactly one execution claim.
  3. Escape smoke tests per trust level: worker attempts undeclared
     read, undeclared egress, vault access; all must fail at the OS
     layer, not the courtesy layer.
  4. Secret scan: no vault material in any event, payload, or
     approval card across recorded corpora.
  5. Isolation descriptor honesty: mechanisms named in effect.executed
     are re-verified against the platform at audit time.

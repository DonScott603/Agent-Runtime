# RFC-0004: Plugin Interface and Lifecycle

Status: ACCEPTED — 2026-07-06. Resolved: D11 = (a) for pure plugins,
(b) for effectful plugins
Change policy: the pure-plugin contract (§3) is kernel law and frozen
after v1. Individual plugin interfaces (§4) churn freely until 1.0 per
the versioning policy (D19), then version explicitly.

## 1. Purpose

Defines the two plugin classes, the contract every plugin signs, how
plugins are delivered, registered, and invoked, and how invocations are
recorded so that replay and verification hold. The kernel depends only
on the interfaces in §4; no plugin type is load-bearing for kernel
correctness.

## 2. Plugin classes (normative)

A plugin is PURE or EFFECTFUL. The kernel enforces the distinction; it
is not a self-description.

PURE: output is a deterministic function of kernel-provided inputs.
No I/O, no ambient reads, no effects. Runs in-process. Invocations are
recorded and re-executable during verify-mode replay.

EFFECTFUL: touches the world (model calls, tools, communication).
Runs behind the effect broker (RFC-0005) in a supervised worker.
Results are recorded and never re-executed during replay.

There is no third class. A plugin that is "mostly pure" is effectful.

## 3. The pure-plugin contract (frozen)

  P1. Inputs arrive only through the invocation call: folded state,
      event refs, configuration. Nothing is read from the environment.
  P2. Time and entropy are requested through kernel handles. The kernel
      answers AND appends clock.read / rng.seed events (RFC-0002 §3),
      so replay reproduces the answers exactly.
  P3. Output is a value (or a deterministic error value). Panics are
      kernel-caught and terminate the run as run.failed with
      diagnostics; a panic is a plugin bug by definition.
  P4. Message-bearing plugins obey the passthrough law (RFC-0001 §3):
      unknown block types are preserved byte-for-byte unless the plugin
      declares transform capability over that type in its registration.
  P5. Every invocation is recorded as plugin.invoked with the plugin's
      code hash, input refs, and output (or output blob ref). Recorded
      outputs are the outputs of record (RFC on replay: trust mode reads
      them; verify mode re-executes and diffs).
  P6. Determinism is verified, not trusted: the conformance harness
      (§8) runs in CI and verify-replay runs continuously.

## 4. Interface kinds at v1

  ContextProvider      (pure)  folded state -> canonical message list
                               for the next model call
  RoutingPolicy        (pure)  inbound event or handoff -> target agent
  MemorySelector       (pure)  folded state -> memory records to expose
                               (persistence is just events; there is no
                               separate memory write path)
  Reducer              (pure)  view definition per RFC-0002; registered
                               with scope (run | workspace | owner)
  Evaluator            (pure)  recorded run prefix -> scores/annotations
  ModelAdapter         (effectful)  canonical <-> provider dialect;
                               manifest declares supported block types,
                               fidelity classes, downgrades (RFC-0001 §6,
                               RFC-0006 §5)
  ToolProvider         (effectful)  a capability's executable surface;
                               always manifest-bound (RFC-0006)
  CommunicationProvider (effectful) outbound send operations plus a
                               supervised inbound watcher that may only
                               append inbound events (RFC-0005 §9)

Adding interface kinds is additive and cheap. Changing an existing
kind's signature before 1.0 is allowed; after 1.0 it is a versioned
interface with explicit negotiation (§6).

## 5. Delivery

### D11 — Plugin delivery mechanism [RESOLVED: (a) pure, (b) effectful]

Constraint: the runtime is Go. Go's native plugin package is not a
viable option (platform-restricted, toolchain-version-locked, fragile).

Options:
  (a) Compile-time registration: plugins are Go modules registered in
      the binary's build; "installing" a pure plugin means rebuilding.
      Fast, in-process (no microkernel tax on hot paths), trivially
      debuggable. Third parties ship source or the owner uses a
      build service.
  (b) Subprocess plugins over RPC for everything: uniform, dynamic
      install, but puts IPC on the context-assembly and routing hot
      paths and complicates the determinism guarantees (P2 handles
      across a process boundary).
  (c) WASM for pure plugins: dynamic install AND in-process AND
      sandboxed, but adds a runtime dependency and toolchain friction
      at v1.

DECISION (accepted): (a) for pure plugins, (b) for effectful ones — which
costs nothing, because effectful plugins already live behind the broker
as processes (RFC-0005). Revisit (c) at the point third-party pure
plugins are actually in demand; the interface contract in §3 is
delivery-agnostic by design, so the migration is packaging, not
semantics.

## 6. Registration and negotiation

Every plugin registers:

    Registration {
      plugin_id, semver
      class:        pure | effectful
      interfaces:   [{kind, interface_version}]
      transforms:   [BlockType]        // P4 declared transforms
      code_hash:    Hash               // build-time for (a); binary
                                       // hash for (b)
      config_schema: ref
    }

The kernel refuses invocation on interface_version mismatch — no silent
adaptation. The registry is itself folded state: plugin.registered /
plugin.removed events, so "which code could have run at offset N" is
answerable from the log alone.

## 7. Configuration

Plugin configuration is owner-visible data bound at the profile level,
recorded as events, and passed per-invocation (P1). Plugins hold no
retained configuration; two invocations with the same inputs and config
are the same invocation, whatever happened between them.

## 8. Conformance suite

  1. Determinism harness: invoke twice with identical inputs across
     process restarts; byte-compare outputs.
  2. Ambient-state detector: invoke under a canary environment (fake
     kernel clock returning a sentinel, empty env, read-only FS view);
     any divergence or syscall outside the allowlist fails the plugin.
  3. Passthrough injection per RFC-0001 §3.
  4. Panic containment: induced panic must yield run.failed with
     diagnostics and no partial events.
  5. Registry fold: plugin.invoked code_hash must match the registered
     hash for every invocation in the corpus.

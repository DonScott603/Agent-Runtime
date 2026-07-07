# Workplan — Stage 1 decomposed into session-sized packages

Each package is one focused Claude Code session (or less). Rules:
finish a package before starting the next unless deps allow parallel
work; every package ends with its named verification green; commit
messages cite the package id and RFC sections.

Legend: [refs] RFC sections · [verify] what must pass · [deps]

WP-01  Canonical serialization (kernel/canon)
  Canonical(), NFC normalization, float rejection, sorted keys.
  [refs] RFC-0001 §5 · [verify] docs/vectors/canon.json, all cases
  byte-exact + hash-exact; property: serialize twice == identical
  [deps] none. DO THIS FIRST — every hash depends on it.

WP-02  Vector harness (corpus/vectors_test)
  A test runner that loads every docs/vectors/*.json and asserts
  against the implementation; unknown vector files fail loudly.
  [verify] canon cases green via WP-01; others skipped-but-registered
  [deps] WP-01

WP-03  Blob store (vault-lite) — content addressing + per-run keys
  Encrypt-at-write, per-run data key wrapped by a master key (file-
  based at Stage 1; platform keystore lands in Stage 2), hash of
  ciphertext, redaction by key destruction.
  [refs] RFC-0002 §6, D9/D10 · [verify] erasure test: destroy key,
  content unreadable, references intact · [deps] WP-01

WP-04  Log writer + per-run chains (kernel/log)
  Gapless seq assign-on-commit, SealEvent, per-run prev_hash, anchor
  event with Merkle root over run heads, crash-safe append (fsync at
  event boundary).
  [refs] RFC-0002 §2, §4, D4/D5 · [verify] docs/vectors/chain.json;
  kill -9 during append recovers to an event boundary, gapless
  [deps] WP-01

WP-05  Fold machinery + reducers + snapshots (kernel/fold)
  Reducer registration, incremental + rebuild folds, snapshot sidecar
  keyed (reducer hash, schema version, offset), totality (unknown
  types skipped).
  [refs] RFC-0002 §7; architecture: reducers · [verify] fold twice,
  state-hash equal; snapshot invalidation on key mismatch rebuilds
  [deps] WP-04

WP-06  Gate: matchers, Resolve, Derive (kernel/gate, kernel/derive)
  The five matchers, Option-B resolution, taint conditions, closed
  derivation grammar. PURE functions; security-critical path.
  [refs] RFC-0003 §3–§7; RFC-0006 §4 · [verify] resolution.json +
  derivation.json all green; property/metamorphic suite (RFC-0003 §9)
  [deps] WP-01, WP-02 · NOTE: human review required (kernel/gate/CLAUDE.md)

WP-07  Run state machine (kernel/run)
  States, transition events, suspension reasons, single-writer rule.
  [refs] RFC-0002 §3; architecture: state machine · [verify] property:
  no illegal transition reachable; suspended run resumable from log
  [deps] WP-05

WP-08  Message schema + invariant validators (kernel/schema)
  Message/ContentBlock (kernel/types.go shapes), validators
  I1–I5 run on append, passthrough helpers for unknown blocks.
  [refs] RFC-0001 §2–§4 · [verify] validator suite over generated
  corpus; passthrough injection test · [deps] WP-04

WP-09  Kernel handles: Clock/Entropy + ambient-state detector
  Handle implementations recording clock.read / rng.seed; CI detector
  (canary clock + env scan) for plugin code.
  [refs] RFC-0002 §3; RFC-0004 P2, §8.2 · [verify] detector catches a
  planted time.Now() in a test plugin · [deps] WP-04

WP-10  Corpus generator (corpus/gen)
  Seeded scenario -> full synthetic runs; first scenario = the
  annotated trace (docs/trace-annotated.md) reproduced event-for-event
  including its chain head.
  [refs] threat-model D16 · [verify] generated trace-scenario head ==
  36283240ba955ea1… (docs/trace-annotated.md) · [deps] WP-04, WP-07, WP-08

WP-11  Playback broker + trust-mode replay (kernel/replay)
  Effect requests answered from record with position+payload matching;
  reconstruction to any offset; fork = borrowed prefix + new run.
  [refs] architecture: replay · [verify] corpus reconstructs; fork at
  arbitrary offset yields runnable prefix · [deps] WP-05, WP-07, WP-10

WP-12  Verify-mode replay + divergence reporting
  Re-execute pure invocations, diff vs record, classify divergence
  (nondeterminism / version drift / corruption) via recorded hashes.
  [refs] architecture: replay grades · [verify] planted plugin change
  detected and classified as drift; clean corpus = zero divergence
  [deps] WP-09, WP-11

WP-13  Model adapter #1: Anthropic (broker-side worker, Stage-1 shim)
  Canonical <-> provider translation ONLY at Stage 1 (broker isolation
  is Stage 2); manifest with block-type support + downgrades.
  [refs] RFC-0001 §6, D3; RFC-0006 · [verify] round-trip fuzz on
  supported types; unsupported block fails closed · [deps] WP-08

WP-14  Model adapter #2: local (llama.cpp/openai-compatible shim)
  The deliberately feature-poor adapter that exercises capability
  negotiation and declared downgrades.
  [verify] same suite as WP-13; a thinking-block conversation fails
  closed without a declared downgrade, succeeds with one + records it
  [deps] WP-13

WP-15  Context providers x2 (plugins/ctx-default, plugins/ctx-window)
  Two genuinely different strategies (full-history vs windowed) to
  prove the interface.
  [refs] RFC-0004 §4 · [verify] determinism harness; passthrough
  injection; verify-replay green over corpus with both · [deps] WP-09, WP-12

WP-16  CLI development host (cli/)
  run/inspect/replay/fold/approve-stub commands over the library.
  [verify] roadmap E4 end-to-end by hand: suspend, kill, reboot
  container, resume · [deps] WP-07, WP-11

WP-17  Ratchet + CI wiring (make conformance)
  Vector harness + determinism + fold ratchet + secret scan as one
  target; corpus from this version frozen as ratchet generation 0.
  [refs] versioning.md §5 · [verify] a deliberately broken canon
  change is caught by the ratchet · [deps] all above

Stage-1 exit = roadmap E1–E5, which map onto WP-13/14 (E1), WP-10/17
(E2), WP-12 (E3), WP-16 (E4), WP-01 (E5).

Stage 2 decomposition happens when Stage 1 is green — write it then,
informed by what Stage 1 taught; do not pre-plan it now.

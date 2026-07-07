# CLAUDE.md

Agent runtime — a microkernel for hosting AI agents. Log-centric:
state, checkpoints, replay, and audit are read strategies over one
append-only event log.

## Constitution (violating any of these is wrong even if tests pass)

1. The runtime owns WHEN; plugins own HOW.
2. No ambient state: time, entropy, and config enter plugin code only
   through kernel-provided handles, and every read is recorded.
3. Effects reach the world only through the broker; the gate's inputs
   never include model-influenced content except as the payload under
   judgment.
4. The log is append-only and sacred: never rewrite stored bytes;
   evolve by upcaster (docs/versioning.md §4).
5. Contract changes to ACCEPTED RFCs (docs/rfc-*.md) require explicit
   owner instruction and an ADR — never make them unilaterally, even
   to fix a bug. Propose instead.

## Authoritative documents

  docs/architecture.md        the design rationale
  docs/rfc-0001..0006         the contracts (all ACCEPTED; D-numbers
                              are resolved decisions — do not reopen)
  docs/threat-model.md        the tiebreaker for security questions
  docs/versioning.md          what may change and how
  docs/adr/                   one file per decision; add one for every
                              new decision of consequence

## Security-critical paths — HUMAN REVIEW REQUIRED

  kernel/gate/        policy resolution, scope matching
  kernel/derive/      manifest scope derivation
  kernel/canon/       canonical serialization + hashing
  broker/             worker launch, isolation, credential binding
  vault/              key handling, signing

Rules for these paths: keep them small enough to hand-review in full;
no new dependencies without an ADR; never add expressiveness to the
matcher set (RFC-0003 D6) or derivation grammar (RFC-0006 D15); flag
every diff here as needs-human-review in the PR description.

## Working rules

- Read the relevant RFC section before implementing against it; cite
  the section in the commit message (e.g. "RFC-0002 §4: per-run
  chains").
- Conformance suites (§9/§10 of each RFC) are the spec. When behavior
  and suite disagree, the RFC text wins; fix whichever diverges and
  say so.
- Every pure component ships with its determinism test (invoke twice,
  byte-compare) in the same commit.
- Run before declaring done: `make conformance` (suites + corpus
  ratchet + secret scan). A red ratchet means the change breaks the
  past — stop and propose, don't force.
- Corpus files are synthetic-only (threat-model.md D16). Never commit
  anything derived from a real run.
- New decision needed mid-task? Draft docs/adr/ADR-XXXX with options
  and a recommendation, mark PROPOSED, and surface it — don't decide
  silently.

## Go conventions

- No floats in any serialized type (RFC-0001 D2): money as integer
  minor units, time as integer epoch fields.
- Pure-plugin errors are values in the output type; a panic in plugin
  code is a bug by contract (RFC-0004 P3).
- Clock/entropy in plugin and kernel code: accept `runtime.Clock` /
  `runtime.Entropy` handles; never call time.Now()/rand directly
  outside the kernel's handle implementations (CI's ambient-state
  detector will catch it — save the round trip).
- Table tests; property tests (rapid) for resolution, matchers,
  canonicalization, fold determinism.
- Iteration over maps feeding any serialized or hashed output must
  sort keys first.

## Layout

  kernel/     log, fold, state machine, gate, derive, canon, registry
  broker/     pipeline, workers, isolation per platform, vault client
  plugins/    first-party pure plugins (compile-time registered)
  caps/       bundled capability manifests + servers
  cli/        development host (Stage 1)
  daemon/     host (Stage 3; empty until then)
  corpus/     generator + synthetic corpora + golden hashes
  docs/       everything above

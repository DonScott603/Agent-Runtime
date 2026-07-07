# Versioning Policy

Status: ACCEPTED — 2026-07-06, except D17 (proposed).

## 1. The principle

There are two kinds of contract in this project and they age
differently. CODE contracts (Go interfaces, package layout, CLI flags)
bind components that upgrade together and may churn freely before 1.0.
DATA contracts (everything persisted or countersigned) bind against the
past — a log written today must fold correctly forever — and are frozen
from the moment real data exists.

## 2. D17 — The pre-1.0 stability line [PROPOSED]

RECOMMENDATION (adopt as law): **the log is sacred from the first real
run.** Concretely, the following are frozen at v0.1, with evolution
only by the mechanisms in §4:

  S1  Canonical serialization rules (RFC-0001 §5, D2) — every hash in
      existence depends on them; they never change, period.
  S2  Event envelope (RFC-0002 §2).
  S3  Message core: Message/ContentBlock shape, shipped core.* block
      types, invariants I1–I5 (RFC-0001 §2–4).
  S4  Shipped event payload schemas (evolve by upcaster only).
  S5  Chain and anchoring rules (RFC-0002 §4).
  S6  Manifest schema and derivation grammar (RFC-0006) — these are
      countersigned; a schema change invalidates signatures and forces
      re-install ceremonies, so changes are additive and rare.

Everything else — plugin interfaces (RFC-0004 §4), broker worker
protocol, CLI, daemon API, resolution internals — may break in any
0.x minor release. Note the asymmetry this creates on purpose:
gate.decision RECORDS are S4-frozen, while the resolution CODE may
still be tuned pre-1.0; past decisions remain explainable from their
recorded rule ids regardless.

## 3. Semver semantics

  0.x.y   y: fixes. x: may break any CODE contract; may never break a
          DATA contract (S1–S6).
  1.0     the code-contract freeze point: plugin interfaces and the
          daemon API begin explicit versioning + negotiation
          (RFC-0004 §6).
  post-1.0  major = code-contract break. DATA contracts still never
          break; there is no major version that abandons old logs.

## 4. Evolution mechanisms (the only legal ones)

  M1  Additive fields: optional, must-ignore for old readers.
  M2  New types (event types, block types, matcher kinds, transforms):
      open sets by design; old readers skip/passthrough by law
      (RFC-0001 §3, RFC-0002 §2).
  M3  Upcasters: pure vN->vN+1 functions, registered with the kernel,
      applied at fold time, never by rewriting stored bytes,
      maintained forever (RFC-0001 §8).
  M4  Supersession: a shipped type is never removed or re-interpreted;
      it is superseded by a new type and the old one folds forever.
  M5  Promotion (ext.* -> core.*): RFC-level event; the ext form
      remains valid history.

Forbidden, permanently: field removal or re-typing in shipped schemas,
re-purposed enum values, serialization rule changes, log rewriting.

## 5. Enforcement: the corpus ratchet

CI keeps a corpus from EVERY released version (synthetic, per D16) and
requires, on every commit:
  1. every historical corpus folds under HEAD with zero errors
     (upcaster totality),
  2. state hashes for pinned (corpus, reducer) pairs match golden
     values (fold determinism across versions),
  3. canonical-bytes stability: re-serialize historical structures,
     byte-compare (S1 guard),
  4. verify-mode replay of the newest corpus: zero unexplained
     divergence.
A red ratchet is release-blocking. The ratchet is the stability line
made mechanical: you cannot merge a change that breaks the past.

## 6. Deprecation communication

Superseded types and interfaces are marked in code and docs with the
successor and the version of supersession. Nothing marked deprecated
gains new features; nothing shipped is ever deleted from the fold path.

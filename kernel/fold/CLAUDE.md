# kernel/fold — local law

State is a fold (architecture §2); this package is where the log
answers questions. Not on the security-critical list, but it owns the
totality contract and the determinism the ratchet will pin — treat
changes with care.

- Totality is law (RFC-0002 §2): the engine NEVER pre-filters event
  types — every reducer whose scope covers an event receives it (the
  chain verifier wants everything). Unknown payload FIELDS are the
  reducer's must-ignore obligation. `Handles` is a VERSION GATE,
  never a subscription filter.
- ONE Apply loop, two entry points: Rebuild is New + Step, literally.
  Never fork the code paths — live and rebuilt state must be unable
  to disagree by construction.
- Pure package: no os, no clock, no entropy, no new dependencies.
  It must never import kernel/log (the round-trip test in kernel/log
  imports fold; a reverse import is a compile-breaking cycle by
  design — that is the tripwire working).
- Failure is sticky per view-INSTANCE (per (view, run) for run scope;
  per view for owner scope). One reducer's failure is invisible to
  every other view and to other runs of the same view. Error codes
  come from docs/errors.md verbatim (SCHEMA_UNKNOWN_VERSION,
  PLUGIN_CONTRACT, PLUGIN_ERROR) — never ad-hoc strings.
- Rejection atomicity (owner A2, WP-05a): ErrOutOfOrder and any
  pre-Apply engine failure reject BEFORE any view sees the event;
  all view states stay byte-unchanged and the fold remains usable.
- String provenance (owner A1, WP-05a): any string entering hashed
  state or comparable output is built from stable structured facts —
  never err.Error() passthrough, panic text, or library text.
  ViewError.Detail may carry diagnostics; determinism comparisons use
  Codes and state hashes only.
- No floats in reducer state (RFC-0001 D2); state hashing only via
  kernel.Canonical (CanonicalStateHash) — those hashes become ratchet
  material in WP-10/17.
- Upcasters (versioning.md M3) and RFC-0002 §5 signature validation
  are reserved seams in upcast.go, identity/absent at Stage 1. Do not
  implement them here without their work packages.

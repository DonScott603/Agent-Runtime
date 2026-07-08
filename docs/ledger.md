# Ledger — work-package status and owner decision queue

Status of workplan packages (docs/workplan.md) and items awaiting an
owner decision. Updated at each package closeout. (File created at
WP-04a closeout; earlier statuses reconstructed from git history.)

## Completed

- WP-01 Canonical serialization (kernel/canon) — canon.json green
  (commit 1547c0a; ADR-0020 pinned UTF-16 key order in WP-02.1).
- WP-02 Vector harness (corpus) — unknown vector files fail loudly;
  WP-02.1 added NormalizeEnvelope + ADR-0019/0020 (commit 9524f17).
- WP-03 Blob store, vault-lite — blob.json green, erasure test,
  security-review advisories addressed (23a2a0b), human review of
  vault/ completed with no amendments (ADR-0021 D21).
- WP-04a Seal + chain (kernel/log, pure) — 2026-07-07. chain.json
  asserted in the harness; determinism + table + property suites;
  STRETCH achieved: all 20 events extracted from
  docs/trace-annotated.md verify to the documented head 36283240….
  Seal core lives in kernel/seal.go (Go import cycle with the Event
  type; types.go delegates one-line, Canonical-style).

## Next

- WP-04b Durable append (kernel/log, systems) — unblocked by WP-04a.

## Owner decision queue

- /vector-add candidates from WP-04a (behavior chain.json does not
  pin; implemented as described, awaiting vectors):
  1. Mixed-run_id input to VerifyChain is CHAIN_BROKEN at the first
     divergent event (per-run chains, RFC-0002 D4) rather than a
     caller-precondition no-check.
  2. Re-seal: an event recorded WITH a stale non-empty event_id still
     seals to the correct identity (zeroing is unconditional).
  3. A signed-event chain case: chain.json has no sig ≠ null event;
     only the trace (seq 114) covers sig-zeroing today.

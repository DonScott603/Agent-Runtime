# Session Ledger

Operational state between sessions. Updated at every session closeout
(see process.md §8). Prune freely; this file is not frozen.

Snapshot: 2026-07-07, WP-04a closed out.

## Completed

  WP-01   kernel/canon — vectors green, property suites, ADR-0019
  WP-02   vector harness — canon.json asserted
  WP-02.1 interstitial — ADR-0020 (UTF-16 key order, astral golden
          independently verified), NormalizeEnvelope + test,
          ADR-0019 accepted, housekeeping
  WP-03   vault-lite — d3b2b47 + 23a2a0b (advisories); blob.json
          goldens verified by THREE independent derivations (Go impl,
          session reference script, external Python cryptography);
          owner grep triage passed; Put routes blobs through the
          synced writeFileAtomic (amendment 2 satisfied)
  Workplan split — WP-04→04a/04b/04c, 05→05a/05b, 06→06a/06b;
          downstream deps re-pointed (08, 09 → 04b; 11 → 05a);
          harness skip labels retagged
  WP-04a  seal + chain — dafe217 + 6e67683 (kernel/log local law);
          chain.json asserted; determinism, table and rapid property
          suites; goldens pre-verified by an independent session
          script before implementation. In-flight checks from the
          launch entry, all green:
            1. chain.json flipped skipped→asserted, conformance GREEN
            2. STRETCH green: the 20 trace events extracted from
               docs/trace-annotated.md verify to head 36283240ba955ea1…
               — vectors, trace, and code now agree independently on
               seal/chain semantics before any byte persists
            3. Judgment probe: session reasoned rather than pattern-
               matched — argued kernel/log is not on the critical list
               but owns the append-only property once 04b persistence
               lands, and wrote local law on that basis
          Deviation (approved): seal core in kernel/seal.go, not
          kernel/log — Go import cycle with the Event type; types.go
          delegates one-line, Canonical-style.

## In flight

  Nothing. Next session launches WP-04b.

## Owner action items

  [x] ADR-0016 / ADR-0017 flips committed as ACCEPTED (bd30c37).
      0017 ("the log is sacred from v0.1") is in the record ahead of
      WP-04b's first persisted event, as required.
  [x] ADR-0021 status line carries the human-review-completed date
      (daa5521).
  [x] ADR-0019's Unicode-tables claim locally verified 2026-07-07 and
      CORRECT as written: x/text@v0.39.0/unicode/norm has
      tables15.0.0.go (//go:build !go1.27, Version = "15.0.0") and
      tables17.0.0.go (//go:build go1.27, Version = "17.0.0");
      toolchain go1.26.1 → 15.0.0 tables active. No ADR edit needed.
  [ ] /vector-add candidates from WP-04a (behavior chain.json does
      not pin; implemented as described, awaiting owner decision):
        1. Mixed-run_id input to VerifyChain is CHAIN_BROKEN at the
           first divergent event (per-run chains, RFC-0002 D4) rather
           than an unchecked caller precondition.
        2. Re-seal: an event recorded WITH a stale non-empty event_id
           still seals to the correct identity (zeroing is
           unconditional).
        3. A signed-event chain case: chain.json has no sig ≠ null
           event; only the trace (seq 114) covers sig-zeroing today.

## Next up

  WP-04b (durable append) — THE critical-path package: 05a, 08, 09, 10
  all depend on it. Fresh session; kill-9 matrix tests written before
  recovery code; SEQ_GAP refusal; fsync at event boundary. Then 04c
  (anchor) any time before the chain-verifier reducer; then 05a.

## Standing context for a new assistant

  Read in order: docs/architecture.md → docs/process.md → this ledger
  → docs/workplan.md. The RFCs are contracts (win on conflict);
  threat-model.md is the security tiebreaker; ADR register is
  docs/adr/README.md (D1–D21 resolved except any listed above).
  The owner's review method is process.md §6; the golden-file rituals
  §5 are the most load-bearing habits — hold them.

# Session Ledger

Operational state between sessions. Updated at every session closeout
(see process.md §8). Prune freely; this file is not frozen.

Snapshot: 2026-07-08, WP-04b.1 closed out.

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
  WP-04a.1 micro-session — the three accepted /vector-add candidates
          landed in chain.json as additive cases (owner-accepted, no
          ADR needed): verify_cases/verify-mixed-run (behavioral,
          resolution.json precedent), seal_cases/reseal-stale-id,
          runs/signed-event-chain. Goldens computed by an independent
          throwaway script implementing the _rules (deleted after);
          harness extended (assertChainRun + the three sections);
          diff on chain.json additions-only (136+/0-); conformance
          GREEN. Interpretations the RFC text left open, now
          vector-pinned: mixed-run input is CHAIN_BROKEN at the first
          divergent event (not a caller precondition); the error
          names the DIVERGENT event's run_id; events[0] defines the
          chain's run_id.

  WP-04b  durable append — kernel/log store.go/record.go/recover.go
          (ADR-0022, PROPOSED): single-file events.log, CRC-framed
          canonical event bodies, WAL-truncate torn tails with durable
          (synced) truncation, LOG_CORRUPT/SEQ_GAP refusal leaving
          bytes untouched. TDD order held: full matrix written against
          panic stubs, failure wall recorded, then implemented.
          Verification, all green (/conformance GREEN):
            1. Fault matrix: 9 named crash points × {empty store,
               after-2-commits} through the writeSyncer seam — zero
               committed-event loss, recovery to an event boundary,
               gapless continuation, reopen-stable; poisoned-store
               refusal (ErrStorePoisoned) folded into every case.
            2. Recovery surgery: SEQ_GAP refusal on a synthetic gap;
               mid-file corruption refuses with bytes untouched; T3/T4
               tails truncate; owner A2 (sub-magic split) and A3
               (truncate-then-sync ordering, asserted via recording
               seam) both tested.
            3. Round-trips byte-identical + re-seal green: chain.json
               top-level run (head 9d1be79f…), signed-event-chain (sig
               survives verbatim), the 20 trace events at base 101
               (head 36283240ba…, next Append = seq 121); Append on a
               virgin store reproduces chain.json event_ids and head
               end-to-end.
            4. rapid model-based append/crash/recover property; record
               encoder determinism; two-stores-byte-identical-files.
            5. Real-process kill capstone (RFC-0002 §9.3): 3 random-
               point kills per run, recovered lastSeq >= last acked in
               every iteration (observed acked+1 cases = the ADR-0022
               accepted-unacked semantics, working as specified).
          Deviations (minor, flagged): stale package-doc line in
          chain.go updated alongside the approved ChainBrokenError
          rider; writeSyncer seam carries Truncate (recovery and A3
          assertion need it); mid-file CRC flip classifies as C3 not
          C2 (same refusal behavior, taxonomy label only).

  WP-04b.1 interstitial — ADR-0022 flipped ACCEPTED (owner,
          2026-07-08; implementation review confirmed the fault
          matrix faithful to the taxonomy, C3 reading of mid-file CRC
          anomalies correct) + the owner-approved taxonomy
          clarification sentence; kernel/log store.go/recover.go
          added to the security-critical list (root CLAUDE.md,
          threat-model §8 O1, local law reconciled) — as of WP-04b
          the package physically owns constitution #4, and recovery
          truncation is the one place committed events could die by
          code. errors.md LOG_CORRUPT row verified verbatim against
          the ADR (no edit). Doc-only; conformance GREEN.

## In flight

  Nothing. Next session launches WP-05a.

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
  [x] /vector-add candidates from WP-04a: all three accepted by the
      owner and landed as additive chain.json cases (WP-04a.1).
  [x] ADR-0022 ACCEPTED (owner, 2026-07-08, WP-04b.1) after
      implementation review. Register now 22/22 ACCEPTED, zero
      pending. The A1 attestation requirement on WP-04c stands in the
      ADR's Consequences.

## Next up

  WP-05a (fold), then WP-04c (anchor; must land before the
  chain-verifier reducer). NOTE for 04c: it inherits the A1
  attestation requirement stated in ADR-0022 Consequences — the
  anchor attests, per container, the base seq and first event_id
  alongside run heads; the any-base recovery rule is accepted only
  because anchoring closes front-truncation.

## Standing context for a new assistant

  Read in order: docs/architecture.md → docs/process.md → this ledger
  → docs/workplan.md. The RFCs are contracts (win on conflict);
  threat-model.md is the security tiebreaker; ADR register is
  docs/adr/README.md (D1–D22, all ACCEPTED, zero pending).
  The owner's review method is process.md §6; the golden-file rituals
  §5 are the most load-bearing habits — hold them.

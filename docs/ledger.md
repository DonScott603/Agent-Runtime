# Session Ledger

Operational state between sessions. Updated at every session closeout
(see process.md §8). Prune freely; this file is not frozen.

Snapshot: 2026-07-08, WP-04c closed out.

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

  WP-05a  fold core — kernel/fold (engine, registry, ViewError,
          CanonicalStateHash, upcast/sig-validation seams reserved) +
          plugins/runstatus + plugins/chainverify + local laws
          (kernel/fold/CLAUDE.md, plugins/CLAUDE.md). ADR-0023
          (reducer identity: declared plugin_id+semver, hash-shaped)
          PROPOSED — recommendation (a) endorsed at plan approval,
          formal acceptance at implementation review per pattern.
          Owner amendments A1 (string provenance: fixed vocabulary +
          structured fields in hashed state; chainverify alarm bytes
          asserted exactly), A2 (rejection atomicity: ErrOutOfOrder
          before any view sees the event; byte-unchanged states
          tested), A3 (ADR revisit triggers: WASM delivery; first
          persisted plugin.registered/invoked) all folded in and
          tested. TDD order held (tests against panic stubs, failure
          wall recorded, then implemented). Verification, all green
          (/conformance GREEN):
            1. Engine: no-pre-filter probe (Handles is a version
               gate, never a filter), scope routing (run_id "" never
               reaches run instances), per-view-INSTANCE sticky
               failure, SCHEMA_UNKNOWN_VERSION / PLUGIN_CONTRACT /
               PLUGIN_ERROR classification, panic containment,
               IdentityHash pinned to independently derived goldens
               (sha256sum outside Go).
            2. Determinism: fold-twice hash equality; rapid property
               incremental == rebuild over random sequences (unknown
               types, gate trips, mixed runs incl. ""), every (view,
               run) hash and failure code.
            3. chainverify == klog.VerifyChain rapid consistency
               (alarm iff error, same first run+seq) — the
               incremental and batch verifiers welded together.
            4. Round-trip integration (kernel/log/fold_roundtrip_
               test.go, internal package as approved — doubles as
               the import-topology tripwire; store.go/recover.go
               untouched): 20 trace events at base 101 through
               recovery -> fold: run-status completed via
               suspended(awaiting_approval) waypoint (prefix rebuild
               + step remainder == full fold), chainverify head ==
               36283240ba955ea1…, then a store-appended run.suspended
               v99 (seq 121) makes run-status unavailable
               (SCHEMA_UNKNOWN_VERSION) while chainverify advances
               over the same event.
          Readings recorded (plan approval 2026-07-08): workplan
          05a's "totality (unknown types skipped)" is reducer-side
          must-ignore, never engine pre-filtering; run_id=="" events
          reach no run-scoped instance (scope routing is WHO sees an
          event; totality is what a seeing reducer must tolerate);
          RFC-0002 §5 sig validation deferred with the seam reserved
          in kernel/fold/upcast.go.

  WP-05a.1 interstitial — ADR-0023 flipped ACCEPTED (owner,
          2026-07-08; implementation review confirmed the
          IdentityHash preimage matches the independently derived
          goldens) + ruling propagation to target files: errors.md
          PLUGIN_CONTRACT read-path note, workplan WP-04c
          anchor-verify obligation (chainverify extension + ADR-0023
          semver bump), root CLAUDE.md conformance fallback
          (`sh scripts/conformance.sh` where make is absent),
          process.md §4 paste-back sentence. Doc-only; conformance
          GREEN.

  WP-04c  anchor event — kernel/anchor.go (Merkle construction +
          payload types, shared by store and verifier),
          kernel/log/anchor.go (WriteAnchor), store.go base-tracking
          + appendLocked extraction, plugins/chainverify 0.1.0→0.2.0
          (anchor verification; first real ADR-0023 bump, identity
          golden pinned via sha256sum), docs/vectors/anchor.json
          (ALLOW_FROZEN additive creation; goldens by independent
          Python script + printf|sha256sum hand-lane; astral-ordering
          and NFC discriminator cases carry the WRONG-implementation
          roots in their notes). ADR-0024 PROPOSED (rulings + riders
          R1/R2 folded; formal acceptance at implementation review
          per pattern). errors.md ANCHOR_MISMATCH row (ADR-0024).
          Workplan rider: moot scheduling sentence deleted.
          Verification, all green (/conformance GREEN, anchor.json
          asserted):
            1. Vectors: 6 root cases + payload case (canonical bytes
               + sha256 pin the payload schema itself); three
               derivation lanes agreed before the Go implementation
               ran (script, sha256sum chain, then Go).
            2. Store: empty-store refusal (plain error, not poisoned,
               next Append seq 1), envelope/payload/base contract,
               base 101 attestation through recovery (trace),
               tamper-flips-root (workplan verify), anchors chain
               through "" (second anchor prev = first), reopen
               byte-identical.
            3. chainverify: happy anchored fold; tampered head /
               wrong base seq / wrong first event_id → exactly one
               ANCHOR_MISMATCH each (anchor_root / anchor_base) with
               expected/got structured fields; Broken untouched,
               heads keep advancing; anchor-as-first-event alarms
               anchor_base (fixpoint corollary, tested);
               {"heads":null} lands anchor_payload (gate-first);
               in-run anchors checked uniformly; v2 anchor →
               SCHEMA_UNKNOWN_VERSION (R2); A1 exact-bytes green
               with base in state.
            4. Properties: weld rapid extended (CHAIN_BROKEN iff
               VerifyChain, same first (run,seq); vocabularies never
               cross codes — ruling rider; wrongRoot anchor alarms
               exactly when reached; incremental==rebuild with
               anchors); AnchorRoot determinism; canon-order
               agreement rapid; anchored trace end-to-end
               (TestFoldRoundTripAnchoredTrace).
          Deviations (flagged in commit): Merkle construction in
          kernel/anchor.go not kernel/log (import topology; seal.go
          precedent — owner-accepted at plan approval); leaf order
          DERIVED from Canonical's emitted key order instead of a
          duplicated comparator (divergence impossible by
          construction; ADR-0024 wording updated);
          TestUnknownTypesThreadChain fixture swapped anchor.appended
          for ext.vendor.custom (the type is no longer unknown to
          this reducer — that is the point of 0.2.0).

## In flight

  Nothing. ADR-0024 awaits owner acceptance at implementation review
  (process.md §6; kernel/log diffs need human review — store.go
  gained base-tracking + appendLocked extraction, anchor.go is new
  append-path code).

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
      implementation review. The A1 attestation requirement on WP-04c
      stands in the ADR's Consequences.
  [x] ADR-0023 ACCEPTED (owner, 2026-07-08, WP-05a.1) after
      implementation review confirmed the IdentityHash preimage
      against the independently derived goldens.
  [ ] ADR-0024 (anchor payload schema + Merkle construction) awaits
      acceptance at implementation review. Review focus per
      threat-model O1: kernel/log/store.go (base-tracking,
      appendLocked extraction), kernel/log/anchor.go (WriteAnchor),
      kernel/anchor.go (the frozen construction).

## Next up

  WP-05b or WP-06a per workplan deps (both unblocked). An
  interstitial WP-04c.1 should flip ADR-0024 after the owner's
  implementation review (the 0022/0023 pattern).

## Standing context for a new assistant

  Read in order: docs/architecture.md → docs/process.md → this ledger
  → docs/workplan.md. The RFCs are contracts (win on conflict);
  threat-model.md is the security tiebreaker; ADR register is
  docs/adr/README.md (D1–D23, all ACCEPTED, zero pending).
  The owner's review method is process.md §6; the golden-file rituals
  §5 are the most load-bearing habits — hold them.

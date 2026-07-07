# RFC-0002: Event Grammar

Status: ACCEPTED — 2026-07-06. Resolved: D4=(b), D5=gapless seq,
D9=(b) per-run keys wrapped by owner master key, D10=(b)
Change policy: envelope is additive-only after v1. New event types are
cheap; envelope changes are ABI changes.

## 1. Purpose

Everything the runtime does is an append to a per-owner event store. This
RFC defines the envelope every event shares, the taxonomy of event types,
the hash-chain and anchoring rules that make the log tamper-evident, blob
referencing, and erasure semantics. State, checkpoints, replay, and audit
are all read strategies over structures defined here.

## 2. Envelope (frozen after v1)

    Event {
      seq:          u64            // global, per-owner, gapless (D5)
      run_id:       RunId | null   // null for owner-scope events (policy)
      event_id:     Hash           // hash of this envelope, see §4
      prev_hash:    Hash           // previous event in this run's chain (D4)
      ts:           Timestamp      // kernel wall clock, informational (D5)
      mono:         u64            // kernel monotonic counter
      principal:    PrincipalId    // kernel-assigned author of the event
      type:         EventType      // namespaced, see §3
      type_version: u16
      payload:      canonical bytes (RFC-0001 §5 rules)
      blobs:        [BlobHash]     // content-addressed references
      sig:          Signature|null // required for consent/policy types §5
    }

Totality rule: every reducer and every reader MUST skip event types and
payload fields it does not recognize. Forward compatibility is a reader
obligation, not a writer courtesy.

## 3. Event taxonomy (initial set; extensible)

  run.created | run.started | run.suspended(reason) | run.resumed
  run.completed | run.failed(error_ref) | run.cancelled

  msg.appended            { message (RFC-0001) or body-blob ref, see D10 }

  effect.proposed         { frozen payload: capability, operation, input }
  gate.decision           { effect_ref, action: allow|ask|deny,
                            rule_id, rule_provenance, derived_scopes }
  approval.requested      { effect_ref, scopes, expiry }
  approval.resolved       { request_ref, decision, resolver_principal }  [signed]
  approval.expired        { request_ref }
  effect.executed         { effect_ref, idempotency_key, broker_worker }
  effect.result           { effect_ref, output|error, taint }
  effect.uncertain        { effect_ref, reason }   // crash before result

  policy.granted          { rule }                                       [signed]
  policy.revoked          { rule_id }                                    [signed]

  plugin.invoked          { plugin_id, code_hash, input_refs, output|ref }
  clock.read              { value }     // when a plugin requested time
  rng.seed                { seed }      // when a plugin requested entropy

  artifact.created        { blob_hash, media_type, producing_effect_ref }
  redaction.applied       { blob_hashes, reason }                        [signed]

  agent.handoff           { from_run, to_run, payload_ref }

Naming rule: `<domain>.<verb-or-noun>`; new domains are open; verbs within
a shipped domain are additive-only.

## 4. Hash chain and anchoring

event_id = H( canonical(envelope with event_id and sig fields zeroed) ).
Each event's prev_hash is the event_id of its predecessor.

### D4 — Chain topology [RESOLVED: (b)]

Options:
  (a) One global chain over seq order. Simple; but concurrent runs
      serialize all appends through one hash dependency, and verifying
      one run requires walking the owner's whole history.
  (b) Per-run chains (prev_hash within run_id), plus the global seq for
      cross-run order, plus a periodic anchor event containing a Merkle
      root over all current run heads + latest owner-scope event.
  (c) Per-run chains, no global anchor (rejected: cross-run deletion of
      an entire run becomes undetectable).

DECISION (accepted): (b). Runs verify independently and in parallel; the
anchor event is what gets signed/exported off-machine (RFC discussion:
audit). Anchor cadence is configurable; it bounds the undetectable-
rewrite window.

### D5 — Ordering and time [RESOLVED: gapless]

  - seq is assigned by the single log-writer (library mode: the process;
    daemon mode: the log service). seq order is THE order; reducers whose
    output depends on cross-run ordering MUST fold in seq order.
  - ts is wall-clock, informational, never load-bearing (clocks jump).
  - mono is load-bearing for intra-process causality assertions.
  - Plugins never read time or entropy ambiently: they request it, and
    the kernel answers AND records clock.read / rng.seed events, so
    replay reproduces the answers.

DECISION (accepted): seq is gapless, assigned on commit — simpler
crash-recovery reasoning. Rejected: merely-monotonic assign-on-receive;
the log-writer is not the bottleneck in a system gated on model latency.

## 5. Signatures

Events of consent or authority — approval.resolved, policy.granted,
policy.revoked, redaction.applied — carry a signature by the resolving
principal's key, held in the credential vault (or platform keystore),
never readable by agent or tool principals. Verification is part of fold:
a consent-type event with a missing/invalid signature is treated as
absent by every reducer (and surfaced as an integrity alarm).

## 6. Blobs

Blobs are content-addressed (hash of ciphertext), stored outside the
event store, referenced by hash from envelopes and payloads. Events are
small; anything large or personal is a blob.

Encryption: every blob is encrypted at write.

### D9 — Erasure key granularity [RESOLVED: (b), with (a) on demand]

Erasure = destroy the key, append redaction.applied. Granularity options:
  (a) per-blob keys (maximal precision; key store grows with data)
  (b) per-run keys (erase a run's content in one operation)
  (c) per-source keys (e.g. everything ingested from gmail under one key)

DECISION (accepted): (b) with (a) available on demand: per-run data keys
wrapped by an owner master key; individual-blob erasure re-wraps the
run key excluding the target. Decide now — retrofitting encryption
into a plaintext blob store is a migration; changing key granularity
later is merely a re-wrap.

### D10 — Message bodies: inline or blob [RESOLVED: (b)]

msg.appended and effect.result carry the bulk of personal content.
Options:
  (a) Inline in payload (simple; but erasure of one message's text
      requires redacting an event, complicating the chain story).
  (b) Bodies always in blobs; the event payload holds structure
      (roles, block types, ids, hashes) but text bodies are blob refs.

DECISION (accepted): (b). It makes erasure uniform (§D9), keeps the event
store small and fast to fold, and means chain verification never
touches personal content. Cost: one indirection on every read; mitigate
with a body cache. This decision is near-impossible to reverse cheaply.

## 7. Snapshots (non-normative reminder)

Snapshots are not events. They are sidecar caches keyed by
(reducer code hash, event schema version, run_id or scope, offset),
invalid on any key mismatch, rebuildable from the log at any time.

## 8. Compaction and retention

A run in a terminal state may be compacted: final per-reducer snapshots
retained hot; raw events moved to cold storage; blob keys retained until
erasure. Compaction never rewrites history — cold events remain part of
the chain and are fetched when a rebuild or forensic fold needs them.

## 9. Conformance suite

  1. Fold determinism: fold every corpus twice, compare state hashes.
  2. Chain verification as a standing background reducer.
  3. Crash-fault injection on the writer: kill -9 during append;
     recovered log must end at an event boundary, gapless seq.
  4. Upcaster totality over historical corpora.
  5. Erasure test: destroy a key, assert content unreadable, chain
     still verifies, redaction event present and signed.

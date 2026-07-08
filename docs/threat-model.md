# Threat Model

Status: ACCEPTED — 2026-07-06, except D16 (proposed) and the standing
review obligations in §8. This document transcribes the security
posture the architecture and RFC-0001..0006 already commit to; nothing
here introduces new mechanisms. When a design question arises, this
document is the tiebreaker.

## 1. Assets

  A. The event log and blob store — everything the owner's agents ever
     saw or did. Confidentiality (encryption at rest, D9/D10),
     integrity (hash chains + anchor, D4), and erasability (crypto-
     erasure) are all load-bearing.
  B. Vault secrets — credentials for the owner's accounts.
  C. Consent integrity — the guarantee that every effect traces to an
     owner-signed policy rule or approval (RFC-0002 §5, RFC-0003).
  D. The host machine — the runtime must not be a privilege-escalation
     vector onto the box it runs on.

## 2. Principals and standing trust

  owner            trusted; the root of all authority (signing keys).
  agent:*          UNTRUSTED. Prompt injection means any agent may be
                   under adversarial control at any moment. This is a
                   design axiom, not a contingency.
  capability:*     untrusted, graduated by trust level (RFC-0006 §6),
                   confined by the broker (RFC-0005 §6).
  pure plugins     trusted CODE at v1 — see §7(R6). Compile-time
                   registration (D11) means they run in-process; the
                   purity contract is a correctness discipline enforced
                   by CI, not a security boundary.
  effectful plugins  untrusted; broker-confined workers.

## 3. Adversaries in scope

  T1  Injected agent: model output steered by hostile content in tool
      results, web pages, email, or documents.
  T2  Malicious or compromised capability: an MCP server that lies,
      exfiltrates, or attacks the host.
  T3  Supply chain / substitution: a swapped binary or edited manifest
      standing in for an installed capability.
  T4  Hostile remote endpoints: servers a worker legitimately talks to,
      returning hostile payloads (a special case of T1/T2 taint).
  T5  Post-incident historical tampering: something that gained write
      access to the store attempting to rewrite what happened.

## 4. Adversaries explicitly OUT of scope

  X1  Root-compromise-forward: an attacker with current administrator
      control of the host can write well-formed future events, read
      RAM, and defeat any local mechanism. The design bounds what they
      can FORGE about the past (anchored chains, signatures) and what
      standing secrets they harvest (vault in platform keystore), but
      does not claim to operate securely on an owned box.
  X2  The owner as adversary: single-owner system; the owner rewriting
      their own history is not an attack (the external anchor makes it
      non-silent, which is sufficient).
  X3  Hardware/side channels between co-resident workers.

## 5. Boundary -> mechanism map

  Influence boundary   gate inputs limited to owner-signed policy,
                       kernel identity, manifest-derived scopes;
                       model-influenced content enters only as the
                       payload under judgment (RFC-0003; arch. doc).
                       Counters T1.
  Effect broker        single doorway; OS-enforced confinement per
                       trust level (RFC-0005 §6, D12 a'). Counters T2,
                       and T4 via taint provenance on all results.
  Manifest ceremony    publisher signature + owner countersignature of
                       the manifest hash; substitution -> refusal
                       (RFC-0006 §7). Counters T3.
  Vault                slots resolved post-freeze, post-gate, injected
                       into workers only; secrets structurally absent
                       from payloads, events, context, approval cards
                       (RFC-0005 §3). Counters T1/T2 exfil of B.
  Log integrity        per-run hash chains, anchored Merkle head,
                       signed consent events (RFC-0002 §4-5).
                       Counters T5.
  Taint escalation     provenance conditions tighten egress after
                       sensitive reads (RFC-0003 §7). Narrows the T1
                       confused-deputy channel.

## 6. Guarantees by platform (D12 a')

  Linux        full: Landlock + seccomp + netns; grants are OS rules.
  Windows v1   best-effort: restricted token + Job Objects + proxy-only
               egress. AppContainer parity is the first post-v1
               hardening milestone; WSL2 documented as the immediate
               full-tier path.
  macOS v1     best-effort: dropped privileges + proxy-only egress.
  All          container escalation for `untrusted` when a runtime is
               detected; never a dependency. effect.executed records
               the mechanisms ACTUALLY applied — the audit trail never
               overstates confinement.

## 7. Residual risks (accepted, named, monitored)

  R1  Confused deputy: permitted scope, hostile content (exfiltration
      inside an allowed send). Mitigated, not eliminated, by taint
      escalation, narrow grants, attested approval cards.
  R2  Approval fatigue: the human is the fallback control and humans
      habituate. Mitigated by narrow-by-default "approve always"
      offers and default-ask making noise visible rather than silent.
  R3  Social engineering via model prose adjacent to approval UI.
      Mitigated by strict visual separation of kernel-attested facts
      from agent-authored text; never eliminated.
  R4  Weaker FS confinement on macOS and v1 Windows (§6): a hostile
      worker there is limited by privilege drop and proxy-only
      network, not by kernel-enforced path rules.
  R5  Warm/reused workers: any worker reuse within a capability is a
      cross-run contamination surface; reuse policy must be conscious
      and recorded.
  R6  In-process pure plugins are kernel-trust at v1 (D11): a
      malicious pure plugin is a compromise, full stop. Accepted
      because v1 pure plugins are first-party source in-repo;
      third-party pure plugins are DEFERRED until a sandboxed
      delivery (WASM) exists. This line is policy: no third-party
      code compiles into the kernel binary.

## 8. Standing obligations

  O1  Security-critical directories (gate, broker, vault, derivation,
      canonical serialization, and kernel/log's durable store +
      recovery — store.go, recover.go, as of WP-04b) require human
      review on every change; keep them small enough that full review
      is feasible (CLAUDE.md).
  O2  The conformance suites of RFC-0001..0006 §9/§10 run in CI and
      are release-blocking.
  O3  Escape smoke tests (RFC-0005 §10.3) run per platform per release.

## 9. D16 — Test corpus governance [PROPOSED]

Replay CI requires recorded runs; recorded runs contain real personal
content. Rules:
  1. Repository and CI corpora are SYNTHETIC ONLY, produced by the
     corpus generator (a Stage-1 deliverable) from seeded scenarios.
  2. Real-run replay happens only on the owner's machine, never in CI,
     never in the repo.
  3. A secret/PII scan gates every commit touching corpus paths.
  4. Corpora from every released version are retained forever as the
     upcaster ratchet (versioning.md §5).

RECOMMENDATION: accept as written; the only cost is building the
generator early, which verify-replay needed anyway.

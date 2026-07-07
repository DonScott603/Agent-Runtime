# Roadmap

Stages ship in order; each has falsifiable exit criteria. "Done" is a
green criterion, not a feeling. Post-stage hardening items are queued,
not optional.

## Stage 1 — Kernel as a library + CLI host

Build: canonical serialization (kernel/canon), event log with per-run
chains + gapless seq, blob store with per-run wrapped keys, fold/
reducer machinery + snapshots, run state machine, message schema +
validators, plugin registry, corpus generator, CLI host, TWO model
adapters, TWO context providers, playback broker + verify-mode replay.

Exit criteria:
  E1  Both adapters pass round-trip fuzz (RFC-0001 §9.1) and the
      passthrough injection suite.
  E2  Synthetic corpus generator produces multi-run corpora covering
      every shipped event type; corpus ratchet (versioning.md §5) is
      wired and green.
  E3  Verify-mode replay of a 50-run synthetic corpus: zero
      unexplained divergence; injected plugin-hash change is detected
      and correctly attributed.
  E4  A run suspended mid-turn survives process kill and machine
      reboot, then resumes to completion from the log alone.
  E5  Canonical bytes cross-checked against an independent JCS
      implementation on a fuzz corpus.

## Stage 2 — The trust layer

Build: vault (platform keystore backed), signing (owner key, consent
events), gate (five matchers, Option-B resolution, taint conditions),
manifest install ceremony + validator, effect broker (pipeline,
idempotency, D13 recovery), Linux isolation tier, approval flow in the
CLI, hash-chain background verifier + anchor export.

Exit criteria:
  E6  Resolution property suite green (RFC-0003 §9), including
      hostile-value corpus and metamorphic tests.
  E7  kill -9 matrix: zero double-sends in 10k trials with the
      sentinel send tool; unannotated operations suspend (D13).
  E8  Escape smoke tests on Linux: undeclared read, undeclared
      egress, vault reach — all fail at the OS layer.
  E9  Secret scan: no vault material in any event/payload/approval
      card across the corpus.
  E10 End-to-end ask: headless run hits ask, suspends; approval via a
      second CLI session resumes it; denial feeds back as a tool
      result and the run adapts.
  E11 Manifest substitution test: edited manifest/binary refused for
      every operation (RFC-0006 §9.5).

## Stage 3 — Daemon + clients

Build: daemon host (supervision, scheduler, log-shaped local API:
append, subscribe, resolve), inbound communication workers, approval
inbox as an API query, first non-CLI client.

Exit criteria:
  E12 Daemon crash/restart mid-run loses nothing; resumes per E4
      semantics under supervision.
  E13 An inbound event (mail/webhook) resolves an awaiting_input
      suspension end-to-end.
  E14 Approval round-trip from a remote client in under 5 seconds
      from event append to resume.

## Stage 4 — Multi-agent

Build: agent registry semantics, agent.handoff flow, routing plugins,
cross-run reducers at owner scope (global inbox, spend).

Exit criteria:
  E15 A handoff between two agents with disjoint capability grants:
      the receiving run cannot exercise the sender's grants (gate
      evidence in the log).
  E16 Owner-scope reducers (inbox, spend) fold correctly across 100+
      concurrent synthetic runs in seq order.

## Post-v1 hardening queue (ordered)

  H1  Windows AppContainer tier (D12 a' milestone) + escape smoke
      tests on Windows.
  H2  Container escalation for `untrusted` when a runtime is detected.
  H3  WASM delivery for third-party pure plugins (lifts threat-model
      R6 restriction).
  H4  Anchor automation (scheduled off-machine export) + optional
      transparency-log publishing.
  H5  Partial resume with approved-subset semantics for parallel
      pending approvals.

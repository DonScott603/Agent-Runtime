# Process — the working method

Status: ACCEPTED — 2026-07-07. This document captures HOW this project
is built, complementing CLAUDE.md (working rules for the coding agent)
and architecture.md (design rationale). It is written for both the
owner and any AI assistant advising the owner. The method below was
evolved and validated across WP-01..WP-03; treat deviations as
decisions worth a sentence of justification, not as violations.

## 1. Session discipline

- One work package per session, fresh session each time. Repo docs
  re-anchor every session for free; long sessions accumulate drift.
- Plan mode first for every implementation session; the owner approves
  the plan before any file is written.
- One verification mode per session (vectors OR property suite OR
  fault-injection). The workplan splits (WP-04a/b/c, 05a/b, 06a/b)
  exist to enforce this; keep the principle when writing new packages:
  a session juggling two verification modes lets the second go shallow
  while the first goes green.
- Security-critical packages produce diffs small enough to hand-review
  in one sitting. If a plan cannot promise that, split the package.
- End every session with /conformance GREEN and a commit citing the
  package id and RFC sections. A surfaced ambiguity is resolved IN THE
  REPO (vector case or ADR) before the next session starts — never
  only in a chat transcript.

## 2. The work-package prompt shape

Every implementation prompt has the same skeleton; only three things
change (package id, reading list, verify line):

  1. "Implement WP-NN from docs/workplan.md. Plan first."
  2. An ordered reading list: the WP entry, its [refs] RFC sections,
     the relevant docs/vectors file, the types/stubs touched, and the
     NEXT package's entry so out-of-scope is explicit.
  3. Task statement with explicit OUT-OF-SCOPE lines ("if you find
     yourself importing os, stop").
  4. TDD order: harness/tests first against the stub, watch them
     fail, then implement.
  5. Constraints block: frozen paths are frozen; a vector mismatch is
     a finding, not a golden to edit; no new deps without ADR; no
     ambient state.
  6. Done-means: /conformance GREEN, commit message format, and
     "list any ambiguity the vectors did not settle and anything you
     deviated from".
  7. Optionally one STRETCH item ("attempt; if it fails, report, do
     not force") and one judgment probe (e.g. "does this new dir need
     a CLAUDE.md? state your reasoning either way") — probes reveal
     whether the session understands the rules or pattern-matches
     them.

## 3. Interstitial sessions (the WP-02.1 pattern)

Review findings between packages get their own small session, not a
rider on the next package. Shape: enumerate tasks in order, cite the
authorizing ADRs in-line (owner acceptance stated in the prompt is
what makes frozen-path edits lawful), set ALLOW_FROZEN=1 only for such
sessions, and constrain: "no behavior changes beyond X; if anything
existing goes red, stop and report."

## 4. Plan review (owner side)

When a session presents a plan: approve with amendments rather than
re-planning. Classify each amendment BLOCKING or non-blocking; put the
verbatim paste-back block at the end so approval is one copy-paste.
Historical hit-rate note: plans have been strong; the recurring
blocking finding is self-certification of goldens (see §5) — check
for it every time a plan creates vectors.

## 5. Golden-vector rituals (the load-bearing habits)

- Goldens are NEVER computed by the implementation under test. An
  independent throwaway script implements the _rules and produces the
  expected values; the implementation must then match. (The vector-add
  skill states this; plans still occasionally violate it — WP-03's
  plan did, caught at review.)
- Where a real independent library exists, use it for a third
  derivation: Python `cryptography` AESGCM reproduces Go gcm.Seal
  layout exactly (validated for blob.json); an independent JCS
  implementation is the E5 cross-check for canon. Three independent
  derivations is the target state for any frozen byte format.
- Same-lineage review is a consistency check, not independence: a
  reviewer subagent re-deriving values in the author's session lineage
  can share the author's misconception. Weight it accordingly.
- Golden regeneration (as opposed to addition) requires an ACCEPTED
  ADR, ALLOW_FROZEN=1, and explicit sign-off language in the commit.
  Additive cases need no ceremony and are encouraged whenever
  implementation reveals an ambiguity (ADR-0020 is the worked
  example: implementation surfaced UTF-16-vs-code-point ordering,
  an ADR pinned it, one golden case now discriminates the orders).

## 6. Review playbook (three passes, in order)

Pass 1 — owner grep triage (~10 min). Mechanical checks: Sync-before-
Rename on every write path; required test cases by name; every
errors.New/fmt.Errorf read once for key material/wrapped crypto errors;
function-level anchors (Put ordering, wipe coverage). A failed triage
stops here and becomes a fix session.

Pass 2 — adversarial evidence-extraction session (fresh session, NOT
the author's). Read-only; forbidden from fixing. Framed as
falsification: "for each claim output SATISFIED (quote file:line),
VIOLATED (quote evidence), or NOT FOUND (state what you searched)."
Ends with a verdict table and the three items most worth human
attention. Advocacy is what you get if the author reviews itself.

Pass 3 — the human read (~30 min, cannot be delegated; threat-model
O1: the agent is the first reader, never the last). Read the highest-
consequence functions in trust-anchor order (e.g. crypto envelope →
rotation failure paths → error ladder), skim the rest against Pass 2's
brief. Close out by recording the review: amend the ADR status line
with the date, commit.

## 7. Standing cautions (learned, easy to lose)

- WP-04b (durable append) is the dependency-graph convergence point:
  05a, 08, 09, 10 all sit behind it. Fresh session, kill-9 matrix
  written BEFORE recovery code.
- kernel.Entropy is the RECORDED handle (answers become rng.seed log
  events). Key material must never flow through it — vault injects
  crypto/rand.Reader raw at the composition root (ADR-0021).
- x/text version bumps AND Go toolchain upgrades count as
  normalization-table changes for kernel/canon: full conformance
  re-run required; any canonical-byte change is a frozen-contract
  change (ADR-0019).
- EraseBlob nil-return means "no key now", not "key destroyed by this
  call" — the WP-04+ redaction flow must confirm prior existence
  before signing redaction.applied (ADR-0021, advisory A1).
- Nil-vs-empty: anything canonicalizing an Event normalizes first
  (kernel.NormalizeEnvelope; ADR-0020 note). One logical event, one
  byte form.

## 8. Session ledger

docs/ledger.md tracks in-flight state between sessions and across
assistant/model transitions. Every session closeout updates it (add
this to done-means when prompting). The ledger is operational, not
frozen; prune completed entries freely.

# RFC-0003: Scope Model and Policy Resolution

Status: ACCEPTED — 2026-07-06. Resolved: D6=(b), D7=Option B,
D8=default-ask
Change policy: the resolution algorithm (§5) is kernel behavior and
frozen after v1. The scope grammar is additive (new qualifier kinds may
be introduced; existing ones never re-interpreted).

## 1. Purpose

Defines what a scope is, how effects are classified into required scopes,
how policy rules are written and attached, and the exact algorithm that
turns (principal, derived scopes, policy set) into allow | ask | deny.
The gate reads only owner-signed policy, kernel-attested identity, and
the capability manifest; model-influenced content is never an input to
resolution (see architecture doc: influence boundary).

## 2. Principals

Kernel-assigned, never self-declared:

    owner
    agent:<agent_id>            (per registered agent)
    capability:<capability_id>  (a capability acting autonomously, e.g.
                                 an inbound communication watcher)
    service:<service_id>        (kernel-adjacent services)

Every run carries the principal of the agent it executes; every event
records its authoring principal.

## 3. Scope grammar

A scope names an operation plus resource qualifiers:

    Scope {
      capability: string          // "email", "fs", "model"
      operation:  string          // "send", "read", "call"
      qualifiers: {kind: Matcher} // typed, per §D6
    }

Examples (display syntax, not storage form):

    fs.read        path=/home/o/projects/**
    email.send     to=*@corp.example
    model.call     spend<=5.00/day
    slack.post     channel=#team-internal
    vault.read     item=github-token

### D6 — Matcher representation [RESOLVED: (b)]

Options:
  (a) String patterns only (globs embedded in one scope string).
      Compact, but parsing is the security boundary: escaping bugs,
      ambiguous grammars, and injection via hostile qualifier values
      (an email address containing a glob metacharacter) become
      permission bugs.
  (b) Structured matchers: qualifiers are typed objects
      (PathPrefix, DomainSuffix, Exact, NumericBudget, OneOf), stored
      canonically; the display syntax is a projection for humans.

DECISION (accepted): (b). Matching becomes typed comparison, not string
parsing; hostile values are data compared against matchers, never
interpreted. Ship exactly five matcher kinds at v1 (Exact, Prefix,
Suffix, OneOf, NumericRange) and resist a general pattern language —
every expressive matcher added is attack surface in the one component
that must be hand-auditable.

## 4. Scope derivation

The capability manifest — reviewed and granted at install time, signed by
the owner — declares for each operation a derivation function from frozen
payload fields to qualifier values:

    email.send:  to      <- payload.input.recipients[]
                 domain  <- suffix(payload.input.recipients[])

Derivation is a pure kernel computation over the frozen payload and the
manifest. Neither the agent's stated intent, the tool's runtime
self-description, nor any context content participates. A payload whose
fields cannot be derived (manifest mismatch) is denied, not guessed.

## 5. Policy rules and resolution

    Rule {
      id, sig                      // owner-signed (RFC-0002 §5)
      principal_sel:  PrincipalMatcher
      scope_sel:      ScopeMatcher (same matcher kinds as §3)
      action:         allow | ask | deny
      level:          owner | workspace | profile | agent | run
      conditions:     [TaintCondition]   // §7, optional
      expiry:         Timestamp | null
    }

### D8 — Default posture [RESOLVED: default-ask]

For a scope with no matching rule:
  - capability not granted at all  -> deny (not negotiable)
  - capability granted, operation within manifest, no rule -> ?

Options: default-allow (usable, dangerous) vs default-ask (safe,
noisier).

DECISION (accepted): default-ask. Granting a capability means "this may run,
under supervision"; silence is not consent. Allows are always explicit
rules, which keeps the audit question "why was this permitted" answerable
by a rule id in every single case.

### D7 — Resolution algorithm [RESOLVED: Option B]

Given all rules matching (principal, scope):

Option A — global deny-override:
    if any matching deny  -> deny
    else if any matching ask -> ask
    else if any matching allow -> allow
    else default (D8)
  Property: maximally fail-safe; but a broad owner-level ask rule
  ("email.send: ask") permanently overrides a deliberate narrow allow
  ("email.send to=reports@corp: allow"), causing approval fatigue and
  pressure toward broad allows — the failure mode narrow grants exist
  to prevent.

Option B — specificity-first:
    take the most specific matching rule; specificity =
      (level order: run > agent > profile > workspace > owner,
       then structural: more qualifiers bound > fewer,
       then matcher tightness: Exact > OneOf > Prefix/Suffix > Range)
    ties at equal specificity -> deny > ask > allow
    deny rules additionally propagate: a deny at ANY level wins
    regardless of specificity (deny is always global).

DECISION (accepted): B. Deny stays absolute (fail-safe preserved); ask/allow
compose the way owners intuitively expect ("ask in general, allow this
specific thing"). The specificity order must be total and documented —
an undecidable tie is a kernel bug, not a judgment call. Every
gate.decision event records the winning rule id and the full candidate
set, so resolution is explainable after the fact.

## 6. Grant lifecycle

  approve-once     approval.resolved bound to one frozen payload; not
                   portable across runs or forks.
  approve-always   emits policy.granted; the DEFAULT offered rule is the
                   narrowest matcher the payload demonstrates
                   (to=alice@corp.example, not to=*), widened only by
                   explicit owner edit.
  revocation       policy.revoked; takes effect at next gate evaluation;
                   never retroactive (past decisions stand in the log).
  expiry           rules may carry expiry; expired rules are absent from
                   resolution but remain in history.

## 7. Taint conditions

Rules may condition on kernel-attested run provenance — the set of scopes
a run has exercised so far (a fold over gate.decision events; never over
content):

    TaintCondition { if_run_used: ScopeMatcher }

Canonical use: escalation of egress after sensitive reads —

    Rule: scope=*.send|*.post|*.upload  action=ask  level=owner
          conditions=[ if_run_used: vault.read * OR fs.read path=~/private/** ]

This expresses "anything that read secrets asks before anything leaves,"
regardless of standing allows, and is evaluated entirely outside the
model influence boundary. Taint conditions may only TIGHTEN outcomes
(they can turn allow into ask/deny; a condition can never turn ask into
allow). This asymmetry is kernel-enforced.

## 8. Non-goals

The scope model does not inspect content. It cannot distinguish a benign
email body from an exfiltrating one sent to an allowed recipient; that
residual risk is addressed by taint escalation (§7), narrow grants (§6),
and the approval inbox rendering attested facts — not by semantic
analysis inside the gate.

## 9. Conformance suite

  1. Resolution is a pure function: property-test totality (every
     (principal, scope, rule-set) resolves; no ties reach "undefined").
  2. Metamorphic tests: adding an allow never flips an existing deny;
     adding any rule never widens an outcome under taint conditions.
  3. Hostile-value corpus: qualifier values containing glob chars,
     unicode confusables, empty strings — matching must be inert.
  4. Explainability: for every resolution in the corpus, the recorded
     rule id re-derives the same outcome in isolation.

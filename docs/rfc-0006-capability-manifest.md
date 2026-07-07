# RFC-0006: Capability Manifest Format

Status: ACCEPTED — 2026-07-06. Resolved: D14=(a) owner-signs-at-install,
D15=(a) declarative-only derivation
Change policy: the manifest schema is additive-only after v1. The
derivation grammar (§4) is frozen: extensions to it are RFC-level
events, exactly like matcher kinds (RFC-0003 D6).

## 1. Purpose

A capability is an MCP server plus a manifest. The manifest is the
signed contract between a piece of third-party functionality and the
runtime's trust machinery: what operations exist, what scopes they
require and how those scopes are derived from payloads, what happens
on uncertain execution, how much isolation the code gets, and what the
owner agreed to at install. MCP provides the tool protocol; the
manifest provides everything MCP does not: permissioning, provenance,
and consent.

Model adapters are capabilities too: an adapter manifest is a manifest
whose operations are model.call (+ streaming variants) and which
additionally carries the block-type support and downgrade tables of
RFC-0001 §6. One format, no special cases.

## 2. Manifest schema

    Manifest {
      capability_id, semver
      publisher:      { name, publisher_key }        // signs §7(1)
      binding:        MCP launch config (command/args/env template)
                      | MCP URL (remote)
      trust_level:    bundled | verified | third-party | untrusted
      operations:     [Operation]
      vault_slots:    [{slot_id, description, injection: env|header|file}]
      ui:             { artifact_viewers, extensions }   // optional
      docs, tests:    refs                               // optional
    }

    Operation {
      name:           string          // becomes <capability>.<name>
      effect_class:   safely-repeatable | suspend-on-uncertain
                      // RFC-0005 D13; linted if absent
      derives:        [Derivation]    // §4
      resources:      [ResourceTemplate]
                      // e.g. path template, domain template — the
                      // broker turns granted instances into Landlock
                      // rules / proxy allowlist entries (RFC-0005 §6)
      downgrades:     [{block_type, transform_id}]
                      // adapters only; RFC-0001 D3 carve-out
    }

## 3. Scope derivation (position)

The gate classifies a frozen payload into required scopes by evaluating
the operation's derivations — a pure kernel computation over payload
bytes and the manifest (RFC-0003 §4). The agent's intent, the server's
runtime self-description, and all context content are excluded by
construction. A payload the derivations cannot classify is denied, not
guessed.

## 4. Derivation grammar

### D15 — Derivation expressiveness [RESOLVED: (a)]

Options:
  (a) Declarative-only: a derivation is a field path plus at most one
      transform from a closed set.
  (b) Embedded expression language (CEL, Lua, WASM hook) evaluated at
      derivation time.

DECISION (accepted): (a), as law. Option (b) puts third-party code inside
the gate — the one component whose inputs must never include anything
a capability author (or their compromised supply chain) controls at
decision time. This is the D6 of manifests: expressiveness inside the
security boundary is attack surface. Hold the line.

The grammar under (a):

    Derivation {
      qualifier:  string              // matcher kind key, RFC-0003 §3
      path:       FieldPath           // dot access, [i] index, [*]
                                      // wildcard over arrays; nothing
                                      // else — no filters, no slices
      transform:  identity | domain_of | suffix | prefix
                | lowercase_nfc | count | byte_len | none
    }

Rules: transforms compose at most once; paths that resolve to nothing
or to unexpected types make the payload underivable (deny); wildcard
paths derive the FULL SET of values, and the gate requires every
element to satisfy policy (one bad recipient fails the whole send).
The grammar is closed: a capability needing more expressiveness is a
capability whose operation is too coarse — split the operation.

## 5. Vault slots

Manifests name slots, never values. The owner binds slots to vault
entries at install or first use; the broker resolves them at worker-
request assembly only (RFC-0005 §3). A slot unbound at execution time
suspends the run with an owner card rather than failing cryptically.

## 6. Trust levels

Requested by the manifest, granted (or reduced — never raised silently)
by the owner at install. Isolation mapping is defined in RFC-0005 §6.
bundled is reserved for capabilities shipped in the runtime's own
repository and built from source; a third-party manifest requesting
bundled is rejected by the validator, not negotiated.

## 7. Install and upgrade ceremony

### D14 — Install trust flow [RESOLVED: (a)]

Options:
  (a) Owner-signs-at-install: the install ceremony renders the
      manifest's requested operations, scopes, resources, trust level,
      and vault slots; the owner countersigns the manifest hash; the
      kernel records capability.installed (owner-signed event,
      RFC-0002 §5). The gate honors only manifests whose hash matches
      an installed, countersigned record.
  (b) Trust-on-first-use: manifest accepted on first invocation,
      pinned thereafter.

DECISION (accepted): (a). It is one review-and-tap at install and it closes
the manifest-substitution family outright: a swapped binary or edited
manifest no longer matches the countersigned hash and every operation
of that capability is dead until re-consent. TOFU protects against
change but blesses whatever was present first — the wrong default for
the component that defines scope semantics.

Upgrade: a new manifest version triggers the same ceremony, rendered
as a DIFF against the installed version with widened scopes,
new operations, raised trust requests, and new vault slots highlighted.
Silent widening is impossible by construction; narrowing may be
auto-accepted by profile policy. Uninstall appends capability.removed
(owner-signed); standing policy rules referencing the capability
become inert (they remain in history; resolution simply never matches
an uninstalled capability).

## 8. Publisher signatures and provenance

The publisher key signs the manifest content (§2); the owner
countersigns at install (§7). Two signatures, two questions: "is this
the artifact the publisher shipped" and "did the owner consent to it."
Key distribution at v1 is deliberately simple — pin-on-install of the
publisher key, recorded in capability.installed — leaving registries
and transparency logs as post-1.0 territory.

## 9. Conformance suite

  1. Manifest validator: schema, closed grammar (§4), trust-level
     rules (§6), effect_class lint (RFC-0005 D13).
  2. Derivation totality fuzz: generated payloads including hostile
     values (glob chars, confusables, deep nesting, huge arrays);
     every payload derives or denies — never errors, never guesses.
  3. Wildcard semantics: multi-recipient payloads where one element
     violates policy must fail the whole effect.
  4. Upgrade-diff test: a widened manifest must surface every widening
     in the ceremony rendering; an unchanged manifest must not
     re-prompt.
  5. Substitution test: edited manifest or binary with stale
     countersignature — every operation must be refused by the gate.

# Decision Register (ADR index)

- ADR-0001 (D1, ACCEPTED): Tool results as content blocks, not a role
- ADR-0002 (D2, ACCEPTED): Canonical serialization: JCS-style JSON, no floats
- ADR-0003 (D3, ACCEPTED): Fail closed on unsupported blocks; declared downgrades only
- ADR-0004 (D4, ACCEPTED): Per-run hash chains with anchored Merkle head
- ADR-0005 (D5, ACCEPTED): Gapless seq; kernel-mediated time and entropy
- ADR-0006 (D6, ACCEPTED): Structured scope matchers; closed set of five
- ADR-0007 (D7, ACCEPTED): Resolution: specificity-first, deny always global
- ADR-0008 (D8, ACCEPTED): Default-ask for granted capabilities
- ADR-0009 (D9, ACCEPTED): Erasure keys: per-run, wrapped by owner master key
- ADR-0010 (D10, ACCEPTED): Message bodies always in blobs
- ADR-0011 (D11, ACCEPTED): Plugin delivery: compile-time pure, subprocess effectful
- ADR-0012 (D12, ACCEPTED): Isolation: platform-tiered native primitives + opportunistic containers (a')
- ADR-0013 (D13, ACCEPTED): Uncertain-effect default: suspend
- ADR-0014 (D14, ACCEPTED): Manifests owner-countersigned at install
- ADR-0015 (D15, ACCEPTED): Derivation grammar: declarative-only, closed
- ADR-0016 (D16, ACCEPTED): Test corpus governance: synthetic-only in CI
- ADR-0017 (D17, ACCEPTED): Stability line: the log is sacred from v0.1
- ADR-0018 (D18, ACCEPTED): License: Apache-2.0
- ADR-0019 (D19, ACCEPTED): kernel/canon NFC dependency: golang.org/x/text/unicode/norm
- ADR-0020 (D20, ACCEPTED): Object-key order: UTF-16 code units (RFC 8785 §3.2.3)
- ADR-0021 (D21, ACCEPTED): Vault-lite blob format and key hierarchy (AES-256-GCM, DEK/KEK/master)
- ADR-0022 (D22, ACCEPTED): Durable append: single-file log, WAL-truncate torn tails, per-platform durability
- ADR-0023 (D23, ACCEPTED): Reducer identity at Stage 1: declared version string, hash-shaped

Note: chat-era labels D18-D20 were renumbered to D16-D18;
this register is authoritative.

Template for new decisions: adr/TEMPLATE.md. Every decision of
consequence gets an ADR; PROPOSED until the owner accepts.

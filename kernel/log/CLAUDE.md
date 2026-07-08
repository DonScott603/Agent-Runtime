# kernel/log — local law

The log is append-only and sacred (constitution #4); from WP-04b on,
this package owns that property. It is not on the repo's human-review
list, but seal + chain is the tamper-evidence primitive — treat
changes with the same care.

- Seal/chain rules are frozen by docs/vectors/chain.json _rules and
  RFC-0002 §4: NormalizeEnvelope before hashing (ADR-0020), zero
  event_id ("") and sig (null), sha256 lowercase hex, genesis
  prev_hash = kernel.ZeroHash.
- VerifyChain REPORTS (*ChainBrokenError naming run + seq) and never
  repairs, truncates, or continues past the first failure
  (docs/errors.md CHAIN_BROKEN).
- Signature validity is fold's job (RFC-0002 §5), never checked here;
  sig is zeroed out of the hash by design.
- Append code (WP-04b+) never rewrites stored bytes; evolve by
  upcaster (docs/versioning.md §4). The pure layer never imports os.

# plugins — local law

First-party PURE plugins, compile-time registered (RFC-0004 D11a).
Purity is a verified contract, not a self-description (RFC-0004 §3):

- P1: inputs arrive only through the invocation call. Nothing is read
  from the environment — no os, no time.Now(), no math/rand; time and
  entropy only via kernel handles (none needed by reducers).
- P3: output is a value or a deterministic error VALUE. A panic is a
  plugin bug by definition; the fold engine contains it as
  PLUGIN_CONTRACT.
- Totality (RFC-0002 §2): reducers receive event types they do not
  know and MUST ignore them; unknown payload fields MUST be ignored
  (unmarshal into structs, never DisallowUnknownFields).
- Every pure component ships its determinism test (invoke twice,
  byte-compare) in the same commit (root CLAUDE.md).
- No floats in any state that gets serialized or hashed (RFC-0001 D2).
  Strings entering hashed state are built from stable structured
  facts, never err.Error()/library text (WP-05a owner A1).
- Identity is declared (plugin_id + semver, ADR-0023): bump the semver
  on ANY behavior change — snapshots and provenance key on it.

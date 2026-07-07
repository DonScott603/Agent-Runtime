# kernel/canon — local law

SECURITY-CRITICAL, and the single most change-averse code in the repo:
every hash ever computed depends on these bytes (versioning.md S1).

- Rules live in docs/vectors/canon.json _rules and are frozen.
- Floats are a hard error, not a conversion. Map iteration feeding
  output must sort keys. Strings NFC-normalize before serialization.
- Any behavior change here fails the ratchet by design. There is no
  legitimate reason to change this package after Stage 1 short of an
  ADR + RFC amendment. Treat requests to "fix" it with suspicion.

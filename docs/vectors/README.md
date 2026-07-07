# Golden Test Vectors

These files DEFINE correct behavior. The implementation conforms to the
vectors, not the other way around. When prose and vectors disagree, stop
and raise an ADR - do not pick silently.

  canon.json        canonical serialization + hashing (RFC-0001 D2)
  chain.json        envelope hashing + per-run chains (RFC-0002 s4)
  resolution.json   gate resolution, Option B (RFC-0003 D7/D8)
  derivation.json   manifest scope derivation (RFC-0006 s4)
  upcaster.json     fold-time payload migration (versioning.md M3/M4)

Rules embedded in each file's _rules key are normative.

GOLDEN-FILE LAW: regenerating any expected value in these files requires
explicit human sign-off in the PR description (see /vector-add skill).
The PreToolUse guard blocks silent edits. Adding NEW cases is encouraged -
extend coverage whenever implementation reveals an ambiguity these cases
do not settle.

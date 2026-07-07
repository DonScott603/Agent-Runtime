---
name: vector-add
description: Add golden test vectors or (rarely) regenerate golden values. Use when implementation reveals an ambiguity the vectors do not settle, when adding coverage, or when a golden hash must change. Enforces the golden-file law.
---

# Vector add / regenerate

ADDING cases (the normal path):
1. Write the new case with input + expected output derived from the
   RFC text; cite the section in the case's "note".
2. If the expected value is a hash, compute it with the reference
   rules in the relevant _rules key — never by running the
   implementation and copying its output (that would make the
   implementation self-certifying).
3. Run /conformance; the new case must pass or expose a real bug.

REGENERATING golden values (exceptional):
1. This means a frozen contract changed. There must be an ACCEPTED
   ADR authorizing it. No ADR — stop.
2. Set ALLOW_FROZEN=1 for the session, make the change, and list in
   the PR description: the ADR id, every golden value changed, and
   the sentence "golden values regenerated with owner sign-off".
3. The ratchet WILL go red for historical corpora; the ADR must say
   what happens to them (usually: an upcaster, never abandonment).

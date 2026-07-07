# kernel/derive — local law

SECURITY-CRITICAL (runs inside the gate's trust boundary).

- The grammar is CLOSED (D15): dot access, [i], [*]; at most one
  transform from the seven in stubs/kernel/types.go. No regex, no
  expression language, no "just this one helper".
- Underivable => DENY. Never guess, never default, never coerce types.
- Derived values are data. If a derived value is ever interpreted as
  a pattern anywhere downstream, that is a vulnerability, not a
  feature. See derivation.json hostile-glob case.
- Wildcard paths derive full sets; the gate requires every element to
  pass. Do not "optimize" to first-element.

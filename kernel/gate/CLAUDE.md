# kernel/gate — local law (loaded when you work here)

SECURITY-CRITICAL. Every diff here is needs-human-review (threat-model O1).

- Resolve() and everything it calls must remain PURE: inputs are
  (granted, rules, scope, runUsed) and nothing else. No context reads,
  no logging side effects, no clock.
- The matcher set is CLOSED at five kinds (D6). Feature pressure is
  answered by splitting operations, never by adding a kind or any
  pattern interpretation. Matching is typed comparison — if you are
  writing a parser here, stop.
- docs/vectors/resolution.json is normative. A failing vector means
  the code is wrong; if you believe the vector is wrong, STOP and
  raise an ADR — do not edit the vector.
- Taint conditions may only tighten (allow->ask/deny). The asymmetry
  is enforced here; test it metamorphically (RFC-0003 §9.2).
- Every Decision records winning rule + full candidate set.

---
name: conformance
description: Run the full conformance gate (vectors, determinism, ratchet, secret scan) and interpret failures. Use before declaring any work package done, before any commit, and whenever asked to "run the checks".
---

# Conformance

1. Run `make conformance`. It executes, in order: vector harness
   (docs/vectors/*), determinism harness (pure components invoked
   twice, byte-compared), fold ratchet (all historical corpora under
   HEAD, golden state hashes), secret/PII scan.
2. Interpreting failures:
   - Vector failure: the CODE is wrong. Fix code. If you believe the
     vector is wrong, STOP — that is an ADR, not an edit (golden-file
     law, docs/vectors/README.md).
   - Determinism failure: hunt ambient state — time.Now(), rand, map
     iteration into serialized output, env reads. The ambient-state
     detector output names the syscall or divergence site.
   - Ratchet failure: your change breaks the past. This is never
     force-through-able. Report it as a finding with the failing
     corpus generation and the divergent state hash, and propose.
   - Secret-scan failure: remove the material AND check how it got
     staged; corpus purity is threat-model D16.
3. Report the outcome as a pass/fail line per suite; on full green,
   cite it in the commit message.

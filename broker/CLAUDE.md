# broker — local law

SECURITY-CRITICAL. Every diff here is needs-human-review.

- The broker discovers work by folding the log (authorized effects
  without effect.executed). It NEVER accepts a payload from a caller.
  If you find yourself adding an "execute this" API, stop.
- Executed bytes == frozen bytes of effect.proposed. No re-generation,
  no "refreshing" the payload after approval.
- Credentials resolve at worker-request assembly, post-gate, and are
  injected into the worker only. If a secret can reach an event,
  payload, or log line, that is a release-blocking bug (E9).
- Idempotency keys derive from log position; recovery follows D13
  exactly — unannotated operations suspend, never re-run.
- effect.executed's isolation descriptor lists mechanisms ACTUALLY
  applied. Never record aspirational confinement.

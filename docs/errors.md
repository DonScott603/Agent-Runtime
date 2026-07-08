# Error Taxonomy

Errors are part of the recorded data contract: many land inside events
(effect.result, gate.decision, run.failed), which makes them S4-frozen
(versioning.md). Never invent ad-hoc error strings; extend this table
via ADR. Wire form inside payloads:

    { "code": "<CODE>", "detail": "<human text>", "refs": {...} }

| Code | Raised by | Recorded as | Run behavior | Notes |
|---|---|---|---|---|
| DENIED | gate | gate.decision(deny) -> effect.result error | continues; agent sees denial as tool result | carries rule_id |
| UNDERIVABLE | derive | gate.decision(deny, rule_id="underivable") | continues; agent sees denial | manifest/path mismatch; never guess (RFC-0006 §4) |
| APPROVAL_EXPIRED | kernel | approval.expired -> effect.result error | resumes with timeout-denial | RFC-0002 §3 |
| APPROVAL_DENIED | owner | approval.resolved(deny) -> effect.result error | resumes; agent adapts | distinct from DENIED (policy vs human) |
| UNCERTAIN | broker | effect.uncertain | suspend(awaiting_input) unless safely-repeatable | D13; owner card resolves |
| CANCELLED | owner/kernel | effect.result error / run.cancelled | terminal or per-effect | voids sibling pending approvals atomically |
| UNSUPPORTED_BLOCK | adapter | effect.result error | continues; routing may pick another adapter | fail-closed (D3); downgrade path records transform id |
| VAULT_SLOT_UNBOUND | broker | run.suspended(awaiting_input) + owner card | suspends | never a cryptic failure (RFC-0006 §5) |
| BUDGET_EXCEEDED | gate (NumericRange) | gate.decision(ask or deny) | per policy | spend scopes on model.call |
| PLUGIN_CONTRACT | kernel | run.failed(diagnostics) | terminal | pure-plugin panic or ambient-state breach (RFC-0004 P3); on the read path (reducers), the view becomes unavailable instead — there is no run to fail; live-path wiring to run.failed is WP-07+ |
| PLUGIN_ERROR | plugin | plugin.invoked output error value | per caller | deterministic error VALUE, not a panic |
| WORKER_LIMIT | broker | effect.result error | continues | rlimit breach; isolation descriptor names the limit |
| CHAIN_BROKEN | verifier reducer | integrity alarm (owner notification) | none (read path) | tamper-evidence tripwire; never auto-"repair" |
| ANCHOR_MISMATCH | verifier reducer | integrity alarm (owner notification) | none (read path) | impeaches the anchor, not the chain — linkage evidence stands; heads keep advancing; never repaired (ADR-0024) |
| SIG_INVALID | fold | consent event treated as absent + alarm | as if unsigned | RFC-0002 §5 |
| SCHEMA_UNKNOWN_VERSION | fold | fold error surfaced to owner | view unavailable | missing upcaster == release-blocking bug |
| SEQ_GAP | log recovery | refuses to open store + alarm | none | should be impossible (D5 gapless); treat as corruption |
| LOG_CORRUPT | log recovery | refuses to open store + alarm | none | framing/CRC/content corruption of committed bytes; torn tails (ADR-0022) are truncated, never corruption |

Rules: DENIED-family errors are NORMAL control flow fed back to the
agent — never terminal. PLUGIN_CONTRACT and SEQ_GAP are bugs — always
loud. UNCERTAIN is a question for a human — never auto-resolved.

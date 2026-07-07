# vault — local law

SECURITY-CRITICAL. Every diff here is needs-human-review.

- Keys and secrets never appear in: events, payloads, error messages,
  logs, panics, or test fixtures. Test with fakes, never real material.
- Blob encryption is per-run data keys wrapped by the master key (D9);
  erasure = key destruction + signed redaction.applied, never file
  deletion alone.
- Owner signing keys are never readable by agent or capability
  principals; signing happens here, behind a narrow API.

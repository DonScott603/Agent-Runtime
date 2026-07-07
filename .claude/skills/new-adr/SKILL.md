---
name: new-adr
description: Create a new Architecture Decision Record when a decision of consequence surfaces mid-task. Use whenever a choice would constrain future work, touch a frozen contract, add a dependency to a security-critical path, or when the user says "new ADR" or "record this decision".
---

# New ADR

1. Read docs/adr/README.md; the next D-number is one past the highest.
2. Copy docs/adr/TEMPLATE.md to docs/adr/ADR-<next, zero-padded>.md.
3. Fill: Context (why forced now, link RFC sections), Options (each
   with the trade it makes), Decision — as a RECOMMENDATION with
   status PROPOSED. You do not accept ADRs; the owner does.
4. Append the entry to docs/adr/README.md index.
5. Surface it: end your reply with the ADR number, the recommendation,
   and what is blocked until it is resolved. Do not proceed with work
   that depends on the unresolved decision.

// Reserved pre-Apply seams (WP-05a). Both are deliberate stubs:
//
//  1. Upcasters (versioning.md M3; RFC-0001 §8): pure vN->vN+1 payload
//     migrations registered with the kernel and applied at fold time,
//     never by rewriting stored bytes. At Stage 1 no payload has ever
//     evolved, so upcast is the identity. When real upcasters land
//     they run HERE — after the version gate (a type_version above
//     the declared max is precisely the missing-upcaster case,
//     SCHEMA_UNKNOWN_VERSION) and before Apply, raising v <= max
//     payloads toward max. Upcasters are total pure migrations; the
//     seam deliberately carries no error.
//
//  2. Signature validation (RFC-0002 §5): a consent-type event with a
//     missing/invalid signature is treated as absent by every reducer
//     and surfaced as an integrity alarm (SIG_INVALID, docs/errors.md).
//     Deferred from WP-05a (needs vault verify keys; no shipped
//     reducer folds consent payloads yet) — flagged at plan approval
//     2026-07-08 so the deferral is a decision, not an omission. It
//     slots in beside upcast when it lands.
package fold

import "github.com/DonScott603/Agent-Runtime/kernel"

// upcast migrates an event's payload toward the reducer's declared
// version. Identity at Stage 1 (see package comment above).
func upcast(e kernel.Event) kernel.Event { return e }

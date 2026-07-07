// Envelope normalization, implemented ahead of WP-04 (WP-02.1;
// ADR-0020 Consequences note).
package kernel

import "encoding/json"

// NormalizeEnvelope returns e with the nil-vs-empty ambiguity removed
// so one logical event has exactly one canonical byte form (RFC-0002
// §4; ADR-0020 Consequences): nil Blobs becomes an empty slice and nil
// Payload becomes the empty JSON object. SealEvent (WP-04) normalizes
// before hashing; anything else canonicalizing an Event must do the
// same.
func NormalizeEnvelope(e Event) Event {
	if e.Blobs == nil {
		e.Blobs = []Hash{}
	}
	if e.Payload == nil {
		e.Payload = json.RawMessage("{}")
	}
	return e
}

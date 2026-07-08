// Anchor payload and Merkle construction (WP-04c; RFC-0002 §4, D4;
// ADR-0022 A1; ADR-0024). It lives beside the frozen shapes rather
// than in kernel/log for the same reason as seal.go: the fold-side
// verifier (plugins/chainverify) must never import kernel/log
// (kernel/log's round-trip test imports the plugin — a reverse import
// is a compile-breaking cycle by design), and both the store and the
// verifier must share ONE implementation of a construction that is
// frozen at first persistence. Rules are pinned by
// docs/vectors/anchor.json _rules; the goldens were independently
// derived (process.md §5).
package kernel

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// AnchorEventType is the owner-scope anchor event (RFC-0002 §3 open
// taxonomy; documented by ADR-0024, no RFC edit — new types are
// additive, M2).
const AnchorEventType = "anchor.appended"

// AnchorContainer is the ADR-0022 A1 attestation: the base seq and
// first event_id of the container (Stage 1: one events.log = one
// container). Front-truncating a container must break this claim.
type AnchorContainer struct {
	BaseSeq      Seq  `json:"base_seq"`
	FirstEventID Hash `json:"first_event_id"`
}

// AnchorPayload is the anchor.appended v1 payload (ADR-0024;
// data-contract from first persistence, ADR-0017). Heads ride in the
// payload so the root recomputes from the payload alone; the ""
// entry is the latest owner-scope event (RFC-0002 D4) — the anchor
// itself is appended to the "" chain AFTER the snapshot, so each
// anchor's heads include its predecessor anchor. No omitempty: every
// key always serializes.
type AnchorPayload struct {
	Container  AnchorContainer `json:"container"`
	Heads      map[RunID]Hash  `json:"heads"`
	MerkleRoot Hash            `json:"merkle_root"`
}

// AnchorRoot computes the Merkle root over a run-head set per the
// anchor.json _rules (RFC 6962 MTH; ADR-0024):
//
//	leaf     = sha256(0x00 || Canonical({"head": <head>, "run_id": <id>}))
//	interior = sha256(0x01 || left || right)      (raw 32-byte digests)
//	split    = largest power of two < n; empty set = sha256("")
//
// Leaves ascend by NFC-normalized run_id in UTF-16 code-unit order
// (ADR-0020). The order is DERIVED from Canonical's emitted key order
// over the heads object rather than re-implemented, so it cannot
// diverge from the one ordering law by construction; canon's refusals
// (invalid UTF-8, NFC key collision) are therefore AnchorRoot's
// refusals too. Total over the empty (or nil) set.
func AnchorRoot(heads map[RunID]Hash) (Hash, error) {
	if len(heads) == 0 {
		sum := sha256.Sum256(nil)
		return hex.EncodeToString(sum[:]), nil
	}
	canonical, err := Canonical(heads)
	if err != nil {
		return "", fmt.Errorf("kernel: anchor root: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(canonical))
	if _, err := dec.Token(); err != nil { // opening '{'
		return "", fmt.Errorf("kernel: anchor root: %w", err)
	}
	leaves := make([][32]byte, 0, len(heads))
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return "", fmt.Errorf("kernel: anchor root: %w", err)
		}
		runID, ok := tok.(string)
		if !ok {
			return "", fmt.Errorf("kernel: anchor root: unexpected token %v in canonical heads", tok)
		}
		var head string
		if err := dec.Decode(&head); err != nil {
			return "", fmt.Errorf("kernel: anchor root: %w", err)
		}
		leaf, err := Canonical(map[string]string{"head": head, "run_id": runID})
		if err != nil {
			return "", fmt.Errorf("kernel: anchor root: %w", err)
		}
		leaves = append(leaves, sha256.Sum256(append([]byte{0x00}, leaf...)))
	}
	root := merkleHead(leaves)
	return hex.EncodeToString(root[:]), nil
}

// merkleHead is the RFC 6962 MTH recursion over leaf hashes. Callers
// guarantee at least one leaf (the empty set short-circuits above).
func merkleHead(leaves [][32]byte) [32]byte {
	if len(leaves) == 1 {
		return leaves[0]
	}
	k := 1
	for k*2 < len(leaves) {
		k *= 2
	}
	left, right := merkleHead(leaves[:k]), merkleHead(leaves[k:])
	preimage := make([]byte, 0, 1+2*sha256.Size)
	preimage = append(preimage, 0x01)
	preimage = append(preimage, left[:]...)
	preimage = append(preimage, right[:]...)
	return sha256.Sum256(preimage)
}

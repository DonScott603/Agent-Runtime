// Anchor Merkle construction tests (WP-04c; ADR-0024). Golden values
// live in docs/vectors/anchor.json (asserted by the corpus harness);
// these tests pin the LAWS: determinism, tamper sensitivity, and the
// weld between leaf ordering and canon's key ordering (ADR-0020 — one
// ordering rule project-wide).
package kernel_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"testing"

	"pgregory.net/rapid"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

func mustRoot(t testing.TB, heads map[kernel.RunID]kernel.Hash) kernel.Hash {
	t.Helper()
	root, err := kernel.AnchorRoot(heads)
	if err != nil {
		t.Fatalf("AnchorRoot: %v", err)
	}
	return root
}

// Determinism law (root CLAUDE.md): invoke twice, byte-compare.
func TestAnchorRootDeterminism(t *testing.T) {
	heads := map[kernel.RunID]kernel.Hash{
		"":         "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"run_0001": "1111111111111111111111111111111111111111111111111111111111111111",
		"run_0002": "2222222222222222222222222222222222222222222222222222222222222222",
		"run_0003": "3333333333333333333333333333333333333333333333333333333333333333",
	}
	if a, b := mustRoot(t, heads), mustRoot(t, heads); a != b {
		t.Fatalf("two computations differ: %s vs %s", a, b)
	}
}

// The workplan's named verify at the pure layer: any single-head
// mutation, any added or removed leaf, and any renamed run flips the
// root.
func TestAnchorRootTamperFlips(t *testing.T) {
	heads := map[kernel.RunID]kernel.Hash{
		"":      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"run_a": "1111111111111111111111111111111111111111111111111111111111111111",
		"run_b": "2222222222222222222222222222222222222222222222222222222222222222",
	}
	baseline := mustRoot(t, heads)

	for run := range heads {
		mutated := cloneHeads(heads)
		mutated[run] = flipHexHash(mutated[run])
		if mustRoot(t, mutated) == baseline {
			t.Errorf("flipping head of run %q did not flip the root", run)
		}
	}
	added := cloneHeads(heads)
	added["run_c"] = "3333333333333333333333333333333333333333333333333333333333333333"
	if mustRoot(t, added) == baseline {
		t.Error("adding a leaf did not flip the root")
	}
	removed := cloneHeads(heads)
	delete(removed, "run_b")
	if mustRoot(t, removed) == baseline {
		t.Error("removing a leaf did not flip the root")
	}
	renamed := cloneHeads(heads)
	renamed["run_z"] = renamed["run_a"]
	delete(renamed, "run_a")
	if mustRoot(t, renamed) == baseline {
		t.Error("renaming a run did not flip the root")
	}
}

func cloneHeads(h map[kernel.RunID]kernel.Hash) map[kernel.RunID]kernel.Hash {
	c := make(map[kernel.RunID]kernel.Hash, len(h))
	maps.Copy(c, h)
	return c
}

func flipHexHash(h kernel.Hash) kernel.Hash {
	if len(h) == 0 {
		return "0"
	}
	c := byte('0')
	if h[0] == '0' {
		c = 'f'
	}
	return kernel.Hash(string(c) + h[1:])
}

// Ordering weld (ADR-0024/ADR-0020): the leaf sequence equals the key
// order of the canonically serialized heads object. The reference
// below takes its ORDER from kernel.Canonical's emitted bytes (the
// vector-pinned authority) and recomputes the tree; AnchorRoot must
// agree for arbitrary unicode run_ids — including the astral/NFC
// regions where UTF-16, code-point, and byte orders diverge. When
// canon refuses the map (NFC key collision), AnchorRoot must refuse
// too.
func TestAnchorRootOrderAgreesWithCanon(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 8).Draw(t, "n")
		heads := make(map[kernel.RunID]kernel.Hash, n)
		for range n {
			id := rapid.String().Draw(t, "run_id")
			heads[id] = "1111111111111111111111111111111111111111111111111111111111111111"
		}

		canonical, canonErr := kernel.Canonical(heads)
		root, rootErr := kernel.AnchorRoot(heads)
		if canonErr != nil {
			if rootErr == nil {
				t.Fatalf("canon refuses the heads map (%v) but AnchorRoot accepted it", canonErr)
			}
			return
		}
		if rootErr != nil {
			t.Fatalf("AnchorRoot errored on a canon-clean map: %v", rootErr)
		}
		if want := referenceRoot(t, canonical, heads); root != want {
			t.Fatalf("root %s disagrees with canon-ordered reference %s", root, want)
		}
	})
}

// referenceRoot recomputes the tree with leaf order read from the
// canonical serialization of the heads object and an independent
// RFC 6962 MTH recursion.
func referenceRoot(t *rapid.T, canonical []byte, heads map[kernel.RunID]kernel.Hash) kernel.Hash {
	var ordered map[kernel.RunID]kernel.Hash // for value lookup by NFC key
	if err := json.Unmarshal(canonical, &ordered); err != nil {
		t.Fatalf("canonical bytes do not re-parse: %v", err)
	}
	keys := orderedKeys(t, canonical)
	if len(keys) != len(heads) {
		t.Fatalf("canonical object has %d keys, heads %d", len(keys), len(heads))
	}
	leaves := make([][32]byte, 0, len(keys))
	for _, k := range keys {
		leafObj, err := kernel.Canonical(map[string]string{"head": string(ordered[k]), "run_id": string(k)})
		if err != nil {
			t.Fatalf("Canonical(leaf): %v", err)
		}
		leaves = append(leaves, sha256.Sum256(append([]byte{0x00}, leafObj...)))
	}
	root := refMTH(leaves)
	return kernel.Hash(hex.EncodeToString(root[:]))
}

// orderedKeys walks the canonical object's token stream, collecting
// top-level keys in emitted order.
func orderedKeys(t *rapid.T, canonical []byte) []kernel.RunID {
	dec := json.NewDecoder(bytes.NewReader(canonical))
	tok, err := dec.Token() // {
	if err != nil || tok != json.Delim('{') {
		t.Fatalf("canonical bytes are not an object: %v %v", tok, err)
	}
	var keys []kernel.RunID
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			t.Fatalf("token: %v", err)
		}
		keys = append(keys, kernel.RunID(k.(string)))
		var v any
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("value: %v", err)
		}
	}
	return keys
}

func refMTH(leaves [][32]byte) [32]byte {
	n := len(leaves)
	if n == 0 {
		return sha256.Sum256(nil)
	}
	if n == 1 {
		return leaves[0]
	}
	k := 1
	for k*2 < n {
		k *= 2
	}
	l, r := refMTH(leaves[:k]), refMTH(leaves[k:])
	pre := append([]byte{0x01}, l[:]...)
	return sha256.Sum256(append(pre, r[:]...))
}

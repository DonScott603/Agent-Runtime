// Determinism and hard-error tests for kernel/canon (WP-01).
// The golden success cases live in docs/vectors/canon.json, asserted by
// the corpus harness; this file covers the properties and the negative
// space the vectors do not: serialize-twice determinism, idempotence,
// float/UTF-8/duplicate-key rejection, UTF-16 key order, struct tags.
package canon_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/DonScott603/Agent-Runtime/kernel"
	"github.com/DonScott603/Agent-Runtime/kernel/canon"
)

// jsonValue generates an arbitrary D2-legal JSON value: null, bool,
// integer (json.Number), unicode string, array, object.
func jsonValue(t *rapid.T, depth int) any {
	max := 6
	if depth <= 0 {
		max = 4 // leaves only
	}
	switch rapid.IntRange(0, max).Draw(t, "kind") {
	case 0:
		return nil
	case 1:
		return rapid.Bool().Draw(t, "bool")
	case 2:
		return json.Number(strconv.FormatInt(rapid.Int64().Draw(t, "int"), 10))
	case 3:
		return json.Number(strconv.FormatUint(rapid.Uint64().Draw(t, "uint"), 10))
	case 4:
		return rapid.String().Draw(t, "str")
	case 5:
		n := rapid.IntRange(0, 4).Draw(t, "arrlen")
		arr := make([]any, n)
		for i := range arr {
			arr[i] = jsonValue(t, depth-1)
		}
		return arr
	default:
		n := rapid.IntRange(0, 4).Draw(t, "maplen")
		m := make(map[string]any, n)
		for i := 0; i < n; i++ {
			m[rapid.String().Draw(t, "key")] = jsonValue(t, depth-1)
		}
		return m
	}
}

// Serialize twice, byte-compare: the determinism test every pure
// component ships with (CLAUDE.md working rules).
func TestCanonicalDeterminism(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		v := jsonValue(t, 3)
		a, errA := canon.Canonical(v)
		b, errB := canon.Canonical(v)
		if (errA == nil) != (errB == nil) {
			t.Fatalf("nondeterministic error: %v vs %v", errA, errB)
		}
		if errA != nil {
			// Only legal failure for generated values: distinct keys
			// colliding after NFC normalization. Anything else is a bug.
			if !errors.Is(errA, canon.ErrDuplicateKey) {
				t.Fatalf("unexpected error on generated value: %v", errA)
			}
			return
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("serialize twice differs:\n a: %q\n b: %q", a, b)
		}
	})
}

// Canonicalization is idempotent: decode the canonical bytes and
// re-canonicalize; the bytes must not change.
func TestCanonicalIdempotence(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		v := jsonValue(t, 3)
		first, err := canon.Canonical(v)
		if err != nil {
			return // NFC key collision in generated input; covered above
		}
		dec := json.NewDecoder(bytes.NewReader(first))
		dec.UseNumber()
		var round any
		if err := dec.Decode(&round); err != nil {
			t.Fatalf("canonical output is not valid JSON: %v\nbytes: %q", err, first)
		}
		second, err := canon.Canonical(round)
		if err != nil {
			t.Fatalf("re-canonicalizing canonical output: %v", err)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("not idempotent:\n first: %q\nsecond: %q", first, second)
		}
	})
}

func TestFloatsAreHardErrors(t *testing.T) {
	type withFloat struct {
		F float64 `json:"f"`
	}
	for name, v := range map[string]any{
		"float64":              3.14,
		"float64-integral":     float64(1), // a conversion, not an integer: still an error
		"float32":              float32(2.5),
		"number-decimal-token": json.Number("1.5"),
		"number-exponent":      json.Number("1e2"),
		"nested-in-map":        map[string]any{"ok": 1, "bad": 0.5},
		"struct-float-field":   withFloat{},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := canon.Canonical(v); !errors.Is(err, canon.ErrFloat) {
				t.Fatalf("want ErrFloat, got %v", err)
			}
		})
	}
}

func TestHardErrors(t *testing.T) {
	t.Run("invalid-utf8-value", func(t *testing.T) {
		if _, err := canon.Canonical("\xff\xfe"); !errors.Is(err, canon.ErrInvalidUTF8) {
			t.Fatalf("want ErrInvalidUTF8, got %v", err)
		}
	})
	t.Run("nfc-key-collision", func(t *testing.T) {
		m := map[string]any{"café": 1, "café": 2} // NFC vs NFD of the same key
		if _, err := canon.Canonical(m); !errors.Is(err, canon.ErrDuplicateKey) {
			t.Fatalf("want ErrDuplicateKey, got %v", err)
		}
	})
	t.Run("custom-marshaler-refused", func(t *testing.T) {
		if _, err := canon.Canonical(time.Time{}); !errors.Is(err, canon.ErrUnsupported) {
			t.Fatalf("want ErrUnsupported for json.Marshaler type, got %v", err)
		}
	})
	t.Run("number-beyond-uint64", func(t *testing.T) {
		if _, err := canon.Canonical(json.Number("18446744073709551616")); !errors.Is(err, canon.ErrUnsupported) {
			t.Fatalf("want ErrUnsupported, got %v", err)
		}
	})
}

// Seq and Mono are uint64 in kernel/types.go; the full range must
// serialize (the int64-max vector alone does not prove this).
func TestUint64Range(t *testing.T) {
	got, err := canon.Canonical(map[string]any{"seq": uint64(18446744073709551615)})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"seq":18446744073709551615}`; string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Object keys sort by UTF-16 code units (RFC 8785 §3.2.3): U+10000
// (surrogate pair D800 DC00) sorts before U+FF61, though byte order
// says otherwise. Interpretation flagged for /vector-add.
func TestKeyOrderUTF16(t *testing.T) {
	got, err := canon.Canonical(map[string]any{"｡": 1, "\U00010000": 2})
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\"\U00010000\":2,\"｡\":1}"; string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Struct serialization honors kernel/types.go json tags and sorts field
// names like any object keys: a struct and its equivalent map produce
// identical bytes. Exercises the kernel.Canonical delegation too.
func TestStructMatchesEquivalentMap(t *testing.T) {
	e := kernel.Event{
		Seq:         1,
		RunID:       "run_0001",
		PrevHash:    kernel.ZeroHash,
		TS:          1751780000,
		Mono:        1,
		Principal:   "owner",
		Type:        "run.created",
		TypeVersion: 1,
		Payload:     json.RawMessage(`{"profile":"work","agent":"demo"}`), // deliberately unsorted
		Blobs:       []kernel.Hash{},
	}
	fromStruct, err := kernel.Canonical(e)
	if err != nil {
		t.Fatal(err)
	}
	fromMap, err := kernel.Canonical(map[string]any{
		"seq": uint64(1), "run_id": "run_0001", "event_id": "",
		"prev_hash": kernel.ZeroHash, "ts": int64(1751780000), "mono": uint64(1),
		"principal": "owner", "type": "run.created", "type_version": uint16(1),
		"payload": map[string]any{"agent": "demo", "profile": "work"},
		"blobs":   []string{}, "sig": nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fromStruct, fromMap) {
		t.Fatalf("struct and equivalent map diverge:\nstruct: %s\n   map: %s", fromStruct, fromMap)
	}
}

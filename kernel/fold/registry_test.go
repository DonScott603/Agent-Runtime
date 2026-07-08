// Registry validation and the ADR-0023 identity preimage (WP-05a).
package fold_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
	"github.com/DonScott603/Agent-Runtime/kernel/fold"
)

// fixtureInitReducer returns a configurable Init — used to prove the
// registry probes Init at construction. init == "" means nil.
type fixtureInitReducer struct{ init string }

func (r fixtureInitReducer) Init() json.RawMessage {
	if r.init == "" {
		return nil
	}
	return json.RawMessage(r.init)
}

func (fixtureInitReducer) Apply(state json.RawMessage, e kernel.Event) (json.RawMessage, error) {
	return state, nil
}

func TestNewRegistryValidation(t *testing.T) {
	valid := reg("ok", fold.ScopeOwner, nil, countingReducer{})
	cases := []struct {
		name string
		regs []fold.Registration
		want string // substring of the expected error
	}{
		{"empty id", []fold.Registration{reg("", fold.ScopeOwner, nil, countingReducer{})}, "id"},
		{"empty semver", []fold.Registration{{ID: "x", Semver: "", Scope: fold.ScopeOwner, Reducer: countingReducer{}}}, "semver"},
		{"bad scope", []fold.Registration{{ID: "x", Semver: "0.1.0", Scope: fold.Scope("workspace"), Reducer: countingReducer{}}}, "scope"},
		{"nil reducer", []fold.Registration{{ID: "x", Semver: "0.1.0", Scope: fold.ScopeOwner, Reducer: nil}}, "reducer"},
		{"duplicate id", []fold.Registration{valid, reg("ok", fold.ScopeRun, nil, countingReducer{})}, "duplicate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fold.NewRegistry(tc.regs...)
			if err == nil {
				t.Fatal("want validation error, got nil")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
	if _, err := fold.NewRegistry(valid, reg("other", fold.ScopeRun, nil, countingReducer{})); err != nil {
		t.Fatalf("valid registrations rejected: %v", err)
	}
}

// A broken Init (nil or invalid JSON) must fail at construction, not
// event-by-event later.
func TestNewRegistryProbesInit(t *testing.T) {
	for name, r := range map[string]kernel.Reducer{
		"invalid json init": fixtureInitReducer{init: `{not json`},
		"nil init":          fixtureInitReducer{},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := fold.NewRegistry(fold.Registration{
				ID: "x", Semver: "0.1.0", Scope: fold.ScopeOwner, Reducer: r,
			})
			if err == nil {
				t.Fatal("want Init-probe error, got nil")
			}
		})
	}
}

// IdentityHash: the ADR-0023 preimage, pinned against goldens derived
// INDEPENDENTLY of the implementation (process.md §5):
//
//	printf '%s' '{"plugin_id":"run-status","semver":"0.1.0"}' | sha256sum
//	  -> 3a4f846f395ab6bc3641ae10cfe3a079a1ab7a43880d65f37e4ee34028c9781b
//	printf '%s' '{"plugin_id":"run-status","semver":"0.2.0"}' | sha256sum
//	  -> b9a6c6ad03733e89d49769975c4f2c2e22b5e2674c0e870ebc8cf8c9cf63579e
func TestIdentityHashPreimage(t *testing.T) {
	got, err := fold.IdentityHash("run-status", "0.1.0")
	if err != nil {
		t.Fatalf("IdentityHash: %v", err)
	}
	const want = "3a4f846f395ab6bc3641ae10cfe3a079a1ab7a43880d65f37e4ee34028c9781b"
	if got != want {
		t.Errorf("IdentityHash mismatch\n got: %s\nwant: %s", got, want)
	}
	again, err := fold.IdentityHash("run-status", "0.1.0")
	if err != nil || again != got {
		t.Errorf("IdentityHash not deterministic: %s vs %s (%v)", got, again, err)
	}
	bumped, err := fold.IdentityHash("run-status", "0.2.0")
	if err != nil {
		t.Fatalf("IdentityHash: %v", err)
	}
	const wantBumped = "b9a6c6ad03733e89d49769975c4f2c2e22b5e2674c0e870ebc8cf8c9cf63579e"
	if bumped != wantBumped {
		t.Errorf("bumped IdentityHash mismatch\n got: %s\nwant: %s", bumped, wantBumped)
	}
	if bumped == got {
		t.Error("semver bump did not change the identity hash")
	}
}

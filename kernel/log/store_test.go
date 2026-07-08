// Exported-API behavior of the durable store (WP-04b; RFC-0002 §2, D5).
package log_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
	klog "github.com/DonScott603/Agent-Runtime/kernel/log"
)

// proto is an unsealed event with deterministic canon-safe fields.
func proto(run kernel.RunID, i int) kernel.Event {
	return kernel.Event{
		RunID: run, TS: 1751780000 + int64(i), Mono: uint64(i + 1),
		Principal: "agent:demo", Type: "msg.appended", TypeVersion: 1,
		Payload: json.RawMessage(fmt.Sprintf(`{"n":%d}`, i)),
	}
}

func openT(t *testing.T) (*klog.Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := klog.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dir
}

// Global seq is gapless ACROSS runs (D5); each run's chain — including
// owner-scope "" — verifies independently (D4).
func TestAppendAssignsGaplessSeqAcrossRuns(t *testing.T) {
	s, _ := openT(t)
	runs := []kernel.RunID{"run_a", "run_b", "run_a", "", "run_c", "run_b", "", "run_a"}
	for i, r := range runs {
		sealed, err := s.Append(proto(r, i))
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
		if sealed.Seq != kernel.Seq(i+1) {
			t.Fatalf("seq %d, want %d (gapless, D5)", sealed.Seq, i+1)
		}
	}
	got, err := s.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != len(runs) {
		t.Fatalf("ReadAll returned %d events, want %d", len(got), len(runs))
	}
	byRun := map[kernel.RunID][]kernel.Event{}
	for _, e := range got {
		byRun[e.RunID] = append(byRun[e.RunID], e)
	}
	for r, evs := range byRun {
		if err := klog.VerifyChain(evs); err != nil {
			t.Errorf("run %q chain does not verify: %v", r, err)
		}
	}
}

func TestAppendOverridesCallerSeqPrevHashEventID(t *testing.T) {
	s, _ := openT(t)
	e := proto("run_a", 0)
	e.Seq = 999
	e.PrevHash = strings.Repeat("11", 32)
	e.EventID = strings.Repeat("22", 32)
	sealed, err := s.Append(e)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if sealed.Seq != 1 {
		t.Errorf("seq %d, want 1 (assign-on-commit overrides the caller)", sealed.Seq)
	}
	if sealed.PrevHash != kernel.ZeroHash {
		t.Errorf("prev_hash %q, want ZeroHash (genesis threading overrides the caller)", sealed.PrevHash)
	}
	if sealed.EventID == strings.Repeat("22", 32) {
		t.Error("caller-supplied event_id survived sealing")
	}
	re, err := kernel.SealEvent(sealed)
	if err != nil || re.EventID != sealed.EventID {
		t.Errorf("sealed event does not re-derive: %v", err)
	}
}

// ADR-0020 one-byte-form law: nil payload/blobs normalize before
// sealing and persist normalized.
func TestAppendNormalizesNilPayloadBlobs(t *testing.T) {
	s, _ := openT(t)
	sealed, err := s.Append(kernel.Event{
		RunID: "run_a", TS: 1, Mono: 1,
		Principal: "agent:demo", Type: "run.created", TypeVersion: 1,
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if string(sealed.Payload) != "{}" {
		t.Errorf("payload %q, want normalized {}", sealed.Payload)
	}
	if sealed.Blobs == nil || len(sealed.Blobs) != 0 {
		t.Errorf("blobs %#v, want normalized empty slice", sealed.Blobs)
	}
	got, err := s.ReadAll()
	if err != nil || len(got) != 1 {
		t.Fatalf("ReadAll: %d events, err %v", len(got), err)
	}
	g, _ := kernel.Canonical(got[0])
	w, _ := kernel.Canonical(sealed)
	if !bytes.Equal(g, w) {
		t.Errorf("normalized event did not round-trip byte-identically\n got: %s\nwant: %s", g, w)
	}
}

// An oversize record is refused before any byte is written: the store
// is NOT poisoned and the sequence is unaffected.
func TestAppendOversizeRecordRefused(t *testing.T) {
	s, _ := openT(t)
	big := proto("run_a", 0)
	big.Payload = json.RawMessage(fmt.Sprintf(`{"x":%q}`, strings.Repeat("a", 1<<20)))
	if _, err := s.Append(big); err == nil {
		t.Fatal("oversize append accepted")
	}
	sealed, err := s.Append(proto("run_a", 1))
	if err != nil {
		t.Fatalf("Append after refused oversize: %v (store must not be poisoned)", err)
	}
	if sealed.Seq != 1 {
		t.Errorf("seq %d, want 1 — the refused append must not consume a seq", sealed.Seq)
	}
}

func TestCloseIdempotentAppendAfterCloseRefused(t *testing.T) {
	s, _ := openT(t)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v (must be idempotent)", err)
	}
	if _, err := s.Append(proto("run_a", 0)); !errors.Is(err, klog.ErrStoreClosed) {
		t.Fatalf("Append after Close error = %v, want ErrStoreClosed", err)
	}
}

// Determinism at the file level: identical append sequences into two
// stores produce byte-identical files — no ambient state (time,
// entropy, map order) in the write path.
func TestTwoStoresIdenticalInputsIdenticalFiles(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	sa, err := klog.Open(dirA)
	if err != nil {
		t.Fatalf("Open A: %v", err)
	}
	sb, err := klog.Open(dirB)
	if err != nil {
		t.Fatalf("Open B: %v", err)
	}
	runs := []kernel.RunID{"run_a", "run_b", "", "run_a", "run_b"}
	for i, r := range runs {
		ea, err := sa.Append(proto(r, i))
		if err != nil {
			t.Fatalf("Append A %d: %v", i, err)
		}
		eb, err := sb.Append(proto(r, i))
		if err != nil {
			t.Fatalf("Append B %d: %v", i, err)
		}
		if ea.EventID != eb.EventID {
			t.Fatalf("event %d: ids diverge across stores", i)
		}
	}
	sa.Close()
	sb.Close()
	a, err := os.ReadFile(filepath.Join(dirA, "events.log"))
	if err != nil {
		t.Fatalf("read A: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dirB, "events.log"))
	if err != nil {
		t.Fatalf("read B: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("identical inputs produced different files — ambient state in the write path")
	}
}

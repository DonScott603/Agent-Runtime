// STRETCH verification (WP-04a): cross-check seal + chain against the
// annotated trace (docs/trace-annotated.md), an artifact generated
// independently of this codebase. The trace file itself is never
// modified to satisfy this test — a mismatch is a finding
// (corpus/CLAUDE.md golden-file law).
package corpus

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
	klog "github.com/DonScott603/Agent-Runtime/kernel/log"
)

const (
	tracePath = "../docs/trace-annotated.md"
	// "Chain head after seq 120" as documented at the end of the trace.
	traceHead   = "36283240ba955ea123c4eb7fe941f07106c027890c019d26d521c50e9529b59a"
	traceEvents = 20 // seq 101..120
)

var jsonFence = regexp.MustCompile("(?s)```json\\s*(.*?)```")

func TestAnnotatedTraceChain(t *testing.T) {
	raw, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("reading %s: %v", tracePath, err)
	}
	blocks := jsonFence.FindAllSubmatch(raw, -1)
	if len(blocks) != traceEvents {
		t.Fatalf("extracted %d json blocks from %s, want %d — extraction brittle or trace changed", len(blocks), tracePath, traceEvents)
	}
	events := make([]kernel.Event, len(blocks))
	for i, m := range blocks {
		if err := json.Unmarshal(m[1], &events[i]); err != nil {
			t.Fatalf("block %d does not parse as an Event: %v", i, err)
		}
	}
	// VerifyChain re-derives every event_id (including seq 114, whose
	// illustrative sig is zeroed out of the hash) and checks linkage.
	if err := klog.VerifyChain(events); err != nil {
		t.Fatalf("trace chain does not verify: %v", err)
	}
	if head := events[len(events)-1].EventID; head != traceHead {
		t.Errorf("trace head mismatch\n got: %s\nwant: %s", head, traceHead)
	}
}

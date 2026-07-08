// White-box test helpers for the WP-04b durable-store suites.
//
// frameRecord and storeImage build file images INDEPENDENTLY of
// encodeRecord/scanLog: behavioral tests never let the code under
// test construct its own inputs (corpus/CLAUDE.md golden-file law,
// applied in spirit).
package log

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

var testCastagnoli = crc32.MakeTable(crc32.Castagnoli)

// frameRecord frames body per ADR-0022 layout v1, independently of
// encodeRecord: len(u32 LE) | crc32c(lenLE||body) | body.
func frameRecord(t testing.TB, body []byte) []byte {
	t.Helper()
	rec := make([]byte, recordHeaderLen+len(body))
	binary.LittleEndian.PutUint32(rec[0:4], uint32(len(body)))
	copy(rec[recordHeaderLen:], body)
	crc := crc32.Update(0, testCastagnoli, rec[0:4])
	crc = crc32.Update(crc, testCastagnoli, body)
	binary.LittleEndian.PutUint32(rec[4:8], crc)
	return rec
}

// eventBody is the canonical (stored) form of e.
func eventBody(t testing.TB, e kernel.Event) []byte {
	t.Helper()
	b, err := kernel.Canonical(e)
	if err != nil {
		t.Fatalf("Canonical: %v", err)
	}
	return b
}

// storeImage builds a layout-v1 file image: magic then one frame per
// event, in order.
func storeImage(t testing.TB, events ...kernel.Event) []byte {
	t.Helper()
	img := []byte(logMagic)
	for _, e := range events {
		img = append(img, frameRecord(t, eventBody(t, e))...)
	}
	return img
}

func writeStoreFile(t testing.TB, dir string, image []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "events.log"), image, 0o600); err != nil {
		t.Fatalf("writing store file: %v", err)
	}
}

func readStoreFile(t testing.TB, dir string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "events.log"))
	if err != nil {
		t.Fatalf("reading store file: %v", err)
	}
	return b
}

// expectedFileLen is the exact byte length of a store holding events:
// the event-boundary assertion of RFC-0002 §9.3.
func expectedFileLen(t testing.TB, events []kernel.Event) int64 {
	t.Helper()
	n := int64(len(logMagic))
	for _, e := range events {
		n += int64(recordHeaderLen + len(eventBody(t, e)))
	}
	return n
}

// protoEvent is an unsealed event with deterministic canon-safe
// fields; the store assigns seq and threading (ts/mono are
// caller-supplied by contract — the writer never reads time).
func protoEvent(run kernel.RunID, i int) kernel.Event {
	return kernel.Event{
		RunID: run, TS: 1751780000 + int64(i), Mono: uint64(i + 1),
		Principal: "agent:demo", Type: "msg.appended", TypeVersion: 1,
		Payload: json.RawMessage(fmt.Sprintf(`{"n":%d}`, i)),
	}
}

// threadSeal replicates the store's assignment discipline on the
// vector-verified pure layer, for computing EXPECTED results: seq =
// last+1, prev_hash = run head (genesis for an unseen run), then
// kernel.SealEvent.
func threadSeal(lastSeq kernel.Seq, heads map[kernel.RunID]kernel.Hash, e kernel.Event) (kernel.Event, error) {
	e.Seq = lastSeq + 1
	e.EventID = ""
	if h, ok := heads[e.RunID]; ok {
		e.PrevHash = h
	} else {
		e = Genesis(e)
	}
	return kernel.SealEvent(e)
}

// sealedChain builds n sealed, threaded events for run with global
// seq starting at base (base != 1 models a container that starts
// mid-stream, ADR-0022).
func sealedChain(t testing.TB, run kernel.RunID, base kernel.Seq, n int) []kernel.Event {
	t.Helper()
	events := make([]kernel.Event, n)
	for i := range events {
		e := protoEvent(run, i)
		e.Seq = base + kernel.Seq(i)
		if i == 0 {
			e = Genesis(e)
		} else {
			var err error
			e, err = NextInChain(events[i-1], e)
			if err != nil {
				t.Fatalf("NextInChain: %v", err)
			}
		}
		sealed, err := kernel.SealEvent(e)
		if err != nil {
			t.Fatalf("SealEvent: %v", err)
		}
		events[i] = sealed
	}
	return events
}

// assertSameEvents compares by canonical bytes — the byte-identity
// law, not mere structural equality.
func assertSameEvents(t testing.TB, got, want []kernel.Event) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d", len(got), len(want))
	}
	for i := range got {
		g, w := eventBody(t, got[i]), eventBody(t, want[i])
		if !bytes.Equal(g, w) {
			t.Fatalf("event %d differs\n got: %s\nwant: %s", i, g, w)
		}
	}
}

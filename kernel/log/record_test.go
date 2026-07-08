package log

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// Determinism law (CLAUDE.md): every pure byte-producer ships with
// its invoke-twice, byte-compare test in the same commit.
func TestEncodeRecordDeterministic(t *testing.T) {
	body := eventBody(t, sealedChain(t, "run_r", 1, 1)[0])
	a, err := encodeRecord(body)
	if err != nil {
		t.Fatalf("encodeRecord: %v", err)
	}
	b, err := encodeRecord(body)
	if err != nil {
		t.Fatalf("encodeRecord (second): %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("encodeRecord is not deterministic")
	}
}

func TestEncodeRecordFraming(t *testing.T) {
	body := eventBody(t, sealedChain(t, "run_r", 1, 1)[0])
	rec, err := encodeRecord(body)
	if err != nil {
		t.Fatalf("encodeRecord: %v", err)
	}
	// Independent re-derivation of the frame (helper, not the
	// implementation) must agree byte-for-byte.
	if want := frameRecord(t, body); !bytes.Equal(rec, want) {
		t.Fatalf("frame mismatch with independent derivation\n got: %x\nwant: %x", rec, want)
	}
	if got := binary.LittleEndian.Uint32(rec[0:4]); got != uint32(len(body)) {
		t.Errorf("len field = %d, want %d", got, len(body))
	}
	if got := rec[recordHeaderLen:]; !bytes.Equal(got, body) {
		t.Error("body not stored verbatim")
	}
}

func TestEncodeRecordRejectsEmptyAndOversize(t *testing.T) {
	if _, err := encodeRecord(nil); err == nil {
		t.Error("empty body accepted")
	}
	if _, err := encodeRecord([]byte(strings.Repeat("x", maxRecordLen+1))); err == nil {
		t.Error("oversize body accepted (> maxRecordLen)")
	}
}

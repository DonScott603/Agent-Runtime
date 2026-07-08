// Recovery and corruption behavior via file surgery (WP-04b;
// ADR-0022 taxonomy). Torn tails truncate; everything else refuses to
// open with the file bytes untouched.
package log

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

// openRefused opens the store expecting refusal, asserting the file
// bytes are untouched afterward (refusal never truncates or writes).
func openRefused(t *testing.T, dir string) error {
	t.Helper()
	before := readStoreFile(t, dir)
	s, err := Open(dir)
	if err == nil {
		s.Close()
		t.Fatal("Open succeeded, want refusal")
	}
	if after := readStoreFile(t, dir); !bytes.Equal(before, after) {
		t.Fatal("refused Open modified the file bytes")
	}
	return err
}

func TestOpenMissingDirCreates(t *testing.T) {
	dir := t.TempDir() + "/a/b"
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open on missing dir: %v", err)
	}
	defer s.Close()
	if got := readStoreFile(t, dir); string(got) != logMagic {
		t.Fatalf("fresh store file = %q, want bare magic", got)
	}
	sealed, err := s.Append(protoEvent("run_a", 0))
	if err != nil || sealed.Seq != 1 {
		t.Fatalf("first Append: seq %d, err %v", sealed.Seq, err)
	}
}

func TestOpenEmptyDirWritesHeader(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if s.LastSeq() != 0 {
		t.Fatalf("LastSeq = %d on empty store", s.LastSeq())
	}
	if got := readStoreFile(t, dir); string(got) != logMagic {
		t.Fatalf("fresh store file = %q, want bare magic", got)
	}
}

func TestReopenHeaderOnlyStore(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.Close()
	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	if s2.LastSeq() != 0 {
		t.Fatalf("LastSeq = %d, want 0", s2.LastSeq())
	}
	if sealed, err := s2.Append(protoEvent("run_a", 0)); err != nil || sealed.Seq != 1 {
		t.Fatalf("Append after reopen: seq %d, err %v", sealed.Seq, err)
	}
}

// Owner A2: a sub-magic file whose bytes are a prefix of the magic is
// a torn creation — reset by Truncate(0) + rewrite + Sync, never an
// in-place patch. Nothing was ever committed.
func TestTornMagicPrefixResets(t *testing.T) {
	dir := t.TempDir()
	writeStoreFile(t, dir, []byte(logMagic[:4]))
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open over torn creation: %v", err)
	}
	defer s.Close()
	if got := readStoreFile(t, dir); string(got) != logMagic {
		t.Fatalf("file = %q after reset, want bare magic", got)
	}
	if sealed, err := s.Append(protoEvent("run_a", 0)); err != nil || sealed.Seq != 1 {
		t.Fatalf("Append after reset: seq %d, err %v", sealed.Seq, err)
	}
}

// Owner A2: sub-magic bytes that are NOT a magic prefix are LOG_CORRUPT.
func TestSubMagicGarbageRefuses(t *testing.T) {
	dir := t.TempDir()
	writeStoreFile(t, dir, []byte{0xde, 0xad, 0xbe})
	err := openRefused(t, dir)
	var ce *CorruptError
	if !errors.As(err, &ce) {
		t.Fatalf("error = %v, want *CorruptError", err)
	}
}

func TestBadMagicRefuses(t *testing.T) {
	dir := t.TempDir()
	writeStoreFile(t, dir, []byte("NOTALOG!"))
	err := openRefused(t, dir)
	var ce *CorruptError
	if !errors.As(err, &ce) {
		t.Fatalf("error = %v, want *CorruptError", err)
	}
	if !strings.Contains(err.Error(), "LOG_CORRUPT") {
		t.Errorf("error does not carry the LOG_CORRUPT code (docs/errors.md): %v", err)
	}
}

// A synthetic gap between valid records: SEQ_GAP, refuse to open
// (docs/errors.md: treat as corruption).
func TestSeqGapRefuses(t *testing.T) {
	dir := t.TempDir()
	e1 := sealedChain(t, "run_a", 1, 1)[0]
	// seq 3 with CORRECT chain linkage, so the only defect is the gap.
	e3 := protoEvent("run_a", 1)
	e3.Seq = 3
	e3.PrevHash = e1.EventID
	e3, err := kernel.SealEvent(e3)
	if err != nil {
		t.Fatalf("SealEvent: %v", err)
	}
	writeStoreFile(t, dir, storeImage(t, e1, e3))
	oerr := openRefused(t, dir)
	var sg *SeqGapError
	if !errors.As(oerr, &sg) {
		t.Fatalf("error = %v, want *SeqGapError", oerr)
	}
	if sg.Want != 2 || sg.Got != 3 {
		t.Errorf("SeqGapError want/got = %d/%d, want 2/3", sg.Want, sg.Got)
	}
	if !strings.Contains(oerr.Error(), "SEQ_GAP") {
		t.Errorf("error does not carry the SEQ_GAP code (docs/errors.md): %v", oerr)
	}
}

// C-case: a CRC-invalid record with valid data after it is corruption
// of committed bytes — refuse, never truncate (mid-file flip).
func TestMidFileCorruptionRefuses(t *testing.T) {
	dir := t.TempDir()
	evs := sealedChain(t, "run_a", 1, 3)
	img := storeImage(t, evs...)
	// Flip one byte inside record 2's body.
	off := len(logMagic) + recordHeaderLen + len(eventBody(t, evs[0])) + recordHeaderLen + 2
	img[off] ^= 0xff
	writeStoreFile(t, dir, img)
	err := openRefused(t, dir)
	var ce *CorruptError
	if !errors.As(err, &ce) {
		t.Fatalf("error = %v, want *CorruptError", err)
	}
}

// T3: the LAST record fails its CRC at exact EOF — indistinguishable
// from a torn append; truncated, prefix survives (ADR-0022 honest
// boundary).
func TestTornContentAtEOFTruncates(t *testing.T) {
	dir := t.TempDir()
	evs := sealedChain(t, "run_a", 1, 2)
	img := storeImage(t, evs...)
	img[len(img)-3] ^= 0xff // flip inside the final record's body
	writeStoreFile(t, dir, img)
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open over T3 tail: %v", err)
	}
	defer s.Close()
	got, err := s.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	assertSameEvents(t, got, evs[:1])
	if size := int64(len(readStoreFile(t, dir))); size != expectedFileLen(t, evs[:1]) {
		t.Fatalf("file size %d, want %d", size, expectedFileLen(t, evs[:1]))
	}
}

// T4: a garbage length whose region still fits one in-flight append —
// torn header, truncate.
func TestGarbageLenWithinExtentTruncates(t *testing.T) {
	dir := t.TempDir()
	evs := sealedChain(t, "run_a", 1, 2)
	img := storeImage(t, evs...)
	var hdr [recordHeaderLen]byte
	binary.LittleEndian.PutUint32(hdr[0:4], maxRecordLen+9)
	binary.LittleEndian.PutUint32(hdr[4:8], 0x1234_5678)
	img = append(img, hdr[:]...)
	writeStoreFile(t, dir, img)
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open over T4 tail: %v", err)
	}
	defer s.Close()
	got, err := s.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	assertSameEvents(t, got, evs)
	if size := int64(len(readStoreFile(t, dir))); size != expectedFileLen(t, evs) {
		t.Fatalf("file size %d, want %d", size, expectedFileLen(t, evs))
	}
}

// C1: an invalid region larger than one possible append cannot be a
// torn write — refuse.
func TestGarbageRegionBeyondExtentRefuses(t *testing.T) {
	dir := t.TempDir()
	evs := sealedChain(t, "run_a", 1, 1)
	img := storeImage(t, evs...)
	var hdr [recordHeaderLen]byte
	binary.LittleEndian.PutUint32(hdr[0:4], maxRecordLen+9)
	binary.LittleEndian.PutUint32(hdr[4:8], 0x1234_5678)
	img = append(img, hdr[:]...)
	img = append(img, bytes.Repeat([]byte{0xcc}, maxRecordLen+1)...)
	writeStoreFile(t, dir, img)
	err := openRefused(t, dir)
	var ce *CorruptError
	if !errors.As(err, &ce) {
		t.Fatalf("error = %v, want *CorruptError", err)
	}
}

// C2: CRC-valid but chain-broken (a second genesis for an existing
// run). The writer never writes such a record — corruption, refuse;
// the error wraps *ChainBrokenError.
func TestCrcValidChainBrokenRefuses(t *testing.T) {
	dir := t.TempDir()
	e1 := sealedChain(t, "run_a", 1, 1)[0]
	e2 := protoEvent("run_a", 1)
	e2.Seq = 2
	e2 = Genesis(e2) // prev_hash zero although run_a has a head
	e2, err := kernel.SealEvent(e2)
	if err != nil {
		t.Fatalf("SealEvent: %v", err)
	}
	writeStoreFile(t, dir, storeImage(t, e1, e2))
	oerr := openRefused(t, dir)
	var cbe *ChainBrokenError
	if !errors.As(oerr, &cbe) {
		t.Fatalf("error = %v, want wrapped *ChainBrokenError", oerr)
	}
	if cbe.Seq != 2 {
		t.Errorf("broken seq = %d, want 2", cbe.Seq)
	}
}

// C2: CRC-valid but the recorded event_id does not re-derive.
func TestCrcValidStaleEventIDRefuses(t *testing.T) {
	dir := t.TempDir()
	evs := sealedChain(t, "run_a", 1, 2)
	evs[1].EventID = strings.Repeat("ab", 32)
	writeStoreFile(t, dir, storeImage(t, evs...))
	oerr := openRefused(t, dir)
	var cbe *ChainBrokenError
	if !errors.As(oerr, &cbe) {
		t.Fatalf("error = %v, want wrapped *ChainBrokenError", oerr)
	}
}

// C2: a CRC-valid record whose body is not in canonical form (the
// byte-identity law: stored form IS the canonical form).
func TestNonCanonicalBytesRefuse(t *testing.T) {
	dir := t.TempDir()
	e := sealedChain(t, "run_a", 1, 1)[0]
	body, err := json.Marshal(e) // std marshal: declaration key order, not canonical
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if c := eventBody(t, e); bytes.Equal(body, c) {
		t.Skip("std marshal happens to be canonical; fixture cannot exercise this case")
	}
	img := append([]byte(logMagic), frameRecord(t, body)...)
	writeStoreFile(t, dir, img)
	err = openRefused(t, dir)
	var ce *CorruptError
	if !errors.As(err, &ce) {
		t.Fatalf("error = %v, want *CorruptError", err)
	}
}

// C3: a CRC-invalid record framed short of EOF with trailing bytes —
// ambiguous, refuse loudly, never truncate.
func TestCrcInvalidFollowedByBytesRefuses(t *testing.T) {
	dir := t.TempDir()
	evs := sealedChain(t, "run_a", 1, 2)
	img := storeImage(t, evs...)
	img[len(logMagic)+recordHeaderLen+1] ^= 0xff // flip inside record 1's body
	writeStoreFile(t, dir, img)
	err := openRefused(t, dir)
	var ce *CorruptError
	if !errors.As(err, &ce) {
		t.Fatalf("error = %v, want *CorruptError", err)
	}
}

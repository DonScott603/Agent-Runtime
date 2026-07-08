// Recovery scan for the durable store (WP-04b; ADR-0022). Pure — it
// classifies a byte image of the log file; all I/O (truncation, the
// durable Sync of a recovery decision) is the store's job.
package log

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

// scanResult is what recovery learned from a raw file image.
//
// A torn tail (torn=true) is [validLen, EOF): bytes of the at-most-one
// append whose Sync never returned — a record that never committed.
// The store truncates them (WAL-truncate, ADR-0022); truncation cannot
// destroy a committed event because every committed record's Sync
// returned before any byte beyond it was written.
type scanResult struct {
	events   []kernel.Event // the fully valid prefix, seq order
	validLen int64          // B: end offset of the last valid record
	torn     bool           // tail [B, EOF) is one torn append
	tornCase string         // "creation", "T1".."T4" (ADR-0022 taxonomy)
	lastSeq  kernel.Seq     // 0 iff no records
	heads    map[kernel.RunID]kernel.Hash
}

// scanLog validates a whole file image per ADR-0022: magic, framing,
// CRC, JSON parse, canonical-byte identity, seal re-derivation,
// consecutive seq from the first record's base (gaplessness is the
// writer's discipline; the file-local check is its projection onto one
// container), and per-run prev_hash threading (genesis = ZeroHash).
//
// Torn tails (T1-T4, including a torn creation shorter than the magic)
// are REPORTED in the result, never errors. Everything else refuses:
// *SeqGapError for a gap between valid records (docs/errors.md
// SEQ_GAP), *CorruptError for C1-C3 (LOG_CORRUPT; chain-level failures
// wrap *ChainBrokenError). Refusal never implies truncation — the
// caller must leave the bytes untouched.
func scanLog(data []byte) (scanResult, error) {
	res := scanResult{heads: map[kernel.RunID]kernel.Hash{}}
	// Magic. A short prefix OF the magic is a torn creation — Open
	// itself never returned, so nothing was ever committed (owner A2).
	// Any other short or mismatched prefix is corruption.
	if len(data) < len(logMagic) {
		if string(data) == logMagic[:len(data)] {
			res.torn, res.tornCase = true, "creation"
			return res, nil
		}
		return res, &CorruptError{Offset: 0, Detail: fmt.Sprintf("%d-byte file is not a prefix of the log magic", len(data))}
	}
	if string(data[:len(logMagic)]) != logMagic {
		return res, &CorruptError{Offset: 0, Detail: fmt.Sprintf("bad magic %q", data[:len(logMagic)])}
	}
	off := int64(len(logMagic))
	res.validLen = off
	size := int64(len(data))
	for off < size {
		rem := size - off
		if rem < recordHeaderLen { // T1: torn header
			res.torn, res.tornCase = true, "T1"
			return res, nil
		}
		length := binary.LittleEndian.Uint32(data[off : off+4])
		wantCRC := binary.LittleEndian.Uint32(data[off+4 : off+8])
		if length == 0 || length > maxRecordLen {
			if rem <= recordHeaderLen+maxRecordLen { // T4: garbage header within one append's extent
				res.torn, res.tornCase = true, "T4"
				return res, nil
			}
			// C1: cannot be one torn append.
			return res, &CorruptError{Offset: off, Detail: fmt.Sprintf("record length %d invalid with %d trailing bytes — exceeds one append's extent", length, rem)}
		}
		end := off + recordHeaderLen + int64(length)
		if end > size { // T2: incomplete body (always within one append)
			res.torn, res.tornCase = true, "T2"
			return res, nil
		}
		body := data[off+recordHeaderLen : end]
		crc := crc32.Update(0, castagnoli, data[off:off+4])
		crc = crc32.Update(crc, castagnoli, body)
		if crc != wantCRC {
			if end == size { // T3: torn content at exact EOF
				res.torn, res.tornCase = true, "T3"
				return res, nil
			}
			// C3: ambiguous — refuse loudly, never truncate.
			return res, &CorruptError{Offset: off, Detail: "record checksum mismatch with data after it"}
		}
		// The CRC binds these bytes to what the writer wrote, and the
		// writer never writes a record that fails the checks below —
		// every failure from here is corruption of committed bytes (C2).
		var ev kernel.Event
		if err := json.Unmarshal(body, &ev); err != nil {
			return res, &CorruptError{Offset: off, Detail: "record body does not parse as an event", Err: err}
		}
		canonical, err := kernel.Canonical(ev)
		if err != nil {
			return res, &CorruptError{Offset: off, Detail: "record body cannot re-canonicalize", Err: err}
		}
		if !bytes.Equal(canonical, body) {
			return res, &CorruptError{Offset: off, Detail: "record body is not the canonical event form"}
		}
		sealed, err := kernel.SealEvent(ev)
		if err != nil {
			return res, &CorruptError{Offset: off, Detail: "event identity underivable",
				Err: &ChainBrokenError{RunID: ev.RunID, Seq: ev.Seq, Detail: err.Error()}}
		}
		if sealed.EventID != ev.EventID {
			return res, &CorruptError{Offset: off, Detail: "event identity broken",
				Err: &ChainBrokenError{RunID: ev.RunID, Seq: ev.Seq,
					Detail: fmt.Sprintf("event_id %q does not re-derive from the zeroed envelope (derived %q)", ev.EventID, sealed.EventID)}}
		}
		if len(res.events) > 0 && ev.Seq != res.lastSeq+1 {
			return res, &SeqGapError{Want: res.lastSeq + 1, Got: ev.Seq, Offset: off}
		}
		want := kernel.ZeroHash
		if h, ok := res.heads[ev.RunID]; ok {
			want = h
		}
		if ev.PrevHash != want {
			return res, &CorruptError{Offset: off, Detail: "chain linkage broken",
				Err: &ChainBrokenError{RunID: ev.RunID, Seq: ev.Seq,
					Detail: fmt.Sprintf("prev_hash %q does not link to run head %q", ev.PrevHash, want)}}
		}
		res.events = append(res.events, ev)
		res.lastSeq = ev.Seq
		res.heads[ev.RunID] = ev.EventID
		res.validLen = end
		off = end
	}
	return res, nil
}

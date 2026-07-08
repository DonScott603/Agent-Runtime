// Record framing for the durable store (WP-04b; ADR-0022). Local
// layout v1, not a data contract: the frozen thing is the BODY (the
// canonical event form, S1/S2), never this frame. Pure — no os.
package log

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

const (
	// logMagic identifies layout v1; evolution is a new magic, never a
	// rewrite of v1 bytes (ADR-0022).
	logMagic = "ARLOG001"

	// recordHeaderLen = u32 len + u32 crc, both little-endian.
	recordHeaderLen = 8

	// maxRecordLen bounds one record's body (ADR-0022: may be raised,
	// never lowered). Event bodies are small by design — message
	// bodies live in blobs (RFC-0002 D10).
	maxRecordLen = 1 << 20
)

// castagnoli is the CRC-32C table (stdlib; no new dependency).
var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// encodeRecord frames canonical event bytes as
// len(u32 LE) | crc32c(lenLE || body) | body.
// The CRC covers the length so a corrupted length fails the checksum
// instead of silently misframing (ADR-0022). Errors on an empty or
// oversized body; it never truncates.
func encodeRecord(body []byte) ([]byte, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("log: encodeRecord: empty body")
	}
	if len(body) > maxRecordLen {
		return nil, fmt.Errorf("log: encodeRecord: body of %d bytes exceeds maxRecordLen %d (ADR-0022)", len(body), maxRecordLen)
	}
	rec := make([]byte, recordHeaderLen+len(body))
	binary.LittleEndian.PutUint32(rec[0:4], uint32(len(body)))
	copy(rec[recordHeaderLen:], body)
	crc := crc32.Update(0, castagnoli, rec[0:4])
	crc = crc32.Update(crc, castagnoli, body)
	binary.LittleEndian.PutUint32(rec[4:8], crc)
	return rec, nil
}

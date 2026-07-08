// Crash-fault injection matrix (WP-04b; RFC-0002 §9.3). Every crash
// point in the append path is a NAMED case exercised in-process and
// deterministically through the writeSyncer seam; the one real-process
// kill is store_kill_test.go. Written FIRST, per the TDD steer.
package log

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

var errInjectedCrash = errors.New("injected crash (fault seam)")

// tornFile is the fault seam: it can tear exactly one write (only the
// first durable(len) bytes land, then the "process dies") or let the
// write land fully and fail the following Sync — the
// fail-after-N-bytes / before-fsync / after-fsync-before-return crash
// points, reproducibly.
type tornFile struct {
	f       *os.File
	mode    string // "" (passthrough), "torn", "sync-fail"
	durable func(recLen int) int
}

func (tf *tornFile) Write(p []byte) (int, error) {
	if tf.mode == "torn" {
		tf.mode = ""
		k := tf.durable(len(p))
		if k > len(p) {
			k = len(p)
		}
		if k > 0 {
			if _, err := tf.f.Write(p[:k]); err != nil {
				return 0, err
			}
		}
		return k, errInjectedCrash
	}
	return tf.f.Write(p)
}

func (tf *tornFile) Sync() error {
	if tf.mode == "sync-fail" {
		tf.mode = ""
		return errInjectedCrash
	}
	return tf.f.Sync()
}

func (tf *tornFile) Truncate(size int64) error { return tf.f.Truncate(size) }
func (tf *tornFile) Close() error              { return tf.f.Close() }

func TestFaultMatrix(t *testing.T) {
	const run = "run_f"
	cases := []struct {
		name      string
		mode      string // "torn" | "sync-fail" | "none"
		durable   func(recLen int) int
		committed bool // the fatal append's record survives recovery
	}{
		{"crash-before-any-byte", "torn", func(int) int { return 0 }, false},
		{"crash-torn-header-1", "torn", func(int) int { return 1 }, false},
		{"crash-torn-header-4", "torn", func(int) int { return 4 }, false},
		{"crash-torn-header-7", "torn", func(int) int { return 7 }, false},
		{"crash-header-only", "torn", func(int) int { return recordHeaderLen }, false},
		{"crash-mid-payload", "torn", func(n int) int { return recordHeaderLen + (n-recordHeaderLen)/2 }, false},
		{"crash-payload-minus-1", "torn", func(n int) int { return n - 1 }, false},
		// ADR-0022 accepted-unacked: the record is fully on disk and
		// valid; recovery cannot distinguish it from committed.
		{"crash-full-record-unsynced", "sync-fail", nil, true},
		// fsync returned; the "process died" before the caller saw the
		// ack. Present exactly once after recovery.
		{"crash-after-sync-before-return", "none", nil, true},
	}
	for _, preCommits := range []int{0, 2} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/pre-%d", tc.name, preCommits), func(t *testing.T) {
				dir := t.TempDir()
				var tf *tornFile
				s, err := openStore(dir, func(f *os.File) writeSyncer { tf = &tornFile{f: f}; return tf })
				if err != nil {
					t.Fatalf("openStore: %v", err)
				}
				var committed []kernel.Event
				for i := 0; i < preCommits; i++ {
					sealed, err := s.Append(protoEvent(run, i))
					if err != nil {
						t.Fatalf("pre-commit %d: %v", i, err)
					}
					committed = append(committed, sealed)
				}

				// Expected result of the fatal append, from the pure
				// layer: a crashed Append returns an error, so the
				// expectation cannot come from its return value.
				heads := map[kernel.RunID]kernel.Hash{}
				var lastSeq kernel.Seq
				if n := len(committed); n > 0 {
					heads[run] = committed[n-1].EventID
					lastSeq = committed[n-1].Seq
				}
				fatalSealed, err := threadSeal(lastSeq, heads, protoEvent(run, preCommits))
				if err != nil {
					t.Fatalf("threadSeal: %v", err)
				}

				if tc.mode != "none" {
					tf.mode, tf.durable = tc.mode, tc.durable
				}
				got, err := s.Append(protoEvent(run, preCommits))
				if tc.mode == "none" {
					if err != nil {
						t.Fatalf("Append: %v", err)
					}
					if got.EventID != fatalSealed.EventID {
						t.Fatalf("Append result diverges from pure-layer expectation: %s vs %s", got.EventID, fatalSealed.EventID)
					}
				} else {
					if err == nil {
						t.Fatal("injected crash did not surface from Append")
					}
					// The store is poisoned until reopened (the
					// append-after-failed-append-refused case, folded
					// into every fault case).
					if _, err := s.Append(protoEvent(run, preCommits+1)); !errors.Is(err, ErrStorePoisoned) {
						t.Fatalf("poisoned Append error = %v, want ErrStorePoisoned", err)
					}
					if _, err := s.ReadAll(); !errors.Is(err, ErrStorePoisoned) {
						t.Fatalf("poisoned ReadAll error = %v, want ErrStorePoisoned", err)
					}
				}
				_ = s.Close() // fd release only, like process death; never writes

				want := committed
				if tc.committed {
					want = append(committed[:len(committed):len(committed)], fatalSealed)
				}

				s2, err := openStore(dir, nil)
				if err != nil {
					t.Fatalf("recovery open: %v", err)
				}
				gotEvents, err := s2.ReadAll()
				if err != nil {
					t.Fatalf("ReadAll after recovery: %v", err)
				}
				assertSameEvents(t, gotEvents, want)
				// Recovered log ends at an event boundary (RFC-0002 §9.3).
				if size := int64(len(readStoreFile(t, dir))); size != expectedFileLen(t, want) {
					t.Fatalf("file size %d, want event boundary %d", size, expectedFileLen(t, want))
				}
				var wantLast kernel.Seq
				if n := len(want); n > 0 {
					wantLast = want[n-1].Seq
				}
				if s2.LastSeq() != wantLast {
					t.Fatalf("LastSeq %d, want %d", s2.LastSeq(), wantLast)
				}
				// Gapless continuation after recovery.
				next, err := s2.Append(protoEvent(run, 90))
				if err != nil {
					t.Fatalf("post-recovery Append: %v", err)
				}
				if next.Seq != wantLast+1 {
					t.Fatalf("post-recovery seq %d, want %d", next.Seq, wantLast+1)
				}
				_ = s2.Close()
				// Recovery idempotence: a second reopen is stable.
				s3, err := openStore(dir, nil)
				if err != nil {
					t.Fatalf("second reopen: %v", err)
				}
				got3, err := s3.ReadAll()
				if err != nil {
					t.Fatalf("ReadAll on second reopen: %v", err)
				}
				assertSameEvents(t, got3, append(want[:len(want):len(want)], next))
				_ = s3.Close()
			})
		}
	}
}

// opRecorder logs the order of writeSyncer calls during Open so
// recovery-durability ordering is assertable.
type opRecorder struct {
	f   *os.File
	ops []string
}

func (r *opRecorder) Write(p []byte) (int, error) {
	r.ops = append(r.ops, fmt.Sprintf("write(%d)", len(p)))
	return r.f.Write(p)
}

func (r *opRecorder) Sync() error {
	r.ops = append(r.ops, "sync")
	return r.f.Sync()
}

func (r *opRecorder) Truncate(size int64) error {
	r.ops = append(r.ops, fmt.Sprintf("truncate(%d)", size))
	return r.f.Truncate(size)
}

func (r *opRecorder) Close() error { return r.f.Close() }

// Owner A3: after any recovery truncation, Sync before Open completes
// — recovery decisions are themselves durable.
func TestRecoveryTruncateSyncsBeforeOpenReturns(t *testing.T) {
	dir := t.TempDir()
	evs := sealedChain(t, "run_a", 1, 2)
	img := append(storeImage(t, evs...), 0x01, 0x02, 0x03) // T1 torn header
	writeStoreFile(t, dir, img)
	var rec *opRecorder
	s, err := openStore(dir, func(f *os.File) writeSyncer { rec = &opRecorder{f: f}; return rec })
	if err != nil {
		t.Fatalf("openStore over torn tail: %v", err)
	}
	defer s.Close()
	b := expectedFileLen(t, evs)
	idxTrunc := -1
	synced := false
	for i, op := range rec.ops {
		if op == fmt.Sprintf("truncate(%d)", b) && idxTrunc == -1 {
			idxTrunc = i
		}
		if idxTrunc != -1 && i > idxTrunc && op == "sync" {
			synced = true
			break
		}
	}
	if idxTrunc == -1 || !synced {
		t.Fatalf("recovery did not truncate-to-%d then sync before Open returned; ops: %v", b, rec.ops)
	}
	if got := int64(len(readStoreFile(t, dir))); got != b {
		t.Fatalf("file size %d after recovery, want %d", got, b)
	}
}

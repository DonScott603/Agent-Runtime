// Real-process kill capstone (WP-04b): the RFC-0002 §9.3 "kill -9
// during append" conformance instance. A child appender is killed at
// a random point (Process.Kill = TerminateProcess on Windows — the
// kill -9 analogue; the API is portable, so the guard is -short, not
// an OS gate); recovery must land on an event boundary, gapless, with
// every ACKNOWLEDGED append present.
package log_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/DonScott603/Agent-Runtime/kernel"
	klog "github.com/DonScott603/Agent-Runtime/kernel/log"
)

// TestHelperAppender is the child process body, not a test: it opens
// the store at LOG_DIR and appends until killed, acking each commit
// on stdout only AFTER Append returns.
func TestHelperAppender(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		t.Skip("helper process body; driven by TestKillDuringAppendRecovers")
	}
	s, err := klog.Open(os.Getenv("LOG_DIR"))
	if err != nil {
		fmt.Printf("helper open error: %v\n", err)
		os.Exit(1)
	}
	for i := 0; ; i++ {
		sealed, err := s.Append(kernel.Event{
			RunID: "run_kill", TS: int64(i), Mono: uint64(i + 1),
			Principal: "agent:kill", Type: "msg.appended", TypeVersion: 1,
			Payload: json.RawMessage(fmt.Sprintf(`{"n":%d}`, i)),
		})
		if err != nil {
			fmt.Printf("helper append error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("acked %d\n", sealed.Seq) // unbuffered: straight to the pipe
	}
}

func TestKillDuringAppendRecovers(t *testing.T) {
	if testing.Short() {
		t.Skip("real-process kill test; skipped under -short")
	}
	for iter := 0; iter < 3; iter++ {
		t.Run(fmt.Sprintf("iter-%d", iter), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "events.log")
			threshold := int64(512 + rand.IntN(8192)) // random kill point
			t.Logf("killing after file grows past %d bytes", threshold)

			cmd := exec.Command(os.Args[0], "-test.run=TestHelperAppender$", "-test.v")
			cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "LOG_DIR="+dir)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatalf("stdout pipe: %v", err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatalf("starting helper: %v", err)
			}

			var lastAcked kernel.Seq
			ackDone := make(chan struct{})
			go func() {
				defer close(ackDone)
				sc := bufio.NewScanner(stdout)
				for sc.Scan() {
					var n uint64
					if _, err := fmt.Sscanf(sc.Text(), "acked %d", &n); err == nil {
						lastAcked = kernel.Seq(n)
					}
				}
			}()

			deadline := time.Now().Add(30 * time.Second)
			for {
				if fi, err := os.Stat(path); err == nil && fi.Size() >= threshold {
					break
				}
				if time.Now().After(deadline) {
					cmd.Process.Kill()
					cmd.Wait()
					<-ackDone
					t.Fatalf("helper never grew the log past %d bytes; stderr:\n%s", threshold, stderr.String())
				}
				time.Sleep(time.Millisecond)
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatalf("kill: %v", err)
			}
			cmd.Wait() // expected to be non-nil: the process was killed
			<-ackDone  // reader saw pipe EOF; lastAcked is stable now

			s, err := klog.Open(dir)
			if err != nil {
				t.Fatalf("recovery Open after kill: %v", err)
			}
			defer s.Close()
			events, err := s.ReadAll()
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			for i, e := range events {
				if e.Seq != kernel.Seq(i+1) {
					t.Fatalf("event %d has seq %d — recovered log not gapless from 1", i, e.Seq)
				}
			}
			if err := klog.VerifyChain(events); err != nil {
				t.Fatalf("recovered chain does not verify: %v", err)
			}
			// Durability: at least every acknowledged append survives
			// (ADR-0022; unacked-but-recovered is permitted).
			if s.LastSeq() < lastAcked {
				t.Fatalf("recovered LastSeq %d < last acknowledged %d — a committed event was lost", s.LastSeq(), lastAcked)
			}
			t.Logf("killed with %d acked, recovered %d events", lastAcked, len(events))
		})
	}
}

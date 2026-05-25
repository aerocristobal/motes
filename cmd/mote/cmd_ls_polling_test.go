// SPDX-License-Identifier: MIT
//
// STORY-EMPTY-001 §2 Scenario 8 — the documented polling idiom terminates
// exactly when work becomes available, with no spurious non-zero exits
// during the empty interval.
//
// Per story §10: polling is the riskiest scenario for flake. We poll
// in-process (no shell-out) with a short interval and a generous deadline,
// and skip under `-short` so the default fast lane stays fast.
package main

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"motes/internal/core"
)

func TestPollingLoop_TerminatesWhenWorkAppears(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping polling end-to-end in -short mode")
	}

	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	const (
		pollInterval = 100 * time.Millisecond
		seedAfter    = 500 * time.Millisecond
		deadline     = 5 * time.Second
	)

	type pollResult struct {
		stdout string
		err    error
	}

	var (
		mu          sync.Mutex
		results     []pollResult
		foundMoteID string
		stop        = make(chan struct{})
		done        = make(chan struct{})
	)

	// Poller goroutine: invokes `ls --ready --json` until it observes a
	// non-empty motes array, then signals via foundMoteID.
	go func() {
		defer close(done)
		deadlineAt := time.Now().Add(deadline)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if time.Now().After(deadlineAt) {
				return
			}

			var perr error
			stdout := captureStdout(func() {
				perr = runLsViaCobra([]string{"ls", "--ready", "--json"})
			})

			mu.Lock()
			results = append(results, pollResult{stdout: stdout, err: perr})
			mu.Unlock()

			if perr == nil {
				var parsed LsOutput
				trimmed := strings.TrimSpace(stdout)
				if jerr := json.Unmarshal([]byte(trimmed), &parsed); jerr == nil && len(parsed.Motes) > 0 {
					mu.Lock()
					foundMoteID = parsed.Motes[0].ID
					mu.Unlock()
					return
				}
			}
			time.Sleep(pollInterval)
		}
	}()

	// After a delay, seed a ready mote. The poller should observe it within
	// one or two more poll intervals.
	time.Sleep(seedAfter)
	mm := core.NewMoteManager(root)
	seeded, err := mm.Create("task", "delayed ready task", core.CreateOpts{})
	if err != nil {
		close(stop)
		t.Fatalf("seed: %v", err)
	}

	select {
	case <-done:
		// Poller returned. Validate.
	case <-time.After(deadline + time.Second):
		close(stop)
		<-done
		t.Fatalf("polling did not observe new mote within deadline (%s)", deadline)
	}

	mu.Lock()
	defer mu.Unlock()

	if foundMoteID == "" {
		t.Fatalf("poller returned without finding any mote; %d polls captured", len(results))
	}
	if foundMoteID != seeded.ID {
		t.Errorf("found mote id: got %q, want %q", foundMoteID, seeded.ID)
	}

	// Every poll up to discovery must have exited 0. While the workspace
	// was empty the response must have been the empty-state envelope.
	for i, r := range results {
		if r.err != nil {
			t.Errorf("poll %d returned spurious error: %v", i, r.err)
		}
	}
	// The last poll is the one that observed the new mote; all earlier
	// polls (results[:len-1]) must have produced the empty envelope.
	for i := 0; i < len(results)-1; i++ {
		if strings.TrimSpace(results[i].stdout) != `{"motes":[]}` {
			t.Errorf("poll %d (during empty interval) produced unexpected stdout: %q",
				i, results[i].stdout)
		}
	}
}

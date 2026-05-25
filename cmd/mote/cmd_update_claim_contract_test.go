// SPDX-License-Identifier: MIT
//
// STORY-EMPTY-001 §2 Scenario 7 — the partner contract: when a specific
// claim attempt loses the race, the resulting non-zero exit must not
// poison subsequent `mote ls --ready --json` calls. The two halves of the
// agent polling idiom (ls --ready and update --claim) compose cleanly.
//
// The exit-code-2 assertion for contention itself lives in
// cmd_update_claim_test.go (TestUpdateClaim_AlreadyClaimed_Exit2). This
// file picks up where that test stops: after the failed claim, the next
// `ls --ready --json` returns the clean empty-state envelope with exit 0.
package main

import (
	"errors"
	"strings"
	"testing"

	"motes/internal/core"
)

func TestClaim_ContentionDoesNotPoisonLaterLsReady(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	m := seedClaimTask(t, memDir, "contested")

	// First agent claims the task.
	t.Setenv("MOTE_AGENT_ID", "codex-beta")
	if err := runClaimViaCobra([]string{"update", m.ID, "--claim"}); err != nil {
		t.Fatalf("first claim: %v", err)
	}

	// Second agent loses the race — exit 2 expected (this is the
	// contention path, already covered in cmd_update_claim_test.go but
	// asserted here too so the regression surface is local).
	t.Setenv("MOTE_AGENT_ID", "claude-alpha")
	err := runClaimViaCobra([]string{"update", m.ID, "--claim", "--json"})
	if err == nil {
		t.Fatal("expected error on contended claim, got nil")
	}
	var ec *exitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("expected *exitCodeError, got %T: %v", err, err)
	}
	if ec.code != 2 {
		t.Errorf("contention should return exit code 2, got %d", ec.code)
	}
	if !errors.Is(err, core.ErrAlreadyClaimed) {
		t.Errorf("expected ErrAlreadyClaimed in chain, got %v", err)
	}

	// Now the partner half: ls --ready must report cleanly. The only
	// task is in_progress (claimed by codex-beta), so no work is ready
	// — the response must be {"motes":[]} + exit 0. A successful poll
	// shows that the failed claim did not corrupt index, cache, or
	// filesystem state.
	var lsErr error
	lsStdout := captureStdout(func() {
		lsErr = runLsViaCobra([]string{"ls", "--ready", "--json"})
	})
	if lsErr != nil {
		t.Fatalf("ls --ready after contention: expected nil error, got %v", lsErr)
	}
	if strings.TrimSpace(lsStdout) != `{"motes":[]}` {
		t.Errorf("ls --ready after contention: expected empty envelope, got %q", lsStdout)
	}
}

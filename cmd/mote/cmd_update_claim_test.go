// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"motes/internal/core"
)

// captureStderr captures output written to os.Stderr during fn.
func captureStderr(fn func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	fn()
	_ = w.Close()
	os.Stderr = old
	data, _ := io.ReadAll(r)
	return string(data)
}

// runClaimViaCobra invokes the update command through cobra with the given
// args, mirroring how the CLI receives input from a shell. Returns the
// error returned by RunE.
func runClaimViaCobra(args []string) error {
	updateClaim = false
	updateJSON = false
	defer func() {
		updateClaim = false
		updateJSON = false
	}()
	rootCmd.SetArgs(args)
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	return rootCmd.Execute()
}

// seedClaimTask creates one active task mote in the test workspace.
func seedClaimTask(t *testing.T, memDir, title string) *core.Mote {
	t.Helper()
	mm := core.NewMoteManager(memDir)
	m, err := mm.Create("task", title, core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	return m
}

// --- HAPPY PATH JSON (Scenario 1) ---

func TestUpdateClaim_HappyPath_JSON(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	m := seedClaimTask(t, memDir, "ready task")
	t.Setenv("MOTE_AGENT_ID", "claude-alpha")

	output := captureStdout(func() {
		if err := runClaimViaCobra([]string{"update", m.ID, "--claim", "--json"}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	var out ClaimOutput
	if err := json.Unmarshal([]byte(output), &out); err != nil {
		t.Fatalf("invalid JSON %q: %v", output, err)
	}
	if !out.Claimed {
		t.Errorf("expected claimed=true, got %+v", out)
	}
	if out.ClaimedBy != "claude-alpha" {
		t.Errorf("ClaimedBy: got %q, want claude-alpha", out.ClaimedBy)
	}
	if out.Status != "in_progress" {
		t.Errorf("Status: got %q, want in_progress", out.Status)
	}

	// Verify on-disk state.
	mm := core.NewMoteManager(memDir)
	got, _ := mm.Read(m.ID)
	if got.Status != "in_progress" {
		t.Errorf("on-disk status: got %q, want in_progress", got.Status)
	}
	if got.ClaimedBy != "claude-alpha" {
		t.Errorf("on-disk claimed_by: got %q, want claude-alpha", got.ClaimedBy)
	}
}

// --- HUMAN OUTPUT (Scenario 2) ---

func TestUpdateClaim_HumanOutput(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	m := seedClaimTask(t, memDir, "human")
	t.Setenv("MOTE_AGENT_ID", "claude-alpha")

	output := captureStdout(func() {
		if err := runClaimViaCobra([]string{"update", m.ID, "--claim"}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	want := "Claimed " + m.ID + " as claude-alpha"
	if !strings.Contains(output, want) {
		t.Errorf("expected %q in stdout, got %q", want, output)
	}
}

// --- BOUNDARY: ready queue empties (Scenario 3) ---

func TestUpdateClaim_LastReadyMote_QueueEmpties(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	m := seedClaimTask(t, memDir, "the only one")
	t.Setenv("MOTE_AGENT_ID", "claude-alpha")

	if err := runClaimViaCobra([]string{"update", m.ID, "--claim", "--json"}); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Now invoke `mote ls --ready --json` and assert STORY-EMPTY-001 contract.
	lsReady = false
	lsJSON = false
	defer func() {
		lsReady = false
		lsJSON = false
	}()

	lsOutput := captureStdout(func() {
		if err := runClaimViaCobra([]string{"ls", "--ready", "--json"}); err != nil {
			t.Errorf("ls --ready --json: %v", err)
		}
	})

	if !strings.Contains(lsOutput, `{"motes":[]}`) {
		t.Errorf("expected empty motes array, got %q", lsOutput)
	}
}

// --- ERROR: already claimed (Scenario 4) — exit 2 ---

func TestUpdateClaim_AlreadyClaimed_Exit2(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	m := seedClaimTask(t, memDir, "contested")

	t.Setenv("MOTE_AGENT_ID", "codex-beta")
	if err := runClaimViaCobra([]string{"update", m.ID, "--claim"}); err != nil {
		t.Fatalf("first claim: %v", err)
	}

	t.Setenv("MOTE_AGENT_ID", "claude-alpha")

	var err error
	stdout := captureStdout(func() {
		err = runClaimViaCobra([]string{"update", m.ID, "--claim", "--json"})
	})

	if err == nil {
		t.Fatal("expected error on second claim, got nil")
	}
	var ec *exitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("expected *exitCodeError, got %T: %v", err, err)
	}
	if ec.code != 2 {
		t.Errorf("expected exit code 2, got %d", ec.code)
	}
	if !errors.Is(err, core.ErrAlreadyClaimed) {
		t.Errorf("expected ErrAlreadyClaimed in chain, got %v", err)
	}
	if !strings.Contains(err.Error(), "codex-beta") {
		t.Errorf("expected error to name current claimer, got %q", err.Error())
	}

	// JSON must still be on stdout despite the error.
	var out ClaimOutput
	if jErr := json.Unmarshal([]byte(stdout), &out); jErr != nil {
		t.Fatalf("expected JSON on stdout, got %q (%v)", stdout, jErr)
	}
	if out.Claimed {
		t.Errorf("expected claimed=false in JSON, got %+v", out)
	}
	if out.CurrentClaimedBy != "codex-beta" {
		t.Errorf("CurrentClaimedBy: got %q, want codex-beta", out.CurrentClaimedBy)
	}
}

// --- ERROR: terminal status (Scenario 5) — exit 1 ---

func TestUpdateClaim_TerminalStatus_Exit1(t *testing.T) {
	cases := []struct {
		status   string
		fragment string
	}{
		{"completed", "status=completed"},
		{"archived", "status=archived"},
		{"deprecated", "status=deprecated"},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			memDir, cleanup := setupIntegrationTest(t)
			defer cleanup()

			m := seedClaimTask(t, memDir, "terminal")
			mm := core.NewMoteManager(memDir)
			if err := mm.Update(m.ID, core.UpdateOpts{Status: core.StringPtr(tc.status)}); err != nil {
				t.Fatalf("setup: %v", err)
			}

			t.Setenv("MOTE_AGENT_ID", "claude-alpha")
			err := runClaimViaCobra([]string{"update", m.ID, "--claim"})

			if err == nil {
				t.Fatal("expected error for terminal status, got nil")
			}
			var ec *exitCodeError
			if !errors.As(err, &ec) {
				t.Fatalf("expected *exitCodeError, got %T", err)
			}
			if ec.code != 1 {
				t.Errorf("expected exit 1, got %d", ec.code)
			}
			if !errors.Is(err, core.ErrNotClaimable) {
				t.Errorf("expected ErrNotClaimable, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.fragment) {
				t.Errorf("expected fragment %q in error, got %q", tc.fragment, err.Error())
			}
		})
	}
}

// --- ERROR: blockers unfinished (Scenario 6) — exit 1 ---

func TestUpdateClaim_BlockersUnfinished_Exit1(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(memDir)
	blocker, _ := mm.Create("task", "blocker", core.CreateOpts{})
	target, _ := mm.Create("task", "blocked", core.CreateOpts{})
	im := core.NewIndexManager(memDir)
	if err := mm.Link(target.ID, "depends_on", blocker.ID, im); err != nil {
		t.Fatalf("Link: %v", err)
	}

	t.Setenv("MOTE_AGENT_ID", "claude-alpha")
	err := runClaimViaCobra([]string{"update", target.ID, "--claim"})

	if err == nil {
		t.Fatal("expected error when blockers unfinished, got nil")
	}
	var ec *exitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("expected *exitCodeError, got %T", err)
	}
	if ec.code != 1 {
		t.Errorf("expected exit 1, got %d", ec.code)
	}
	if !errors.Is(err, core.ErrNotReady) {
		t.Errorf("expected ErrNotReady, got %v", err)
	}
	if !strings.Contains(err.Error(), "1 unfinished blocker") {
		t.Errorf("expected blocker count in error, got %q", err.Error())
	}
}

// --- ERROR: MOTE_AGENT_ID env var (Scenario 7) — exit 1 ---

func TestUpdateClaim_MissingAgentID_Exit1(t *testing.T) {
	cases := []struct {
		name     string
		setVal   string
		unset    bool
		fragment string
	}{
		{"unset", "", true, "MOTE_AGENT_ID is required for --claim"},
		{"empty", "", false, "MOTE_AGENT_ID is required for --claim"},
		{"path_traversal", "../../etc/pwd", false, "invalid MOTE_AGENT_ID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			memDir, cleanup := setupIntegrationTest(t)
			defer cleanup()

			m := seedClaimTask(t, memDir, "subject")

			if tc.unset {
				_ = os.Unsetenv("MOTE_AGENT_ID")
			} else {
				t.Setenv("MOTE_AGENT_ID", tc.setVal)
			}

			err := runClaimViaCobra([]string{"update", m.ID, "--claim"})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var ec *exitCodeError
			if !errors.As(err, &ec) {
				t.Fatalf("expected *exitCodeError, got %T", err)
			}
			if ec.code != 1 {
				t.Errorf("expected exit 1, got %d", ec.code)
			}
			if !strings.Contains(err.Error(), tc.fragment) {
				t.Errorf("expected %q in error, got %q", tc.fragment, err.Error())
			}

			// On-disk mote unchanged.
			mm := core.NewMoteManager(memDir)
			got, _ := mm.Read(m.ID)
			if got.Status != "active" {
				t.Errorf("on-disk status mutated: got %q, want active", got.Status)
			}
			if got.ClaimedBy != "" {
				t.Errorf("on-disk claimed_by stamped despite error: %q", got.ClaimedBy)
			}
		})
	}
}

// --- ERROR: mutually exclusive with field flags ---

func TestUpdateClaim_MutuallyExclusiveWithStatus_Exit1(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	m := seedClaimTask(t, memDir, "subject")
	t.Setenv("MOTE_AGENT_ID", "claude-alpha")

	err := runClaimViaCobra([]string{"update", m.ID, "--claim", "--status=completed"})
	if err == nil {
		t.Fatal("expected error for --claim + --status, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutually-exclusive error, got %q", err.Error())
	}
}

// Reference captureStderr so unused-import warnings don't appear if test list
// changes; the helper is exported for parity with captureStdout.
var _ = captureStderr

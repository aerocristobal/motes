// SPDX-License-Identifier: MIT
package main

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"motes/internal/core"
)

func resetUpdateFlags() {
	updateStatus, updateTitle, updateBody, updateSize, updateParent = "", "", "", "", ""
	updateWeight = 0
	updateAddTag, updateAccept = nil, nil
	updateForce, updateQuiet, updateClaim, updateJSON = false, false, false, false
	updateExecutionAgentType = ""
	updateExecutionSuggestedModel = ""
	updateExecutionReasoningEffort = ""
	updateExecutionMode = ""
	updateExecutionParallelGroup = ""
	updateDue = ""
	updateDefer = ""
	// pflag retains Changed=true across Execute() invocations; clear so each
	// test starts from a clean slate.
	updateCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
}

func runUpdateViaCobra(args []string) error {
	resetUpdateFlags()
	defer resetUpdateFlags()
	rootCmd.SetArgs(args)
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	return rootCmd.Execute()
}

func TestUpdate_SetExecutionField(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	m := seedClaimTask(t, memDir, "exec target")

	if err := runUpdateViaCobra([]string{"update", m.ID, "--execution-mode=delegated", "--execution-suggested-model=sonnet"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	mm := core.NewMoteManager(memDir)
	got, _ := mm.Read(m.ID)
	if got.ExecutionMode != "delegated" || got.ExecutionSuggestedModel != "sonnet" {
		t.Errorf("not set: mode=%q model=%q", got.ExecutionMode, got.ExecutionSuggestedModel)
	}
}

func TestUpdate_ClearExecutionFieldWithEmptyString(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	m := seedClaimTask(t, memDir, "clear target")

	// First set a value
	if err := runUpdateViaCobra([]string{"update", m.ID, "--execution-parallel-group=group-A"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Now clear it
	if err := runUpdateViaCobra([]string{"update", m.ID, "--execution-parallel-group="}); err != nil {
		t.Fatalf("clear: %v", err)
	}

	mm := core.NewMoteManager(memDir)
	got, _ := mm.Read(m.ID)
	if got.ExecutionParallelGroup != "" {
		t.Errorf("expected cleared, got %q", got.ExecutionParallelGroup)
	}

	// And the on-disk key should be absent (omitempty)
	path, _ := mm.MoteFilePath(m.ID)
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "execution_parallel_group:") {
		t.Errorf("cleared key should be absent from frontmatter:\n%s", string(raw))
	}
}

func TestUpdate_ChangeAfterLaunch_WarningEmitted(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	m := seedClaimTask(t, memDir, "claim target")
	t.Setenv("MOTE_AGENT_ID", "claude-alpha")

	// Claim it first.
	if err := runClaimViaCobra([]string{"update", m.ID, "--claim"}); err != nil {
		t.Fatalf("claim: %v", err)
	}

	stderrBuf := captureStderr(func() {
		if err := runUpdateViaCobra([]string{"update", m.ID, "--execution-suggested-model=opus"}); err != nil {
			t.Errorf("update: %v", err)
		}
	})
	if !strings.Contains(stderrBuf, "changing execution metadata after dispatch") {
		t.Errorf("expected change-after-launch warning on stderr; got: %q", stderrBuf)
	}
}

func TestUpdate_ExecutionFlagsMutexWithClaim(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	m := seedClaimTask(t, memDir, "mutex")
	t.Setenv("MOTE_AGENT_ID", "claude-alpha")

	err := runUpdateViaCobra([]string{"update", m.ID, "--claim", "--execution-mode=parallel"})
	if err == nil {
		t.Fatal("expected mutex error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutex message: %v", err)
	}
}

func TestUpdate_InvalidExecutionMode_Rejected(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()
	m := seedClaimTask(t, memDir, "rejection")

	err := runUpdateViaCobra([]string{"update", m.ID, "--execution-mode=fire_and_forget"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "invalid execution_mode") {
		t.Errorf("error should mention 'invalid execution_mode': %v", err)
	}
	mm := core.NewMoteManager(memDir)
	got, _ := mm.Read(m.ID)
	if got.ExecutionMode != "" {
		t.Errorf("mode should not have been written on rejected update, got %q", got.ExecutionMode)
	}
}

func TestUpdate_NonexistentMote_ReturnsError(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()
	err := runUpdateViaCobra([]string{"update", "motes-doesnotexist", "--execution-mode=parallel"})
	if err == nil {
		t.Fatal("expected error for nonexistent mote")
	}
}

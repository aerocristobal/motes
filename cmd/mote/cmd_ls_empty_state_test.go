// SPDX-License-Identifier: MIT
//
// STORY-EMPTY-001 — Empty-state semantics for `mote ls --ready --json`.
//
// Scenarios 1–6 from STORY-EMPTY-001 §2. Scenario 7 (claim contention does
// not poison later ls) lives in cmd_update_claim_contract_test.go; the
// timing-sensitive polling-loop check (Scenario 8) lives in
// cmd_ls_polling_test.go.
//
// The contract these tests pin down:
//
//	stdout == `{"motes":[]}\n` and exit 0 when nothing is claimable, in
//	every empty-state condition. Empty is a normal outcome, not an error.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/core"

	"github.com/spf13/cobra"
)

// runLsViaCobra invokes `mote ls ...` through cobra, mirroring how a real
// shell would launch the binary. It resets the ls flag globals before and
// after so tests are hermetic.
func runLsViaCobra(args []string) error {
	lsType = ""
	lsTag = ""
	lsStatus = ""
	lsStale = false
	lsReady = false
	lsCompact = false
	lsParent = ""
	lsJSON = false
	defer func() {
		lsType = ""
		lsTag = ""
		lsStatus = ""
		lsStale = false
		lsReady = false
		lsCompact = false
		lsParent = ""
		lsJSON = false
	}()
	rootCmd.SetArgs(args)
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	return rootCmd.Execute()
}

// --- SCENARIO 1: empty workspace ---

func TestLs_ReadyJSON_EmptyWorkspace_ReturnsEmptyMotesArray(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	var err error
	stdout := captureStdout(func() {
		err = runLsViaCobra([]string{"ls", "--ready", "--json"})
	})

	if err != nil {
		t.Fatalf("expected nil error (exit 0), got %v", err)
	}
	trimmed := strings.TrimSpace(stdout)
	if trimmed != `{"motes":[]}` {
		t.Errorf("expected stdout to be exactly {\"motes\":[]}, got: %q", trimmed)
	}

	// Round-trip parse to catch shape drift (e.g., "motes":null).
	var parsed struct {
		Motes []map[string]any `json:"motes"`
	}
	if uerr := json.Unmarshal([]byte(trimmed), &parsed); uerr != nil {
		t.Fatalf("output is not valid JSON: %v", uerr)
	}
	if parsed.Motes == nil {
		t.Error("motes must be [] (empty array), not null")
	}
	if len(parsed.Motes) != 0 {
		t.Errorf("expected empty motes array, got %d entries", len(parsed.Motes))
	}
}

// --- SCENARIO 2: all motes in_progress ---

func TestLs_ReadyJSON_AllInProgress_ReturnsEmptyMotesArray(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	for i := 0; i < 3; i++ {
		m, err := mm.Create("task", "in-flight task", core.CreateOpts{})
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := mm.Update(m.ID, core.UpdateOpts{Status: core.StringPtr("in_progress")}); err != nil {
			t.Fatalf("set in_progress: %v", err)
		}
	}

	var err error
	stdout := captureStdout(func() {
		err = runLsViaCobra([]string{"ls", "--ready", "--json"})
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if strings.TrimSpace(stdout) != `{"motes":[]}` {
		t.Errorf("expected empty motes array, got %q", stdout)
	}
}

// --- SCENARIO 3: all work blocked ---

// Story §2 Scenario 3 narrates "B has status active". With B=active and no
// blockers, B itself would surface as ready. To honor the intent ("every
// active task has unfinished blockers, nothing is claimable") we seed B as
// in_progress: A is blocked by live B, and B itself is excluded by the
// "status==active" predicate. Net result: empty array, exit 0.
func TestLs_ReadyJSON_AllBlocked_ReturnsEmptyMotesArray(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	blocker, err := mm.Create("task", "blocker (in flight)", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	if err := mm.Update(blocker.ID, core.UpdateOpts{Status: core.StringPtr("in_progress")}); err != nil {
		t.Fatalf("set blocker in_progress: %v", err)
	}
	dependent, err := mm.Create("task", "depends on blocker", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed dependent: %v", err)
	}
	im := core.NewIndexManager(root)
	if err := mm.Link(dependent.ID, "depends_on", blocker.ID, im); err != nil {
		t.Fatalf("link: %v", err)
	}

	var rerr error
	stdout := captureStdout(func() {
		rerr = runLsViaCobra([]string{"ls", "--ready", "--json"})
	})

	if rerr != nil {
		t.Fatalf("expected nil error, got %v", rerr)
	}
	if strings.TrimSpace(stdout) != `{"motes":[]}` {
		t.Errorf("expected empty motes array, got %q", stdout)
	}
}

// --- SCENARIO 4: one ready mote (non-empty path stays well-formed) ---

func TestLs_ReadyJSON_OneReadyMote_ReturnsArrayOfOne(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	seeded, err := mm.Create("task", "the only ready one", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	var rerr error
	stdout := captureStdout(func() {
		rerr = runLsViaCobra([]string{"ls", "--ready", "--json"})
	})
	if rerr != nil {
		t.Fatalf("expected nil error, got %v", rerr)
	}

	var parsed LsOutput
	if uerr := json.Unmarshal([]byte(stdout), &parsed); uerr != nil {
		t.Fatalf("invalid JSON: %v (stdout=%q)", uerr, stdout)
	}
	if len(parsed.Motes) != 1 {
		t.Fatalf("expected exactly 1 mote, got %d (stdout=%q)", len(parsed.Motes), stdout)
	}
	if parsed.Motes[0].ID == "" {
		t.Error("motes[0].id must be non-empty")
	}
	if parsed.Motes[0].ID != seeded.ID {
		t.Errorf("motes[0].id: got %q, want %q", parsed.Motes[0].ID, seeded.ID)
	}
}

// --- SCENARIO 5: unknown flag is a real error ---

func TestLs_ReadyJSON_UnknownFlag_NonZeroExit(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Capture both streams so we can assert "no partial JSON on stdout".
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr

	err := runLsViaCobra([]string{"ls", "--ready", "--json", "--not-a-real-flag"})

	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr

	var sb bytes.Buffer
	_, _ = sb.ReadFrom(rOut)
	stdout := sb.String()
	sb.Reset()
	_, _ = sb.ReadFrom(rErr)
	stderr := sb.String()

	if err == nil {
		t.Fatal("expected error for unknown flag, got nil")
	}
	// The error from cobra mentions the bad flag.
	if !strings.Contains(err.Error(), "not-a-real-flag") && !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("expected error to name the unknown flag, got: %v", err)
	}
	// No partial JSON envelope on stdout. Cobra may print usage to stdout
	// when SilenceUsage is unset, but `{"motes":` must never appear.
	if strings.Contains(stdout, `{"motes":`) {
		t.Errorf("stdout must not contain {\"motes\": when flag parsing fails; got %q", stdout)
	}
	_ = stderr // stderr content is implementation-defined; presence-of-error is enough via the returned err
}

// --- SCENARIO 6: graceful degradation when a node file is malformed ---
//
// `mote ls --ready` does NOT consult `.memory/index.jsonl`; it scans
// `nodes/` directly via ReadAllParallel. The realistic "corrupt workspace"
// failure mode is a malformed .md file in nodes/. The contract is graceful
// degradation: warn on stderr, skip the file, return a well-formed JSON
// envelope on stdout, exit 0.
func TestLs_ReadyJSON_MalformedNode_GracefulDegradation(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Drop a garbage .md file directly into nodes/. ReadAllParallel must
	// emit a stderr warning ("warning: skipping ...") and continue.
	garbagePath := filepath.Join(root, "nodes", "motes-garbage.md")
	if err := os.WriteFile(garbagePath, []byte("not valid frontmatter\n%%%---\n"), 0644); err != nil {
		t.Fatalf("seed garbage: %v", err)
	}

	// Capture stderr alongside stdout — we need both to validate the
	// degradation contract.
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr

	err := runLsViaCobra([]string{"ls", "--ready", "--json"})

	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr

	var sb bytes.Buffer
	_, _ = sb.ReadFrom(rOut)
	stdout := sb.String()
	sb.Reset()
	_, _ = sb.ReadFrom(rErr)
	stderr := sb.String()

	if err != nil {
		t.Fatalf("graceful degradation contract: expected exit 0, got %v", err)
	}
	trimmed := strings.TrimSpace(stdout)
	if trimmed != `{"motes":[]}` {
		t.Errorf("expected empty motes array (no valid motes seeded), got %q", trimmed)
	}
	if !strings.Contains(stderr, "warning: skipping") {
		t.Errorf("expected stderr to contain skip warning, got %q", stderr)
	}
}

// Silence unused-import warning for cobra when the package is re-shaped.
var _ = cobra.Command{}

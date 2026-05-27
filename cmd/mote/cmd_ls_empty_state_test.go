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
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/core"
)

// resetLsFlags zeroes the ls flag globals so test invocations are hermetic.
func resetLsFlags() {
	lsType = ""
	lsTag = ""
	lsStatus = ""
	lsStale = false
	lsReady = false
	lsCompact = false
	lsParent = ""
	lsJSON = false
	lsExplain = false
	lsOverdue = false
	lsIncludeDeferred = false
	lsDueBefore = ""
	lsDueAfter = ""
	lsMetadataField = nil
	lsHasMetadataKey = nil
}

// runLsViaCobra invokes `mote ls ...` through cobra, mirroring how a real
// shell would launch the binary, with cobra error/usage output silenced
// for clean test logs. Use runLsViaCobraNoisy when the test needs to
// observe cobra's stderr writes (e.g., unknown-flag scenario).
//
// Persistent root flags (--plain, --pretty, --no-color) are reset to zero
// before AND after invocation so that an earlier test that set them by
// argument cannot leak into the current invocation's outputMode resolution.
func runLsViaCobra(args []string) error {
	resetLsFlags()
	resetPersistentLayoutFlags()
	defer resetLsFlags()
	defer resetPersistentLayoutFlags()
	rootCmd.SetArgs(args)
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	return rootCmd.Execute()
}

// runLsViaCobraNoisy is runLsViaCobra without SilenceErrors — cobra writes
// its error message to stderr the way the real CLI does in production.
// SilenceUsage stays true so we don't drown the test log in usage banners.
func runLsViaCobraNoisy(args []string) error {
	resetLsFlags()
	resetPersistentLayoutFlags()
	defer resetLsFlags()
	defer resetPersistentLayoutFlags()
	rootCmd.SetArgs(args)
	rootCmd.SilenceErrors = false
	rootCmd.SilenceUsage = true
	return rootCmd.Execute()
}

// resetPersistentLayoutFlags zeroes the rootCmd-level layout-mode flags so
// they don't leak across test invocations. Cobra parses flag values into the
// pointed globals but never clears them on subsequent Execute() calls, so a
// test that runs `--plain` leaves plainFlag=true for the next test until we
// explicitly clear it.
func resetPersistentLayoutFlags() {
	plainFlag = false
	prettyFlag = false
	noColorFlag = false
}

// stripJSONDeprecationNotice removes the STORY-JSCHEMA-001 one-line legacy
// JSON deprecation notice from a captured stderr buffer. The notice is the
// new contract introduced in v0.4.35 and is orthogonal to the empty-state
// assertions in this file (which predate it). Tests that want to verify
// stderr is otherwise empty can compose: `strip…(stderr) != ""`.
func stripJSONDeprecationNotice(s string) string {
	out := []string{}
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "MOTE_JSON_ENVELOPE") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// captureBothStreams redirects os.Stdout and os.Stderr around fn and
// returns whatever each stream wrote. Used by tests that need to assert
// "stderr is empty" or "stderr contains X" alongside stdout content.
func captureBothStreams(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr

	fn()

	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr

	var sb bytes.Buffer
	_, _ = io.Copy(&sb, rOut)
	stdout = sb.String()
	sb.Reset()
	_, _ = io.Copy(&sb, rErr)
	stderr = sb.String()
	return stdout, stderr
}

// --- SCENARIO 1: empty workspace ---

func TestLs_ReadyJSON_EmptyWorkspace_ReturnsEmptyMotesArray(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	var err error
	stdout, stderr := captureBothStreams(t, func() {
		err = runLsViaCobra([]string{"ls", "--ready", "--json"})
	})

	if err != nil {
		t.Fatalf("expected nil error (exit 0), got %v", err)
	}
	trimmed := strings.TrimSpace(stdout)
	if trimmed != `{"motes":[]}` {
		t.Errorf("expected stdout to be exactly {\"motes\":[]}, got: %q", trimmed)
	}
	if rest := stripJSONDeprecationNotice(stderr); strings.TrimSpace(rest) != "" {
		t.Errorf("expected empty stderr aside from the legacy JSON deprecation notice (story §2 Scenario 1), got: %q", stderr)
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
	stdout, stderr := captureBothStreams(t, func() {
		err = runLsViaCobra([]string{"ls", "--ready", "--json"})
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if strings.TrimSpace(stdout) != `{"motes":[]}` {
		t.Errorf("expected empty motes array, got %q", stdout)
	}
	if rest := stripJSONDeprecationNotice(stderr); strings.TrimSpace(rest) != "" {
		t.Errorf("expected empty stderr aside from the legacy JSON deprecation notice (story §2 Scenario 2), got: %q", stderr)
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
	stdout, stderr := captureBothStreams(t, func() {
		rerr = runLsViaCobra([]string{"ls", "--ready", "--json"})
	})

	if rerr != nil {
		t.Fatalf("expected nil error, got %v", rerr)
	}
	if strings.TrimSpace(stdout) != `{"motes":[]}` {
		t.Errorf("expected empty motes array, got %q", stdout)
	}
	if rest := stripJSONDeprecationNotice(stderr); strings.TrimSpace(rest) != "" {
		t.Errorf("expected empty stderr aside from the legacy JSON deprecation notice (story §2 Scenario 3), got: %q", stderr)
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

	// Use the noisy variant so cobra writes its error to stderr the way
	// the real CLI does — story §2 Scenario 5 requires "stderr contains
	// an error message about the unknown flag".
	var err error
	stdout, stderr := captureBothStreams(t, func() {
		err = runLsViaCobraNoisy([]string{"ls", "--ready", "--json", "--not-a-real-flag"})
	})

	if err == nil {
		t.Fatal("expected error for unknown flag, got nil")
	}
	if !strings.Contains(err.Error(), "not-a-real-flag") && !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("expected returned error to name the unknown flag, got: %v", err)
	}
	if !strings.Contains(stderr, "not-a-real-flag") && !strings.Contains(stderr, "unknown flag") {
		t.Errorf("expected stderr to describe the unknown flag (story §2 Scenario 5), got: %q", stderr)
	}
	// No partial JSON envelope anywhere on stdout. Specifically the empty
	// envelope must NOT leak when flag parsing fails.
	if strings.Contains(stdout, `{"motes":`) {
		t.Errorf("stdout must not contain {\"motes\": when flag parsing fails; got %q", stdout)
	}
}

// --- SCENARIO 6: graceful degradation when a node file is malformed ---
//
// `mote ls --ready` does NOT consult `.memory/index.jsonl`; it scans
// `nodes/` directly via ReadAllParallel. The realistic "corrupt workspace"
// failure mode is a malformed .md file in nodes/. The contract is graceful
// degradation: warn on stderr, skip the file, surface valid motes
// alongside, return a well-formed JSON envelope on stdout, exit 0.
func TestLs_ReadyJSON_MalformedNode_GracefulDegradation(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Seed one valid ready task — graceful degradation means it should
	// still surface in the JSON envelope despite the broken sibling.
	mm := core.NewMoteManager(root)
	valid, err := mm.Create("task", "valid task next to garbage", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed valid: %v", err)
	}

	// Drop a garbage .md file directly into nodes/. ReadAllParallel must
	// emit a stderr warning ("warning: skipping ...") and continue.
	garbagePath := filepath.Join(root, "nodes", "motes-garbage.md")
	if werr := os.WriteFile(garbagePath, []byte("not valid frontmatter\n%%%---\n"), 0644); werr != nil {
		t.Fatalf("seed garbage: %v", werr)
	}

	var rerr error
	stdout, stderr := captureBothStreams(t, func() {
		rerr = runLsViaCobra([]string{"ls", "--ready", "--json"})
	})

	if rerr != nil {
		t.Fatalf("graceful degradation contract: expected exit 0, got %v", rerr)
	}

	// Stdout must still be valid JSON and must surface the valid mote.
	var parsed LsOutput
	if uerr := json.Unmarshal([]byte(stdout), &parsed); uerr != nil {
		t.Fatalf("expected well-formed JSON despite garbage node, got %q (%v)", stdout, uerr)
	}
	if len(parsed.Motes) != 1 {
		t.Fatalf("expected exactly 1 mote (the valid one), got %d (stdout=%q)", len(parsed.Motes), stdout)
	}
	if parsed.Motes[0].ID != valid.ID {
		t.Errorf("motes[0].id: got %q, want %q", parsed.Motes[0].ID, valid.ID)
	}

	if !strings.Contains(stderr, "warning: skipping") {
		t.Errorf("expected stderr to contain skip warning, got %q", stderr)
	}
	if !strings.Contains(stderr, "motes-garbage.md") {
		t.Errorf("expected stderr to name the skipped file motes-garbage.md, got %q", stderr)
	}
}

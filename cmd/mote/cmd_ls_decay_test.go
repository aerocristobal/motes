// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/core"
)

// STORY-DECAY-001 — Visual Decay: Closed Items Mute the Entire Row
//
// Feature file (living docs): features/rendering/visual_decay.feature
// Scenario mapping:
//   Scenario 1, 2: TestLs_ForcedTTY_MutesClosedRows
//   Scenario 3:    TestLs_NonTTY_NoAnsiEscapes + TestLs_NonTTY_ByteStableAgainstSnapshot
//   Scenario 4:    TestLs_NoColorEnv_DisablesMutingOnTTY
//   Scenario 5:    TestLs_NoColorFlag_DisablesMutingOnTTY
//   Scenario 6:    TestLs_JSON_NoAnsiEscapes + TestLs_JSON_ByteStableAgainstSnapshot
//   Scenario 7:    TestPulse_ExcludesClosedAndIsUnaffected (in cmd_ls_decay_test.go)
//   Scenario 9:    TestLs_DeprecatedPrefix_PreservedAndMuted
//   Scenario 10:   TestLs_ForcedTTY_ColumnAlignmentPreserved

// createDeterministicMoteWithStatus writes a mote YAML fixture with stable
// timestamps and an explicit status, then rebuilds the index. Sister to
// createDeterministicMote in cmd_show_helpers_test.go.
func createDeterministicMoteWithStatus(t *testing.T, root, id, status, title string) {
	t.Helper()
	content := fmt.Sprintf(`---
id: %s
type: task
status: %s
title: %s
tags:
    - testing
weight: 0.5
origin: normal
action: ""
created_at: 2026-01-01T00:00:00Z
last_accessed: 2026-01-02T00:00:00Z
access_count: 0
depends_on: []
blocks: []
relates_to: []
builds_on: []
contradicts: []
supersedes: []
caused_by: []
informed_by: []
acceptance: []
acceptance_met: []
---
`, id, status, title)
	path := filepath.Join(root, "nodes", id+".md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	motes, err := mm.ReadAllParallel()
	if err != nil {
		t.Fatalf("read motes: %v", err)
	}
	if err := im.Rebuild(motes); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
}

// resetDecayFlags resets the persistent --no-color flag between tests.
// resetLsFlags only covers local ls flags, not persistent root flags.
func resetDecayFlags() {
	noColorFlag = false
}

func TestLs_NonTTY_NoAnsiEscapes(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetDecayFlags()

	createDeterministicMoteWithStatus(t, root, "proj-T1ABC", "active", "Add login flow")
	createDeterministicMoteWithStatus(t, root, "proj-T2DEF", "completed", "Refactor router")

	var err error
	stdout, _ := captureBothStreams(t, func() {
		err = runLsViaCobra([]string{"ls"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("non-TTY output contains ANSI escape: %q", stdout)
	}
	if !strings.Contains(stdout, "proj-T1ABC") {
		t.Errorf("missing active mote in output: %q", stdout)
	}
	if !strings.Contains(stdout, "proj-T2DEF") {
		t.Errorf("missing completed mote in output: %q", stdout)
	}
}

func TestLs_JSON_NoAnsiEscapes(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetDecayFlags()

	createDeterministicMoteWithStatus(t, root, "proj-T1ABC", "active", "Add login flow")
	createDeterministicMoteWithStatus(t, root, "proj-T2DEF", "completed", "Refactor router")

	// Even on a forced TTY the --json path must remain byte-clean.
	t.Setenv("MOTE_FORCE_TTY", "1")

	var err error
	stdout, _ := captureBothStreams(t, func() {
		err = runLsViaCobra([]string{"ls", "--json"})
	})
	if err != nil {
		t.Fatalf("ls --json: %v", err)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("--json must not contain ANSI escapes: %q", stdout)
	}
	// Round-trip validate JSON.
	var parsed LsOutput
	jsonStart := strings.Index(stdout, "{")
	if jsonStart < 0 {
		t.Fatalf("no JSON in output: %q", stdout)
	}
	if uerr := json.Unmarshal([]byte(stdout[jsonStart:]), &parsed); uerr != nil {
		t.Fatalf("invalid JSON output: %v\n%s", uerr, stdout)
	}
	if len(parsed.Motes) != 2 {
		t.Errorf("expected 2 motes in JSON, got %d", len(parsed.Motes))
	}
}

func TestLs_ForcedTTY_MutesClosedRows(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetDecayFlags()

	createDeterministicMoteWithStatus(t, root, "proj-T1ABC", "active", "Add login flow")
	createDeterministicMoteWithStatus(t, root, "proj-T2DEF", "completed", "Refactor router")
	createDeterministicMoteWithStatus(t, root, "proj-T3GHI", "active", "Wire OAuth")

	t.Setenv("MOTE_FORCE_TTY", "1")
	t.Setenv("NO_COLOR", "")

	var err error
	stdout, _ := captureBothStreams(t, func() {
		err = runLsViaCobra([]string{"ls"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}

	var activeRows, completedRows int
	for _, line := range strings.Split(stdout, "\n") {
		switch {
		case strings.Contains(line, "proj-T1ABC"), strings.Contains(line, "proj-T3GHI"):
			activeRows++
			if strings.Contains(line, "\x1b[2m") {
				t.Errorf("active row must NOT be muted; got %q", line)
			}
		case strings.Contains(line, "proj-T2DEF"):
			completedRows++
			if !strings.Contains(line, "\x1b[2m") {
				t.Errorf("completed row MUST be muted; got %q", line)
			}
			if !strings.Contains(line, "\x1b[0m") {
				t.Errorf("completed row must end with reset escape; got %q", line)
			}
		}
	}
	if activeRows != 2 {
		t.Errorf("expected 2 active rows, found %d", activeRows)
	}
	if completedRows != 1 {
		t.Errorf("expected 1 completed row, found %d", completedRows)
	}
}

func TestLs_ForcedTTY_MutesAllClosedStatuses(t *testing.T) {
	// Scenario 2: every closed status produces a muted row.
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetDecayFlags()

	createDeterministicMoteWithStatus(t, root, "proj-X1AAA", "completed", "Done thing")
	createDeterministicMoteWithStatus(t, root, "proj-X1BBB", "archived", "Archived thing")
	createDeterministicMoteWithStatus(t, root, "proj-X1CCC", "deprecated", "Old thing")

	t.Setenv("MOTE_FORCE_TTY", "1")
	t.Setenv("NO_COLOR", "")

	var err error
	stdout, _ := captureBothStreams(t, func() {
		err = runLsViaCobra([]string{"ls"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	for _, id := range []string{"proj-X1AAA", "proj-X1BBB", "proj-X1CCC"} {
		found := false
		for _, line := range strings.Split(stdout, "\n") {
			if strings.Contains(line, id) {
				found = true
				if !strings.Contains(line, "\x1b[2m") {
					t.Errorf("%s row must be muted; got %q", id, line)
				}
			}
		}
		if !found {
			t.Errorf("%s row not in output: %q", id, stdout)
		}
	}
}

func TestLs_NoColorEnv_DisablesMutingOnTTY(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetDecayFlags()

	// Seed a deprecated mote so we can also assert the backward-compat
	// "[deprecated]" text marker still appears when NO_COLOR is set
	// (Scenario 4's second Then-clause).
	createDeterministicMoteWithStatus(t, root, "proj-T2DEF", "completed", "Refactor router")
	createDeterministicMoteWithStatus(t, root, "proj-T3GHI", "deprecated", "Old auth approach")

	t.Setenv("MOTE_FORCE_TTY", "1")
	t.Setenv("NO_COLOR", "1")

	var err error
	stdout, _ := captureBothStreams(t, func() {
		err = runLsViaCobra([]string{"ls"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("NO_COLOR=1 should suppress ANSI even on TTY; got %q", stdout)
	}
	if !strings.Contains(stdout, "[deprecated]") {
		t.Errorf("NO_COLOR=1 must still preserve the [deprecated] text marker; got %q", stdout)
	}
}

func TestLs_NoColorFlag_DisablesMutingOnTTY(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetDecayFlags()

	createDeterministicMoteWithStatus(t, root, "proj-T2DEF", "completed", "Refactor router")

	t.Setenv("MOTE_FORCE_TTY", "1")
	t.Setenv("NO_COLOR", "")

	var err error
	stdout, _ := captureBothStreams(t, func() {
		err = runLsViaCobra([]string{"ls", "--no-color"})
	})
	if err != nil {
		t.Fatalf("ls --no-color: %v", err)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("--no-color should suppress ANSI even on TTY; got %q", stdout)
	}
}

func TestLs_DeprecatedPrefix_PreservedAndMuted(t *testing.T) {
	// Scenario 9: backward-compat — text prefix kept AND row muted.
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetDecayFlags()

	createDeterministicMoteWithStatus(t, root, "proj-D1XYZ", "deprecated", "Old auth approach")

	t.Setenv("MOTE_FORCE_TTY", "1")
	t.Setenv("NO_COLOR", "")

	var err error
	stdout, _ := captureBothStreams(t, func() {
		err = runLsViaCobra([]string{"ls"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	if !strings.Contains(stdout, "[deprecated]") {
		t.Errorf("deprecated text marker must remain for backward compat; got %q", stdout)
	}
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "proj-D1XYZ") {
			if !strings.Contains(line, "\x1b[2m") {
				t.Errorf("deprecated row should be muted; got %q", line)
			}
			return
		}
	}
	t.Fatalf("deprecated row not found in output: %q", stdout)
}

func TestLs_ForcedTTY_ColumnAlignmentPreserved(t *testing.T) {
	// Scenario 10: stripping ANSI escapes must leave columns aligned.
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetDecayFlags()

	createDeterministicMoteWithStatus(t, root, "proj-T1ABC", "active", "Short")
	createDeterministicMoteWithStatus(t, root, "proj-T2DEF", "completed", "A medium title")
	createDeterministicMoteWithStatus(t, root, "proj-T3GHI", "deprecated", "A much longer title here")

	t.Setenv("MOTE_FORCE_TTY", "1")
	t.Setenv("NO_COLOR", "")

	var err error
	stdout, _ := captureBothStreams(t, func() {
		err = runLsViaCobra([]string{"ls"})
	})
	if err != nil {
		t.Fatalf("ls: %v", err)
	}

	// Strip ANSI; data rows (those containing "proj-") must all start with the
	// same column layout (24-char ID column, 2-space gap, then type column).
	var dataLines []string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "proj-") {
			dataLines = append(dataLines, line)
		}
	}
	if len(dataLines) != 3 {
		t.Fatalf("expected 3 data rows, got %d: %q", len(dataLines), stdout)
	}
	// The runes 0..25 (24-char ID + 2 spaces) should be visually identical-length
	// when ANSI is stripped. We use a less brittle test: every stripped row has
	// "task" starting at byte 26 (since type is "task" for all rows).
	for _, line := range dataLines {
		stripped := stripCSI(line)
		if !strings.HasPrefix(stripped[26:], "task") {
			t.Errorf("type column misaligned in row: stripped=%q", stripped)
		}
	}
}

// TestLs_NonTTY_ByteStableAgainstSnapshot verifies that non-TTY plain text
// output is byte-identical to a committed golden fixture. Regenerate with:
//
//	UPDATE_GOLDEN=1 go test ./cmd/mote/ -run TestLs_NonTTY_ByteStableAgainstSnapshot
func TestLs_NonTTY_ByteStableAgainstSnapshot(t *testing.T) {
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	goldenPath := filepath.Join(origCwd, "testdata", "ls_mixed.golden")

	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetDecayFlags()

	createDeterministicMoteWithStatus(t, root, "proj-T1ABC", "active", "Add login flow")
	createDeterministicMoteWithStatus(t, root, "proj-T2DEF", "completed", "Refactor router")
	createDeterministicMoteWithStatus(t, root, "proj-T3GHI", "deprecated", "Old auth")

	var runErr error
	got, _ := captureBothStreams(t, func() {
		runErr = runLsViaCobra([]string{"ls"})
	})
	if runErr != nil {
		t.Fatalf("ls: %v", runErr)
	}

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("regenerated %s (%d bytes)", goldenPath, len(got))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run UPDATE_GOLDEN=1 to create): %v", err)
	}
	if got != string(want) {
		t.Errorf("non-TTY ls drifted from snapshot.\n--- want (%d) ---\n%s\n--- got (%d) ---\n%s",
			len(want), string(want), len(got), got)
	}
}

// TestLs_JSON_ByteStableAgainstSnapshot guards Scenario 6.
//
//	UPDATE_GOLDEN=1 go test ./cmd/mote/ -run TestLs_JSON_ByteStableAgainstSnapshot
func TestLs_JSON_ByteStableAgainstSnapshot(t *testing.T) {
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	goldenPath := filepath.Join(origCwd, "testdata", "ls_mixed.json.golden")

	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetDecayFlags()

	createDeterministicMoteWithStatus(t, root, "proj-T1ABC", "active", "Add login flow")
	createDeterministicMoteWithStatus(t, root, "proj-T2DEF", "completed", "Refactor router")
	createDeterministicMoteWithStatus(t, root, "proj-T3GHI", "deprecated", "Old auth")

	// Even with TTY forced, --json must be unaffected.
	t.Setenv("MOTE_FORCE_TTY", "1")

	var runErr error
	got, _ := captureBothStreams(t, func() {
		runErr = runLsViaCobra([]string{"ls", "--json"})
	})
	if runErr != nil {
		t.Fatalf("ls --json: %v", runErr)
	}

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("regenerated %s (%d bytes)", goldenPath, len(got))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run UPDATE_GOLDEN=1 to create): %v", err)
	}
	if got != string(want) {
		t.Errorf("--json drifted from snapshot.\n--- want (%d) ---\n%s\n--- got (%d) ---\n%s",
			len(want), string(want), len(got), got)
	}
}

// TestPulse_ExcludesClosedAndIsUnaffected covers Scenario 7.
func TestPulse_ExcludesClosedAndIsUnaffected(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()
	defer resetDecayFlags()

	createDeterministicMoteWithStatus(t, root, "proj-T1ABC", "active", "Add login flow")
	createDeterministicMoteWithStatus(t, root, "proj-T2DEF", "completed", "Refactor router")

	t.Setenv("MOTE_FORCE_TTY", "1")
	t.Setenv("NO_COLOR", "")

	var err error
	stdout, _ := captureBothStreams(t, func() {
		err = runLsViaCobra([]string{"pulse"})
	})
	if err != nil {
		t.Fatalf("pulse: %v", err)
	}
	if !strings.Contains(stdout, "proj-T1ABC") {
		t.Errorf("pulse should include active task; got %q", stdout)
	}
	if strings.Contains(stdout, "proj-T2DEF") {
		t.Errorf("pulse should exclude completed task; got %q", stdout)
	}
	// And the visible active row must NOT be muted.
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "proj-T1ABC") && strings.Contains(line, "\x1b[2m") {
			t.Errorf("pulse active row should not be muted; got %q", line)
		}
	}
}

// stripCSI removes ANSI CSI escape sequences for in-test alignment checks.
// Mirrors format.StripANSI without forcing the cmd/mote test binary to import
// the format internals; this keeps cmd/mote tests self-contained.
func stripCSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) {
				j++ // consume the final byte
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

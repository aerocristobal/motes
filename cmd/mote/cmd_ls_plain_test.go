// SPDX-License-Identifier: MIT
//
// STORY-PLAIN-001 — coverage for `mote ls --plain`.
package main

import (
	"strings"
	"testing"

	"motes/internal/core"
	"motes/internal/format"
)

func TestLsPlain_NoANSI_NoHeader_NoPadding(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	if _, err := mm.Create("task", "plain probe one", core.CreateOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	resetModeFlags(t)
	plainFlag = true

	stdout, stderr := captureBothStreams(t, func() {
		if err := runLsViaCobra([]string{"ls", "--plain"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if stderr != "" {
		t.Errorf("plain mode must not emit stderr, got %q", stderr)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("plain output contains ANSI escapes: %q", stdout)
	}
	if format.StripANSI(stdout) != stdout {
		t.Errorf("plain output not equal to its ANSI-stripped form")
	}
	for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "ID") {
			t.Errorf("plain output emitted header row: %q", line)
		}
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "===") {
			t.Errorf("plain output emitted separator: %q", line)
		}
		if strings.Contains(line, "  ") {
			t.Errorf("plain output contains multi-space padding: %q", line)
		}
	}
}

func TestLsPlain_FirstTokenIsMoteID(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	seeded, err := mm.Create("task", "first-token check", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	resetModeFlags(t)
	plainFlag = true

	stdout, _ := captureBothStreams(t, func() {
		if err := runLsViaCobra([]string{"ls", "--plain"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly 1 data line, got %d (stdout=%q)", len(lines), stdout)
	}
	first := strings.SplitN(lines[0], " ", 2)[0]
	if first != seeded.ID {
		t.Errorf("first whitespace-delimited token = %q, want %q", first, seeded.ID)
	}
}

func TestLsPlain_DeprecatedMarkerAfterID(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	seeded, err := mm.Create("task", "going stale", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := mm.Update(seeded.ID, core.UpdateOpts{Status: core.StringPtr("deprecated")}); err != nil {
		t.Fatalf("deprecate: %v", err)
	}

	resetModeFlags(t)
	plainFlag = true

	stdout, _ := captureBothStreams(t, func() {
		if err := runLsViaCobra([]string{"ls", "--plain"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly 1 line, got %d (stdout=%q)", len(lines), stdout)
	}
	if !strings.Contains(lines[0], "[deprecated]") {
		t.Errorf("plain output must keep textual [deprecated] marker; got %q", lines[0])
	}
	if !strings.HasPrefix(lines[0], seeded.ID+" ") {
		t.Errorf("mote id must still be the first whitespace token (sprint Scenario 6); got %q", lines[0])
	}
}

func TestLsPlain_ComposesWithReadyFilter(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	ready, err := mm.Create("task", "ready one", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	blocked, err := mm.Create("task", "blocked one", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	blocker, err := mm.Create("task", "blocker", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := mm.Update(blocker.ID, core.UpdateOpts{Status: core.StringPtr("in_progress")}); err != nil {
		t.Fatalf("set blocker in_progress: %v", err)
	}
	im := core.NewIndexManager(root)
	if err := mm.Link(blocked.ID, "depends_on", blocker.ID, im); err != nil {
		t.Fatalf("link: %v", err)
	}

	resetModeFlags(t)
	plainFlag = true

	stdout, _ := captureBothStreams(t, func() {
		if err := runLsViaCobra([]string{"ls", "--ready", "--plain"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("ready filter under --plain: expected 1 line, got %d (stdout=%q)", len(lines), stdout)
	}
	if !strings.HasPrefix(lines[0], ready.ID+" ") {
		t.Errorf("plain --ready filtered the wrong mote; got %q want prefix %q", lines[0], ready.ID)
	}
}

func TestLsPlain_EmptyWorkspace_NoOutput(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	resetModeFlags(t)
	plainFlag = true

	stdout, stderr := captureBothStreams(t, func() {
		if err := runLsViaCobra([]string{"ls", "--plain"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if stdout != "" {
		t.Errorf("plain mode on empty workspace must produce zero stdout; got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("plain mode on empty workspace must produce zero stderr; got %q", stderr)
	}
}

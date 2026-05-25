// SPDX-License-Identifier: MIT
//
// STORY-TIME-001 §6.4 — `mote ls` scheduling flags CLI integration.
package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"motes/internal/core"
)

func TestRunLs_Ready_HidesDeferred(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	def := time.Now().Add(24 * time.Hour)
	deferred, _ := mm.Create("task", "deferred", core.CreateOpts{DeferUntil: &def, Local: true})
	_, _ = mm.Create("task", "ready", core.CreateOpts{Local: true})

	var rerr error
	stdout := captureStdout(func() {
		rerr = runLsViaCobra([]string{"ls", "--ready", "--json"})
	})
	if rerr != nil {
		t.Fatalf("ls: %v", rerr)
	}

	var parsed LsOutput
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	for _, m := range parsed.Motes {
		if m.ID == deferred.ID {
			t.Errorf("deferred mote %s must not appear in --ready", m.ID)
		}
	}
}

func TestRunLs_Ready_IncludeDeferred_ShowsDeferred(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	def := time.Now().Add(24 * time.Hour)
	deferred, _ := mm.Create("task", "deferred", core.CreateOpts{DeferUntil: &def, Local: true})

	var rerr error
	stdout := captureStdout(func() {
		rerr = runLsViaCobra([]string{"ls", "--ready", "--include-deferred", "--json"})
	})
	if rerr != nil {
		t.Fatalf("ls: %v", rerr)
	}

	var parsed LsOutput
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	var found bool
	for _, m := range parsed.Motes {
		if m.ID == deferred.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("--include-deferred should surface %s", deferred.ID)
	}
}

func TestRunLs_Overdue_HappyPath(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	past1 := time.Now().Add(-24 * time.Hour)
	past2 := time.Now().Add(-2 * time.Hour)
	a, _ := mm.Create("task", "very late", core.CreateOpts{DueAt: &past1, Local: true})
	b, _ := mm.Create("task", "a little late", core.CreateOpts{DueAt: &past2, Local: true})

	var rerr error
	stdout := captureStdout(func() {
		rerr = runLsViaCobra([]string{"ls", "--overdue", "--json"})
	})
	if rerr != nil {
		t.Fatalf("ls: %v", rerr)
	}

	var parsed LsOutput
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if len(parsed.Motes) != 2 {
		t.Fatalf("expected 2 overdue, got %d\n%s", len(parsed.Motes), stdout)
	}
	// Sorted by DueAt ascending — most overdue first.
	if parsed.Motes[0].ID != a.ID || parsed.Motes[1].ID != b.ID {
		t.Errorf("sort order: got [%s,%s], want [%s,%s]",
			parsed.Motes[0].ID, parsed.Motes[1].ID, a.ID, b.ID)
	}
}

func TestRunLs_DueBefore_HappyPath(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	past := time.Now().Add(-2 * time.Hour)
	future := time.Now().Add(72 * time.Hour)
	a, _ := mm.Create("task", "past", core.CreateOpts{DueAt: &past, Local: true})
	_, _ = mm.Create("task", "future", core.CreateOpts{DueAt: &future, Local: true})

	var rerr error
	stdout := captureStdout(func() {
		rerr = runLsViaCobra([]string{"ls", "--due-before=now", "--json"})
	})
	if rerr != nil {
		t.Fatalf("ls: %v", rerr)
	}

	var parsed LsOutput
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if len(parsed.Motes) != 1 || parsed.Motes[0].ID != a.ID {
		t.Errorf("--due-before=now: got %d motes; want [%s]", len(parsed.Motes), a.ID)
	}
}

func TestRunLs_DueAfter_HappyPath(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	past := time.Now().Add(-2 * time.Hour)
	future := time.Now().Add(72 * time.Hour)
	_, _ = mm.Create("task", "past", core.CreateOpts{DueAt: &past, Local: true})
	b, _ := mm.Create("task", "future", core.CreateOpts{DueAt: &future, Local: true})

	var rerr error
	stdout := captureStdout(func() {
		rerr = runLsViaCobra([]string{"ls", "--due-after=now", "--json"})
	})
	if rerr != nil {
		t.Fatalf("ls: %v", rerr)
	}

	var parsed LsOutput
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if len(parsed.Motes) != 1 || parsed.Motes[0].ID != b.ID {
		t.Errorf("--due-after=now: got %d motes; want [%s]", len(parsed.Motes), b.ID)
	}
}

func TestRunLs_DueBefore_NoMatches_EmptyJSONExit0(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	var rerr error
	stdout, stderr := captureBothStreams(t, func() {
		rerr = runLsViaCobra([]string{"ls", "--due-before=now", "--json"})
	})
	if rerr != nil {
		t.Fatalf("expected nil error (story §23.16 preserved), got %v", rerr)
	}
	if strings.TrimSpace(stdout) != `{"motes":[]}` {
		t.Errorf("expected {\"motes\":[]}, got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}
}

func TestRunLs_Overdue_ExcludesCompleted(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	past := time.Now().Add(-24 * time.Hour)
	completed, _ := mm.Create("task", "done late", core.CreateOpts{DueAt: &past, Local: true})
	if err := mm.Update(completed.ID, core.UpdateOpts{Status: core.StringPtr("completed")}); err != nil {
		t.Fatalf("update: %v", err)
	}

	var rerr error
	stdout := captureStdout(func() {
		rerr = runLsViaCobra([]string{"ls", "--overdue", "--json"})
	})
	if rerr != nil {
		t.Fatalf("ls: %v", rerr)
	}

	var parsed LsOutput
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, m := range parsed.Motes {
		if m.ID == completed.ID {
			t.Errorf("completed mote %s must not appear in --overdue", completed.ID)
		}
	}
}

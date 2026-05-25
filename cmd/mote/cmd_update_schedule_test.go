// SPDX-License-Identifier: MIT
//
// STORY-TIME-001 §6.3 — `mote update --due / --defer` CLI integration.
package main

import (
	"strings"
	"testing"
	"time"

	"motes/internal/core"
)

func TestRunUpdate_DueFlag_HappyPath(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	m, _ := mm.Create("task", "to-be-updated", core.CreateOpts{Local: true})

	if err := runUpdateViaCobra([]string{"update", m.ID, "--due=+6h"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := mm.Read(m.ID)
	if got.DueAt == nil {
		t.Fatal("DueAt should be set")
	}
	delta := time.Until(got.DueAt)
	if delta < 5*time.Hour+30*time.Minute || delta > 6*time.Hour+30*time.Minute {
		t.Errorf("DueAt distance from now: got %v, want ~6h", delta)
	}
}

func TestRunUpdate_DeferEmptyString_ClearsField(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	def := time.Now().Add(24 * time.Hour)
	m, err := mm.Create("task", "deferred", core.CreateOpts{DeferUntil: &def, Local: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := runUpdateViaCobra([]string{"update", m.ID, "--defer="}); err != nil {
		t.Fatalf("update --defer='': %v", err)
	}
	got, _ := mm.Read(m.ID)
	if got.DeferUntil != nil {
		t.Errorf("DeferUntil should be nil after --defer='', got %v", got.DeferUntil)
	}
}

func TestRunUpdate_DueEmptyString_ClearsField(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	due := time.Now().Add(48 * time.Hour)
	m, err := mm.Create("task", "with-due", core.CreateOpts{DueAt: &due, Local: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := runUpdateViaCobra([]string{"update", m.ID, "--due="}); err != nil {
		t.Fatalf("update --due='': %v", err)
	}
	got, _ := mm.Read(m.ID)
	if got.DueAt != nil {
		t.Errorf("DueAt should be nil after --due='', got %v", got.DueAt)
	}
}

func TestRunUpdate_NonexistentMote_ReturnsExistingError(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	err := runUpdateViaCobra([]string{"update", "motes-doesnotexistxx", "--due=+1d"})
	if err == nil {
		t.Fatal("expected error for nonexistent mote")
	}
	// We don't pin the literal substring — what matters is that the error
	// is propagated unchanged from the existing Update path.
	if !strings.Contains(strings.ToLower(err.Error()), "open") &&
		!strings.Contains(strings.ToLower(err.Error()), "no such file") &&
		!strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Errorf("expected not-found-style error, got: %v", err)
	}
}

func TestRunUpdate_DueAndClaim_MutuallyExclusive(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	m, _ := mm.Create("task", "claim-target", core.CreateOpts{Local: true})
	t.Setenv("MOTE_AGENT_ID", "agent-A")

	err := runUpdateViaCobra([]string{"update", m.ID, "--claim", "--due=+1d"})
	if err == nil {
		t.Fatal("expected mutual-exclusion error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutual-exclusion message, got: %v", err)
	}
}

func TestRunUpdate_DeferInPast_Rejected(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	m, _ := mm.Create("task", "t", core.CreateOpts{Local: true})

	err := runUpdateViaCobra([]string{"update", m.ID, "--defer=-1h"})
	if err == nil {
		t.Fatal("expected rejection for past defer")
	}
	if !strings.Contains(err.Error(), "future") && !strings.Contains(err.Error(), "invalid time") {
		t.Errorf("expected 'future' or 'invalid time' in error, got: %v", err)
	}
}

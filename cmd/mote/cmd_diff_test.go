package main

import (
	"strings"
	"testing"

	"motes/internal/core"
)

func TestDiff_NoSnapshot(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Some Task", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	id := motes[0].ID

	output := captureStdout(func() {
		diffCmd.RunE(diffCmd, []string{id})
	})

	if !strings.Contains(output, "No prior snapshot") {
		t.Errorf("expected no-snapshot message, got:\n%s", output)
	}
}

func TestDiff_NoChanges(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Unchanged Task", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	id := motes[0].ID

	// Create a snapshot
	m, _ := mm.Read(id)
	snap := core.SnapshotMote(m)
	mm.SaveSnapshot(id, snap)

	output := captureStdout(func() {
		diffCmd.RunE(diffCmd, []string{id})
	})

	if !strings.Contains(output, "No changes") {
		t.Errorf("expected no-changes message, got:\n%s", output)
	}
}

func TestDiff_StatusChange(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Task to Complete", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	id := motes[0].ID

	// Snapshot in active state
	m, _ := mm.Read(id)
	snap := core.SnapshotMote(m)
	mm.SaveSnapshot(id, snap)

	// Change status
	completed := "completed"
	mm.Update(id, core.UpdateOpts{Status: &completed})

	output := captureStdout(func() {
		diffCmd.RunE(diffCmd, []string{id})
	})

	if !strings.Contains(output, "status") {
		t.Errorf("expected status diff, got:\n%s", output)
	}
	if !strings.Contains(output, "active") && !strings.Contains(output, "completed") {
		t.Errorf("expected old/new status values, got:\n%s", output)
	}
}

func TestDiff_TagChange(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Tagged Task", Tags: []string{"alpha"}},
	})

	mm := core.NewMoteManager(root)
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	id := motes[0].ID

	// Snapshot
	m, _ := mm.Read(id)
	snap := core.SnapshotMote(m)
	mm.SaveSnapshot(id, snap)

	// Change tags
	mm.Update(id, core.UpdateOpts{Tags: []string{"alpha", "beta"}})

	output := captureStdout(func() {
		diffCmd.RunE(diffCmd, []string{id})
	})

	if !strings.Contains(output, "tags") {
		t.Errorf("expected tags diff, got:\n%s", output)
	}
}

func TestDiff_LinkChange(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Task A", Tags: []string{"test"}},
		{Type: "task", Title: "Task B", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	im.Load()
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	idA, idB := motes[0].ID, motes[1].ID

	// Snapshot before link
	m, _ := mm.Read(idA)
	snap := core.SnapshotMote(m)
	mm.SaveSnapshot(idA, snap)

	// Add link
	mm.Link(idA, "relates_to", idB, im)

	output := captureStdout(func() {
		diffCmd.RunE(diffCmd, []string{idA})
	})

	if !strings.Contains(output, "links.relates_to") {
		t.Errorf("expected link diff, got:\n%s", output)
	}
}

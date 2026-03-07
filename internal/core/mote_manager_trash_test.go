package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMoteManager_Delete(t *testing.T) {
	root, mm := setupTestMemory(t)
	im := NewIndexManager(root)
	im.Rebuild(nil)

	m, err := mm.Create("lesson", "Test delete", CreateOpts{Tags: []string{"test"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := mm.Delete(m.ID, im); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Should not be readable from nodes
	if _, err := mm.Read(m.ID); err == nil {
		t.Error("expected error reading deleted mote from nodes")
	}

	// Should exist in trash
	trashPath := filepath.Join(root, "trash", m.ID+".md")
	if _, err := os.Stat(trashPath); err != nil {
		t.Errorf("mote should exist in trash: %v", err)
	}

	// Parse from trash, verify deleted_at is set
	trashed, err := ParseMote(trashPath)
	if err != nil {
		t.Fatalf("parse trashed mote: %v", err)
	}
	if trashed.DeletedAt == nil {
		t.Error("deleted_at should be set")
	}
}

func TestMoteManager_DeleteCleansUpLinks(t *testing.T) {
	root, mm := setupTestMemory(t)
	im := NewIndexManager(root)
	im.Rebuild(nil)

	m1, _ := mm.Create("task", "Task A", CreateOpts{})
	m2, _ := mm.Create("task", "Task B", CreateOpts{})

	// Link m1 relates_to m2
	if err := mm.Link(m1.ID, "relates_to", m2.ID, im); err != nil {
		t.Fatalf("Link: %v", err)
	}

	// Delete m2
	if err := mm.Delete(m2.ID, im); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// m1 should no longer have m2 in relates_to
	m1Updated, err := mm.Read(m1.ID)
	if err != nil {
		t.Fatalf("Read m1: %v", err)
	}
	if sliceContains(m1Updated.RelatesTo, m2.ID) {
		t.Errorf("m1 still has %s in relates_to after deletion", m2.ID)
	}

	// Index should have no edges for m2
	idx, _ := im.Load()
	if idx.HasEdges(m2.ID) {
		t.Error("index still has edges for deleted mote")
	}
}

func TestMoteManager_Restore(t *testing.T) {
	root, mm := setupTestMemory(t)
	im := NewIndexManager(root)
	im.Rebuild(nil)

	m, err := mm.Create("decision", "Test restore", CreateOpts{Body: "restore me"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := mm.Delete(m.ID, im); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if err := mm.Restore(m.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Should be readable from nodes again
	restored, err := mm.Read(m.ID)
	if err != nil {
		t.Fatalf("Read after restore: %v", err)
	}
	if restored.DeletedAt != nil {
		t.Error("deleted_at should be cleared after restore")
	}
	if restored.Body != "restore me" {
		t.Errorf("body mismatch: got %q", restored.Body)
	}

	// Should not exist in trash
	trashPath := filepath.Join(root, "trash", m.ID+".md")
	if _, err := os.Stat(trashPath); !os.IsNotExist(err) {
		t.Error("mote should not exist in trash after restore")
	}
}

func TestMoteManager_ListTrash(t *testing.T) {
	root, mm := setupTestMemory(t)
	im := NewIndexManager(root)
	im.Rebuild(nil)

	// Empty trash
	motes, err := mm.ListTrash()
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if len(motes) != 0 {
		t.Errorf("expected empty trash, got %d", len(motes))
	}

	// Delete two motes
	m1, _ := mm.Create("lesson", "Trash 1", CreateOpts{})
	m2, _ := mm.Create("lesson", "Trash 2", CreateOpts{})
	mm.Delete(m1.ID, im)
	mm.Delete(m2.ID, im)

	motes, err = mm.ListTrash()
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if len(motes) != 2 {
		t.Errorf("expected 2 trashed motes, got %d", len(motes))
	}
}

func TestMoteManager_PurgeTrash(t *testing.T) {
	root, mm := setupTestMemory(t)
	im := NewIndexManager(root)
	im.Rebuild(nil)

	m1, _ := mm.Create("lesson", "Old mote", CreateOpts{})
	m2, _ := mm.Create("lesson", "New mote", CreateOpts{})
	mm.Delete(m1.ID, im)
	mm.Delete(m2.ID, im)

	// Backdate m1's deleted_at to 31 days ago
	trashPath := filepath.Join(root, "trash", m1.ID+".md")
	trashed, _ := ParseMote(trashPath)
	old := time.Now().UTC().Add(-31 * 24 * time.Hour)
	trashed.DeletedAt = &old
	data, _ := SerializeMote(trashed)
	AtomicWrite(trashPath, data, 0644)

	// Purge with 30-day retention — should only purge m1
	purged, err := mm.PurgeTrash(30, false)
	if err != nil {
		t.Fatalf("PurgeTrash: %v", err)
	}
	if len(purged) != 1 || purged[0] != m1.ID {
		t.Errorf("expected to purge %s, got %v", m1.ID, purged)
	}

	// m2 should still be in trash
	remaining, _ := mm.ListTrash()
	if len(remaining) != 1 {
		t.Errorf("expected 1 remaining in trash, got %d", len(remaining))
	}
}

func TestMoteManager_PurgeTrashAll(t *testing.T) {
	root, mm := setupTestMemory(t)
	im := NewIndexManager(root)
	im.Rebuild(nil)

	m1, _ := mm.Create("lesson", "Mote A", CreateOpts{})
	m2, _ := mm.Create("lesson", "Mote B", CreateOpts{})
	mm.Delete(m1.ID, im)
	mm.Delete(m2.ID, im)

	purged, err := mm.PurgeTrash(30, true)
	if err != nil {
		t.Fatalf("PurgeTrash --all: %v", err)
	}
	if len(purged) != 2 {
		t.Errorf("expected 2 purged, got %d", len(purged))
	}

	remaining, _ := mm.ListTrash()
	if len(remaining) != 0 {
		t.Errorf("expected empty trash, got %d", len(remaining))
	}
}

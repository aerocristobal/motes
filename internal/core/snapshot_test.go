// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotMote_Fields(t *testing.T) {
	m := &Mote{
		ID:     "test-123",
		Status: "active",
		Title:  "Test Mote",
		Tags:   []string{"a", "b"},
		Weight: 0.5,
		Body:   "Some body content",
	}
	m.RelatesTo = []string{"other-1"}

	snap := SnapshotMote(m)

	if snap.Status != "active" {
		t.Errorf("status = %q, want %q", snap.Status, "active")
	}
	if snap.Title != "Test Mote" {
		t.Errorf("title = %q, want %q", snap.Title, "Test Mote")
	}
	if len(snap.Tags) != 2 || snap.Tags[0] != "a" {
		t.Errorf("tags = %v, want [a b]", snap.Tags)
	}
	if snap.Weight != 0.5 {
		t.Errorf("weight = %f, want 0.5", snap.Weight)
	}
	if snap.BodyHash == "" {
		t.Error("body_hash should not be empty")
	}
	if len(snap.Links["relates_to"]) != 1 || snap.Links["relates_to"][0] != "other-1" {
		t.Errorf("links = %v, want relates_to=[other-1]", snap.Links)
	}
	if snap.SnapshotAt == "" {
		t.Error("snapshot_at should not be empty")
	}
}

func TestSaveLoadSnapshot_Roundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	mm := NewMoteManager(tmpDir)

	m := &Mote{
		ID:     "test-rt",
		Status: "active",
		Title:  "Roundtrip Test",
		Tags:   []string{"x"},
		Weight: 0.75,
		Body:   "body text",
	}

	snap := SnapshotMote(m)
	if err := mm.SaveSnapshot("test-rt", snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// Verify file exists
	path := filepath.Join(tmpDir, ".snapshots", "test-rt.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("snapshot file not created: %v", err)
	}

	loaded, err := mm.LoadSnapshot("test-rt")
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}

	if loaded.Status != snap.Status {
		t.Errorf("status = %q, want %q", loaded.Status, snap.Status)
	}
	if loaded.Title != snap.Title {
		t.Errorf("title = %q, want %q", loaded.Title, snap.Title)
	}
	if loaded.BodyHash != snap.BodyHash {
		t.Errorf("body_hash mismatch")
	}
}

func TestDiffMote_NoChanges(t *testing.T) {
	m := &Mote{
		ID:     "test-nc",
		Status: "active",
		Title:  "No Change",
		Tags:   []string{"a"},
		Weight: 0.5,
		Body:   "same body",
	}
	snap := SnapshotMote(m)
	diffs := DiffMote(m, &snap)
	if len(diffs) != 0 {
		t.Errorf("expected 0 diffs, got %d: %+v", len(diffs), diffs)
	}
}

func TestDiffMote_DetectsChanges(t *testing.T) {
	m := &Mote{
		ID:     "test-dc",
		Status: "active",
		Title:  "Original Title",
		Tags:   []string{"a"},
		Weight: 0.5,
		Body:   "original body",
	}
	snap := SnapshotMote(m)

	// Modify the mote
	m.Status = "completed"
	m.Title = "Updated Title"
	m.Tags = []string{"a", "b"}
	m.Body = "modified body"
	m.RelatesTo = []string{"other-1"}

	diffs := DiffMote(m, &snap)

	fields := map[string]bool{}
	for _, d := range diffs {
		fields[d.Field] = true
	}

	for _, expected := range []string{"status", "title", "tags", "body", "links.relates_to"} {
		if !fields[expected] {
			t.Errorf("missing diff for field %q; got fields: %v", expected, fields)
		}
	}
}

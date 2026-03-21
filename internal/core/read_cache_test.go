package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadCache_HitOnUnchangedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	os.WriteFile(path, []byte("content"), 0644)

	rc := NewReadCache()
	m := &Mote{ID: "test-1", Title: "Test"}
	rc.Put("test-1", path, m)

	got, ok := rc.Get("test-1", path)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.ID != "test-1" {
		t.Errorf("got ID %q, want %q", got.ID, "test-1")
	}
}

func TestReadCache_MissOnModifiedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	os.WriteFile(path, []byte("original"), 0644)

	rc := NewReadCache()
	m := &Mote{ID: "test-1", Title: "Test"}
	rc.Put("test-1", path, m)

	// Ensure mtime changes — some filesystems have 1s granularity
	time.Sleep(10 * time.Millisecond)
	now := time.Now().Add(time.Second)
	os.Chtimes(path, now, now)

	_, ok := rc.Get("test-1", path)
	if ok {
		t.Fatal("expected cache miss after file modification")
	}
}

func TestReadCache_MissOnUnknownID(t *testing.T) {
	rc := NewReadCache()
	_, ok := rc.Get("nonexistent", "/tmp/no-such-file")
	if ok {
		t.Fatal("expected cache miss for unknown ID")
	}
}

func TestReadCache_MissOnDeletedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	os.WriteFile(path, []byte("content"), 0644)

	rc := NewReadCache()
	rc.Put("test-1", path, &Mote{ID: "test-1"})

	os.Remove(path)

	_, ok := rc.Get("test-1", path)
	if ok {
		t.Fatal("expected cache miss after file deletion")
	}
}

func TestMoteManager_ReadUsesCache(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, ".memory")
	os.MkdirAll(filepath.Join(root, "nodes"), 0755)

	mm := NewMoteManager(root)
	mm.SetGlobalRoot(root)
	m, err := mm.Create("lesson", "Cache test", CreateOpts{Tags: []string{"test"}})
	if err != nil {
		t.Fatal(err)
	}

	// First read populates cache
	m1, err := mm.Read(m.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Second read should return cached version (same pointer won't be guaranteed,
	// but the data should match)
	m2, err := mm.Read(m.ID)
	if err != nil {
		t.Fatal(err)
	}

	if m1.Title != m2.Title {
		t.Errorf("titles don't match: %q vs %q", m1.Title, m2.Title)
	}
}

func TestMoteManager_ReadAllParallelPopulatesCache(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, ".memory")
	os.MkdirAll(filepath.Join(root, "nodes"), 0755)

	mm := NewMoteManager(root)
	mm.SetGlobalRoot(root)
	mm.Create("lesson", "Mote A", CreateOpts{Tags: []string{"test"}})
	mm.Create("decision", "Mote B", CreateOpts{Tags: []string{"test"}})

	motes, err := mm.ReadAllParallel()
	if err != nil {
		t.Fatal(err)
	}
	if len(motes) < 2 {
		t.Fatalf("expected 2+ motes, got %d", len(motes))
	}

	// Subsequent Read calls should hit cache
	for _, m := range motes {
		got, err := mm.Read(m.ID)
		if err != nil {
			t.Errorf("Read(%s) failed: %v", m.ID, err)
		}
		if got.Title != m.Title {
			t.Errorf("Read(%s) title mismatch", m.ID)
		}
	}
}

package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupTestMemory(t *testing.T) (string, *MoteManager) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, ".memory")
	if err := os.MkdirAll(filepath.Join(root, "nodes"), 0755); err != nil {
		t.Fatal(err)
	}
	return root, NewMoteManager(root)
}

func TestMoteManager_CreateRead(t *testing.T) {
	root, mm := setupTestMemory(t)
	_ = root

	m, err := mm.Create("task", "Test task", CreateOpts{
		Tags:   []string{"oauth", "auth"},
		Weight: 0.8,
		Origin: "failure",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if m.Type != "task" {
		t.Errorf("Type: got %q", m.Type)
	}
	if m.Status != "active" {
		t.Errorf("Status: got %q", m.Status)
	}
	if m.Weight != 0.8 {
		t.Errorf("Weight: got %f", m.Weight)
	}
	if m.Origin != "failure" {
		t.Errorf("Origin: got %q", m.Origin)
	}
	if m.AccessCount != 0 {
		t.Errorf("AccessCount: got %d", m.AccessCount)
	}
	if m.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if len(m.Tags) != 2 {
		t.Errorf("Tags: got %v", m.Tags)
	}

	// Read back
	read, err := mm.Read(m.ID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if read.ID != m.ID {
		t.Errorf("ID mismatch: got %q, want %q", read.ID, m.ID)
	}
	if read.Title != "Test task" {
		t.Errorf("Title: got %q", read.Title)
	}
}

func TestMoteManager_CreateDefaults(t *testing.T) {
	_, mm := setupTestMemory(t)

	m, err := mm.Create("lesson", "Default test", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if m.Weight != 0.5 {
		t.Errorf("default weight: got %f, want 0.5", m.Weight)
	}
	if m.Origin != "normal" {
		t.Errorf("default origin: got %q, want normal", m.Origin)
	}
}

func TestMoteManager_CreateWithBody(t *testing.T) {
	_, mm := setupTestMemory(t)

	m, err := mm.Create("decision", "Auth approach", CreateOpts{
		Body: "We chose JWT over sessions.\n",
	})
	if err != nil {
		t.Fatal(err)
	}

	read, err := mm.Read(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read.Body != "We chose JWT over sessions.\n" {
		t.Errorf("Body: got %q", read.Body)
	}
}

func TestMoteManager_ReadMissing(t *testing.T) {
	_, mm := setupTestMemory(t)

	_, err := mm.Read("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for missing mote")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected IsNotExist error, got: %v", err)
	}
}

func TestMoteManager_Update(t *testing.T) {
	_, mm := setupTestMemory(t)

	m, _ := mm.Create("task", "Original", CreateOpts{})

	err := mm.Update(m.ID, UpdateOpts{
		Status: StringPtr("completed"),
		Weight: Float64Ptr(0.9),
	})
	if err != nil {
		t.Fatal(err)
	}

	read, _ := mm.Read(m.ID)
	if read.Status != "completed" {
		t.Errorf("Status: got %q", read.Status)
	}
	if read.Weight != 0.9 {
		t.Errorf("Weight: got %f", read.Weight)
	}
}

func TestMoteManager_Deprecate(t *testing.T) {
	_, mm := setupTestMemory(t)

	m, _ := mm.Create("decision", "Old way", CreateOpts{})
	if err := mm.Deprecate(m.ID, "new-id"); err != nil {
		t.Fatal(err)
	}

	read, _ := mm.Read(m.ID)
	if read.Status != "deprecated" {
		t.Errorf("Status: got %q", read.Status)
	}
	if read.DeprecatedBy != "new-id" {
		t.Errorf("DeprecatedBy: got %q", read.DeprecatedBy)
	}
}

func TestMoteManager_ListNoFilter(t *testing.T) {
	_, mm := setupTestMemory(t)

	mm.Create("task", "T1", CreateOpts{})
	mm.Create("lesson", "L1", CreateOpts{})
	mm.Create("decision", "D1", CreateOpts{})

	motes, err := mm.List(ListFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(motes) != 3 {
		t.Errorf("expected 3, got %d", len(motes))
	}
}

func TestMoteManager_ListByType(t *testing.T) {
	_, mm := setupTestMemory(t)

	mm.Create("task", "T1", CreateOpts{})
	mm.Create("task", "T2", CreateOpts{})
	mm.Create("lesson", "L1", CreateOpts{})

	motes, err := mm.List(ListFilters{Type: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if len(motes) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(motes))
	}
}

func TestMoteManager_ListByTag(t *testing.T) {
	_, mm := setupTestMemory(t)

	mm.Create("task", "T1", CreateOpts{Tags: []string{"oauth"}})
	mm.Create("task", "T2", CreateOpts{Tags: []string{"docker"}})
	mm.Create("lesson", "L1", CreateOpts{Tags: []string{"oauth", "auth"}})

	motes, err := mm.List(ListFilters{Tag: "oauth"})
	if err != nil {
		t.Fatal(err)
	}
	if len(motes) != 2 {
		t.Errorf("expected 2 with oauth tag, got %d", len(motes))
	}
}

func TestMoteManager_ListByStatus(t *testing.T) {
	_, mm := setupTestMemory(t)

	m1, _ := mm.Create("task", "Active", CreateOpts{})
	m2, _ := mm.Create("task", "Deprecated", CreateOpts{})
	_ = m1
	mm.Update(m2.ID, UpdateOpts{Status: StringPtr("deprecated")})

	motes, err := mm.List(ListFilters{Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if len(motes) != 1 {
		t.Errorf("expected 1 active, got %d", len(motes))
	}
}

func TestMoteManager_ListStale(t *testing.T) {
	_, mm := setupTestMemory(t)

	// Stale: accessed 91 days ago
	m1, _ := mm.Create("task", "Stale", CreateOpts{})
	old := time.Now().Add(-91 * 24 * time.Hour)
	mm.Update(m1.ID, UpdateOpts{
		LastAccessed: &old,
	})

	// Fresh: accessed 10 days ago
	m2, _ := mm.Create("task", "Fresh", CreateOpts{})
	recent := time.Now().Add(-10 * 24 * time.Hour)
	mm.Update(m2.ID, UpdateOpts{
		LastAccessed: &recent,
	})

	// Never accessed (nil) = stale
	mm.Create("task", "Never accessed", CreateOpts{})

	motes, err := mm.List(ListFilters{Stale: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(motes) != 2 {
		t.Errorf("expected 2 stale, got %d", len(motes))
	}
}

func TestMoteManager_ReadAllParallel_50(t *testing.T) {
	_, mm := setupTestMemory(t)

	ids := make(map[string]bool)
	for range 50 {
		m, err := mm.Create("task", "Parallel test", CreateOpts{})
		if err != nil {
			t.Fatal(err)
		}
		ids[m.ID] = true
	}

	motes, err := mm.ReadAllParallel()
	if err != nil {
		t.Fatal(err)
	}
	if len(motes) != 50 {
		t.Errorf("expected 50, got %d", len(motes))
	}

	seen := make(map[string]bool)
	for _, m := range motes {
		if seen[m.ID] {
			t.Errorf("duplicate: %s", m.ID)
		}
		seen[m.ID] = true
	}
}

func TestMoteManager_ReadAllParallel_Malformed(t *testing.T) {
	root, mm := setupTestMemory(t)

	// Create 5 valid motes
	for range 5 {
		if _, err := mm.Create("task", "Valid", CreateOpts{}); err != nil {
			t.Fatal(err)
		}
	}

	// Create 1 malformed file
	badPath := filepath.Join(root, "nodes", "bad-mote.md")
	os.WriteFile(badPath, []byte("not valid yaml at all"), 0644)

	motes, err := mm.ReadAllParallel()
	if err != nil {
		t.Fatal(err)
	}
	if len(motes) != 5 {
		t.Errorf("expected 5 valid motes, got %d", len(motes))
	}
}

func TestMoteManager_AppendAccessBatch(t *testing.T) {
	root, mm := setupTestMemory(t)

	m, _ := mm.Create("task", "Test", CreateOpts{})
	if err := mm.AppendAccessBatch(m.ID); err != nil {
		t.Fatal(err)
	}

	batchPath := filepath.Join(root, ".access_batch.jsonl")
	data, err := os.ReadFile(batchPath)
	if err != nil {
		t.Fatal(err)
	}

	var entry AccessBatchEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &entry); err != nil {
		t.Fatalf("parse batch entry: %v", err)
	}
	if entry.MoteID != m.ID {
		t.Errorf("MoteID: got %q, want %q", entry.MoteID, m.ID)
	}
}

func TestMoteManager_FlushAccessBatch(t *testing.T) {
	root, mm := setupTestMemory(t)

	m, _ := mm.Create("task", "Test", CreateOpts{})

	// Write 3 access entries
	for range 3 {
		mm.AppendAccessBatch(m.ID)
	}

	if err := mm.FlushAccessBatch(); err != nil {
		t.Fatal(err)
	}

	// Mote should be updated
	read, _ := mm.Read(m.ID)
	if read.AccessCount != 3 {
		t.Errorf("AccessCount: got %d, want 3", read.AccessCount)
	}
	if read.LastAccessed == nil {
		t.Error("LastAccessed should be set")
	}

	// Batch file should be removed
	batchPath := filepath.Join(root, ".access_batch.jsonl")
	if _, err := os.Stat(batchPath); !os.IsNotExist(err) {
		t.Error("batch file should be removed after flush")
	}
}

func TestMoteManager_FlushAccessBatch_Empty(t *testing.T) {
	_, mm := setupTestMemory(t)

	// No batch file — should succeed silently
	if err := mm.FlushAccessBatch(); err != nil {
		t.Fatalf("FlushAccessBatch on empty: %v", err)
	}
}

func TestMoteManager_ScopeDerivation(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "myproject", ".memory")
	os.MkdirAll(filepath.Join(root, "nodes"), 0755)

	mm := NewMoteManager(root)
	m, err := mm.Create("task", "Scope test", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(m.ID, "myproject-") {
		t.Errorf("ID should start with 'myproject-', got %q", m.ID)
	}
}

func TestCreate_InvalidType(t *testing.T) {
	_, mm := setupTestMemory(t)
	_, err := mm.Create("bogus", "title", CreateOpts{})
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestCreate_EmptyTitle(t *testing.T) {
	_, mm := setupTestMemory(t)
	_, err := mm.Create("task", "", CreateOpts{})
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestCreate_WhitespaceTitle(t *testing.T) {
	_, mm := setupTestMemory(t)
	_, err := mm.Create("task", "   ", CreateOpts{})
	if err == nil {
		t.Fatal("expected error for whitespace-only title")
	}
}

func TestCreate_InvalidTag(t *testing.T) {
	_, mm := setupTestMemory(t)
	_, err := mm.Create("task", "title", CreateOpts{Tags: []string{"valid", "has spaces"}})
	if err == nil {
		t.Fatal("expected error for invalid tag")
	}
}

func TestCreate_InvalidWeight(t *testing.T) {
	_, mm := setupTestMemory(t)
	_, err := mm.Create("task", "title", CreateOpts{Weight: 5.0})
	if err == nil {
		t.Fatal("expected error for invalid weight")
	}
}

func TestCreate_InvalidOrigin(t *testing.T) {
	_, mm := setupTestMemory(t)
	_, err := mm.Create("task", "title", CreateOpts{Origin: "bogus"})
	if err == nil {
		t.Fatal("expected error for invalid origin")
	}
}

func TestCreate_InvalidSize(t *testing.T) {
	_, mm := setupTestMemory(t)
	_, err := mm.Create("task", "title", CreateOpts{Size: "xxl"})
	if err == nil {
		t.Fatal("expected error for invalid size")
	}
}

func TestUpdate_InvalidStatus(t *testing.T) {
	_, mm := setupTestMemory(t)
	m, err := mm.Create("task", "title", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	err = mm.Update(m.ID, UpdateOpts{Status: StringPtr("bogus")})
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestUpdate_InvalidWeight(t *testing.T) {
	_, mm := setupTestMemory(t)
	m, err := mm.Create("task", "title", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	err = mm.Update(m.ID, UpdateOpts{Weight: Float64Ptr(2.0)})
	if err == nil {
		t.Fatal("expected error for invalid weight")
	}
}

func TestUpdate_EmptyTitle(t *testing.T) {
	_, mm := setupTestMemory(t)
	m, err := mm.Create("task", "title", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	err = mm.Update(m.ID, UpdateOpts{Title: StringPtr("")})
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestUpdate_InvalidTag(t *testing.T) {
	_, mm := setupTestMemory(t)
	m, err := mm.Create("task", "title", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	err = mm.Update(m.ID, UpdateOpts{Tags: []string{"has spaces"}})
	if err == nil {
		t.Fatal("expected error for invalid tag")
	}
}

func TestUpdate_InvalidSize(t *testing.T) {
	_, mm := setupTestMemory(t)
	m, err := mm.Create("task", "title", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	err = mm.Update(m.ID, UpdateOpts{Size: StringPtr("xxl")})
	if err == nil {
		t.Fatal("expected error for invalid size")
	}
}

func TestMoteManager_CreateSetsFilePath(t *testing.T) {
	_, mm := setupTestMemory(t)

	m, err := mm.Create("task", "Path test", CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(m.FilePath, m.ID+".md") {
		t.Errorf("FilePath %q should end with %s.md", m.FilePath, m.ID)
	}
}

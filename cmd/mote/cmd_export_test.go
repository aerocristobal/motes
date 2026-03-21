package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/core"
)

func TestExportRoundTrip(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, ".memory")
	os.MkdirAll(filepath.Join(root, "nodes"), 0755)
	t.Setenv("MOTE_GLOBAL_ROOT", root)

	mm := core.NewMoteManager(root)

	// Create a mote with external refs
	m, err := mm.Create("lesson", "Test Lesson", core.CreateOpts{
		Tags:   []string{"test", "export"},
		Weight: 0.7,
		Body:   "This is the body.",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Add external ref
	m.ExternalRefs = []core.ExternalRef{
		{Provider: "github", ID: "42", URL: "https://github.com/test/42"},
	}
	data, _ := core.SerializeMote(m)
	path, _ := mm.MoteFilePath(m.ID)
	core.AtomicWrite(path, data, 0644)

	// Re-read to verify
	m2, err := mm.Read(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(m2.ExternalRefs) != 1 {
		t.Fatalf("expected 1 external ref, got %d", len(m2.ExternalRefs))
	}

	// Test export conversion
	exported := moteToExport(m2)
	if exported.ID != m.ID {
		t.Errorf("expected ID %s, got %s", m.ID, exported.ID)
	}
	if exported.Body != "This is the body." {
		t.Errorf("expected body 'This is the body.', got %q", exported.Body)
	}
	if len(exported.ExternalRefs) != 1 {
		t.Errorf("expected 1 external ref in export, got %d", len(exported.ExternalRefs))
	}

	// Test JSON encoding
	jsonBytes, err := json.Marshal(exported)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonBytes), "github") {
		t.Error("expected JSON to contain 'github'")
	}

	// Test round-trip via JSONL
	var imported ExportMote
	if err := json.Unmarshal(jsonBytes, &imported); err != nil {
		t.Fatal(err)
	}
	if imported.Title != "Test Lesson" {
		t.Errorf("expected title 'Test Lesson', got %q", imported.Title)
	}
	if imported.Body != "This is the body." {
		t.Errorf("expected body round-trip, got %q", imported.Body)
	}
}

func TestImportDedup(t *testing.T) {
	// Test that content hash dedup works
	em1 := &ExportMote{Type: "lesson", Title: "Same Title", Body: "Same Body"}
	em2 := &ExportMote{Type: "lesson", Title: "Same Title", Body: "Same Body"}
	em3 := &ExportMote{Type: "lesson", Title: "Different", Body: "Same Body"}

	h1 := exportMoteContentHash(em1)
	h2 := exportMoteContentHash(em2)
	h3 := exportMoteContentHash(em3)

	if h1 != h2 {
		t.Error("identical motes should have same hash")
	}
	if h1 == h3 {
		t.Error("different motes should have different hashes")
	}
}

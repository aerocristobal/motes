package strata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCorpus_CreatesNew(t *testing.T) {
	root, sm, _ := setupStrataTest(t)

	// Create a source file
	srcDir := filepath.Join(filepath.Dir(root), "src")
	os.MkdirAll(srcDir, 0755)
	path := writeTestFile(t, srcDir, "main.go", "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n")

	changed, err := sm.EnsureCorpus("_codebase", []string{path})
	if err != nil {
		t.Fatalf("EnsureCorpus: %v", err)
	}
	if changed != 1 {
		t.Errorf("changed = %d, want 1", changed)
	}

	// Verify corpus exists
	corpora, err := sm.ListCorpora()
	if err != nil {
		t.Fatalf("ListCorpora: %v", err)
	}
	found := false
	for _, c := range corpora {
		if c.Manifest.Name == "_codebase" {
			found = true
			if c.Manifest.ChunkCount == 0 {
				t.Error("expected chunks in corpus")
			}
		}
	}
	if !found {
		t.Error("_codebase corpus not found after EnsureCorpus")
	}
}

func TestEnsureCorpus_AddsNewFiles(t *testing.T) {
	root, sm, _ := setupStrataTest(t)

	srcDir := filepath.Join(filepath.Dir(root), "src")
	os.MkdirAll(srcDir, 0755)
	pathA := writeTestFile(t, srcDir, "a.go", "package main\n\n// FileA has scoring logic.\nfunc ScoreA() int { return 1 }\n")

	// Create initial corpus
	changed, err := sm.EnsureCorpus("_codebase", []string{pathA})
	if err != nil {
		t.Fatalf("initial EnsureCorpus: %v", err)
	}
	if changed != 1 {
		t.Errorf("initial changed = %d, want 1", changed)
	}

	// Add a second file
	pathB := writeTestFile(t, srcDir, "b.go", "package main\n\n// FileB has indexing logic.\nfunc IndexB() int { return 2 }\n")

	changed, err = sm.EnsureCorpus("_codebase", []string{pathB})
	if err != nil {
		t.Fatalf("second EnsureCorpus: %v", err)
	}
	if changed != 1 {
		t.Errorf("second changed = %d, want 1", changed)
	}

	// Verify both files are in the corpus
	manifest, err := sm.loadManifest("_codebase")
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	if len(manifest.SourcePaths) != 2 {
		t.Errorf("SourcePaths = %d, want 2: %v", len(manifest.SourcePaths), manifest.SourcePaths)
	}
}

func TestEnsureCorpus_SkipsUnchanged(t *testing.T) {
	root, sm, _ := setupStrataTest(t)

	srcDir := filepath.Join(filepath.Dir(root), "src")
	os.MkdirAll(srcDir, 0755)
	path := writeTestFile(t, srcDir, "unchanged.go", "package main\n\nfunc Unchanged() {}\n")

	// Initial create
	_, err := sm.EnsureCorpus("_codebase", []string{path})
	if err != nil {
		t.Fatalf("initial EnsureCorpus: %v", err)
	}

	// Same file, no changes
	changed, err := sm.EnsureCorpus("_codebase", []string{path})
	if err != nil {
		t.Fatalf("second EnsureCorpus: %v", err)
	}
	if changed != 0 {
		t.Errorf("changed = %d, want 0 (no changes)", changed)
	}
}

func TestEnsureCorpus_EmptyPaths(t *testing.T) {
	_, sm, _ := setupStrataTest(t)

	changed, err := sm.EnsureCorpus("_codebase", nil)
	if err != nil {
		t.Fatalf("EnsureCorpus with nil: %v", err)
	}
	if changed != 0 {
		t.Errorf("changed = %d, want 0", changed)
	}
}

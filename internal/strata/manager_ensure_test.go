// SPDX-License-Identifier: MIT
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

// TestEnsureCorpus_BasenameCollisionNoExplosion is the regression test for the
// chunk-explosion bug that ballooned chunks.jsonl to multiple GB and OOMed
// session-end with ~47GB RSS. When multiple source files share a basename
// (e.g. services/*/install.sh), prior versions keyed the reuse map by
// basename, causing each unchanged-path lookup to pull chunks for every
// colliding file. Each ensure run multiplied chunks by the collision count.
func TestEnsureCorpus_BasenameCollisionNoExplosion(t *testing.T) {
	root, sm, _ := setupStrataTest(t)
	parent := filepath.Dir(root)

	// Three files that all share basename "install.sh" but live in distinct
	// directories — mirrors services/{a,b,c}/install.sh in the wild.
	dirs := []string{"svc-a", "svc-b", "svc-c"}
	var paths []string
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(parent, d), 0755)
		paths = append(paths, writeTestFile(t, filepath.Join(parent, d), "install.sh",
			"#!/usr/bin/env bash\necho installing "+d+"\n"))
	}

	if _, err := sm.EnsureCorpus("_codebase", paths); err != nil {
		t.Fatalf("initial EnsureCorpus: %v", err)
	}
	baseline, err := sm.loadManifest("_codebase")
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	initialChunks := baseline.ChunkCount

	// Trigger a re-ingest by adding a new file with a different basename, so
	// ensure runs the "unchanged paths reuse cached chunks" branch for all
	// three install.sh files. Repeat several times to surface any
	// multiplication on each pass.
	newDir := filepath.Join(parent, "extra")
	os.MkdirAll(newDir, 0755)
	for i := 0; i < 5; i++ {
		extra := writeTestFile(t, newDir, "trigger.sh",
			"#!/usr/bin/env bash\necho pass "+string(rune('0'+i))+"\n")
		if _, err := sm.EnsureCorpus("_codebase", []string{extra}); err != nil {
			t.Fatalf("EnsureCorpus pass %d: %v", i, err)
		}
	}

	final, err := sm.loadManifest("_codebase")
	if err != nil {
		t.Fatalf("loadManifest final: %v", err)
	}

	// Expected: baseline chunks + a small constant for the one trigger file
	// (last write wins). Bug behaviour: chunks grow as ~initialChunks * 3^N.
	if final.ChunkCount > initialChunks+50 {
		t.Fatalf("chunk explosion: initial=%d final=%d (basename collision dedup broken)",
			initialChunks, final.ChunkCount)
	}
}

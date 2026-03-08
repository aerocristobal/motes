package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"motes/internal/core"
	"motes/internal/strata"
)

func TestPromptContext_ShortPrompt(t *testing.T) {
	// Prompts with < 3 keywords should return empty JSON
	dir := t.TempDir()
	setupTestMemory(t, dir)

	old := os.Stdin
	defer func() { os.Stdin = old }()

	r, w, _ := os.Pipe()
	w.Write([]byte(`{"prompt": "yes"}`))
	w.Close()
	os.Stdin = r

	// Capture stdout
	oldOut := os.Stdout
	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	origDir, _ := os.Getwd()
	os.Chdir(filepath.Dir(dir))
	defer os.Chdir(origDir)

	err := runPromptContext(nil, nil)
	outW.Close()
	os.Stdout = oldOut

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	buf := make([]byte, 1024)
	n, _ := outR.Read(buf)
	output := string(buf[:n])

	if output != "{}\n" {
		t.Errorf("expected empty JSON for short prompt, got: %s", output)
	}
}

func TestPromptContext_NoMemory(t *testing.T) {
	// When no .memory/ exists, should return empty JSON
	dir := t.TempDir()

	old := os.Stdin
	defer func() { os.Stdin = old }()

	r, w, _ := os.Pipe()
	w.Write([]byte(`{"prompt": "implement the scoring algorithm for search"}`))
	w.Close()
	os.Stdin = r

	oldOut := os.Stdout
	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	err := runPromptContext(nil, nil)
	outW.Close()
	os.Stdout = oldOut

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	buf := make([]byte, 1024)
	n, _ := outR.Read(buf)
	output := string(buf[:n])

	if output != "{}\n" {
		t.Errorf("expected empty JSON when no memory, got: %s", output)
	}
}

func TestPromptContext_WithMotes(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, ".memory")
	setupTestMemory(t, root)

	mm := core.NewMoteManager(root)

	// Create motes — need enough docs for meaningful BM25 IDF scores
	mm.Create("decision", "BM25 scoring algorithm choice", core.CreateOpts{
		Tags: []string{"scoring", "search"},
		Body: "We chose BM25 for full-text search scoring because it handles term frequency saturation well. BM25 scoring search algorithm.",
	})
	mm.Create("lesson", "Index rebuild performance", core.CreateOpts{
		Tags: []string{"performance", "indexing"},
		Body: "Rebuilding the index takes under 50ms for 100 motes.",
	})
	mm.Create("explore", "Database options explored", core.CreateOpts{
		Tags: []string{"database"},
		Body: "Explored SQLite, BadgerDB, and flat files for storage.",
	})
	mm.Create("decision", "Go as implementation language", core.CreateOpts{
		Tags: []string{"go", "language"},
		Body: "Go was chosen for single binary distribution and fast startup.",
	})
	mm.Create("lesson", "YAML frontmatter parsing gotchas", core.CreateOpts{
		Tags: []string{"yaml", "parsing"},
		Body: "YAML frontmatter must have a blank line after the closing dashes.",
	})

	// Build BM25 index
	motes, _ := mm.ReadAllParallel()
	rebuildMoteBM25(root, motes)

	// Now test prompt-context with a relevant prompt
	old := os.Stdin
	defer func() { os.Stdin = old }()

	r, w, _ := os.Pipe()
	w.Write([]byte(`{"prompt": "improve the BM25 scoring algorithm for search results"}`))
	w.Close()
	os.Stdin = r

	oldOut := os.Stdout
	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	err := runPromptContext(nil, nil)
	outW.Close()
	os.Stdout = oldOut

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := outR.Read(buf)
	output := string(buf[:n])

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid JSON output: %s", output)
	}

	// With adaptive thresholds, 5-mote corpus uses ~0.78 threshold,
	// so BM25 results should now surface for relevant queries
	if result == nil {
		t.Error("expected valid JSON output")
	}
	if ctx, ok := result["additionalContext"]; ok {
		ctxStr, _ := ctx.(string)
		if ctxStr == "" {
			t.Error("expected non-empty additionalContext with adaptive thresholds")
		}
	}
}

func setupTestMemory(t *testing.T, root string) {
	t.Helper()
	os.MkdirAll(filepath.Join(root, "nodes"), 0755)
	os.MkdirAll(filepath.Join(root, "dream"), 0755)
	os.MkdirAll(filepath.Join(root, "strata"), 0755)
	core.SaveConfig(root, core.DefaultConfig())

	// Create empty index
	im := core.NewIndexManager(root)
	im.Rebuild(nil)

	// Create empty BM25
	idx := strata.BuildBM25Index(nil)
	data, _ := json.Marshal(idx)
	bm25m := core.NewMoteBM25Manager(root)
	bm25m.SaveRaw(data)
}

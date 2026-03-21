package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/core"
	"motes/internal/strata"
)

func TestQueryStrataForChangedFiles_WithCorpus(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	cfg, _ := core.LoadConfig(memDir)

	// Create a source file to ingest
	srcDir := filepath.Join(filepath.Dir(memDir), "src")
	os.MkdirAll(srcDir, 0755)
	scorePath := filepath.Join(srcDir, "score.go")
	os.WriteFile(scorePath, []byte("package core\n\n// ScoreEngine handles scoring and weighting of motes.\ntype ScoreEngine struct {\n\tconfig ScoringConfig\n}\n"), 0644)

	// Ingest into a corpus
	sm := strata.NewStrataManager(memDir, cfg.Strata)
	mm := core.NewMoteManager(memDir)
	if err := sm.AddCorpus("codebase", []string{scorePath}, false, mm); err != nil {
		t.Fatalf("AddCorpus: %v", err)
	}

	// Query with a changed file that matches
	results := queryStrataForChangedFiles(memDir, []string{"internal/core/score.go"}, cfg)
	if len(results) == 0 {
		t.Fatal("expected code context results from strata query, got none")
	}

	// Verify results reference the ingested content
	found := false
	for _, r := range results {
		if strings.Contains(r.Chunk.Text, "ScoreEngine") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected results to contain ScoreEngine content")
	}
}

func TestQueryStrataForChangedFiles_NoChanges(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	cfg, _ := core.LoadConfig(memDir)

	results := queryStrataForChangedFiles(memDir, nil, cfg)
	if len(results) != 0 {
		t.Errorf("expected no results for empty changed files, got %d", len(results))
	}
}

func TestQueryStrataForChangedFiles_BinaryFilesFiltered(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	cfg, _ := core.LoadConfig(memDir)

	// Only binary/non-code files
	results := queryStrataForChangedFiles(memDir, []string{
		"image.png",
		"data.bin",
		"archive.zip",
	}, cfg)
	if len(results) != 0 {
		t.Errorf("expected no results for binary files, got %d", len(results))
	}
}

func TestQueryStrataForChangedFiles_NoCorpora(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	cfg, _ := core.LoadConfig(memDir)

	// Code files but no corpora exist
	results := queryStrataForChangedFiles(memDir, []string{"cmd/main.go"}, cfg)
	if len(results) != 0 {
		t.Errorf("expected no results when no corpora exist, got %d", len(results))
	}
}

func TestIsCodeFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"internal/core/score.go", true},
		{"README.md", true},
		{"script.py", true},
		{"image.png", false},
		{"data.bin", false},
		{"archive.zip", false},
		{"video.mp4", false},
	}
	for _, tt := range tests {
		got := strata.IsCodeFile(tt.path)
		if got != tt.want {
			t.Errorf("IsCodeFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

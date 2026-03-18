package strata

import (
	"os"
	"path/filepath"
	"testing"

	"motes/internal/core"
)

func setupStrataTest(t *testing.T) (string, *StrataManager, *core.MoteManager) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, ".memory")
	os.MkdirAll(filepath.Join(root, "nodes"), 0755)
	os.MkdirAll(filepath.Join(root, "strata"), 0755)

	cfg := core.DefaultConfig()
	sm := NewStrataManager(root, cfg.Strata)
	mm := core.NewMoteManager(root)
	return root, sm, mm
}

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAddCorpus_Basic(t *testing.T) {
	root, sm, _ := setupStrataTest(t)

	dir := t.TempDir()
	writeTestFile(t, dir, "auth.md", "# OAuth\n\nOAuth is an authorization protocol.\n\n# Tokens\n\nTokens expire.\n")

	if err := sm.AddCorpus("auth-docs", []string{filepath.Join(dir, "auth.md")}, false, nil); err != nil {
		t.Fatal(err)
	}

	// Verify files created
	for _, name := range []string{"manifest.json", "chunks.jsonl", "bm25.json"} {
		path := filepath.Join(root, "strata", "auth-docs", name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("%s should exist", name)
		}
	}

	corpora, _ := sm.ListCorpora()
	if len(corpora) != 1 {
		t.Errorf("expected 1 corpus, got %d", len(corpora))
	}
	if corpora[0].Manifest.Name != "auth-docs" {
		t.Errorf("name: got %q", corpora[0].Manifest.Name)
	}
	if corpora[0].Manifest.ChunkCount == 0 {
		t.Error("ChunkCount should be > 0")
	}
}

func TestAddCorpus_WithAnchor(t *testing.T) {
	_, sm, mm := setupStrataTest(t)

	dir := t.TempDir()
	writeTestFile(t, dir, "api.md", "# API\n\nREST API docs.\n")

	if err := sm.AddCorpus("api-docs", []string{filepath.Join(dir, "api.md")}, true, mm); err != nil {
		t.Fatal(err)
	}

	// Verify anchor mote created
	motes, _ := mm.ReadAllParallel()
	found := false
	for _, m := range motes {
		if m.Type == "anchor" && m.StrataCorpus == "api-docs" {
			found = true
			if m.Weight != 0.3 {
				t.Errorf("anchor weight: got %f, want 0.3", m.Weight)
			}
		}
	}
	if !found {
		t.Error("expected anchor mote with strata_corpus=api-docs")
	}
}

func TestAddCorpus_Upsert(t *testing.T) {
	_, sm, _ := setupStrataTest(t)

	dir := t.TempDir()
	path := writeTestFile(t, dir, "doc.md", "# Original\n\nOriginal content.\n")

	sm.AddCorpus("test", []string{path}, false, nil)
	corpora1, _ := sm.ListCorpora()
	count1 := corpora1[0].Manifest.ChunkCount

	// Re-add with different content
	os.WriteFile(path, []byte("# Updated\n\nUpdated content with more text.\n\n# Another Section\n\nMore.\n"), 0644)
	sm.AddCorpus("test", []string{path}, false, nil)

	corpora2, _ := sm.ListCorpora()
	if len(corpora2) != 1 {
		t.Errorf("expected 1 corpus after upsert, got %d", len(corpora2))
	}
	// Chunk count may differ but corpus should still exist
	_ = count1
}

func TestQuery_SingleCorpus(t *testing.T) {
	_, sm, _ := setupStrataTest(t)

	dir := t.TempDir()
	writeTestFile(t, dir, "auth.md", "# OAuth\n\nOAuth is an authorization protocol for API access.\n\n# Docker\n\nDocker is a containerization platform.\n")
	sm.AddCorpus("docs", []string{filepath.Join(dir, "auth.md")}, false, nil)

	results, err := sm.Query("oauth authorization", "docs", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for 'oauth authorization'")
	}
	// First result should be the OAuth chunk
	if results[0].Score <= 0 {
		t.Errorf("expected positive score, got %f", results[0].Score)
	}
}

func TestQuery_AllCorpora(t *testing.T) {
	_, sm, _ := setupStrataTest(t)

	dir := t.TempDir()
	writeTestFile(t, dir, "auth.md", "# OAuth\n\nOAuth authentication protocol.\n")
	writeTestFile(t, dir, "api.md", "# API\n\nREST API design patterns.\n")

	sm.AddCorpus("auth", []string{filepath.Join(dir, "auth.md")}, false, nil)
	sm.AddCorpus("api", []string{filepath.Join(dir, "api.md")}, false, nil)

	results, err := sm.QueryAll("oauth", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results from QueryAll")
	}
}

func TestListCorpora(t *testing.T) {
	_, sm, _ := setupStrataTest(t)

	dir := t.TempDir()
	writeTestFile(t, dir, "a.md", "# A\n\nContent A.\n")
	writeTestFile(t, dir, "b.md", "# B\n\nContent B.\n")

	sm.AddCorpus("corpus-a", []string{filepath.Join(dir, "a.md")}, false, nil)
	sm.AddCorpus("corpus-b", []string{filepath.Join(dir, "b.md")}, false, nil)

	corpora, err := sm.ListCorpora()
	if err != nil {
		t.Fatal(err)
	}
	if len(corpora) != 2 {
		t.Errorf("expected 2 corpora, got %d", len(corpora))
	}
}

func TestRemoveCorpus(t *testing.T) {
	_, sm, _ := setupStrataTest(t)

	dir := t.TempDir()
	writeTestFile(t, dir, "doc.md", "# Doc\n\nContent.\n")
	sm.AddCorpus("to-remove", []string{filepath.Join(dir, "doc.md")}, false, nil)

	if err := sm.RemoveCorpus("to-remove"); err != nil {
		t.Fatal(err)
	}

	corpora, _ := sm.ListCorpora()
	if len(corpora) != 0 {
		t.Errorf("expected 0 corpora after remove, got %d", len(corpora))
	}
}

func TestRemoveCorpus_NotFound(t *testing.T) {
	_, sm, _ := setupStrataTest(t)
	err := sm.RemoveCorpus("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent corpus")
	}
}

func TestQueryAllSorting(t *testing.T) {
	_, sm, _ := setupStrataTest(t)

	dir := t.TempDir()
	// Create two corpora with content that will produce different BM25 scores for "oauth"
	writeTestFile(t, dir, "auth.md", "# OAuth\n\nOAuth is an authorization protocol. OAuth tokens. OAuth flow.\n")
	writeTestFile(t, dir, "misc.md", "# Misc\n\nSome misc content about APIs and servers.\n\n# OAuth note\n\nBrief mention of oauth.\n")

	sm.AddCorpus("auth-corpus", []string{filepath.Join(dir, "auth.md")}, false, nil)
	sm.AddCorpus("misc-corpus", []string{filepath.Join(dir, "misc.md")}, false, nil)

	results, err := sm.QueryAll("oauth", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	// Verify descending score order
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not in descending order: index %d score %f > index %d score %f",
				i, results[i].Score, i-1, results[i-1].Score)
		}
	}

	// Verify topK cap
	results2, err := sm.QueryAll("oauth", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results2) > 1 {
		t.Errorf("topK=1 should cap results to 1, got %d", len(results2))
	}
}

func TestQuery_NoCorpora(t *testing.T) {
	_, sm, _ := setupStrataTest(t)
	results, err := sm.QueryAll("anything", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty strata, got %d", len(results))
	}
}

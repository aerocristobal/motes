package strata

import (
	"encoding/json"
	"testing"
)

func TestBuildBM25Index(t *testing.T) {
	chunks := []Chunk{
		{ID: "c1", Text: "OAuth token refresh authentication"},
		{ID: "c2", Text: "Docker container networking setup"},
		{ID: "c3", Text: "OAuth authentication headers API"},
	}
	idx := BuildBM25Index(chunks)

	if idx.DocCount != 3 {
		t.Errorf("DocCount: got %d, want 3", idx.DocCount)
	}
	if idx.AvgDocLen == 0 {
		t.Error("AvgDocLen should not be 0")
	}
	if _, ok := idx.IDF["oauth"]; !ok {
		t.Error("IDF should contain 'oauth'")
	}
	if len(idx.Docs) != 3 {
		t.Errorf("Docs: got %d, want 3", len(idx.Docs))
	}
}

func TestBM25Search_SingleTerm(t *testing.T) {
	chunks := []Chunk{
		{ID: "c1", Text: "OAuth token refresh authentication headers"},
		{ID: "c2", Text: "Docker container networking setup and configuration"},
		{ID: "c3", Text: "API endpoint design and REST conventions"},
	}
	idx := BuildBM25Index(chunks)
	results := idx.Search("oauth", 3)

	if len(results) == 0 {
		t.Fatal("expected results for 'oauth'")
	}
	if results[0].Chunk.ID != "c1" {
		t.Errorf("expected c1 first, got %s", results[0].Chunk.ID)
	}
}

func TestBM25Search_MultiTerm(t *testing.T) {
	chunks := []Chunk{
		{ID: "c1", Text: "OAuth token refresh and authentication"},
		{ID: "c2", Text: "Docker token management system"},
		{ID: "c3", Text: "OAuth authentication token API headers refresh"},
	}
	idx := BuildBM25Index(chunks)
	results := idx.Search("oauth token refresh", 3)

	if len(results) == 0 {
		t.Fatal("expected results")
	}
	// c3 has more matching terms
	if results[0].Chunk.ID != "c3" && results[0].Chunk.ID != "c1" {
		t.Errorf("expected c3 or c1 first, got %s", results[0].Chunk.ID)
	}
}

func TestBM25Search_NoMatch(t *testing.T) {
	chunks := []Chunk{
		{ID: "c1", Text: "Docker container networking"},
	}
	idx := BuildBM25Index(chunks)
	results := idx.Search("oauth authentication", 3)

	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestBM25Search_TopK(t *testing.T) {
	chunks := []Chunk{
		{ID: "c1", Text: "OAuth implementation guide"},
		{ID: "c2", Text: "OAuth token refresh flow"},
		{ID: "c3", Text: "OAuth authentication headers"},
	}
	idx := BuildBM25Index(chunks)
	results := idx.Search("oauth", 2)

	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}
}

func TestBM25Search_EmptyIndex(t *testing.T) {
	idx := BuildBM25Index(nil)
	results := idx.Search("oauth", 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty index, got %d", len(results))
	}
}

func TestBM25Index_Roundtrip(t *testing.T) {
	chunks := []Chunk{
		{ID: "c1", Text: "OAuth token authentication"},
		{ID: "c2", Text: "Docker setup guide"},
	}
	idx := BuildBM25Index(chunks)

	data, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}

	var loaded BM25Index
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}

	if loaded.DocCount != idx.DocCount {
		t.Errorf("DocCount: got %d, want %d", loaded.DocCount, idx.DocCount)
	}
	if len(loaded.Docs) != len(idx.Docs) {
		t.Errorf("Docs: got %d, want %d", len(loaded.Docs), len(idx.Docs))
	}

	// Verify search still works after roundtrip
	results := loaded.Search("oauth", 3)
	if len(results) == 0 {
		t.Error("search should work after roundtrip")
	}
}

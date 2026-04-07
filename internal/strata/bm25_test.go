// SPDX-License-Identifier: AGPL-3.0-or-later
package strata

import (
	"encoding/json"
	"fmt"
	"math"
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

func TestFindSimilar(t *testing.T) {
	chunks := []Chunk{
		{ID: "m1", Text: "OAuth token refresh authentication flow for API clients"},
		{ID: "m2", Text: "OAuth authentication headers and token validation"},
		{ID: "m3", Text: "Docker container networking and port mapping setup"},
		{ID: "m4", Text: "Kubernetes pod scheduling and resource allocation"},
		{ID: "m5", Text: "OAuth client credentials grant type implementation"},
	}
	idx := BuildBM25Index(chunks)

	// m1 should be most similar to m2 and m5 (OAuth content)
	results := idx.FindSimilar("m1", 3, 0.1, 8)
	if len(results) == 0 {
		t.Fatal("expected similar results for m1")
	}

	// Self should not appear
	for _, r := range results {
		if r.DocID == "m1" {
			t.Error("self should be excluded from results")
		}
	}

	// Top result should be OAuth-related (m2 or m5)
	top := results[0].DocID
	if top != "m2" && top != "m5" {
		t.Errorf("expected m2 or m5 as most similar to m1, got %s", top)
	}

	// Docker/k8s docs should not rank highly for an OAuth doc
	for _, r := range results {
		if r.DocID == "m3" || r.DocID == "m4" {
			// If they appear, they should score much lower than top
			if r.Score > results[0].Score*0.5 {
				t.Errorf("unrelated doc %s scored too high: %.3f vs top %.3f", r.DocID, r.Score, results[0].Score)
			}
		}
	}
}

func TestFindSimilar_ExcludesSelf(t *testing.T) {
	chunks := []Chunk{
		{ID: "a", Text: "unique terms only in document a"},
		{ID: "b", Text: "completely different vocabulary here"},
	}
	idx := BuildBM25Index(chunks)
	results := idx.FindSimilar("a", 5, 0, 8)
	for _, r := range results {
		if r.DocID == "a" {
			t.Error("self should not appear in FindSimilar results")
		}
	}
}

func TestFindSimilar_UnknownDoc(t *testing.T) {
	chunks := []Chunk{
		{ID: "a", Text: "some text here"},
	}
	idx := BuildBM25Index(chunks)
	results := idx.FindSimilar("nonexistent", 3, 0, 8)
	if len(results) != 0 {
		t.Errorf("expected 0 results for unknown doc, got %d", len(results))
	}
}

func TestFindSimilar_MinScore(t *testing.T) {
	chunks := []Chunk{
		{ID: "a", Text: "OAuth token refresh"},
		{ID: "b", Text: "OAuth authentication"},
		{ID: "c", Text: "Docker containers"},
	}
	idx := BuildBM25Index(chunks)

	// With very high minScore, should filter out weak matches
	results := idx.FindSimilar("a", 5, 100.0, 8)
	if len(results) != 0 {
		t.Errorf("expected 0 results with high minScore, got %d", len(results))
	}
}

func TestFindSimilar_EmptyIndex(t *testing.T) {
	idx := BuildBM25Index(nil)
	results := idx.FindSimilar("a", 3, 0, 8)
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty index, got %d", len(results))
	}
}

func TestAdaptiveThresholds_SmallCorpus(t *testing.T) {
	chunks := make([]Chunk, 5)
	for i := range chunks {
		chunks[i] = Chunk{ID: fmt.Sprintf("c%d", i), Text: fmt.Sprintf("document number %d with some content", i)}
	}
	idx := BuildBM25Index(chunks)
	if idx.Thresholds == nil {
		t.Fatal("expected Thresholds to be set")
	}
	if idx.Thresholds.PromptContext >= 1.0 {
		t.Errorf("small corpus prompt_context should be < 1.0, got %.3f", idx.Thresholds.PromptContext)
	}
	if idx.Thresholds.ContentSimilarity >= 0.5 {
		t.Errorf("small corpus content_similarity should be < 0.5, got %.3f", idx.Thresholds.ContentSimilarity)
	}
}

func TestAdaptiveThresholds_ReferenceCorpus(t *testing.T) {
	chunks := make([]Chunk, 50)
	for i := range chunks {
		chunks[i] = Chunk{ID: fmt.Sprintf("c%d", i), Text: fmt.Sprintf("document number %d with some content", i)}
	}
	idx := BuildBM25Index(chunks)
	if idx.Thresholds == nil {
		t.Fatal("expected Thresholds to be set")
	}
	const epsilon = 0.01
	if math.Abs(idx.Thresholds.PromptContext-2.0) > epsilon {
		t.Errorf("reference corpus prompt_context should be ~2.0, got %.3f", idx.Thresholds.PromptContext)
	}
	if math.Abs(idx.Thresholds.ContentSimilarity-1.0) > epsilon {
		t.Errorf("reference corpus content_similarity should be ~1.0, got %.3f", idx.Thresholds.ContentSimilarity)
	}
}

func TestAdaptiveThresholds_LargeCorpus(t *testing.T) {
	chunks := make([]Chunk, 500)
	for i := range chunks {
		chunks[i] = Chunk{ID: fmt.Sprintf("c%d", i), Text: fmt.Sprintf("document number %d with some content", i)}
	}
	idx := BuildBM25Index(chunks)
	if idx.Thresholds == nil {
		t.Fatal("expected Thresholds to be set")
	}
	if idx.Thresholds.PromptContext <= 2.0 {
		t.Errorf("large corpus prompt_context should be > 2.0, got %.3f", idx.Thresholds.PromptContext)
	}
	if idx.Thresholds.ContentSimilarity <= 1.0 {
		t.Errorf("large corpus content_similarity should be > 1.0, got %.3f", idx.Thresholds.ContentSimilarity)
	}
}

func TestAdaptiveThresholds_Clamp(t *testing.T) {
	// Very small corpus — should hit floor
	idx := BuildBM25Index([]Chunk{{ID: "c1", Text: "hello"}})
	if idx.Thresholds.PromptContext < 0.3 {
		t.Errorf("floor violation: prompt_context %.3f < 0.3", idx.Thresholds.PromptContext)
	}
	if idx.Thresholds.ContentSimilarity < 0.15 {
		t.Errorf("floor violation: content_similarity %.3f < 0.15", idx.Thresholds.ContentSimilarity)
	}

	// Very large corpus — should hit ceiling (3× base)
	chunks := make([]Chunk, 100000)
	for i := range chunks {
		chunks[i] = Chunk{ID: fmt.Sprintf("c%d", i), Text: "x"}
	}
	idx2 := BuildBM25Index(chunks)
	if idx2.Thresholds.PromptContext > 6.0 {
		t.Errorf("ceiling violation: prompt_context %.3f > 6.0", idx2.Thresholds.PromptContext)
	}
	if idx2.Thresholds.ContentSimilarity > 3.0 {
		t.Errorf("ceiling violation: content_similarity %.3f > 3.0", idx2.Thresholds.ContentSimilarity)
	}
}

func TestAdaptiveThresholds_NilFallback(t *testing.T) {
	idx := &BM25Index{DocCount: 10}
	// Thresholds is nil
	if got := idx.ThresholdFor("prompt_context"); got != 2.0 {
		t.Errorf("nil fallback prompt_context: got %.3f, want 2.0", got)
	}
	if got := idx.ThresholdFor("content_similarity"); got != 1.0 {
		t.Errorf("nil fallback content_similarity: got %.3f, want 1.0", got)
	}
	if got := idx.ThresholdFor("unknown"); got != 1.0 {
		t.Errorf("nil fallback unknown: got %.3f, want 1.0", got)
	}
}

func TestAdaptiveThresholds_JSONRoundtrip(t *testing.T) {
	chunks := make([]Chunk, 20)
	for i := range chunks {
		chunks[i] = Chunk{ID: fmt.Sprintf("c%d", i), Text: fmt.Sprintf("document %d content", i)}
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

	if loaded.Thresholds == nil {
		t.Fatal("Thresholds should survive JSON roundtrip")
	}
	if loaded.Thresholds.PromptContext != idx.Thresholds.PromptContext {
		t.Errorf("PromptContext mismatch: got %.3f, want %.3f", loaded.Thresholds.PromptContext, idx.Thresholds.PromptContext)
	}
	if loaded.Thresholds.ContentSimilarity != idx.Thresholds.ContentSimilarity {
		t.Errorf("ContentSimilarity mismatch: got %.3f, want %.3f", loaded.Thresholds.ContentSimilarity, idx.Thresholds.ContentSimilarity)
	}
}

func TestSetCalibration(t *testing.T) {
	idx := &BM25Index{DocCount: 50}
	// Simulate score distribution
	scores := []float64{0.1, 0.3, 0.5, 0.7, 0.8, 1.0, 1.2, 1.5, 2.0, 3.0}
	idx.SetCalibration(scores)

	if idx.Thresholds == nil {
		t.Fatal("expected Thresholds after calibration")
	}
	if !idx.Thresholds.Calibrated {
		t.Error("expected Calibrated to be true")
	}
	if idx.Thresholds.CalibrationN != 10 {
		t.Errorf("CalibrationN: got %d, want 10", idx.Thresholds.CalibrationN)
	}
	// p75 of [0.1, 0.3, 0.5, 0.7, 0.8, 1.0, 1.2, 1.5, 2.0, 3.0] = index 6.75 ≈ 1.425
	if idx.Thresholds.PromptContext < 1.0 || idx.Thresholds.PromptContext > 2.0 {
		t.Errorf("calibrated prompt_context out of expected range: %.3f", idx.Thresholds.PromptContext)
	}
	// p50 = index 4.5 ≈ 0.9
	if idx.Thresholds.ContentSimilarity < 0.5 || idx.Thresholds.ContentSimilarity > 1.5 {
		t.Errorf("calibrated content_similarity out of expected range: %.3f", idx.Thresholds.ContentSimilarity)
	}
}

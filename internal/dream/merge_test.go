// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/core"
)

func TestFindMergeCandidates_Cluster(t *testing.T) {
	root, mm, im := setupTestMotes(t)

	// Create 4 near-identical motes with the same content
	body := "BM25 scoring uses term frequency and inverse document frequency to rank documents by relevance"
	for i := 0; i < 4; i++ {
		_, err := mm.Create("lesson", "BM25 scoring lesson", core.CreateOpts{
			Tags: []string{"bm25", "scoring"},
			Body: body,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	cfg := core.DefaultConfig().Dream
	// Use very low threshold so identical docs match
	cfg.PreScan.ContentSimilarity.Enabled = true
	cfg.PreScan.ContentSimilarity.MinScore = 0.01
	cfg.PreScan.MergeSimilarityMultiplier = 1.0

	ps := NewPreScanner(root, mm, im, cfg)
	motes, _ := mm.ReadAllParallel()
	idx, _ := im.Load()

	clusters := ps.findMergeCandidates(motes, idx)
	if len(clusters) == 0 {
		t.Fatal("expected at least 1 merge cluster from 4 identical motes")
	}
	if len(clusters[0].MoteIDs) < 3 {
		t.Errorf("expected cluster size >= 3, got %d", len(clusters[0].MoteIDs))
	}
	if clusters[0].AvgSimilarity <= 0 {
		t.Error("expected positive avg similarity")
	}
}

func TestFindMergeCandidates_NoPairs(t *testing.T) {
	root, mm, im := setupTestMotes(t)

	// Create dissimilar motes
	mm.Create("lesson", "Go concurrency patterns", core.CreateOpts{Tags: []string{"go"}, Body: "goroutines channels select"})
	mm.Create("decision", "Database migration strategy", core.CreateOpts{Tags: []string{"db"}, Body: "postgres migration rollback"})
	mm.Create("context", "Frontend architecture", core.CreateOpts{Tags: []string{"react"}, Body: "components hooks state"})

	cfg := core.DefaultConfig().Dream
	cfg.PreScan.ContentSimilarity.Enabled = true
	cfg.PreScan.ContentSimilarity.MinScore = 1.0
	cfg.PreScan.MergeSimilarityMultiplier = 5.0 // Very high threshold

	ps := NewPreScanner(root, mm, im, cfg)
	motes, _ := mm.ReadAllParallel()
	idx, _ := im.Load()

	clusters := ps.findMergeCandidates(motes, idx)
	if len(clusters) != 0 {
		t.Errorf("expected 0 merge clusters from dissimilar motes, got %d", len(clusters))
	}
}

func TestFindMergeCandidates_PairOnly(t *testing.T) {
	root, mm, im := setupTestMotes(t)

	// Only 2 similar motes — below size 3 threshold
	body := "BM25 scoring uses term frequency and inverse document frequency"
	mm.Create("lesson", "BM25 lesson A", core.CreateOpts{Tags: []string{"bm25"}, Body: body})
	mm.Create("lesson", "BM25 lesson B", core.CreateOpts{Tags: []string{"bm25"}, Body: body})
	// Add a dissimilar mote to avoid empty index
	mm.Create("decision", "Unrelated decision", core.CreateOpts{Tags: []string{"other"}, Body: "completely different content here"})

	cfg := core.DefaultConfig().Dream
	cfg.PreScan.ContentSimilarity.Enabled = true
	cfg.PreScan.ContentSimilarity.MinScore = 0.01
	cfg.PreScan.MergeSimilarityMultiplier = 1.0

	ps := NewPreScanner(root, mm, im, cfg)
	motes, _ := mm.ReadAllParallel()
	idx, _ := im.Load()

	clusters := ps.findMergeCandidates(motes, idx)
	for _, c := range clusters {
		if len(c.MoteIDs) < 3 {
			continue
		}
		t.Errorf("expected no clusters of size >= 3 from only 2 similar motes, got cluster of size %d", len(c.MoteIDs))
	}
}

func TestApplyVision_MergeSuggestion(t *testing.T) {
	root, mm, im := setupTestMotes(t)
	os.MkdirAll(filepath.Join(root, "dream"), 0755)

	// Create source motes
	m1, _ := mm.Create("lesson", "Lesson A", core.CreateOpts{Tags: []string{"scoring"}, Body: "First lesson about scoring"})
	m2, _ := mm.Create("lesson", "Lesson B", core.CreateOpts{Tags: []string{"scoring", "bm25"}, Body: "Second lesson about scoring"})
	m3, _ := mm.Create("lesson", "Lesson C", core.CreateOpts{Tags: []string{"scoring"}, Body: "Third lesson about scoring"})

	// Create an external mote linked to m1
	ext, _ := mm.Create("decision", "External decision", core.CreateOpts{Tags: []string{"architecture"}})
	mm.Link(ext.ID, "relates_to", m1.ID, im)

	// Rebuild index so edges are visible
	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	v := Vision{
		Type:        "merge_suggestion",
		Action:      "merge",
		SourceMotes: []string{m1.ID, m2.ID, m3.ID},
		Tags:        []string{"scoring", "bm25"},
		Rationale:   "Merged scoring lesson\nCombined insights about scoring from multiple lessons.",
		Severity:    "medium",
	}

	if err := ApplyVision(v, mm, im, root, nil); err != nil {
		t.Fatalf("ApplyVision merge_suggestion: %v", err)
	}

	// Verify source motes are deprecated
	for _, id := range []string{m1.ID, m2.ID, m3.ID} {
		m, err := mm.Read(id)
		if err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		if m.Status != "deprecated" {
			t.Errorf("source mote %s should be deprecated, got %s", id, m.Status)
		}
	}

	// Find the merged mote (newest mote that isn't one of the originals or ext)
	allMotes, _ := mm.ReadAllParallel()
	var merged *core.Mote
	originals := map[string]bool{m1.ID: true, m2.ID: true, m3.ID: true, ext.ID: true}
	for _, m := range allMotes {
		if !originals[m.ID] {
			merged = m
			break
		}
	}
	if merged == nil {
		t.Fatal("merged mote not found")
	}
	if merged.Title != "Merged scoring lesson" {
		t.Errorf("expected title 'Merged scoring lesson', got %q", merged.Title)
	}
	if !strings.Contains(merged.Body, "Combined insights") {
		t.Error("expected merged body to contain combined insights text")
	}
	if merged.Type != "lesson" {
		t.Errorf("expected type lesson, got %s", merged.Type)
	}
}

func TestApplyVision_MergeSuggestion_InvalidInput(t *testing.T) {
	root, mm, im := setupTestMotes(t)

	// Too few source motes
	v := Vision{
		Type:        "merge_suggestion",
		Action:      "merge",
		SourceMotes: []string{"a", "b"},
		Rationale:   "test",
	}
	if err := ApplyVision(v, mm, im, root, nil); err == nil {
		t.Error("expected error for < 3 source motes")
	}

	// Missing rationale
	v2 := Vision{
		Type:        "merge_suggestion",
		Action:      "merge",
		SourceMotes: []string{"a", "b", "c"},
		Rationale:   "",
	}
	if err := ApplyVision(v2, mm, im, root, nil); err == nil {
		t.Error("expected error for empty rationale")
	}
}

func TestScoreStructure_MergeSuggestion(t *testing.T) {
	v := Vision{
		Type:        "merge_suggestion",
		Action:      "merge",
		SourceMotes: []string{"a", "b", "c"},
		Tags:        []string{"test"},
		Rationale:   strings.Repeat("x", 150), // > 100 chars
	}
	score := scoreStructure(v)
	// Common: type(1) + action(1) + rationale(1) + rationale_length_bonus(0.5) = 3.5/3.5
	// merge_suggestion: sources>=3(1) + tags(1) + rationale>100(1) = 3/3
	// Total: 6.5/6.5 = 1.0
	if score < 0.9 {
		t.Errorf("expected high structure score for complete merge vision, got %.2f", score)
	}

	// Incomplete merge vision
	v2 := Vision{
		Type:        "merge_suggestion",
		Action:      "merge",
		SourceMotes: []string{"a"},
		Rationale:   "short",
	}
	score2 := scoreStructure(v2)
	if score2 >= score {
		t.Errorf("incomplete merge vision should score lower: %.2f >= %.2f", score2, score)
	}
}

func TestApplyVision_ActionExtraction(t *testing.T) {
	root, mm, im := setupTestMotes(t)
	os.MkdirAll(filepath.Join(root, "dream"), 0755)

	m, err := mm.Create("lesson", "Rate limit lesson", core.CreateOpts{
		Tags: []string{"api"},
		Body: "Stripe returns 200 with error body for rate limits. Always check the response body.",
	})
	if err != nil {
		t.Fatal(err)
	}

	v := Vision{
		Type:        "action_extraction",
		Action:      "add_action",
		SourceMotes: []string{m.ID},
		Rationale:   "Check response body for error field even on 2xx status codes",
		Severity:    "low",
	}

	if err := ApplyVision(v, mm, im, root, nil); err != nil {
		t.Fatalf("ApplyVision action_extraction: %v", err)
	}

	updated, err := mm.Read(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Action != v.Rationale {
		t.Errorf("action = %q, want %q", updated.Action, v.Rationale)
	}
}

func TestApplyVision_ActionExtraction_InvalidInput(t *testing.T) {
	root, mm, im := setupTestMotes(t)

	// Missing source motes
	v := Vision{
		Type:      "action_extraction",
		Action:    "add_action",
		Rationale: "some action",
	}
	if err := ApplyVision(v, mm, im, root, nil); err == nil {
		t.Error("expected error for missing source motes")
	}

	// Missing rationale
	v2 := Vision{
		Type:        "action_extraction",
		Action:      "add_action",
		SourceMotes: []string{"some-id"},
	}
	if err := ApplyVision(v2, mm, im, root, nil); err == nil {
		t.Error("expected error for empty rationale")
	}
}

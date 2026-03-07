package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIndexManager_RebuildEmpty(t *testing.T) {
	dir := t.TempDir()
	im := NewIndexManager(dir)

	if err := im.Rebuild(nil); err != nil {
		t.Fatalf("Rebuild empty: %v", err)
	}

	idx, err := im.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(idx.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(idx.Edges))
	}
	if idx.TagStats == nil {
		t.Error("TagStats should not be nil")
	}
}

func TestIndexManager_RebuildWithEdges(t *testing.T) {
	dir := t.TempDir()
	im := NewIndexManager(dir)

	motes := []*Mote{
		{ID: "p-A", Tags: []string{"oauth", "api"}, DependsOn: []string{"p-B"}, RelatesTo: []string{"p-C"}},
		{ID: "p-B", Tags: []string{"oauth", "auth"}, Blocks: []string{"p-A"}},
		{ID: "p-C", Tags: []string{"api"}, BuildsOn: []string{"p-A"}},
	}

	if err := im.Rebuild(motes); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	idx, err := im.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// 5 edges: A->B depends_on, A->C relates_to, B->A blocks, C->A builds_on, A->C built_by_ref
	if len(idx.Edges) != 5 {
		t.Errorf("expected 5 edges, got %d", len(idx.Edges))
	}

	// Tag stats
	if idx.TagStats["oauth"] != 2 {
		t.Errorf("oauth count: got %d, want 2", idx.TagStats["oauth"])
	}
	if idx.TagStats["api"] != 2 {
		t.Errorf("api count: got %d, want 2", idx.TagStats["api"])
	}
	if idx.TagStats["auth"] != 1 {
		t.Errorf("auth count: got %d, want 1", idx.TagStats["auth"])
	}

	// Outgoing from A: depends_on->B, relates_to->C, built_by_ref->C
	neighbors := idx.Neighbors("p-A", nil)
	if len(neighbors) != 3 {
		t.Errorf("A outgoing: got %d, want 3", len(neighbors))
	}

	// Filtered neighbors
	dependsOnly := idx.Neighbors("p-A", map[string]bool{"depends_on": true})
	if len(dependsOnly) != 1 {
		t.Errorf("A depends_on: got %d, want 1", len(dependsOnly))
	}
}

func TestIndexManager_LoadMissing(t *testing.T) {
	dir := t.TempDir()
	im := NewIndexManager(dir)

	idx, err := im.Load()
	if err != nil {
		t.Fatalf("Load missing should not error: %v", err)
	}
	if len(idx.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(idx.Edges))
	}
}

func TestIndexManager_LoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	im := NewIndexManager(dir)

	motes := []*Mote{
		{ID: "p-A", Tags: []string{"x", "y"}, RelatesTo: []string{"p-B"}},
		{ID: "p-B", Tags: []string{"y", "z"}, InformedBy: []string{"p-A"}},
	}

	if err := im.Rebuild(motes); err != nil {
		t.Fatal(err)
	}

	// Create a fresh IndexManager to force re-read from disk
	im2 := NewIndexManager(dir)
	idx, err := im2.Load()
	if err != nil {
		t.Fatal(err)
	}

	if len(idx.Edges) != 2 {
		t.Errorf("edges: got %d, want 2", len(idx.Edges))
	}
	if idx.TagStats["y"] != 2 {
		t.Errorf("tag y: got %d, want 2", idx.TagStats["y"])
	}
}

func TestIndexManager_AddEdge(t *testing.T) {
	dir := t.TempDir()
	im := NewIndexManager(dir)
	im.Rebuild(nil) // empty index

	edge := Edge{Source: "p-A", Target: "p-B", EdgeType: "relates_to"}
	if err := im.AddEdge(edge); err != nil {
		t.Fatal(err)
	}

	idx, _ := im.Load()
	if len(idx.Edges) != 1 {
		t.Errorf("edges: got %d, want 1", len(idx.Edges))
	}

	// Duplicate should be no-op
	if err := im.AddEdge(edge); err != nil {
		t.Fatal(err)
	}
	idx, _ = im.Load()
	if len(idx.Edges) != 1 {
		t.Errorf("after dup, edges: got %d, want 1", len(idx.Edges))
	}
}

func TestIndexManager_RemoveEdge(t *testing.T) {
	dir := t.TempDir()
	im := NewIndexManager(dir)
	im.Rebuild(nil)

	im.AddEdge(Edge{Source: "p-A", Target: "p-B", EdgeType: "relates_to"})
	im.AddEdge(Edge{Source: "p-A", Target: "p-C", EdgeType: "depends_on"})

	if err := im.RemoveEdge("p-A", "p-B", "relates_to"); err != nil {
		t.Fatal(err)
	}

	idx, _ := im.Load()
	if len(idx.Edges) != 1 {
		t.Errorf("edges: got %d, want 1", len(idx.Edges))
	}
	if idx.Edges[0].Target != "p-C" {
		t.Errorf("remaining edge target: got %s, want p-C", idx.Edges[0].Target)
	}
}

func TestEdgeIndex_Neighbors(t *testing.T) {
	dir := t.TempDir()
	im := NewIndexManager(dir)

	motes := []*Mote{
		{ID: "p-A", RelatesTo: []string{"p-B"}, DependsOn: []string{"p-C"}},
	}
	im.Rebuild(motes)
	idx, _ := im.Load()

	all := idx.Neighbors("p-A", nil)
	if len(all) != 2 {
		t.Errorf("all neighbors: got %d, want 2", len(all))
	}

	filtered := idx.Neighbors("p-A", map[string]bool{"depends_on": true})
	if len(filtered) != 1 || filtered[0].Target != "p-C" {
		t.Errorf("filtered: got %v", filtered)
	}

	// Non-existent mote
	none := idx.Neighbors("p-Z", nil)
	if len(none) != 0 {
		t.Errorf("nonexistent: got %d, want 0", len(none))
	}
}

func TestExtractBodyLinks(t *testing.T) {
	// Basic extraction
	links := ExtractBodyLinks("See [[motes-A]] and [[motes-B]].", "self")
	if len(links) != 2 || links[0] != "motes-A" || links[1] != "motes-B" {
		t.Errorf("basic: got %v", links)
	}

	// Self-link exclusion
	links = ExtractBodyLinks("Ref [[self]] and [[other]].", "self")
	if len(links) != 1 || links[0] != "other" {
		t.Errorf("self-exclusion: got %v", links)
	}

	// Dedup
	links = ExtractBodyLinks("[[dup]] then [[dup]] again.", "x")
	if len(links) != 1 || links[0] != "dup" {
		t.Errorf("dedup: got %v", links)
	}

	// No matches
	links = ExtractBodyLinks("No links here.", "x")
	if len(links) != 0 {
		t.Errorf("no matches: got %v", links)
	}
}

func TestRebuild_BodyWikiLinks(t *testing.T) {
	dir := t.TempDir()
	im := NewIndexManager(dir)

	motes := []*Mote{
		{ID: "p-A", Body: "References [[p-B]] in body."},
	}
	if err := im.Rebuild(motes); err != nil {
		t.Fatal(err)
	}
	idx, _ := im.Load()

	found := false
	for _, e := range idx.Edges {
		if e.Source == "p-A" && e.Target == "p-B" && e.EdgeType == "body_ref" {
			found = true
		}
	}
	if !found {
		t.Error("expected body_ref edge from p-A to p-B")
	}
}

func TestRebuild_BodyWikiLinks_CoexistsWithFrontmatter(t *testing.T) {
	dir := t.TempDir()
	im := NewIndexManager(dir)

	motes := []*Mote{
		{ID: "p-A", RelatesTo: []string{"p-B"}, Body: "Also see [[p-B]] in text."},
	}
	if err := im.Rebuild(motes); err != nil {
		t.Fatal(err)
	}
	idx, _ := im.Load()

	hasRelatesTo := false
	hasBodyRef := false
	for _, e := range idx.Edges {
		if e.Source == "p-A" && e.Target == "p-B" {
			if e.EdgeType == "relates_to" {
				hasRelatesTo = true
			}
			if e.EdgeType == "body_ref" {
				hasBodyRef = true
			}
		}
	}
	if !hasRelatesTo {
		t.Error("expected relates_to edge")
	}
	if !hasBodyRef {
		t.Error("expected body_ref edge")
	}
}

func TestIndexManager_FileCreated(t *testing.T) {
	dir := t.TempDir()
	im := NewIndexManager(dir)
	im.Rebuild(nil)

	path := filepath.Join(dir, "index.jsonl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("index.jsonl should exist after Rebuild")
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"fmt"
	"strings"
	"testing"

	"motes/internal/core"
)

func TestDoctorChecks_CleanGraph(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Task A", Tags: []string{"test"}},
		{Type: "task", Title: "Task B", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	idx, err := im.Load()
	if err != nil {
		t.Fatal(err)
	}

	// Link them so they aren't isolated
	motes, _ := mm.ReadAllParallel()
	if len(motes) >= 2 {
		im.AddEdge(core.Edge{Source: motes[0].ID, Target: motes[1].ID, EdgeType: "relates_to"})
		idx, _ = im.Load()
	}

	moteMap := make(map[string]*core.Mote, len(motes))
	for _, m := range motes {
		moteMap[m.ID] = m
	}

	cfg, _ := core.LoadConfig(root)
	issues := runDoctorChecks(mm, im, idx, moteMap, cfg)

	// Filter to only the new check categories (stale is expected for fresh motes with nil last_accessed)
	var relevant []doctorIssue
	for _, iss := range issues {
		if iss.Category == "orphaned_edge" || iss.Category == "circular_dep" {
			relevant = append(relevant, iss)
		}
	}
	if len(relevant) != 0 {
		t.Errorf("expected no orphaned_edge/circular_dep issues on clean graph, got %d: %+v", len(relevant), relevant)
	}
}

func TestDoctorChecks_OrphanedEdge(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Existing mote", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	motes, _ := mm.ReadAllParallel()
	existingID := motes[0].ID

	// Add edge pointing to a non-existent mote
	im.AddEdge(core.Edge{Source: existingID, Target: "ghost-mote-id", EdgeType: "depends_on"})
	idx, _ := im.Load()

	moteMap := map[string]*core.Mote{existingID: motes[0]}
	cfg, _ := core.LoadConfig(root)
	issues := runDoctorChecks(mm, im, idx, moteMap, cfg)

	var orphaned []doctorIssue
	for _, iss := range issues {
		if iss.Category == "orphaned_edge" {
			orphaned = append(orphaned, iss)
		}
	}
	if len(orphaned) == 0 {
		t.Fatal("expected orphaned_edge issue, got none")
	}
	found := false
	for _, iss := range orphaned {
		if strings.Contains(iss.Detail, "ghost-mote-id") && strings.Contains(iss.Detail, "not found") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected orphaned edge detail with 'not found' for ghost-mote-id, got: %+v", orphaned)
	}
}

func TestDoctorChecks_OrphanedEdgeInTrash(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Mote A", Tags: []string{"test"}},
		{Type: "task", Title: "Mote B (will be trashed)", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	motes, _ := mm.ReadAllParallel()
	aID := motes[0].ID
	bID := motes[1].ID

	// Trash B, then add a stale edge A -> B (simulates index not cleaned up)
	mm.Delete(bID, im)
	im.AddEdge(core.Edge{Source: aID, Target: bID, EdgeType: "depends_on"})

	// Reload — B is now in trash, not in active motes
	idx, _ := im.Load()
	activeMotes, _ := mm.ReadAllParallel()
	moteMap := make(map[string]*core.Mote, len(activeMotes))
	for _, m := range activeMotes {
		moteMap[m.ID] = m
	}

	cfg, _ := core.LoadConfig(root)
	issues := runDoctorChecks(mm, im, idx, moteMap, cfg)

	var orphaned []doctorIssue
	for _, iss := range issues {
		if iss.Category == "orphaned_edge" {
			orphaned = append(orphaned, iss)
		}
	}
	if len(orphaned) == 0 {
		t.Fatal("expected orphaned_edge issue for trashed mote, got none")
	}
	found := false
	for _, iss := range orphaned {
		if strings.Contains(iss.Detail, bID) && strings.Contains(iss.Detail, "in trash") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected orphaned edge detail with 'in trash' for %s, got: %+v", bID, orphaned)
	}
}

func TestDoctorChecks_CircularDep(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Cycle A", Tags: []string{"test"}},
		{Type: "task", Title: "Cycle B", Tags: []string{"test"}},
		{Type: "task", Title: "Cycle C", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	motes, _ := mm.ReadAllParallel()
	ids := make([]string, len(motes))
	moteMap := make(map[string]*core.Mote)
	for i, m := range motes {
		ids[i] = m.ID
		moteMap[m.ID] = m
	}

	// Create cycle: A -> B -> C -> A
	im.AddEdge(core.Edge{Source: ids[0], Target: ids[1], EdgeType: "depends_on"})
	im.AddEdge(core.Edge{Source: ids[1], Target: ids[2], EdgeType: "depends_on"})
	im.AddEdge(core.Edge{Source: ids[2], Target: ids[0], EdgeType: "depends_on"})
	idx, _ := im.Load()

	cfg, _ := core.LoadConfig(root)
	issues := runDoctorChecks(mm, im, idx, moteMap, cfg)

	var cycles []doctorIssue
	for _, iss := range issues {
		if iss.Category == "circular_dep" {
			cycles = append(cycles, iss)
		}
	}
	if len(cycles) == 0 {
		t.Fatal("expected circular_dep issue, got none")
	}
	// Detail should contain all three IDs
	detail := cycles[0].Detail
	for _, id := range ids {
		if !strings.Contains(detail, id) {
			t.Errorf("expected cycle detail to contain %s, got: %s", id, detail)
		}
	}
}

func TestDoctorChecks_DiamondNoCycle(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Diamond Top", Tags: []string{"test"}},
		{Type: "task", Title: "Diamond Left", Tags: []string{"test"}},
		{Type: "task", Title: "Diamond Right", Tags: []string{"test"}},
		{Type: "task", Title: "Diamond Bottom", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	motes, _ := mm.ReadAllParallel()
	moteMap := make(map[string]*core.Mote)
	ids := make([]string, len(motes))
	for i, m := range motes {
		ids[i] = m.ID
		moteMap[m.ID] = m
	}

	// Diamond: Top -> Left -> Bottom, Top -> Right -> Bottom
	im.AddEdge(core.Edge{Source: ids[0], Target: ids[1], EdgeType: "depends_on"})
	im.AddEdge(core.Edge{Source: ids[0], Target: ids[2], EdgeType: "depends_on"})
	im.AddEdge(core.Edge{Source: ids[1], Target: ids[3], EdgeType: "depends_on"})
	im.AddEdge(core.Edge{Source: ids[2], Target: ids[3], EdgeType: "depends_on"})
	idx, _ := im.Load()

	cfg, _ := core.LoadConfig(root)
	issues := runDoctorChecks(mm, im, idx, moteMap, cfg)

	for _, iss := range issues {
		if iss.Category == "circular_dep" {
			t.Errorf("diamond dependency should NOT be flagged as circular, got: %+v", iss)
		}
	}
}

func TestDetectDependsCycles(t *testing.T) {
	tests := []struct {
		name       string
		edges      []core.Edge
		wantCycles int
	}{
		{
			name:       "empty graph",
			edges:      nil,
			wantCycles: 0,
		},
		{
			name: "simple cycle A->B->A",
			edges: []core.Edge{
				{Source: "A", Target: "B", EdgeType: "depends_on"},
				{Source: "B", Target: "A", EdgeType: "depends_on"},
			},
			wantCycles: 1,
		},
		{
			name: "three-node cycle",
			edges: []core.Edge{
				{Source: "A", Target: "B", EdgeType: "depends_on"},
				{Source: "B", Target: "C", EdgeType: "depends_on"},
				{Source: "C", Target: "A", EdgeType: "depends_on"},
			},
			wantCycles: 1,
		},
		{
			name: "diamond — no cycle",
			edges: []core.Edge{
				{Source: "A", Target: "B", EdgeType: "depends_on"},
				{Source: "A", Target: "C", EdgeType: "depends_on"},
				{Source: "B", Target: "D", EdgeType: "depends_on"},
				{Source: "C", Target: "D", EdgeType: "depends_on"},
			},
			wantCycles: 0,
		},
		{
			name: "self-loop",
			edges: []core.Edge{
				{Source: "A", Target: "A", EdgeType: "depends_on"},
			},
			wantCycles: 1,
		},
		{
			name: "non-depends_on edges ignored",
			edges: []core.Edge{
				{Source: "A", Target: "B", EdgeType: "relates_to"},
				{Source: "B", Target: "A", EdgeType: "relates_to"},
			},
			wantCycles: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := &core.EdgeIndex{
				Edges:    tt.edges,
				TagStats: map[string]int{},
				MoteIDs:  map[string]bool{},
			}
			cycles := detectDependsCycles(idx)
			if len(cycles) != tt.wantCycles {
				t.Errorf("got %d cycles, want %d: %v", len(cycles), tt.wantCycles, cycles)
			}
		})
	}
}

func TestDoctorChecks_BloatDetection(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	// Create 20 motes so we meet the minimum threshold
	for i := range 20 {
		mm.Create("lesson", fmt.Sprintf("Lesson %d", i), core.CreateOpts{Local: true})
	}
	idx, _ := im.Load()
	motes, _ := mm.ReadAllParallel()
	moteMap := make(map[string]*core.Mote, len(motes))
	for _, m := range motes {
		moteMap[m.ID] = m
	}
	cfg, _ := core.LoadConfig(root)

	issues := runDoctorChecks(mm, im, idx, moteMap, cfg)

	var bloatIssues []doctorIssue
	for _, iss := range issues {
		if iss.Category == "bloat" {
			bloatIssues = append(bloatIssues, iss)
		}
	}
	if len(bloatIssues) != 1 {
		t.Errorf("expected 1 bloat issue, got %d", len(bloatIssues))
	}
	if len(bloatIssues) > 0 && bloatIssues[0].MoteID != "(graph)" {
		t.Errorf("bloat issue MoteID should be '(graph)', got %q", bloatIssues[0].MoteID)
	}
}

func TestDoctorChecks_NoBloatWhenSmall(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	// Only 5 motes — below the 20-mote minimum
	for i := range 5 {
		mm.Create("lesson", fmt.Sprintf("Lesson %d", i), core.CreateOpts{Local: true})
	}
	idx, _ := im.Load()
	motes, _ := mm.ReadAllParallel()
	moteMap := make(map[string]*core.Mote, len(motes))
	for _, m := range motes {
		moteMap[m.ID] = m
	}
	cfg, _ := core.LoadConfig(root)

	issues := runDoctorChecks(mm, im, idx, moteMap, cfg)
	for _, iss := range issues {
		if iss.Category == "bloat" {
			t.Errorf("bloat should not fire for nebula with < 20 motes")
		}
	}
}

// --- runDoctorAdvisories tests ---

func makeMoteMap(n int) map[string]*core.Mote {
	m := make(map[string]*core.Mote, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("mote-%02d", i)
		m[id] = &core.Mote{ID: id, Type: "task", Status: "active"}
	}
	return m
}

func TestDoctorAdvisories_EmptyGraph(t *testing.T) {
	dir := t.TempDir()
	im := core.NewIndexManager(dir)
	idx, _ := im.Load()
	cfg := core.DefaultConfig()

	advisories := runDoctorAdvisories(idx, map[string]*core.Mote{}, cfg)
	if len(advisories) != 0 {
		t.Errorf("expected no advisories for empty graph, got %d", len(advisories))
	}
}

func TestDoctorAdvisories_HighLinkDensity(t *testing.T) {
	dir := t.TempDir()
	im := core.NewIndexManager(dir)
	idx, _ := im.Load()

	// 2 motes, 20 distinct edges injected directly → avg 10 links/mote (> threshold of 8)
	// AddEdge deduplicates same source+target+type, so inject into Edges slice directly.
	moteMap := makeMoteMap(2)
	for i := 0; i < 20; i++ {
		idx.Edges = append(idx.Edges, core.Edge{
			Source:   fmt.Sprintf("mote-%02d", i%2),
			Target:   fmt.Sprintf("target-%d", i),
			EdgeType: "relates_to",
		})
	}
	_ = im

	cfg := core.DefaultConfig()
	advisories := runDoctorAdvisories(idx, moteMap, cfg)

	found := false
	for _, a := range advisories {
		if strings.Contains(a, "High link density") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected High link density advisory, got: %v", advisories)
	}
}

func TestDoctorAdvisories_NoWarningBelowLinkThreshold(t *testing.T) {
	dir := t.TempDir()
	im := core.NewIndexManager(dir)

	// 10 motes, 5 edges → avg 0.5 links/mote (below threshold)
	moteMap := makeMoteMap(10)
	_ = im.AddEdge(core.Edge{Source: "mote-00", Target: "mote-01", EdgeType: "relates_to"})
	idx, _ := im.Load()

	cfg := core.DefaultConfig()
	advisories := runDoctorAdvisories(idx, moteMap, cfg)

	for _, a := range advisories {
		if strings.Contains(a, "High link density") {
			t.Errorf("unexpected High link density advisory: %s", a)
		}
	}
}

func TestDoctorAdvisories_TagFragmentation(t *testing.T) {
	dir := t.TempDir()
	im := core.NewIndexManager(dir)

	// Build an index with high singleton rate
	moteMap := makeMoteMap(3)
	// 3 singleton tags + 1 shared tag = 75% singletons (> 50%)
	_ = im.AddEdge(core.Edge{Source: "mote-00", Target: "mote-01", EdgeType: "relates_to"})
	idx, _ := im.Load()

	// Manually inject tag stats with high singleton fraction
	idx.TagStats = map[string]int{
		"unique-a": 1,
		"unique-b": 1,
		"unique-c": 1,
		"shared":   4,
	}

	cfg := core.DefaultConfig()
	advisories := runDoctorAdvisories(idx, moteMap, cfg)

	found := false
	for _, a := range advisories {
		if strings.Contains(a, "Tag fragmentation") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Tag fragmentation advisory, got: %v", advisories)
	}
}

func TestDoctorAdvisories_NoFragmentationBelowThreshold(t *testing.T) {
	dir := t.TempDir()
	im := core.NewIndexManager(dir)
	moteMap := makeMoteMap(2)
	_ = im.AddEdge(core.Edge{Source: "mote-00", Target: "mote-01", EdgeType: "relates_to"})
	idx, _ := im.Load()

	// Only 1/5 = 20% singletons (below 50% threshold)
	idx.TagStats = map[string]int{
		"singleton": 1,
		"tag-a":     3,
		"tag-b":     5,
		"tag-c":     2,
		"tag-d":     4,
	}

	cfg := core.DefaultConfig()
	for _, a := range runDoctorAdvisories(idx, moteMap, cfg) {
		if strings.Contains(a, "Tag fragmentation") {
			t.Errorf("unexpected fragmentation advisory: %s", a)
		}
	}
}

func TestDoctorAdvisories_CustomLinkThreshold(t *testing.T) {
	dir := t.TempDir()
	im := core.NewIndexManager(dir)
	idx, _ := im.Load()

	// 2 motes, 8 distinct edges injected → avg 4 links/mote
	moteMap := makeMoteMap(2)
	for i := 0; i < 8; i++ {
		idx.Edges = append(idx.Edges, core.Edge{
			Source:   fmt.Sprintf("mote-%02d", i%2),
			Target:   fmt.Sprintf("target-%d", i),
			EdgeType: "relates_to",
		})
	}
	_ = im

	// Custom threshold of 3.0 → avg 4 links/mote should trigger advisory
	cfg := core.DefaultConfig()
	cfg.Doctor.MaxAvgLinks = 3.0
	advisories := runDoctorAdvisories(idx, moteMap, cfg)

	found := false
	for _, a := range advisories {
		if strings.Contains(a, "High link density") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected advisory with custom threshold 3.0, got: %v", advisories)
	}
}

// --- maxDependsOnDepth tests ---

func TestMaxDependsOnDepth_Empty(t *testing.T) {
	dir := t.TempDir()
	im := core.NewIndexManager(dir)
	idx, _ := im.Load()

	depth := maxDependsOnDepth(idx)
	if depth != 0 {
		t.Errorf("expected depth 0 for empty graph, got %d", depth)
	}
}

func TestMaxDependsOnDepth_LinearChain(t *testing.T) {
	dir := t.TempDir()
	im := core.NewIndexManager(dir)

	// A→B→C→D→E: chain of 4 edges, depth = 4
	chain := []string{"A", "B", "C", "D", "E"}
	for i := 0; i < len(chain)-1; i++ {
		_ = im.AddEdge(core.Edge{Source: chain[i], Target: chain[i+1], EdgeType: "depends_on"})
	}
	idx, _ := im.Load()

	depth := maxDependsOnDepth(idx)
	if depth != 4 {
		t.Errorf("expected depth 4 for A→B→C→D→E, got %d", depth)
	}
}

func TestMaxDependsOnDepth_NoDepends(t *testing.T) {
	dir := t.TempDir()
	im := core.NewIndexManager(dir)

	// Only relates_to edges — no depends_on
	_ = im.AddEdge(core.Edge{Source: "X", Target: "Y", EdgeType: "relates_to"})
	idx, _ := im.Load()

	depth := maxDependsOnDepth(idx)
	if depth != 0 {
		t.Errorf("expected depth 0 with no depends_on edges, got %d", depth)
	}
}

func TestDoctorAdvisories_DeepChain(t *testing.T) {
	dir := t.TempDir()
	im := core.NewIndexManager(dir)
	moteMap := makeMoteMap(12)

	// Build 11-node chain (depth 10, at threshold) then add 1 more → depth 11 > 10
	ids := make([]string, 12)
	for i := 0; i < 12; i++ {
		ids[i] = fmt.Sprintf("mote-%02d", i)
	}
	for i := 0; i < 11; i++ {
		_ = im.AddEdge(core.Edge{Source: ids[i], Target: ids[i+1], EdgeType: "depends_on"})
	}
	idx, _ := im.Load()

	cfg := core.DefaultConfig() // MaxChainDepth = 10
	advisories := runDoctorAdvisories(idx, moteMap, cfg)

	found := false
	for _, a := range advisories {
		if strings.Contains(a, "Deep dependency chain") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Deep dependency chain advisory, got: %v", advisories)
	}
}

func TestDoctorAdvisories_ChainAtThreshold_NoWarning(t *testing.T) {
	dir := t.TempDir()
	im := core.NewIndexManager(dir)
	moteMap := makeMoteMap(11)

	// Build chain of depth exactly 10 (10 edges, 11 nodes) — at threshold, not over
	ids := make([]string, 11)
	for i := 0; i < 11; i++ {
		ids[i] = fmt.Sprintf("mote-%02d", i)
	}
	for i := 0; i < 10; i++ {
		_ = im.AddEdge(core.Edge{Source: ids[i], Target: ids[i+1], EdgeType: "depends_on"})
	}
	idx, _ := im.Load()

	cfg := core.DefaultConfig() // MaxChainDepth = 10
	for _, a := range runDoctorAdvisories(idx, moteMap, cfg) {
		if strings.Contains(a, "Deep dependency chain") {
			t.Errorf("advisory should not fire at exactly the threshold: %s", a)
		}
	}
}

func TestDoctorChecks_ConceptRefNotOrphaned(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "context", Title: "Context mote", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	motes, _ := mm.ReadAllParallel()
	existingID := motes[0].ID

	// Add a concept_ref edge (e.g. from body text [[some-concept]]) — target is a concept term, not a mote ID.
	im.AddEdge(core.Edge{Source: existingID, Target: "some-concept", EdgeType: "concept_ref"})
	idx, _ := im.Load()

	moteMap := map[string]*core.Mote{existingID: motes[0]}
	cfg, _ := core.LoadConfig(root)
	issues := runDoctorChecks(mm, im, idx, moteMap, cfg)

	for _, iss := range issues {
		if iss.Category == "orphaned_edge" && strings.Contains(iss.Detail, "some-concept") {
			t.Errorf("concept_ref edge should not be reported as orphaned_edge, got: %+v", iss)
		}
	}
}

func TestDoctorChecks_CrossProjectRef(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "context", Title: "Cross-ref mote", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	motes, _ := mm.ReadAllParallel()
	m := motes[0]

	// Simulate a cross-project informed_by ref (prefix "otherproject" not in loaded set)
	m.InformedBy = []string{"otherproject-Tabcdef12345"}
	sourceData, _ := core.SerializeMote(m)
	_ = core.AtomicWrite(m.FilePath, sourceData, 0644)

	motes, _ = mm.ReadAllParallel()
	moteMap := make(map[string]*core.Mote)
	for _, mo := range motes {
		moteMap[mo.ID] = mo
	}
	idx, _ := im.Load()
	cfg, _ := core.LoadConfig(root)
	issues := runDoctorChecks(mm, im, idx, moteMap, cfg)

	var crossRefs, brokenLinks []doctorIssue
	for _, iss := range issues {
		switch iss.Category {
		case "cross_project_ref":
			crossRefs = append(crossRefs, iss)
		case "broken_link":
			if strings.Contains(iss.Detail, "otherproject-Tabcdef12345") {
				brokenLinks = append(brokenLinks, iss)
			}
		}
	}

	if len(crossRefs) == 0 {
		t.Error("expected cross_project_ref issue for unknown-prefix ref, got none")
	}
	if len(brokenLinks) > 0 {
		t.Errorf("cross-project ref should not be reported as broken_link, got: %+v", brokenLinks)
	}
}

func TestDoctorChecks_KnownPrefixBrokenLink(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "context", Title: "Ref mote", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)

	motes, _ := mm.ReadAllParallel()
	m := motes[0]
	// Derive the project prefix from the loaded mote's ID (e.g. "test12345")
	prefix := extractMotePrefix(m.ID)

	// Add a broken link to a mote in the SAME project (known prefix) that doesn't exist
	ghostID := prefix + "-Tdeadbeefcafe"
	m.DependsOn = []string{ghostID}
	sourceData, _ := core.SerializeMote(m)
	_ = core.AtomicWrite(m.FilePath, sourceData, 0644)

	motes, _ = mm.ReadAllParallel()
	moteMap := make(map[string]*core.Mote)
	for _, mo := range motes {
		moteMap[mo.ID] = mo
	}
	idx, _ := im.Load()
	cfg, _ := core.LoadConfig(root)
	issues := runDoctorChecks(mm, im, idx, moteMap, cfg)

	var broken []doctorIssue
	for _, iss := range issues {
		if iss.Category == "broken_link" && strings.Contains(iss.Detail, ghostID) {
			broken = append(broken, iss)
		}
	}
	if len(broken) == 0 {
		t.Errorf("expected broken_link for same-project missing mote %s, got none", ghostID)
	}
}

package main

import (
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

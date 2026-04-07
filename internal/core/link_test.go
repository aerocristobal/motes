// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"testing"
)

func setupTestLink(t *testing.T) (string, *MoteManager, *IndexManager) {
	t.Helper()
	root, mm := setupTestMemory(t)
	im := NewIndexManager(root)
	im.Load()
	return root, mm, im
}

func TestLink_RelatesTo_Symmetric(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("task", "A", CreateOpts{})
	b, _ := mm.Create("task", "B", CreateOpts{})

	if err := mm.Link(a.ID, "relates_to", b.ID, im); err != nil {
		t.Fatal(err)
	}

	aRead, _ := mm.Read(a.ID)
	bRead, _ := mm.Read(b.ID)

	if !sliceContains(aRead.RelatesTo, b.ID) {
		t.Errorf("A.RelatesTo should contain B")
	}
	if !sliceContains(bRead.RelatesTo, a.ID) {
		t.Errorf("B.RelatesTo should contain A (symmetric)")
	}

	idx, _ := im.Load()
	if len(idx.Edges) != 2 {
		t.Errorf("expected 2 index edges, got %d", len(idx.Edges))
	}
}

func TestLink_DependsOn_InversePair(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("task", "A", CreateOpts{})
	b, _ := mm.Create("task", "B", CreateOpts{})

	if err := mm.Link(a.ID, "depends_on", b.ID, im); err != nil {
		t.Fatal(err)
	}

	aRead, _ := mm.Read(a.ID)
	bRead, _ := mm.Read(b.ID)

	if !sliceContains(aRead.DependsOn, b.ID) {
		t.Errorf("A.DependsOn should contain B")
	}
	if !sliceContains(bRead.Blocks, a.ID) {
		t.Errorf("B.Blocks should contain A (inverse)")
	}

	idx, _ := im.Load()
	if len(idx.Edges) != 2 {
		t.Errorf("expected 2 index edges, got %d", len(idx.Edges))
	}
}

func TestLink_Blocks_InversePair(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("task", "A", CreateOpts{})
	b, _ := mm.Create("task", "B", CreateOpts{})

	if err := mm.Link(a.ID, "blocks", b.ID, im); err != nil {
		t.Fatal(err)
	}

	aRead, _ := mm.Read(a.ID)
	bRead, _ := mm.Read(b.ID)

	if !sliceContains(aRead.Blocks, b.ID) {
		t.Errorf("A.Blocks should contain B")
	}
	if !sliceContains(bRead.DependsOn, a.ID) {
		t.Errorf("B.DependsOn should contain A (inverse)")
	}
}

func TestLink_Contradicts_Symmetric(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("decision", "A", CreateOpts{})
	b, _ := mm.Create("decision", "B", CreateOpts{})

	if err := mm.Link(a.ID, "contradicts", b.ID, im); err != nil {
		t.Fatal(err)
	}

	aRead, _ := mm.Read(a.ID)
	bRead, _ := mm.Read(b.ID)

	if !sliceContains(aRead.Contradicts, b.ID) {
		t.Errorf("A.Contradicts should contain B")
	}
	if !sliceContains(bRead.Contradicts, a.ID) {
		t.Errorf("B.Contradicts should contain A (symmetric)")
	}
}

func TestLink_BuildsOn_IndexReverse(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("lesson", "A", CreateOpts{})
	b, _ := mm.Create("lesson", "B", CreateOpts{})

	if err := mm.Link(a.ID, "builds_on", b.ID, im); err != nil {
		t.Fatal(err)
	}

	aRead, _ := mm.Read(a.ID)
	bRead, _ := mm.Read(b.ID)

	if !sliceContains(aRead.BuildsOn, b.ID) {
		t.Errorf("A.BuildsOn should contain B")
	}
	if len(bRead.BuildsOn) != 0 {
		t.Errorf("B.BuildsOn should be empty (directional), got %v", bRead.BuildsOn)
	}

	// Index should have forward + built_by_ref reverse
	idx, _ := im.Load()
	if len(idx.Edges) != 2 {
		t.Errorf("expected 2 index edges (forward + built_by_ref), got %d", len(idx.Edges))
	}
	hasReverse := false
	for _, e := range idx.Edges {
		if e.Source == b.ID && e.Target == a.ID && e.EdgeType == "built_by_ref" {
			hasReverse = true
		}
	}
	if !hasReverse {
		t.Error("index should have built_by_ref reverse edge")
	}
}

func TestLink_Supersedes_AutoDeprecate(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("decision", "New way", CreateOpts{})
	b, _ := mm.Create("decision", "Old way", CreateOpts{})

	if err := mm.Link(a.ID, "supersedes", b.ID, im); err != nil {
		t.Fatal(err)
	}

	aRead, _ := mm.Read(a.ID)
	bRead, _ := mm.Read(b.ID)

	if !sliceContains(aRead.Supersedes, b.ID) {
		t.Errorf("A.Supersedes should contain B")
	}
	if bRead.Status != "deprecated" {
		t.Errorf("B.Status should be deprecated, got %q", bRead.Status)
	}
	if bRead.DeprecatedBy != a.ID {
		t.Errorf("B.DeprecatedBy should be %q, got %q", a.ID, bRead.DeprecatedBy)
	}
}

func TestLink_CausedBy_Directional(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("lesson", "A", CreateOpts{})
	b, _ := mm.Create("task", "B", CreateOpts{})

	if err := mm.Link(a.ID, "caused_by", b.ID, im); err != nil {
		t.Fatal(err)
	}

	aRead, _ := mm.Read(a.ID)
	bRead, _ := mm.Read(b.ID)

	if !sliceContains(aRead.CausedBy, b.ID) {
		t.Errorf("A.CausedBy should contain B")
	}
	// B should have no links back to A
	if len(bRead.CausedBy) != 0 || len(bRead.InformedBy) != 0 {
		t.Errorf("B should have no reverse links")
	}

	idx, _ := im.Load()
	if len(idx.Edges) != 1 {
		t.Errorf("expected 1 index edge (directional only), got %d", len(idx.Edges))
	}
}

func TestLink_InformedBy_Directional(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("decision", "A", CreateOpts{})
	b, _ := mm.Create("lesson", "B", CreateOpts{})

	if err := mm.Link(a.ID, "informed_by", b.ID, im); err != nil {
		t.Fatal(err)
	}

	aRead, _ := mm.Read(a.ID)
	if !sliceContains(aRead.InformedBy, b.ID) {
		t.Errorf("A.InformedBy should contain B")
	}

	idx, _ := im.Load()
	if len(idx.Edges) != 1 {
		t.Errorf("expected 1 index edge, got %d", len(idx.Edges))
	}
}

func TestLink_DuplicateIsIdempotent(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("task", "A", CreateOpts{})
	b, _ := mm.Create("task", "B", CreateOpts{})

	mm.Link(a.ID, "relates_to", b.ID, im)
	mm.Link(a.ID, "relates_to", b.ID, im)

	aRead, _ := mm.Read(a.ID)
	count := 0
	for _, id := range aRead.RelatesTo {
		if id == b.ID {
			count++
		}
	}
	if count != 1 {
		t.Errorf("B should appear exactly once in A.RelatesTo, got %d", count)
	}
}

func TestLink_InvalidType(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("task", "A", CreateOpts{})
	b, _ := mm.Create("task", "B", CreateOpts{})

	err := mm.Link(a.ID, "foo_bar", b.ID, im)
	if err == nil {
		t.Fatal("expected error for invalid link type")
	}
}

func TestLink_SelfLink(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("task", "A", CreateOpts{})

	err := mm.Link(a.ID, "relates_to", a.ID, im)
	if err == nil {
		t.Fatal("expected error for self-link")
	}
}

func TestUnlink_RelatesTo_Symmetric(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("task", "A", CreateOpts{})
	b, _ := mm.Create("task", "B", CreateOpts{})

	mm.Link(a.ID, "relates_to", b.ID, im)

	if err := mm.Unlink(a.ID, "relates_to", b.ID, im); err != nil {
		t.Fatal(err)
	}

	aRead, _ := mm.Read(a.ID)
	bRead, _ := mm.Read(b.ID)

	if len(aRead.RelatesTo) != 0 {
		t.Errorf("A.RelatesTo should be empty, got %v", aRead.RelatesTo)
	}
	if len(bRead.RelatesTo) != 0 {
		t.Errorf("B.RelatesTo should be empty (symmetric unlink), got %v", bRead.RelatesTo)
	}

	idx, _ := im.Load()
	if len(idx.Edges) != 0 {
		t.Errorf("expected 0 index edges after unlink, got %d", len(idx.Edges))
	}
}

func TestUnlink_DependsOn_InversePair(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("task", "A", CreateOpts{})
	b, _ := mm.Create("task", "B", CreateOpts{})

	mm.Link(a.ID, "depends_on", b.ID, im)

	if err := mm.Unlink(a.ID, "depends_on", b.ID, im); err != nil {
		t.Fatal(err)
	}

	aRead, _ := mm.Read(a.ID)
	bRead, _ := mm.Read(b.ID)

	if len(aRead.DependsOn) != 0 {
		t.Errorf("A.DependsOn should be empty, got %v", aRead.DependsOn)
	}
	if len(bRead.Blocks) != 0 {
		t.Errorf("B.Blocks should be empty (inverse unlink), got %v", bRead.Blocks)
	}
}

func TestUnlink_DirectionalOnly(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("lesson", "A", CreateOpts{})
	b, _ := mm.Create("task", "B", CreateOpts{})

	mm.Link(a.ID, "caused_by", b.ID, im)

	if err := mm.Unlink(a.ID, "caused_by", b.ID, im); err != nil {
		t.Fatal(err)
	}

	aRead, _ := mm.Read(a.ID)
	if len(aRead.CausedBy) != 0 {
		t.Errorf("A.CausedBy should be empty, got %v", aRead.CausedBy)
	}
}

func TestListReady_NoBlockers(t *testing.T) {
	_, mm, _ := setupTestLink(t)

	mm.Create("task", "Ready task", CreateOpts{})

	motes, err := mm.List(ListFilters{Ready: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(motes) != 1 {
		t.Errorf("expected 1 ready task, got %d", len(motes))
	}
}

func TestListReady_AllBlockersCompleted(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("task", "Depends on B and C", CreateOpts{})
	b, _ := mm.Create("task", "B", CreateOpts{})
	c, _ := mm.Create("task", "C", CreateOpts{})

	mm.Link(a.ID, "depends_on", b.ID, im)
	mm.Link(a.ID, "depends_on", c.ID, im)
	mm.Update(b.ID, UpdateOpts{Status: StringPtr("completed")})
	mm.Update(c.ID, UpdateOpts{Status: StringPtr("completed")})

	motes, err := mm.List(ListFilters{Ready: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(motes) != 1 {
		t.Errorf("expected 1 ready task, got %d", len(motes))
	}
	if len(motes) > 0 && motes[0].ID != a.ID {
		t.Errorf("expected task A, got %s", motes[0].ID)
	}
}

func TestListReady_SomeBlockersActive(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("task", "Blocked", CreateOpts{})
	b, _ := mm.Create("task", "Done", CreateOpts{})
	c, _ := mm.Create("task", "Still active", CreateOpts{})

	mm.Link(a.ID, "depends_on", b.ID, im)
	mm.Link(a.ID, "depends_on", c.ID, im)
	mm.Update(b.ID, UpdateOpts{Status: StringPtr("completed")})
	// c remains active

	motes, err := mm.List(ListFilters{Ready: true})
	if err != nil {
		t.Fatal(err)
	}
	// A is blocked by C (active), B and C themselves have no deps so B is completed (excluded) and C is active with no deps (ready)
	for _, m := range motes {
		if m.ID == a.ID {
			t.Errorf("task A should NOT be ready (blocked by active C)")
		}
	}
}

func TestListReady_OnlyActiveTasks(t *testing.T) {
	_, mm, _ := setupTestLink(t)

	a, _ := mm.Create("task", "Completed task", CreateOpts{})
	mm.Update(a.ID, UpdateOpts{Status: StringPtr("completed")})
	mm.Create("decision", "Not a task", CreateOpts{})

	motes, err := mm.List(ListFilters{Ready: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(motes) != 0 {
		t.Errorf("expected 0 ready tasks (completed and non-task excluded), got %d", len(motes))
	}
}

func TestRebuild_BuiltByRef(t *testing.T) {
	root, mm, _ := setupTestLink(t)

	a, _ := mm.Create("lesson", "Builds on B", CreateOpts{})
	b, _ := mm.Create("lesson", "B", CreateOpts{})

	// Manually set builds_on via Update doesn't exist yet, so use Link
	im := NewIndexManager(root)
	im.Load()
	mm.Link(a.ID, "builds_on", b.ID, im)

	// Now rebuild from frontmatter
	im2 := NewIndexManager(root)
	motes, _ := mm.ReadAllParallel()
	im2.Rebuild(motes)

	idx, _ := im2.Load()
	hasForward := false
	hasReverse := false
	for _, e := range idx.Edges {
		if e.Source == a.ID && e.Target == b.ID && e.EdgeType == "builds_on" {
			hasForward = true
		}
		if e.Source == b.ID && e.Target == a.ID && e.EdgeType == "built_by_ref" {
			hasReverse = true
		}
	}
	if !hasForward {
		t.Error("missing forward builds_on edge")
	}
	if !hasReverse {
		t.Error("missing built_by_ref reverse edge from Rebuild")
	}
}

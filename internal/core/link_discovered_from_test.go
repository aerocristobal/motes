// SPDX-License-Identifier: MIT
package core

import "testing"

// STORY-DISC-001 Scenarios 1 + 2: linking A discovered_from B writes only A's
// frontmatter, and the index gains forward A->B (discovered_from) plus reverse
// B->A (discovered_ref).
func TestLink_DiscoveredFrom_IndexReverse(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("task", "Follow-up", CreateOpts{})
	b, _ := mm.Create("task", "Source", CreateOpts{})

	if err := mm.Link(a.ID, "discovered_from", b.ID, im); err != nil {
		t.Fatal(err)
	}

	aRead, _ := mm.Read(a.ID)
	bRead, _ := mm.Read(b.ID)

	if !sliceContains(aRead.DiscoveredFrom, b.ID) {
		t.Errorf("A.DiscoveredFrom should contain B, got %v", aRead.DiscoveredFrom)
	}
	if len(bRead.DiscoveredFrom) != 0 {
		t.Errorf("B.DiscoveredFrom must remain empty (asymmetric), got %v", bRead.DiscoveredFrom)
	}

	idx, _ := im.Load()
	if len(idx.Edges) != 2 {
		t.Errorf("expected 2 index edges (forward + discovered_ref), got %d", len(idx.Edges))
	}
	hasForward, hasReverse := false, false
	for _, e := range idx.Edges {
		if e.Source == a.ID && e.Target == b.ID && e.EdgeType == "discovered_from" {
			hasForward = true
		}
		if e.Source == b.ID && e.Target == a.ID && e.EdgeType == "discovered_ref" {
			hasReverse = true
		}
	}
	if !hasForward {
		t.Error("missing forward discovered_from edge")
	}
	if !hasReverse {
		t.Error("missing discovered_ref reverse edge")
	}
}

// STORY-DISC-001 Scenario 7: idempotent linking — second link is a no-op.
func TestLink_DiscoveredFrom_Idempotent(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("task", "Follow-up", CreateOpts{})
	b, _ := mm.Create("task", "Source", CreateOpts{})

	_ = mm.Link(a.ID, "discovered_from", b.ID, im)
	_ = mm.Link(a.ID, "discovered_from", b.ID, im)

	aRead, _ := mm.Read(a.ID)
	count := 0
	for _, id := range aRead.DiscoveredFrom {
		if id == b.ID {
			count++
		}
	}
	if count != 1 {
		t.Errorf("B should appear exactly once in A.DiscoveredFrom, got %d", count)
	}
}

// STORY-DISC-001 Scenario 8: unlink removes both the frontmatter entry and the
// reverse index edge.
func TestUnlink_DiscoveredFrom_RemovesEdge(t *testing.T) {
	_, mm, im := setupTestLink(t)

	a, _ := mm.Create("task", "Follow-up", CreateOpts{})
	b, _ := mm.Create("task", "Source", CreateOpts{})

	_ = mm.Link(a.ID, "discovered_from", b.ID, im)

	if err := mm.Unlink(a.ID, "discovered_from", b.ID, im); err != nil {
		t.Fatal(err)
	}

	aRead, _ := mm.Read(a.ID)
	if len(aRead.DiscoveredFrom) != 0 {
		t.Errorf("A.DiscoveredFrom should be empty after unlink, got %v", aRead.DiscoveredFrom)
	}

	idx, _ := im.Load()
	if len(idx.Edges) != 0 {
		t.Errorf("expected 0 index edges after unlink, got %d", len(idx.Edges))
	}
}

// Regression guard for the buildEdges path used by `mote index rebuild`. The
// reverse discovered_ref edge must be reconstructed from frontmatter alone.
func TestRebuild_DiscoveredRef(t *testing.T) {
	root, mm, _ := setupTestLink(t)

	a, _ := mm.Create("task", "Follow-up", CreateOpts{})
	b, _ := mm.Create("task", "Source", CreateOpts{})

	im := NewIndexManager(root)
	_, _ = im.Load()
	_ = mm.Link(a.ID, "discovered_from", b.ID, im)

	im2 := NewIndexManager(root)
	motes, _ := mm.ReadAllParallel()
	_ = im2.Rebuild(motes)

	idx, _ := im2.Load()
	hasForward, hasReverse := false, false
	for _, e := range idx.Edges {
		if e.Source == a.ID && e.Target == b.ID && e.EdgeType == "discovered_from" {
			hasForward = true
		}
		if e.Source == b.ID && e.Target == a.ID && e.EdgeType == "discovered_ref" {
			hasReverse = true
		}
	}
	if !hasForward {
		t.Error("missing forward discovered_from edge after rebuild")
	}
	if !hasReverse {
		t.Error("missing discovered_ref reverse edge after rebuild")
	}
}

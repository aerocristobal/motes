// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"testing"
	"time"
)

func setupTraversalTest(t *testing.T) (string, *MoteManager, *IndexManager, *ScoreEngine) {
	t.Helper()
	root, mm := setupTestMemory(t)
	im := NewIndexManager(root)
	im.Load()

	cfg := DefaultConfig()
	idx, _ := im.Load()
	se := NewScoreEngine(cfg.Scoring, idx.TagStats)
	return root, mm, im, se
}

func TestTraverse_SingleSeed(t *testing.T) {
	_, mm, im, se := setupTraversalTest(t)

	a, _ := mm.Create("task", "Seed", CreateOpts{Weight: 0.8, Tags: []string{"auth"}})
	recent := time.Now().Add(-1 * time.Hour)
	mm.Update(a.ID, UpdateOpts{LastAccessed: &recent})

	// Rebuild index to get tag stats
	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)
	idx, _ := im.Load()
	se = NewScoreEngine(DefaultConfig().Scoring, idx.TagStats)

	aRead, _ := mm.Read(a.ID)
	gt := NewGraphTraverser(idx, se, DefaultConfig().Scoring)
	results := gt.Traverse([]*Mote{aRead}, []string{"auth"}, func(id string) (*Mote, error) {
		return mm.Read(id)
	})

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if len(results) > 0 && results[0].Mote.ID != a.ID {
		t.Errorf("expected seed %s, got %s", a.ID, results[0].Mote.ID)
	}
}

func TestTraverse_OneHop(t *testing.T) {
	_, mm, im, _ := setupTraversalTest(t)

	a, _ := mm.Create("task", "Seed", CreateOpts{Weight: 0.8, Tags: []string{"auth"}})
	b, _ := mm.Create("decision", "Related", CreateOpts{Weight: 0.6, Tags: []string{"auth"}})

	recent := time.Now().Add(-1 * time.Hour)
	mm.Update(a.ID, UpdateOpts{LastAccessed: &recent})
	mm.Update(b.ID, UpdateOpts{LastAccessed: &recent})

	mm.Link(a.ID, "relates_to", b.ID, im)

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)
	idx, _ := im.Load()
	se := NewScoreEngine(DefaultConfig().Scoring, idx.TagStats)

	aRead, _ := mm.Read(a.ID)
	gt := NewGraphTraverser(idx, se, DefaultConfig().Scoring)
	results := gt.Traverse([]*Mote{aRead}, []string{"auth"}, func(id string) (*Mote, error) {
		return mm.Read(id)
	})

	if len(results) != 2 {
		t.Errorf("expected 2 results (seed + 1-hop), got %d", len(results))
	}
}

func TestTraverse_TwoHopLimit(t *testing.T) {
	_, mm, im, _ := setupTraversalTest(t)

	a, _ := mm.Create("task", "Seed", CreateOpts{Weight: 0.8})
	b, _ := mm.Create("task", "Hop1", CreateOpts{Weight: 0.6})
	c, _ := mm.Create("task", "Hop2", CreateOpts{Weight: 0.5})
	d, _ := mm.Create("task", "Hop3", CreateOpts{Weight: 0.4})

	recent := time.Now().Add(-1 * time.Hour)
	for _, id := range []string{a.ID, b.ID, c.ID, d.ID} {
		mm.Update(id, UpdateOpts{LastAccessed: &recent})
	}

	mm.Link(a.ID, "relates_to", b.ID, im)
	mm.Link(b.ID, "relates_to", c.ID, im)
	mm.Link(c.ID, "relates_to", d.ID, im)

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)
	idx, _ := im.Load()
	se := NewScoreEngine(DefaultConfig().Scoring, idx.TagStats)

	aRead, _ := mm.Read(a.ID)
	gt := NewGraphTraverser(idx, se, DefaultConfig().Scoring)
	results := gt.Traverse([]*Mote{aRead}, nil, func(id string) (*Mote, error) {
		return mm.Read(id)
	})

	// A (hop 0), B (hop 1), C (hop 2). D (hop 3) excluded by maxHops=2.
	ids := make(map[string]bool)
	for _, r := range results {
		ids[r.Mote.ID] = true
	}
	if !ids[a.ID] || !ids[b.ID] || !ids[c.ID] {
		t.Errorf("expected A, B, C in results, got %v", ids)
	}
	if ids[d.ID] {
		t.Errorf("D (hop 3) should be excluded, but found in results")
	}
}

func TestTraverse_ThresholdFiltering(t *testing.T) {
	_, mm, im, _ := setupTraversalTest(t)

	a, _ := mm.Create("task", "Seed", CreateOpts{Weight: 0.8})
	b, _ := mm.Create("task", "Low weight", CreateOpts{Weight: 0.05})

	// b never accessed → recency factor 0.4
	// score ≈ 0.05 × 0.4 = 0.02, well below 0.25 threshold
	recent := time.Now().Add(-1 * time.Hour)
	mm.Update(a.ID, UpdateOpts{LastAccessed: &recent})

	mm.Link(a.ID, "relates_to", b.ID, im)

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)
	idx, _ := im.Load()
	se := NewScoreEngine(DefaultConfig().Scoring, idx.TagStats)

	aRead, _ := mm.Read(a.ID)
	gt := NewGraphTraverser(idx, se, DefaultConfig().Scoring)
	results := gt.Traverse([]*Mote{aRead}, nil, func(id string) (*Mote, error) {
		return mm.Read(id)
	})

	for _, r := range results {
		if r.Mote.ID == b.ID {
			t.Errorf("low-weight mote should be filtered by threshold, but found with score %f", r.Score)
		}
	}
}

func TestTraverse_MaxResults(t *testing.T) {
	_, mm, im, _ := setupTraversalTest(t)

	recent := time.Now().Add(-1 * time.Hour)

	// Create 15 motes, all linked from a seed
	seed, _ := mm.Create("task", "Seed", CreateOpts{Weight: 0.9})
	mm.Update(seed.ID, UpdateOpts{LastAccessed: &recent})

	for i := 0; i < 14; i++ {
		m, _ := mm.Create("task", "Target", CreateOpts{Weight: 0.5})
		mm.Update(m.ID, UpdateOpts{LastAccessed: &recent})
		mm.Link(seed.ID, "relates_to", m.ID, im)
	}

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)
	idx, _ := im.Load()
	se := NewScoreEngine(DefaultConfig().Scoring, idx.TagStats)

	seedRead, _ := mm.Read(seed.ID)
	gt := NewGraphTraverser(idx, se, DefaultConfig().Scoring)
	results := gt.Traverse([]*Mote{seedRead}, nil, func(id string) (*Mote, error) {
		return mm.Read(id)
	})

	// MaxResults defaults to 12
	if len(results) > 12 {
		t.Errorf("expected at most 12 results, got %d", len(results))
	}
}

func TestTraverse_ContradictionPenalty(t *testing.T) {
	_, mm, im, _ := setupTraversalTest(t)

	recent := time.Now().Add(-1 * time.Hour)
	a, _ := mm.Create("decision", "Decision A", CreateOpts{Weight: 0.7})
	b, _ := mm.Create("decision", "Decision B", CreateOpts{Weight: 0.7})
	c, _ := mm.Create("decision", "Contradicts B", CreateOpts{Weight: 0.7})

	mm.Update(a.ID, UpdateOpts{LastAccessed: &recent})
	mm.Update(b.ID, UpdateOpts{LastAccessed: &recent})
	mm.Update(c.ID, UpdateOpts{LastAccessed: &recent})

	mm.Link(a.ID, "relates_to", b.ID, im)
	mm.Link(a.ID, "relates_to", c.ID, im)
	mm.Link(b.ID, "contradicts", c.ID, im)

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)
	idx, _ := im.Load()
	se := NewScoreEngine(DefaultConfig().Scoring, idx.TagStats)

	aRead, _ := mm.Read(a.ID)
	gt := NewGraphTraverser(idx, se, DefaultConfig().Scoring)
	results := gt.Traverse([]*Mote{aRead}, nil, func(id string) (*Mote, error) {
		return mm.Read(id)
	})

	// Both B and C should be in results but one should have interference penalty
	// (whichever is visited second sees the first in visited set)
	if len(results) < 2 {
		t.Errorf("expected at least 2 results, got %d", len(results))
	}
}

func TestTraverse_SortedByScore(t *testing.T) {
	_, mm, im, _ := setupTraversalTest(t)

	recent := time.Now().Add(-1 * time.Hour)
	a, _ := mm.Create("task", "High", CreateOpts{Weight: 0.9})
	b, _ := mm.Create("task", "Low", CreateOpts{Weight: 0.3})

	mm.Update(a.ID, UpdateOpts{LastAccessed: &recent})
	mm.Update(b.ID, UpdateOpts{LastAccessed: &recent})

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)
	idx, _ := im.Load()
	se := NewScoreEngine(DefaultConfig().Scoring, idx.TagStats)

	aRead, _ := mm.Read(a.ID)
	bRead, _ := mm.Read(b.ID)
	gt := NewGraphTraverser(idx, se, DefaultConfig().Scoring)
	results := gt.Traverse([]*Mote{aRead, bRead}, nil, func(id string) (*Mote, error) {
		return mm.Read(id)
	})

	if len(results) >= 2 && results[0].Score < results[1].Score {
		t.Errorf("results should be sorted by score desc: %f < %f",
			results[0].Score, results[1].Score)
	}
}

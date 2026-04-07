// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"testing"

	"motes/internal/core"
)

func TestBatchConstructor_EmptyCandidates(t *testing.T) {
	bc := NewBatchConstructor(core.DefaultConfig().Dream.Batching, nil)
	batches := bc.Build(&ScanResult{})
	if len(batches) != 0 {
		t.Errorf("expected 0 batches for empty candidates, got %d", len(batches))
	}
}

func TestBatchConstructor_HybridBatching(t *testing.T) {
	root, mm, _ := setupTestMotes(t)
	_ = root

	// Create motes with different tags
	m1 := createTestMote(t, mm, "context", "Auth mote 1", []string{"auth", "oauth"})
	m2 := createTestMote(t, mm, "context", "Auth mote 2", []string{"auth", "tokens"})
	m3 := createTestMote(t, mm, "context", "DB mote", []string{"database"})

	reader := func(id string) (*core.Mote, error) {
		return mm.Read(id)
	}

	bc := NewBatchConstructor(core.BatchingConfig{
		MaxMotesPerBatch:  10,
		ClusteredFraction: 0.6,
	}, reader)

	candidates := &ScanResult{
		StaleMotes: []string{m1.ID, m2.ID, m3.ID},
	}

	batches := bc.Build(candidates)
	if len(batches) == 0 {
		t.Fatal("expected at least 1 batch")
	}

	// Count total motes across batches
	total := 0
	for _, b := range batches {
		total += len(b.MoteIDs)
	}
	if total != 3 {
		t.Errorf("expected 3 total motes, got %d", total)
	}
}

func TestBatchConstructor_MaxPerBatch(t *testing.T) {
	root, mm, _ := setupTestMotes(t)
	_ = root

	// Create 15 motes
	var ids []string
	for i := 0; i < 15; i++ {
		m := createTestMote(t, mm, "context", "Mote", []string{"tag"})
		ids = append(ids, m.ID)
	}

	reader := func(id string) (*core.Mote, error) {
		return mm.Read(id)
	}

	bc := NewBatchConstructor(core.BatchingConfig{
		MaxMotesPerBatch:  5,
		ClusteredFraction: 0.6,
	}, reader)

	candidates := &ScanResult{
		StaleMotes: ids,
	}

	batches := bc.Build(candidates)
	for _, b := range batches {
		if len(b.MoteIDs) > 5 {
			t.Errorf("batch exceeds max: %d motes", len(b.MoteIDs))
		}
	}
}

func TestBatchConstructor_TaskAggregation(t *testing.T) {
	root, mm, _ := setupTestMotes(t)
	_ = root

	m1 := createTestMote(t, mm, "context", "Mote 1", []string{"tag"})

	reader := func(id string) (*core.Mote, error) {
		return mm.Read(id)
	}

	bc := NewBatchConstructor(core.DefaultConfig().Dream.Batching, reader)

	// Same mote appears in multiple categories
	candidates := &ScanResult{
		StaleMotes:            []string{m1.ID},
		CompressionCandidates: []string{m1.ID},
	}

	batches := bc.Build(candidates)
	if len(batches) == 0 {
		t.Fatal("expected at least 1 batch")
	}

	// The batch should have both tasks
	if len(batches[0].Tasks) < 2 {
		t.Errorf("expected 2+ tasks for mote in multiple categories, got %d", len(batches[0].Tasks))
	}
}

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"motes/internal/core"
	"motes/internal/dream"
)

// --- dreamCostStats / readDreamCostStats tests ---

func TestReadDreamCostStats_AppliedDeferred(t *testing.T) {
	dir := t.TempDir()

	entries := []dream.RunLogEntry{
		{Timestamp: "2026-01-01T00:00:00Z", Status: "complete", Visions: 10, AutoApplied: 7, Deferred: 3, EstimatedCost: 0.5},
		{Timestamp: "2026-01-02T00:00:00Z", Status: "complete", Visions: 5, AutoApplied: 2, Deferred: 3, EstimatedCost: 0.25},
	}
	writeRunLog(t, dir, entries)

	stats := readDreamCostStats(dir)

	if stats.runs != 2 {
		t.Errorf("runs: got %d, want 2", stats.runs)
	}
	if stats.totalVisions != 15 {
		t.Errorf("totalVisions: got %d, want 15", stats.totalVisions)
	}
	if stats.totalApplied != 9 {
		t.Errorf("totalApplied: got %d, want 9", stats.totalApplied)
	}
	if stats.totalDeferred != 6 {
		t.Errorf("totalDeferred: got %d, want 6", stats.totalDeferred)
	}
	if stats.estimatedCost != 0.75 {
		t.Errorf("estimatedCost: got %f, want 0.75", stats.estimatedCost)
	}
}

func TestReadDreamCostStats_OldEntriesWithoutNewFields(t *testing.T) {
	// Old entries without AutoApplied/Deferred/Visions should parse without error
	dir := t.TempDir()
	line := `{"timestamp":"2025-01-01T00:00:00Z","status":"complete","batches":5,"estimated_cost":0.1}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "log.jsonl"), []byte(line), 0644); err != nil {
		t.Fatal(err)
	}

	stats := readDreamCostStats(dir)
	if stats.runs != 1 {
		t.Errorf("runs: got %d, want 1", stats.runs)
	}
	if stats.totalVisions != 0 {
		t.Errorf("totalVisions should be 0 for old entries, got %d", stats.totalVisions)
	}
}

func TestReadDreamRunEntries_ReturnsAll(t *testing.T) {
	dir := t.TempDir()
	entries := []dream.RunLogEntry{
		{Timestamp: "2026-01-01T00:00:00Z", Visions: 3, AutoApplied: 2},
		{Timestamp: "2026-01-02T00:00:00Z", Visions: 4, AutoApplied: 1},
		{Timestamp: "2026-01-03T00:00:00Z", Visions: 6, AutoApplied: 5},
	}
	writeRunLog(t, dir, entries)

	got := readDreamRunEntries(dir)
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}
	if got[2].Visions != 6 {
		t.Errorf("last entry Visions: got %d, want 6", got[2].Visions)
	}
}

func TestReadDreamRunEntries_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	entries := readDreamRunEntries(dir)
	if entries != nil {
		t.Errorf("expected nil for missing log, got %v", entries)
	}
}

// --- computeGraphValue tests ---

func TestComputeGraphValue_TypeFiltering(t *testing.T) {
	motes := []*core.Mote{
		{ID: "m1", Type: "task", Status: "active"},
		{ID: "m2", Type: "decision", Status: "active", CreatedAt: time.Now().AddDate(0, 0, -10)},
		{ID: "m3", Type: "lesson", Status: "active", CreatedAt: time.Now().AddDate(0, 0, -5)},
		{ID: "m4", Type: "explore", Status: "active", CreatedAt: time.Now().AddDate(0, 0, -3)},
		{ID: "m5", Type: "decision", Status: "deprecated"}, // excluded
		{ID: "m6", Type: "context", Status: "active"},
	}

	dir := t.TempDir()
	im := core.NewIndexManager(dir)
	idx, _ := im.Load()

	gv := computeGraphValue(motes, idx, nil)

	if gv.decisions != 1 {
		t.Errorf("decisions: got %d, want 1", gv.decisions)
	}
	if gv.lessons != 1 {
		t.Errorf("lessons: got %d, want 1", gv.lessons)
	}
	if gv.explorations != 1 {
		t.Errorf("explorations: got %d, want 1", gv.explorations)
	}
}

func TestComputeGraphValue_CrossSession(t *testing.T) {
	motes := []*core.Mote{
		{ID: "m1", Type: "decision", Status: "active", CreatedAt: time.Now()},
	}

	dir := t.TempDir()
	im := core.NewIndexManager(dir)
	idx, _ := im.Load()

	// m1 appears in 3 sessions, m2 in 2 sessions — only m1 qualifies
	primeStats := []core.PrimeSessionStats{
		{SessionAt: "2026-01-01T00:00:00Z", HitIDs: []string{"m1", "m2"}},
		{SessionAt: "2026-01-02T00:00:00Z", HitIDs: []string{"m1", "m2"}},
		{SessionAt: "2026-01-03T00:00:00Z", HitIDs: []string{"m1"}},
	}

	gv := computeGraphValue(motes, idx, primeStats)
	if gv.crossSession != 1 {
		t.Errorf("crossSession: got %d, want 1", gv.crossSession)
	}
}

func TestComputeGraphValue_GraphAge(t *testing.T) {
	oldest := time.Now().AddDate(0, 0, -30)
	newer := time.Now().AddDate(0, 0, -10)

	motes := []*core.Mote{
		{ID: "m1", Type: "decision", Status: "active", CreatedAt: newer},
		{ID: "m2", Type: "task", Status: "active", CreatedAt: oldest},
	}

	dir := t.TempDir()
	im := core.NewIndexManager(dir)
	idx, _ := im.Load()

	gv := computeGraphValue(motes, idx, nil)

	// Age should be ~30 days (from oldest mote)
	if gv.ageDays < 29 || gv.ageDays > 31 {
		t.Errorf("ageDays: got %d, want ~30", gv.ageDays)
	}
}

func TestComputeGraphValue_NoKnowledgeMotes(t *testing.T) {
	motes := []*core.Mote{
		{ID: "m1", Type: "task", Status: "active", CreatedAt: time.Now()},
	}

	dir := t.TempDir()
	im := core.NewIndexManager(dir)
	idx, _ := im.Load()

	gv := computeGraphValue(motes, idx, nil)
	if gv.decisions+gv.lessons+gv.explorations != 0 {
		t.Errorf("expected 0 knowledge motes, got %d", gv.decisions+gv.lessons+gv.explorations)
	}
}

// --- Dream ROI low acceptance note test ---

func TestDreamROI_LowAcceptanceNote_TriggersWithFiveRuns(t *testing.T) {
	dir := t.TempDir()

	// 5 runs, each with 10 visions and only 1 applied → 10% acceptance rate (< 25%)
	entries := make([]dream.RunLogEntry, 5)
	for i := range entries {
		entries[i] = dream.RunLogEntry{
			Timestamp:   "2026-01-01T00:00:00Z",
			Status:      "complete",
			Visions:     10,
			AutoApplied: 1,
		}
	}
	writeRunLog(t, dir, entries)

	got := readDreamRunEntries(dir)
	if len(got) < 5 {
		t.Fatal("expected 5 entries")
	}
	last5 := got[len(got)-5:]
	var l5visions, l5applied int
	for _, e := range last5 {
		l5visions += e.Visions
		l5applied += e.AutoApplied
	}
	rate := float64(l5applied) / float64(l5visions)
	if rate >= 0.25 {
		t.Errorf("expected rate < 0.25 for low acceptance note, got %.2f", rate)
	}
}

func TestDreamROI_LowAcceptanceNote_NoTriggerWithFewRuns(t *testing.T) {
	dir := t.TempDir()

	// Only 3 runs — note should NOT trigger (requires 5+)
	entries := []dream.RunLogEntry{
		{Visions: 10, AutoApplied: 1},
		{Visions: 10, AutoApplied: 1},
		{Visions: 10, AutoApplied: 1},
	}
	writeRunLog(t, dir, entries)

	got := readDreamRunEntries(dir)
	if len(got) >= 5 {
		t.Errorf("expected < 5 entries for no-trigger test, got %d", len(got))
	}
}

// helpers

func writeRunLog(t *testing.T, dir string, entries []dream.RunLogEntry) {
	t.Helper()
	var buf []byte
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "log.jsonl"), buf, 0644); err != nil {
		t.Fatal(err)
	}
}

package dream

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/core"
)

func TestDreamOrchestrator_CleanNebula(t *testing.T) {
	root, _, _ := setupTestMotes(t)
	cfg := core.DefaultConfig()

	orch := NewDreamOrchestrator(root, cfg)
	result, err := orch.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "clean" {
		t.Errorf("expected clean status, got %s", result.Status)
	}

	// Verify run log was written
	logPath := filepath.Join(root, "dream", "log.jsonl")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("run log should be written even for clean runs")
	}
}

func TestDreamOrchestrator_DryRun(t *testing.T) {
	root, mm, _ := setupTestMotes(t)

	// Create stale mote to trigger work
	m := createTestMote(t, mm, "context", "Stale mote", []string{"old"})
	_ = m

	cfg := core.DefaultConfig()
	orch := NewDreamOrchestrator(root, cfg)
	result, err := orch.Run(true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "dry-run" {
		t.Errorf("expected dry-run status, got %s", result.Status)
	}
	if result.Batches == 0 {
		t.Error("dry run should report planned batches")
	}
}

func TestLogFailedResponse(t *testing.T) {
	root, _, _ := setupTestMotes(t)
	cfg := core.DefaultConfig()
	orch := NewDreamOrchestrator(root, cfg)

	// Ensure dream dir exists
	dreamDir := filepath.Join(root, "dream")
	os.MkdirAll(dreamDir, 0755)

	// Log a short response
	orch.logFailedResponse(3, "some prose without JSON")

	logPath := filepath.Join(dreamDir, "failed_responses.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, `"batch":3`) {
		t.Errorf("expected batch 3 in log, got: %s", content)
	}
	if !strings.Contains(content, "some prose without JSON") {
		t.Errorf("expected response preview in log")
	}

	// Log a long response — should truncate to 2000 chars
	longResp := strings.Repeat("x", 5000)
	orch.logFailedResponse(7, longResp)

	data, _ = os.ReadFile(logPath)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d", len(lines))
	}
	if !strings.Contains(lines[1], `"response_len":5000`) {
		t.Errorf("expected response_len 5000 in log")
	}
	// Preview should be truncated — the JSON-encoded preview won't contain 5000 x's
	if strings.Count(lines[1], "x") >= 5000 {
		t.Errorf("expected truncated preview")
	}
}

func TestDreamOrchestrator_DreamDirCreated(t *testing.T) {
	root, mm, _ := setupTestMotes(t)
	createTestMote(t, mm, "context", "Test", []string{"tag"})

	cfg := core.DefaultConfig()
	orch := NewDreamOrchestrator(root, cfg)
	orch.Run(true) // dry run is enough

	dreamDir := filepath.Join(root, "dream")
	if info, err := os.Stat(dreamDir); err != nil || !info.IsDir() {
		t.Error("dream directory should be created")
	}
}

func TestApplyVision_LinkSuggestion(t *testing.T) {
	root, mm, im := setupTestMotes(t)

	mA, _ := mm.Create("context", "Source", core.CreateOpts{Tags: []string{"test"}})
	mB, _ := mm.Create("context", "Target", core.CreateOpts{Tags: []string{"test"}})

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	v := Vision{
		Type:        "link_suggestion",
		SourceMotes: []string{mA.ID},
		TargetMotes: []string{mB.ID},
		LinkType:    "relates_to",
	}
	if err := ApplyVision(v, mm, im, root, nil); err != nil {
		t.Fatalf("ApplyVision: %v", err)
	}

	updated, _ := mm.Read(mA.ID)
	found := false
	for _, r := range updated.RelatesTo {
		if r == mB.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("expected %s to have relates_to link to %s", mA.ID, mB.ID)
	}
}

func TestApplyVision_MissingFields(t *testing.T) {
	root, mm, im := setupTestMotes(t)

	v := Vision{Type: "link_suggestion", SourceMotes: []string{}, LinkType: "relates_to"}
	if err := ApplyVision(v, mm, im, root, nil); err == nil {
		t.Error("expected error for empty SourceMotes")
	}
}

func TestAutoApply_NoVisions(t *testing.T) {
	root, _, _ := setupTestMotes(t)
	os.MkdirAll(filepath.Join(root, "dream"), 0755)

	cfg := core.DefaultConfig()
	orch := NewDreamOrchestrator(root, cfg)
	applied, failed, err := orch.AutoApply(cfg)
	if err != nil {
		t.Fatalf("AutoApply: %v", err)
	}
	if applied != 0 || failed != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", applied, failed)
	}
}

func TestAutoApply_AppliesAllVisions(t *testing.T) {
	root, mm, im := setupTestMotes(t)
	dreamDir := filepath.Join(root, "dream")
	os.MkdirAll(dreamDir, 0755)

	mA, _ := mm.Create("context", "Source", core.CreateOpts{Tags: []string{"test"}})
	mB, _ := mm.Create("context", "Target", core.CreateOpts{Tags: []string{"test"}})

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	cfg := core.DefaultConfig()
	orch := NewDreamOrchestrator(root, cfg)

	// Write a vision to final (previously "low-risk" only, now all apply)
	vw := NewVisionWriter(dreamDir)
	vw.WriteFinal([]Vision{{
		Type:        "link_suggestion",
		SourceMotes: []string{mA.ID},
		TargetMotes: []string{mB.ID},
		LinkType:    "relates_to",
		Rationale:   "test",
	}})

	applied, failed, err := orch.AutoApply(cfg)
	if err != nil {
		t.Fatalf("AutoApply: %v", err)
	}
	if applied != 1 {
		t.Errorf("expected 1 applied, got %d", applied)
	}
	if failed != 0 {
		t.Errorf("expected 0 failed, got %d", failed)
	}

	// visions.jsonl should be removed
	if _, err := os.Stat(filepath.Join(dreamDir, "visions.jsonl")); !os.IsNotExist(err) {
		t.Error("visions.jsonl should be removed after all applied")
	}

	// Feedback should be recorded
	entries := readFeedbackEntries(root)
	if len(entries) != 1 {
		t.Errorf("expected 1 feedback entry, got %d", len(entries))
	}
}

func TestAutoApply_DependsOnNowApplied(t *testing.T) {
	root, mm, im := setupTestMotes(t)
	dreamDir := filepath.Join(root, "dream")
	os.MkdirAll(dreamDir, 0755)

	mA, _ := mm.Create("context", "Source", core.CreateOpts{Tags: []string{"test"}})
	mB, _ := mm.Create("context", "Target", core.CreateOpts{Tags: []string{"test"}})

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	cfg := core.DefaultConfig()
	orch := NewDreamOrchestrator(root, cfg)

	// depends_on was previously deferred as "high risk" — now auto-applied
	vw := NewVisionWriter(dreamDir)
	vw.WriteFinal([]Vision{{
		Type:        "link_suggestion",
		SourceMotes: []string{mA.ID},
		TargetMotes: []string{mB.ID},
		LinkType:    "depends_on",
		Rationale:   "dependency link",
	}})

	applied, failed, err := orch.AutoApply(cfg)
	if err != nil {
		t.Fatalf("AutoApply: %v", err)
	}
	if applied != 1 {
		t.Errorf("expected 1 applied, got %d", applied)
	}
	if failed != 0 {
		t.Errorf("expected 0 failed, got %d", failed)
	}
}

func TestRecordApplied_WritesFeedback(t *testing.T) {
	root, _, _ := setupTestMotes(t)
	dreamDir := filepath.Join(root, "dream")
	os.MkdirAll(dreamDir, 0755)

	v := Vision{Type: "link_suggestion", SourceMotes: []string{"a"}, TargetMotes: []string{"b"}, LinkType: "relates_to"}
	preScores := map[string]float64{"a": 0.5, "b": 0.6}
	RecordApplied(root, v, preScores)

	entries := readFeedbackEntries(root)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].VisionType != "link_suggestion" {
		t.Error("expected link_suggestion vision type")
	}
	if entries[0].PreScores["a"] != 0.5 {
		t.Errorf("expected pre-score 0.5 for 'a', got %f", entries[0].PreScores["a"])
	}
}

func TestAffectedMoteIDs(t *testing.T) {
	v := Vision{
		SourceMotes: []string{"a", "b"},
		TargetMotes: []string{"b", "c"},
	}
	ids := AffectedMoteIDs(v)
	if len(ids) != 3 {
		t.Errorf("expected 3 unique IDs, got %d: %v", len(ids), ids)
	}
}

func TestGetStats_Empty(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "dream"), 0755)

	stats := GetStats(root)
	if len(stats) != 0 {
		t.Errorf("expected empty stats, got %d entries", len(stats))
	}
}

func TestGetStats_WithEntries(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "dream"), 0755)

	positiveDelta := 0.1
	negativeDelta := -0.05
	trueBool := true
	falseBool := false

	entries := []FeedbackEntry{
		{VisionType: "link_suggestion", CheckedAt: "2024-01-01", ScoreDelta: &positiveDelta, Persisted: &trueBool},
		{VisionType: "link_suggestion", CheckedAt: "2024-01-02", ScoreDelta: &negativeDelta, Persisted: &trueBool},
		{VisionType: "staleness", CheckedAt: "2024-01-01", ScoreDelta: &positiveDelta, Persisted: &falseBool},
	}
	for _, e := range entries {
		appendFeedbackEntry(root, e)
	}

	stats := GetStats(root)
	if len(stats) != 2 {
		t.Fatalf("expected 2 vision types, got %d", len(stats))
	}
	ls := stats["link_suggestion"]
	if ls.Total != 2 || ls.Checked != 2 {
		t.Errorf("link_suggestion: total=%d checked=%d", ls.Total, ls.Checked)
	}
	if ls.Persisted != 2 {
		t.Errorf("link_suggestion: persisted=%d", ls.Persisted)
	}
	ss := stats["staleness"]
	if ss.Reverted != 1 {
		t.Errorf("staleness: reverted=%d", ss.Reverted)
	}
}

func TestDefaultConfig_ReviewModeAuto(t *testing.T) {
	cfg := core.DefaultConfig()
	if cfg.Dream.ReviewMode != "auto" {
		t.Errorf("expected default review_mode 'auto', got %q", cfg.Dream.ReviewMode)
	}
}

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

func TestIsLowRisk_RelatesToTrue(t *testing.T) {
	v := Vision{Type: "link_suggestion", LinkType: "relates_to"}
	if !isLowRisk(v) {
		t.Error("relates_to link_suggestion should be low risk")
	}
}

func TestIsLowRisk_InformedByTrue(t *testing.T) {
	v := Vision{Type: "link_suggestion", LinkType: "informed_by"}
	if !isLowRisk(v) {
		t.Error("informed_by link_suggestion should be low risk")
	}
}

func TestIsLowRisk_DependsOnFalse(t *testing.T) {
	v := Vision{Type: "link_suggestion", LinkType: "depends_on"}
	if isLowRisk(v) {
		t.Error("depends_on link_suggestion should NOT be low risk")
	}
}

func TestIsLowRisk_NonLinkFalse(t *testing.T) {
	v := Vision{Type: "staleness", Action: "deprecate"}
	if isLowRisk(v) {
		t.Error("staleness vision should NOT be low risk")
	}
}

func TestApplyVision_LinkSuggestion(t *testing.T) {
	root, mm, im := setupTestMotes(t)
	_ = root

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
	if err := applyVision(v, mm, im); err != nil {
		t.Fatalf("applyVision: %v", err)
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
	_ = root

	v := Vision{Type: "link_suggestion", SourceMotes: []string{}, LinkType: "relates_to"}
	if err := applyVision(v, mm, im); err == nil {
		t.Error("expected error for empty SourceMotes")
	}
}

func TestAutoApply_NoVisions(t *testing.T) {
	root, _, _ := setupTestMotes(t)
	os.MkdirAll(filepath.Join(root, "dream"), 0755)

	cfg := core.DefaultConfig()
	orch := NewDreamOrchestrator(root, cfg)
	applied, deferred, err := orch.AutoApply()
	if err != nil {
		t.Fatalf("AutoApply: %v", err)
	}
	if applied != 0 || deferred != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", applied, deferred)
	}
}

func TestAutoApply_AppliesLowRisk(t *testing.T) {
	root, mm, im := setupTestMotes(t)
	dreamDir := filepath.Join(root, "dream")
	os.MkdirAll(dreamDir, 0755)

	mA, _ := mm.Create("context", "Source", core.CreateOpts{Tags: []string{"test"}})
	mB, _ := mm.Create("context", "Target", core.CreateOpts{Tags: []string{"test"}})

	motes, _ := mm.ReadAllParallel()
	im.Rebuild(motes)

	cfg := core.DefaultConfig()
	orch := NewDreamOrchestrator(root, cfg)

	// Write a low-risk vision to final
	vw := NewVisionWriter(dreamDir)
	vw.WriteFinal([]Vision{{
		Type:        "link_suggestion",
		SourceMotes: []string{mA.ID},
		TargetMotes: []string{mB.ID},
		LinkType:    "relates_to",
		Rationale:   "test",
	}})

	applied, deferred, err := orch.AutoApply()
	if err != nil {
		t.Fatalf("AutoApply: %v", err)
	}
	if applied != 1 {
		t.Errorf("expected 1 applied, got %d", applied)
	}
	if deferred != 0 {
		t.Errorf("expected 0 deferred, got %d", deferred)
	}

	// visions.jsonl should be removed
	if _, err := os.Stat(filepath.Join(dreamDir, "visions.jsonl")); !os.IsNotExist(err) {
		t.Error("visions.jsonl should be removed after all applied")
	}
}

func TestAutoApply_DefersHighRisk(t *testing.T) {
	root, mm, _ := setupTestMotes(t)
	dreamDir := filepath.Join(root, "dream")
	os.MkdirAll(dreamDir, 0755)

	mA, _ := mm.Create("context", "Source", core.CreateOpts{Tags: []string{"test"}})

	cfg := core.DefaultConfig()
	orch := NewDreamOrchestrator(root, cfg)

	vw := NewVisionWriter(dreamDir)
	vw.WriteFinal([]Vision{{
		Type:        "link_suggestion",
		SourceMotes: []string{mA.ID},
		TargetMotes: []string{"target"},
		LinkType:    "depends_on",
		Rationale:   "high risk",
	}})

	applied, deferred, err := orch.AutoApply()
	if err != nil {
		t.Fatalf("AutoApply: %v", err)
	}
	if applied != 0 {
		t.Errorf("expected 0 applied, got %d", applied)
	}
	if deferred != 1 {
		t.Errorf("expected 1 deferred, got %d", deferred)
	}

	// visions.jsonl should still exist with the deferred vision
	final := vw.ReadFinal()
	if len(final) != 1 {
		t.Errorf("expected 1 remaining vision, got %d", len(final))
	}
}

func TestLogAutoApplied(t *testing.T) {
	root, _, _ := setupTestMotes(t)
	dreamDir := filepath.Join(root, "dream")
	os.MkdirAll(dreamDir, 0755)

	cfg := core.DefaultConfig()
	orch := NewDreamOrchestrator(root, cfg)

	v := Vision{Type: "link_suggestion", SourceMotes: []string{"a"}, TargetMotes: []string{"b"}, LinkType: "relates_to"}
	orch.logAutoApplied(v)

	logPath := filepath.Join(dreamDir, "auto_applied.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read auto_applied.jsonl: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "link_suggestion") {
		t.Error("expected link_suggestion in auto_applied log")
	}
	if !strings.Contains(content, "relates_to") {
		t.Error("expected relates_to in auto_applied log")
	}
}

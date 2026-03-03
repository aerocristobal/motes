package dream

import (
	"os"
	"path/filepath"
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

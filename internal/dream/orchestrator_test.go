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

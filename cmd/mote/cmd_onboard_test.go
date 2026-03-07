package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateClaudeSettings_BdPrime(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	input := `{
  "hooks": {
    "SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "bd prime"}]}],
    "PreCompact": [{"matcher": "", "hooks": [{"type": "command", "command": "bd prime"}]}]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	migrated, err := migrateClaudeSettings(dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if migrated != 2 {
		t.Errorf("expected 2 migrated hooks, got %d", migrated)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "mote prime") {
		t.Error("expected settings to contain 'mote prime' after migration")
	}
	if strings.Contains(content, "bd prime") {
		t.Error("expected settings to NOT contain 'bd prime' after migration")
	}
}

func TestMigrateClaudeSettings_BdSync(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	input := `{
  "hooks": {
    "SessionEnd": [{"matcher": "", "hooks": [{"type": "command", "command": "bd sync"}]}]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	migrated, err := migrateClaudeSettings(dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if migrated != 1 {
		t.Errorf("expected 1 migrated hook, got %d", migrated)
	}

	data, _ := os.ReadFile(settingsPath)
	if !strings.Contains(string(data), "mote session-end") {
		t.Error("expected settings to contain 'mote session-end' after migration")
	}
}

func TestMigrateClaudeSettings_NoBdHooks(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	input := `{
  "hooks": {
    "SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "mote prime"}]}]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	migrated, err := migrateClaudeSettings(dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if migrated != 0 {
		t.Errorf("expected 0 migrated hooks, got %d", migrated)
	}

	// Verify file was not modified
	data, _ := os.ReadFile(settingsPath)
	if string(data) != input {
		t.Error("expected settings file to be unchanged when no bd hooks found")
	}
}

func TestMigrateClaudeSettings_DryRun(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	input := `{
  "hooks": {
    "SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "bd prime"}]}]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	migrated, err := migrateClaudeSettings(dir, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if migrated != 1 {
		t.Errorf("expected 1 detected hook, got %d", migrated)
	}

	// Verify file was NOT modified (dry run)
	data, _ := os.ReadFile(settingsPath)
	if string(data) != input {
		t.Error("dry run should not modify settings file")
	}
}

func TestMigrateClaudeSettings_NoFile(t *testing.T) {
	dir := t.TempDir()

	migrated, err := migrateClaudeSettings(dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if migrated != 0 {
		t.Errorf("expected 0 migrated hooks for missing file, got %d", migrated)
	}
}

// --- ensureClaudeHooks tests ---

func TestEnsureClaudeHooks_CreatesNew(t *testing.T) {
	dir := t.TempDir()

	if err := ensureClaudeHooks(dir, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	hooks := settings["hooks"].(map[string]interface{})
	for _, event := range []string{"SessionStart", "PreCompact"} {
		if !hookEventHasCommand(hooks, event, "mote prime") {
			t.Errorf("expected %s hook with 'mote prime'", event)
		}
	}
}

func TestEnsureClaudeHooks_MergesExisting(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	existing := `{
  "hooks": {
    "SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "echo hello"}]}]
  },
  "other_key": true
}`
	os.WriteFile(settingsPath, []byte(existing), 0644)

	if err := ensureClaudeHooks(dir, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(settingsPath)
	var settings map[string]interface{}
	json.Unmarshal(data, &settings)

	// Verify other_key preserved
	if settings["other_key"] != true {
		t.Error("expected other_key to be preserved")
	}

	hooks := settings["hooks"].(map[string]interface{})

	// SessionStart should have 2 entries (original + mote prime)
	entries := hooks["SessionStart"].([]interface{})
	if len(entries) != 2 {
		t.Errorf("expected 2 SessionStart entries, got %d", len(entries))
	}

	// PreCompact should be added
	if !hookEventHasCommand(hooks, "PreCompact", "mote prime") {
		t.Error("expected PreCompact hook to be added")
	}

	// Original hook should still be there
	if !hookEventHasCommand(hooks, "SessionStart", "echo hello") {
		t.Error("expected original SessionStart hook to be preserved")
	}
}

func TestEnsureClaudeHooks_Idempotent(t *testing.T) {
	dir := t.TempDir()

	// First call
	ensureClaudeHooks(dir, false)
	data1, _ := os.ReadFile(filepath.Join(dir, "settings.json"))

	// Second call
	ensureClaudeHooks(dir, false)
	data2, _ := os.ReadFile(filepath.Join(dir, "settings.json"))

	if string(data1) != string(data2) {
		t.Error("expected idempotent behavior — file changed on second call")
	}
}

// --- ensureMoteSkills tests ---

func TestEnsureMoteSkills_CreatesNew(t *testing.T) {
	dir := t.TempDir()

	if err := ensureMoteSkills(dir, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, name := range []string{"mote-capture", "mote-retrieve"} {
		path := filepath.Join(dir, ".claude", "skills", name, "SKILL.md")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected %s to be created", path)
		}
		data, _ := os.ReadFile(path)
		if len(data) == 0 {
			t.Errorf("expected non-empty skill file for %s", name)
		}
	}
}

func TestEnsureMoteSkills_Idempotent(t *testing.T) {
	dir := t.TempDir()

	ensureMoteSkills(dir, false)

	capturePath := filepath.Join(dir, ".claude", "skills", "mote-capture", "SKILL.md")
	data1, _ := os.ReadFile(capturePath)

	ensureMoteSkills(dir, false)
	data2, _ := os.ReadFile(capturePath)

	if string(data1) != string(data2) {
		t.Error("expected skill content to be unchanged on second call")
	}
}

// --- cleanup flag test ---

func TestOnboard_CleanupFlag(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Create a fake .beads directory
	cwd, _ := os.Getwd()
	beadsDir := filepath.Join(cwd, ".beads")
	os.MkdirAll(beadsDir, 0755)
	os.WriteFile(filepath.Join(beadsDir, "issues.jsonl"), []byte{}, 0644)

	onboardDryRun = false
	onboardGlobal = false
	onboardCleanup = true
	onboardIncludeClosed = false
	defer func() { onboardCleanup = false }()

	if err := runOnboard(onboardCmd, nil); err != nil {
		t.Fatalf("onboard --cleanup: %v", err)
	}

	if dirExists(beadsDir) {
		t.Error("expected .beads/ to be removed with --cleanup")
	}
}

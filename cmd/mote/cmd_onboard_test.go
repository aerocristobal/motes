package main

import (
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

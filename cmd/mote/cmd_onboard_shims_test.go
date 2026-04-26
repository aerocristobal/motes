// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureManagedSection_AppendsToEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")

	changed, err := ensureManagedSection(path, "hello world", false)
	if err != nil {
		t.Fatalf("ensureManagedSection: %v", err)
	}
	if !changed {
		t.Error("expected changed=true on first write")
	}
	got, _ := os.ReadFile(path)
	s := string(got)
	if !strings.Contains(s, "<!-- BEGIN motes-managed -->") {
		t.Errorf("missing begin marker:\n%s", s)
	}
	if !strings.Contains(s, "<!-- END motes-managed -->") {
		t.Errorf("missing end marker:\n%s", s)
	}
	if !strings.Contains(s, "hello world") {
		t.Errorf("missing content:\n%s", s)
	}
}

func TestEnsureManagedSection_PreservesUserContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")

	userContent := "# My personal CLAUDE.md\n\nHere are my rules.\n"
	if err := os.WriteFile(path, []byte(userContent), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := ensureManagedSection(path, "motes content", false); err != nil {
		t.Fatalf("ensureManagedSection: %v", err)
	}
	got, _ := os.ReadFile(path)
	s := string(got)
	if !strings.Contains(s, "Here are my rules.") {
		t.Errorf("user content lost:\n%s", s)
	}
	if !strings.Contains(s, "motes content") {
		t.Errorf("managed content not added:\n%s", s)
	}
}

func TestEnsureManagedSection_ReplacesExistingBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")

	initial := "user prefix\n\n<!-- BEGIN motes-managed -->\nold content\n<!-- END motes-managed -->\n\nuser suffix\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	changed, err := ensureManagedSection(path, "new content", false)
	if err != nil {
		t.Fatalf("ensureManagedSection: %v", err)
	}
	if !changed {
		t.Error("expected changed=true when content differs")
	}
	got, _ := os.ReadFile(path)
	s := string(got)

	if !strings.Contains(s, "user prefix") {
		t.Errorf("user prefix lost:\n%s", s)
	}
	if !strings.Contains(s, "user suffix") {
		t.Errorf("user suffix lost:\n%s", s)
	}
	if !strings.Contains(s, "new content") {
		t.Errorf("new content not present:\n%s", s)
	}
	if strings.Contains(s, "old content") {
		t.Errorf("old content not replaced:\n%s", s)
	}
}

func TestEnsureManagedSection_IdempotentWhenIdentical(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")

	if _, err := ensureManagedSection(path, "static content", false); err != nil {
		t.Fatal(err)
	}
	changed, err := ensureManagedSection(path, "static content", false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("expected changed=false on no-op rewrite")
	}
}

func TestEnsureGeminiGlobalShim_WritesShimFileAndUpdatesSettings(t *testing.T) {
	home := t.TempDir()

	if err := ensureGeminiGlobalShim(home, false); err != nil {
		t.Fatalf("ensureGeminiGlobalShim: %v", err)
	}

	shimPath := filepath.Join(home, ".gemini", "motes-shim.md")
	if _, err := os.Stat(shimPath); err != nil {
		t.Fatalf("shim file not written: %v", err)
	}
	body, _ := os.ReadFile(shimPath)
	if !strings.Contains(string(body), "@~/.motes/MOTES.md") {
		t.Errorf("shim missing @-import:\n%s", body)
	}

	settingsPath := filepath.Join(home, ".gemini", "settings.json")
	settingsBytes, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not written: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(settingsBytes, &settings); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}
	ctx, ok := settings["context"].(map[string]any)
	if !ok {
		t.Fatalf("missing context block: %v", settings)
	}
	files, _ := ctx["fileName"].([]any)
	found := false
	for _, f := range files {
		if s, _ := f.(string); s == "motes-shim.md" {
			found = true
		}
	}
	if !found {
		t.Errorf("motes-shim.md not in context.fileName: %v", files)
	}
}

func TestEnsureGeminiGlobalShim_PreservesExistingSettings(t *testing.T) {
	home := t.TempDir()
	geminiDir := filepath.Join(home, ".gemini")
	if err := os.MkdirAll(geminiDir, 0755); err != nil {
		t.Fatal(err)
	}
	pre := `{
  "theme": "dark",
  "context": {
    "fileName": ["GEMINI.md", "AGENTS.md", "MY_RULES.md"]
  },
  "hooks": {"SessionStart": []}
}`
	if err := os.WriteFile(filepath.Join(geminiDir, "settings.json"), []byte(pre), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ensureGeminiGlobalShim(home, false); err != nil {
		t.Fatalf("ensureGeminiGlobalShim: %v", err)
	}

	body, _ := os.ReadFile(filepath.Join(geminiDir, "settings.json"))
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}
	if got["theme"] != "dark" {
		t.Errorf("theme not preserved: %v", got["theme"])
	}
	ctx := got["context"].(map[string]any)
	files := ctx["fileName"].([]any)
	expected := map[string]bool{"GEMINI.md": false, "AGENTS.md": false, "MY_RULES.md": false, "motes-shim.md": false}
	for _, f := range files {
		s, _ := f.(string)
		if _, want := expected[s]; want {
			expected[s] = true
		}
	}
	for name, present := range expected {
		if !present {
			t.Errorf("expected %s in fileName: %v", name, files)
		}
	}
}

func TestEnsureGeminiGlobalShim_IdempotentSettingsFile(t *testing.T) {
	home := t.TempDir()
	if err := ensureGeminiGlobalShim(home, false); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(home, ".gemini", "settings.json"))

	if err := ensureGeminiGlobalShim(home, false); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(home, ".gemini", "settings.json"))

	// Re-running should produce identical settings (no duplicate fileName entries).
	if string(first) != string(second) {
		t.Errorf("settings.json drifted on re-run\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	// Verify no duplicate motes-shim.md entries.
	var s map[string]any
	json.Unmarshal(second, &s)
	files := s["context"].(map[string]any)["fileName"].([]any)
	count := 0
	for _, f := range files {
		if str, _ := f.(string); str == "motes-shim.md" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one motes-shim.md entry, got %d", count)
	}
}

func TestEnsureGeminiGlobalShim_DoesNotTouchGEMINIMD(t *testing.T) {
	home := t.TempDir()
	geminiDir := filepath.Join(home, ".gemini")
	if err := os.MkdirAll(geminiDir, 0755); err != nil {
		t.Fatal(err)
	}
	preserved := "# user-authored GEMINI.md\nuser content here"
	if err := os.WriteFile(filepath.Join(geminiDir, "GEMINI.md"), []byte(preserved), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ensureGeminiGlobalShim(home, false); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(filepath.Join(geminiDir, "GEMINI.md"))
	if string(got) != preserved {
		t.Errorf("GEMINI.md was modified by motes:\nexpected:\n%s\ngot:\n%s", preserved, got)
	}
}

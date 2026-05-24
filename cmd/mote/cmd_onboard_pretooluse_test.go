// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/claudehooks"
)

// scriptInstallExpectations lists the files we expect installPreToolUseScripts
// to write, paired with their embedded source bytes.
var scriptInstallExpectations = []struct {
	relPath string
	content []byte
}{
	{"hooks/block-interactive-cmds.sh", claudehooks.BlockInteractiveCmds},
	{"hooks/block-gh-watch.sh", claudehooks.BlockGhWatch},
	{"hooks/block-mote-rm.sh", claudehooks.BlockMoteRm},
}

// expectedPreToolUseCommands is the list of command strings (in any order) we
// expect to find registered in settings.json hooks.PreToolUse[].
var expectedPreToolUseCommands = []string{
	"$HOME/.claude/hooks/block-interactive-cmds.sh",
	"$HOME/.claude/hooks/block-gh-watch.sh",
	"$HOME/.claude/hooks/block-mote-rm.sh",
}

func TestEnsurePreToolUseHooks_CreatesNew(t *testing.T) {
	dir := t.TempDir()

	if err := ensurePreToolUseHooks(dir, false); err != nil {
		t.Fatalf("ensurePreToolUseHooks: %v", err)
	}

	for _, exp := range scriptInstallExpectations {
		path := filepath.Join(dir, exp.relPath)
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", exp.relPath, err)
		}
		if fi.Mode().Perm()&0o111 == 0 {
			t.Errorf("expected %s to be executable, got mode %v", exp.relPath, fi.Mode().Perm())
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, exp.content) {
			t.Errorf("%s content does not match embedded asset", exp.relPath)
		}
	}

	commands := readPreToolUseCommands(t, filepath.Join(dir, "settings.json"))
	for _, want := range expectedPreToolUseCommands {
		if !containsString(commands, want) {
			t.Errorf("settings.json missing PreToolUse command %q (have %v)", want, commands)
		}
	}
}

func TestEnsurePreToolUseHooks_PreservesExistingEntries(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	seed := `{
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "./custom-hook.sh"}]}
    ],
    "SessionStart": [
      {"matcher": "startup", "hooks": [{"type": "command", "command": "mote prime --hook --mode=startup"}]}
    ]
  }
}
`
	if err := os.WriteFile(settingsPath, []byte(seed), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ensurePreToolUseHooks(dir, false); err != nil {
		t.Fatalf("ensurePreToolUseHooks: %v", err)
	}

	commands := readPreToolUseCommands(t, settingsPath)

	// Existing custom hook must survive.
	if !containsString(commands, "./custom-hook.sh") {
		t.Errorf("seeded ./custom-hook.sh was lost: %v", commands)
	}
	// All three new hooks must be present.
	for _, want := range expectedPreToolUseCommands {
		if !containsString(commands, want) {
			t.Errorf("new hook %q not registered: %v", want, commands)
		}
	}

	// SessionStart hooks must not be disturbed.
	data, _ := os.ReadFile(settingsPath)
	if !strings.Contains(string(data), "mote prime --hook --mode=startup") {
		t.Error("unrelated SessionStart hook was lost")
	}
}

func TestEnsurePreToolUseHooks_Idempotent(t *testing.T) {
	dir := t.TempDir()

	if err := ensurePreToolUseHooks(dir, false); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}

	if err := ensurePreToolUseHooks(dir, false); err != nil {
		t.Fatalf("second run: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(first, second) {
		t.Errorf("second run modified settings.json\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	// And no duplicate command entries.
	commands := readPreToolUseCommands(t, filepath.Join(dir, "settings.json"))
	for _, want := range expectedPreToolUseCommands {
		if countString(commands, want) != 1 {
			t.Errorf("command %q appears %d times, expected exactly 1", want, countString(commands, want))
		}
	}
}

func TestInstallPreToolUseScripts_UpdatesStaleContent(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}

	staleTarget := filepath.Join(hooksDir, "block-mote-rm.sh")
	if err := os.WriteFile(staleTarget, []byte("#!/usr/bin/env bash\n# stale\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := installPreToolUseScripts(dir, false); err != nil {
		t.Fatalf("installPreToolUseScripts: %v", err)
	}

	got, err := os.ReadFile(staleTarget)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, claudehooks.BlockMoteRm) {
		t.Error("stale block-mote-rm.sh was not refreshed to embedded content")
	}

	fi, _ := os.Stat(staleTarget)
	if fi.Mode().Perm()&0o111 == 0 {
		t.Errorf("refreshed script should be executable, got %v", fi.Mode().Perm())
	}
}

func TestEnsurePreToolUseHooks_DryRunMakesNoChanges(t *testing.T) {
	dir := t.TempDir()

	if err := ensurePreToolUseHooks(dir, true); err != nil {
		t.Fatalf("dry-run: %v", err)
	}

	// settings.json must not be created in dry-run mode.
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); !os.IsNotExist(err) {
		t.Errorf("dry-run created settings.json (err=%v)", err)
	}
	// Neither should the hook scripts.
	if _, err := os.Stat(filepath.Join(dir, "hooks", "block-interactive-cmds.sh")); !os.IsNotExist(err) {
		t.Errorf("dry-run created hook script (err=%v)", err)
	}
}

// --- helpers ------------------------------------------------------------------

// readPreToolUseCommands extracts every command string from
// settings.json's hooks.PreToolUse[].hooks[].command, in registration order.
func readPreToolUseCommands(t *testing.T, settingsPath string) []string {
	t.Helper()
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var settings struct {
		Hooks struct {
			PreToolUse []struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Type    string `json:"type"`
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"PreToolUse"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings.json: %v\n%s", err, data)
	}
	var cmds []string
	for _, entry := range settings.Hooks.PreToolUse {
		for _, h := range entry.Hooks {
			cmds = append(cmds, h.Command)
		}
	}
	return cmds
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func countString(haystack []string, needle string) int {
	n := 0
	for _, s := range haystack {
		if s == needle {
			n++
		}
	}
	return n
}

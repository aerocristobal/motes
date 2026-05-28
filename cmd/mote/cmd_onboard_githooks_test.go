// SPDX-License-Identifier: MIT
//
// STORY-HOOKINST-001 Scenario 2 — `mote onboard` invokes the githooks
// install path transparently, and degrades gracefully when the current
// directory isn't a git working tree.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnsureProjectGithooks_WritesHooks_InGitRepo exercises the helper
// `runCommonSetup` calls. Confirms onboard installs both hooks in a real
// git working tree.
func TestEnsureProjectGithooks_WritesHooks_InGitRepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, _ := captureBothStreams(t, func() {
		ensureProjectGithooks(dir, false)
	})
	if !strings.Contains(stdout, "install githook:") {
		t.Errorf("onboard summary should list installed hooks; got %q", stdout)
	}
	for _, ev := range []string{"post-checkout", "pre-commit"} {
		path := filepath.Join(dir, ".git", "hooks", ev)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("hook %s not installed: %v", ev, err)
		}
	}
}

// TestEnsureProjectGithooks_NotGitRepo_OneLineNote confirms onboard prints
// a single non-warning informational line when the project isn't a git
// checkout (STORY-HOOKINST-001 Scenario 5 — "skipped with a one-line note").
// Stderr stays empty so this case is never confused with a real failure.
func TestEnsureProjectGithooks_NotGitRepo_OneLineNote(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr := captureBothStreams(t, func() {
		ensureProjectGithooks(dir, false)
	})
	if !strings.Contains(stdout, "skipped githook install") {
		t.Errorf("stdout should carry the one-line skip note; got %q", stdout)
	}
	if strings.Count(stdout, "\n") != 1 {
		t.Errorf("expected exactly one note line; got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr must stay empty (not a warning); got %q", stderr)
	}
}

// TestEnsureProjectGithooks_Conflict_Warns confirms onboard prints a
// one-line warning per conflicting hook but does NOT fail or write.
func TestEnsureProjectGithooks_Conflict_Warns(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	userBody := []byte("#!/bin/sh\necho user\n")
	hookPath := filepath.Join(hooksDir, "post-checkout")
	if err := os.WriteFile(hookPath, userBody, 0o755); err != nil {
		t.Fatal(err)
	}

	_, stderr := captureBothStreams(t, func() {
		ensureProjectGithooks(dir, false)
	})
	if !strings.Contains(stderr, "githook conflict") {
		t.Errorf("stderr should warn about the conflict; got %q", stderr)
	}
	if !strings.Contains(stderr, "--force") {
		t.Errorf("warning should mention --force override; got %q", stderr)
	}
	// User file untouched.
	got, _ := os.ReadFile(hookPath)
	if string(got) != string(userBody) {
		t.Errorf("user hook modified despite conflict; got %q", got)
	}
}

// TestEnsureProjectGithooks_DryRun_NoWrites confirms onboard --dry-run
// previews hook actions without touching the filesystem.
func TestEnsureProjectGithooks_DryRun_NoWrites(t *testing.T) {
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, _ := captureBothStreams(t, func() {
		ensureProjectGithooks(dir, true)
	})
	if !strings.Contains(stdout, "would install githook:") {
		t.Errorf("dry-run summary should use the 'would' prefix; got %q", stdout)
	}
	entries, _ := os.ReadDir(hooksDir)
	for _, e := range entries {
		if !e.IsDir() && !strings.HasSuffix(e.Name(), ".sample") {
			t.Errorf("dry-run created file %s", e.Name())
		}
	}
}

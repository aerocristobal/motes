// SPDX-License-Identifier: MIT
//
// STORY-HOOKINST-001 Scenario 3 (line 2) — `mote doctor --fix` repairs
// drifted mote-managed git hooks in place. Never touches user-authored
// files.
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/githooks"
)

// installFreshHooks runs githooks.Install in a temp dir and returns the
// resolved git hooks directory.
func installFreshHooks(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := githooks.Install(dir, githooks.InstallOpts{}); err != nil {
		t.Fatalf("seed Install: %v", err)
	}
	return dir
}

func TestRunGithookDriftCheck_DetectsDrift(t *testing.T) {
	dir := installFreshHooks(t)
	hookPath := filepath.Join(dir, ".git", "hooks", "post-checkout")
	f, err := os.OpenFile(hookPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("# tampered\n")
	_ = f.Close()

	issues, fixed := runGithookDriftCheck(dir, false)
	if len(fixed) != 0 {
		t.Errorf("non-fix mode should not repair; got %v", fixed)
	}
	if len(issues) == 0 {
		t.Fatal("drift not detected")
	}
	var found bool
	for _, iss := range issues {
		if iss.Category == "git_hook_drift" && strings.Contains(iss.Detail, hookPath) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected git_hook_drift issue naming %s; got %+v", hookPath, issues)
	}
}

func TestRunGithookDriftCheck_FixRepairs(t *testing.T) {
	dir := installFreshHooks(t)
	hookPath := filepath.Join(dir, ".git", "hooks", "post-checkout")
	original, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hookPath, append(original, []byte("# tampered\n")...), 0o755); err != nil {
		t.Fatal(err)
	}

	issues, fixed := runGithookDriftCheck(dir, true)
	if len(issues) != 0 {
		t.Errorf("repaired drift should not appear as issue; got %+v", issues)
	}
	if len(fixed) == 0 {
		t.Fatal("--fix should report what it repaired")
	}
	repaired, _ := os.ReadFile(hookPath)
	if bytes.Contains(repaired, []byte("# tampered")) {
		t.Errorf("--fix did not remove the drifted line")
	}
	if !bytes.Equal(repaired, original) {
		t.Errorf("--fix did not restore the canonical template body")
	}
}

func TestRunGithookDriftCheck_NotGitRepo_Silent(t *testing.T) {
	dir := t.TempDir()
	issues, fixed := runGithookDriftCheck(dir, true)
	if len(issues) != 0 || len(fixed) != 0 {
		t.Errorf("expected silence outside a git repo; got issues=%v fixed=%v", issues, fixed)
	}
}

func TestRunGithookDriftCheck_UserAuthored_DoesNotSurface(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(dir, ".git", "hooks", "post-checkout")
	userBody := []byte("#!/bin/sh\n# my hook\n")
	if err := os.WriteFile(hookPath, userBody, 0o755); err != nil {
		t.Fatal(err)
	}

	issues, fixed := runGithookDriftCheck(dir, true)
	if len(issues) != 0 {
		t.Errorf("doctor should be silent on user-authored hooks; got %+v", issues)
	}
	if len(fixed) != 0 {
		t.Errorf("--fix must never touch user-authored hooks; got %v", fixed)
	}
	got, _ := os.ReadFile(hookPath)
	if !bytes.Equal(got, userBody) {
		t.Errorf("user hook modified despite --fix; got %q", got)
	}
}

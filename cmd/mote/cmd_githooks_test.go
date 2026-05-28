// SPDX-License-Identifier: MIT
//
// STORY-HOOKINST-001 — CLI surface of `mote githooks install`.
//
// Pins the exit-code mapping from internal/githooks errors to documented
// process exit codes (0/1/2/3) and the --dry-run / --force flag plumbing.
package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/githooks"
)

// resetGithooksFlags zeroes the cmd_githooks.go flag globals so test
// invocations don't leak --force or --dry-run into each other.
func resetGithooksFlags() {
	githooksInstallDryRun = false
	githooksInstallForce = false
}

// runGithooksViaCobra invokes the CLI through rootCmd. Mirrors runLsViaCobra
// from cmd_ls_empty_state_test.go.
func runGithooksViaCobra(args []string) error {
	resetGithooksFlags()
	defer resetGithooksFlags()
	rootCmd.SetArgs(args)
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	return rootCmd.Execute()
}

// mkGitRepoAndChdir creates a temp dir with .git/hooks/ and chdirs into it,
// returning a cleanup that restores the original cwd.
func mkGitRepoAndChdir(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return dir, func() { _ = os.Chdir(orig) }
}

func TestCmdGithooksInstall_FreshRepo_Exit0(t *testing.T) {
	_, cleanup := mkGitRepoAndChdir(t)
	defer cleanup()

	var err error
	stdout, _ := captureBothStreams(t, func() {
		err = runGithooksViaCobra([]string{"githooks", "install"})
	})
	if err != nil {
		t.Fatalf("want nil (exit 0), got %v", err)
	}
	if !strings.Contains(stdout, "install:") {
		t.Errorf("stdout should report install actions; got %q", stdout)
	}
}

func TestCmdGithooksInstall_NotAGitRepo_Exit3(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(orig) }()

	var err error
	captureBothStreams(t, func() {
		err = runGithooksViaCobra([]string{"githooks", "install"})
	})
	var ec *exitCodeError
	if !errors.As(err, &ec) || ec.code != 3 {
		t.Fatalf("want exitCodeError{code:3}, got %v", err)
	}
	if !errors.Is(ec.err, githooks.ErrNotGitRepo) {
		t.Errorf("wrapped error should be ErrNotGitRepo, got %v", ec.err)
	}
	// main.go is what prints the error message in production; the test
	// observes the underlying error directly. The directory checked must
	// surface in the wrapped error so it reaches the user via main.go's
	// `fmt.Fprintln(os.Stderr, ec.err)` path.
	if !strings.Contains(ec.err.Error(), dir) {
		t.Errorf("wrapped error should name the directory checked; got %q", ec.err.Error())
	}
}

func TestCmdGithooksInstall_Conflict_Exit2(t *testing.T) {
	dir, cleanup := mkGitRepoAndChdir(t)
	defer cleanup()
	hookPath := filepath.Join(dir, ".git", "hooks", "post-checkout")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\necho user\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var err error
	stdout, _ := captureBothStreams(t, func() {
		err = runGithooksViaCobra([]string{"githooks", "install"})
	})
	var ec *exitCodeError
	if !errors.As(err, &ec) || ec.code != 2 {
		t.Fatalf("want exitCodeError{code:2}, got %v", err)
	}
	if !errors.Is(ec.err, githooks.ErrConflict) {
		t.Errorf("wrapped error should be ErrConflict, got %v", ec.err)
	}
	if !strings.Contains(stdout, "conflict:") {
		t.Errorf("stdout should flag conflict; got %q", stdout)
	}
	if !strings.Contains(stdout, "--force") {
		t.Errorf("conflict line should mention --force; got %q", stdout)
	}
}

func TestCmdGithooksInstall_Force_OverridesConflict_Exit0(t *testing.T) {
	dir, cleanup := mkGitRepoAndChdir(t)
	defer cleanup()
	hookPath := filepath.Join(dir, ".git", "hooks", "post-checkout")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\necho user\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var err error
	captureBothStreams(t, func() {
		err = runGithooksViaCobra([]string{"githooks", "install", "--force"})
	})
	if err != nil {
		t.Fatalf("want nil (exit 0) with --force, got %v", err)
	}
	body, _ := os.ReadFile(hookPath)
	if !strings.Contains(string(body), "managed-by: mote githooks install") {
		t.Errorf("--force did not write the embedded template; got %q", body)
	}
}

func TestCmdGithooksInstall_DryRun_ConflictStillExits0(t *testing.T) {
	dir, cleanup := mkGitRepoAndChdir(t)
	defer cleanup()
	hookPath := filepath.Join(dir, ".git", "hooks", "post-checkout")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\necho user\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var err error
	stdout, _ := captureBothStreams(t, func() {
		err = runGithooksViaCobra([]string{"githooks", "install", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("dry-run should exit 0 even with conflict, got %v", err)
	}
	if !strings.Contains(stdout, "conflict:") {
		t.Errorf("dry-run stdout should still report conflict; got %q", stdout)
	}
	// Nothing should have been written.
	body, _ := os.ReadFile(hookPath)
	if string(body) != "#!/bin/sh\necho user\n" {
		t.Errorf("dry-run modified the hook: %q", body)
	}
}

func TestCmdGithooksInstall_DryRun_FreshRepo_NoWrites(t *testing.T) {
	dir, cleanup := mkGitRepoAndChdir(t)
	defer cleanup()

	var err error
	stdout, _ := captureBothStreams(t, func() {
		err = runGithooksViaCobra([]string{"githooks", "install", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if !strings.Contains(stdout, "would install:") {
		t.Errorf("dry-run should preview install actions; got %q", stdout)
	}
	// .git/hooks/ should still contain no real hook scripts.
	entries, err := os.ReadDir(filepath.Join(dir, ".git", "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sample") && !e.IsDir() {
			t.Errorf("dry-run created file %s", e.Name())
		}
	}
}

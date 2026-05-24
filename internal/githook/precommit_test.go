// Package githook_test contains integration tests for the
// contributor-only Go pre-commit hook at .githooks/pre-commit and the
// Makefile install-hooks target.
//
// These tests live in their own directory with no production code:
// `internal/githook/` exists only to host this test file so that the
// hook script can be exercised under `go test ./...` without polluting
// any other package.
package githook_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot walks up from the test file's directory until it finds the
// repo root (the directory containing .githooks/pre-commit). Tests use
// this to locate the script under test without depending on the
// caller's cwd.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine caller path")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, ".githooks", "pre-commit")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not locate repo root from test file")
	return ""
}

// hookPath returns the absolute path to .githooks/pre-commit.
func hookPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), ".githooks", "pre-commit")
}

// readHook returns the hook script content for static assertions.
func readHook(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(hookPath(t))
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}
	return string(b)
}

// run executes a command inside dir and returns stdout+stderr combined
// along with the exit error (or nil on success).
func run(t *testing.T, dir string, env []string, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// initRepo creates a tempdir, runs `git init`, sets a local
// user.name/email so commits succeed in CI sandboxes, and copies the
// repo's .githooks/pre-commit into .git/hooks/pre-commit.
//
// Returns the temp repo path.
func initRepo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()

	if _, err := run(t, tmp, nil, "git", "init", "-q", "-b", "main"); err != nil {
		t.Skipf("git init failed (git not available?): %v", err)
	}
	if _, err := run(t, tmp, nil, "git", "config", "user.email", "test@example.com"); err != nil {
		t.Fatalf("git config email: %v", err)
	}
	if _, err := run(t, tmp, nil, "git", "config", "user.name", "Test"); err != nil {
		t.Fatalf("git config name: %v", err)
	}
	if _, err := run(t, tmp, nil, "git", "config", "commit.gpgsign", "false"); err != nil {
		t.Fatalf("git config gpgsign: %v", err)
	}
	// Override any inherited global/system core.hooksPath so the hook
	// we install at .git/hooks/pre-commit is the one git actually runs.
	if _, err := run(t, tmp, nil, "git", "config", "core.hooksPath", ".git/hooks"); err != nil {
		t.Fatalf("git config core.hooksPath: %v", err)
	}

	// Install the hook.
	src, err := os.ReadFile(hookPath(t))
	if err != nil {
		t.Fatalf("read source hook: %v", err)
	}
	dst := filepath.Join(tmp, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(dst, src, 0o755); err != nil {
		t.Fatalf("write hook: %v", err)
	}

	// Seed a minimal Go module + an initial commit so HEAD exists for
	// --new-from-rev=HEAD and golangci-lint has a module to typecheck.
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module hooktest\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "seed.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	if _, err := run(t, tmp, nil, "git", "add", "go.mod", "seed.go"); err != nil {
		t.Fatalf("git add seed: %v", err)
	}
	// --no-verify on the seed commit so the hook itself doesn't bootstrap the seed.
	if _, err := run(t, tmp, nil, "git", "commit", "-q", "--no-verify", "-m", "seed"); err != nil {
		t.Fatalf("seed commit: %v", err)
	}

	return tmp
}

// --- STATIC SCRIPT CONTENT CHECKS (Scenarios 3, 4, 6) ---

func TestPreCommit_ScriptContainsCGOEnabledZero(t *testing.T) {
	if !strings.Contains(readHook(t), "CGO_ENABLED=0") {
		t.Error(".githooks/pre-commit must run the lint pass with CGO_ENABLED=0 (Scenario 6)")
	}
}

func TestPreCommit_ScriptContainsNewFromRevHEAD(t *testing.T) {
	if !strings.Contains(readHook(t), "--new-from-rev=HEAD") {
		t.Error(".githooks/pre-commit must use --new-from-rev=HEAD so baseline lint warnings don't block commits (Scenarios 3, 4)")
	}
}

func TestPreCommit_ScriptIsExecutable(t *testing.T) {
	info, err := os.Stat(hookPath(t))
	if err != nil {
		t.Fatalf("stat hook: %v", err)
	}
	mode := info.Mode().Perm()
	if mode&0o111 == 0 {
		t.Errorf(".githooks/pre-commit must be executable; got mode %o", mode)
	}
}

// --- FAST EXIT (Scenario 2) ---

// TestPreCommit_NoGoFilesFastExit confirms that committing only non-Go
// files does not invoke gofmt or `go run golangci-lint`. We layer a
// stub PATH that shadows gofmt and go with binaries that touch marker
// files — if the hook ever calls them on a no-Go commit, the markers
// appear and the test fails.
func TestPreCommit_NoGoFilesFastExit(t *testing.T) {
	tmp := initRepo(t)

	if err := os.WriteFile(filepath.Join(tmp, "notes.md"), []byte("# notes\n"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	if _, err := run(t, tmp, nil, "git", "add", "notes.md"); err != nil {
		t.Fatalf("git add: %v", err)
	}

	stubDir := t.TempDir()
	markerDir := t.TempDir()
	for _, name := range []string{"gofmt", "go"} {
		stub := "#!/bin/sh\ntouch " + filepath.Join(markerDir, name+"-invoked") + "\nexit 1\n"
		if err := os.WriteFile(filepath.Join(stubDir, name), []byte(stub), 0o755); err != nil {
			t.Fatalf("write stub %s: %v", name, err)
		}
	}
	// Prepend stub dir to PATH so it takes precedence over the real binaries.
	env := []string{"PATH=" + stubDir + ":" + os.Getenv("PATH")}

	out, err := run(t, tmp, env, "git", "commit", "-q", "-m", "docs")
	if err != nil {
		t.Fatalf("commit should have succeeded via fast-exit path; got error: %v\noutput:\n%s", err, out)
	}
	for _, name := range []string{"gofmt", "go"} {
		if _, err := os.Stat(filepath.Join(markerDir, name+"-invoked")); err == nil {
			t.Errorf("fast-exit path was supposed to skip %s, but the stub was invoked", name)
		}
	}
}

// --- HAPPY PATH: gofmt + re-stage (Scenario 1) ---

// TestPreCommit_GofmtRewritesAndRestages ensures the hook formats a
// poorly-formatted staged file in place and re-adds it before the
// commit snapshot is taken.
func TestPreCommit_GofmtRewritesAndRestages(t *testing.T) {
	tmp := initRepo(t)

	// `gofmt` reformats many things, but the cheapest reproducible
	// difference is a tab-indented body where source has spaces.
	unformatted := "package main\n\nfunc x()    {\n  return\n}\n"
	if err := os.WriteFile(filepath.Join(tmp, "formatme.go"), []byte(unformatted), 0o644); err != nil {
		t.Fatalf("write formatme: %v", err)
	}
	if _, err := run(t, tmp, nil, "git", "add", "formatme.go"); err != nil {
		t.Fatalf("git add: %v", err)
	}

	// This test exercises gofmt + re-stage, which the other tests
	// (BlocksNewViolation, AllowsBaselineViolation) do not. The lint
	// pass is exercised by those gated tests, so here we always strip
	// it out — that keeps default `go test ./...` fast and dodges
	// false positives like the linter flagging `func x()` as unused.
	patchHookSkipLint(t, tmp)

	out, err := run(t, tmp, nil, "git", "commit", "-q", "-m", "add formatme")
	if err != nil {
		t.Fatalf("commit failed: %v\noutput:\n%s", err, out)
	}

	// The committed snapshot must be gofmt-clean.
	got, err := run(t, tmp, nil, "git", "show", "HEAD:formatme.go")
	if err != nil {
		t.Fatalf("git show: %v", err)
	}
	cmd := exec.Command("gofmt")
	cmd.Stdin = strings.NewReader(got)
	expected, err := cmd.Output()
	if err != nil {
		t.Fatalf("gofmt on committed content: %v", err)
	}
	if string(expected) != got {
		t.Errorf("committed snapshot was not gofmt-clean.\ngot:\n%s\nexpected (after gofmt):\n%s", got, string(expected))
	}
}

// patchHookSkipLint rewrites the hook in the temp repo to remove the
// `go run golangci-lint ...` line. Used by gofmt-only tests so we don't
// pay the cost of downloading golangci-lint on every `go test ./...`.
func patchHookSkipLint(t *testing.T, repo string) {
	t.Helper()
	dst := filepath.Join(repo, ".git", "hooks", "pre-commit")
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read installed hook: %v", err)
	}
	lines := strings.Split(string(b), "\n")
	out := make([]string, 0, len(lines))
	skip := false
	for _, ln := range lines {
		if strings.Contains(ln, "golangci-lint") {
			skip = true
			continue
		}
		if skip && strings.HasPrefix(strings.TrimSpace(ln), "run ") {
			// continuation line of the backslash-broken command
			skip = false
			continue
		}
		skip = false
		out = append(out, ln)
	}
	if err := os.WriteFile(dst, []byte(strings.Join(out, "\n")), 0o755); err != nil {
		t.Fatalf("rewrite hook: %v", err)
	}
}

// --- BLOCK ON NEW VIOLATION (Scenario 3) ---

// TestPreCommit_BlocksNewViolation introduces a new unused-import and
// expects the hook to abort. Gated by MOTES_HOOK_LINT_TESTS=1 because
// it downloads golangci-lint on first run.
func TestPreCommit_BlocksNewViolation(t *testing.T) {
	if os.Getenv("MOTES_HOOK_LINT_TESTS") != "1" {
		t.Skip("set MOTES_HOOK_LINT_TESTS=1 to run lint-dependent tests")
	}
	tmp := initRepo(t)

	bad := "package main\n\nimport \"fmt\"\n\nfunc bad() {}\n"
	if err := os.WriteFile(filepath.Join(tmp, "bad.go"), []byte(bad), 0o644); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	if _, err := run(t, tmp, nil, "git", "add", "bad.go"); err != nil {
		t.Fatalf("git add: %v", err)
	}

	out, err := run(t, tmp, nil, "git", "commit", "-m", "introduce bad")
	if err == nil {
		t.Fatalf("expected commit to fail on new lint violation; output:\n%s", out)
	}
	low := strings.ToLower(out)
	if !strings.Contains(low, "unused") && !strings.Contains(low, "imported and not used") {
		t.Errorf("expected unused-import diagnostic in output, got:\n%s", out)
	}
}

// --- ALLOW BASELINE VIOLATION (Scenario 4) ---

// TestPreCommit_AllowsBaselineViolation seeds a baseline violation
// into HEAD via --no-verify, then touches the file with a benign edit
// and expects the hook to allow the commit. Gated by
// MOTES_HOOK_LINT_TESTS=1.
func TestPreCommit_AllowsBaselineViolation(t *testing.T) {
	if os.Getenv("MOTES_HOOK_LINT_TESTS") != "1" {
		t.Skip("set MOTES_HOOK_LINT_TESTS=1 to run lint-dependent tests")
	}
	tmp := initRepo(t)

	// Pre-existing ineffassign violation: `x := 1` is overwritten before
	// use. This is a regular lint diagnostic (not a typecheck error),
	// so `--new-from-rev=HEAD` filters it out for any future commit
	// whose diff doesn't add new lines on the same range.
	baseline := "package main\n\nfunc legacy() int {\n\tx := 1\n\tx = 2\n\treturn x\n}\n"
	if err := os.WriteFile(filepath.Join(tmp, "legacy.go"), []byte(baseline), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	if _, err := run(t, tmp, nil, "git", "add", "legacy.go"); err != nil {
		t.Fatalf("git add legacy: %v", err)
	}
	if _, err := run(t, tmp, nil, "git", "commit", "-q", "--no-verify", "-m", "seed baseline"); err != nil {
		t.Fatalf("baseline commit: %v", err)
	}

	// Benign edit: add a comment line at the bottom. The baseline
	// violation on the `x := 1` line is unchanged; the diff only
	// adds a comment line, so no NEW violations are introduced.
	touched := "package main\n\nfunc legacy() int {\n\tx := 1\n\tx = 2\n\treturn x\n}\n\n// touched\n"
	if err := os.WriteFile(filepath.Join(tmp, "legacy.go"), []byte(touched), 0o644); err != nil {
		t.Fatalf("touch legacy: %v", err)
	}
	if _, err := run(t, tmp, nil, "git", "add", "legacy.go"); err != nil {
		t.Fatalf("git add touched: %v", err)
	}

	out, err := run(t, tmp, nil, "git", "commit", "-m", "touch comment")
	if err != nil {
		t.Fatalf("expected commit to succeed despite pre-existing baseline violation; got error: %v\noutput:\n%s", err, out)
	}
}

// TestMakeInstall_TriggersInstallHooks verifies the story's
// Scenario 5 literally — that running `make install` (not just
// `make install-hooks`) wires the hook. We use `make -n` (dry run)
// so the test doesn't need a build of the `mote` binary or write
// access to ~/.local/bin/.
func TestMakeInstall_TriggersInstallHooks(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not available")
	}
	makefileBytes, err := os.ReadFile(filepath.Join(repoRoot(t), "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	tmp := t.TempDir()
	if _, err := run(t, tmp, nil, "git", "init", "-q", "-b", "main"); err != nil {
		t.Skipf("git init failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "Makefile"), makefileBytes, 0o644); err != nil {
		t.Fatalf("write Makefile: %v", err)
	}

	out, err := run(t, tmp, nil, "make", "-n", "install")
	if err != nil {
		t.Fatalf("make -n install: %v\n%s", err, out)
	}
	if !strings.Contains(out, "core.hooksPath .githooks") {
		t.Errorf("`make install` must transitively run install-hooks; dry-run output did not mention `core.hooksPath .githooks`:\n%s", out)
	}
}

// --- MAKEFILE install-hooks WIRES core.hooksPath (Scenario 5) ---

// TestMakeInstall_WiresHooksPath drives the real Makefile from a temp
// git checkout that contains only the Makefile and a .git/ dir. After
// `make install-hooks`, `git config --get core.hooksPath` must return
// `.githooks`.
//
// Sub-tests cover: fresh repo, idempotent re-run, foreign-value
// override.
func TestMakeInstall_WiresHooksPath(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not available")
	}

	makefileBytes, err := os.ReadFile(filepath.Join(repoRoot(t), "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}

	cases := []struct {
		name     string
		existing string
	}{
		{name: "fresh repo with no override", existing: ""},
		{name: "idempotent re-run with .githooks already set", existing: ".githooks"},
		{name: "overrides foreign hooksPath", existing: "/some/other/path"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			if _, err := run(t, tmp, nil, "git", "init", "-q", "-b", "main"); err != nil {
				t.Skipf("git init failed: %v", err)
			}
			if err := os.WriteFile(filepath.Join(tmp, "Makefile"), makefileBytes, 0o644); err != nil {
				t.Fatalf("write Makefile: %v", err)
			}
			if tc.existing != "" {
				if _, err := run(t, tmp, nil, "git", "config", "core.hooksPath", tc.existing); err != nil {
					t.Fatalf("seed existing hooksPath: %v", err)
				}
			}

			if out, err := run(t, tmp, nil, "make", "install-hooks"); err != nil {
				t.Fatalf("make install-hooks failed: %v\n%s", err, out)
			}

			got, err := run(t, tmp, nil, "git", "config", "--get", "core.hooksPath")
			if err != nil {
				t.Fatalf("git config --get core.hooksPath failed: %v\n%s", err, got)
			}
			got = strings.TrimSpace(got)
			if got != ".githooks" {
				t.Errorf("core.hooksPath = %q, want %q", got, ".githooks")
			}

			// Idempotency: a second run should leave the value unchanged.
			if out, err := run(t, tmp, nil, "make", "install-hooks"); err != nil {
				t.Fatalf("make install-hooks (re-run) failed: %v\n%s", err, out)
			}
			got2, _ := run(t, tmp, nil, "git", "config", "--get", "core.hooksPath")
			if strings.TrimSpace(got2) != ".githooks" {
				t.Errorf("after re-run, core.hooksPath = %q, want %q", strings.TrimSpace(got2), ".githooks")
			}
		})
	}
}

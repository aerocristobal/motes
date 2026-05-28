// Shared helpers for the script-exec tests (check_test.go, bump_test.go).
// Mirrors the repoRoot + run pattern from internal/ci/lint_actions_test.go
// so script behavior is validated under `go test ./...` without a separate
// shell-test framework.
//
// Story: STORY-VERSIONS-001.
package version_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// repoRoot walks up from this test file's directory until it finds the
// repo root (the directory containing scripts/check-versions.sh).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "check-versions.sh")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root from " + file)
		}
		dir = parent
	}
}

// runCheck invokes scripts/check-versions.sh against testRoot and returns
// (exitCode, combinedOutput).
func runCheck(t *testing.T, testRoot string) (int, string) {
	t.Helper()
	script := filepath.Join(repoRoot(t), "scripts", "check-versions.sh")
	return runScript(t, script, testRoot)
}

// runBump invokes scripts/bump-version.sh against testRoot with --root and
// any additional args, returning (exitCode, combinedOutput).
func runBump(t *testing.T, testRoot string, args ...string) (int, string) {
	t.Helper()
	script := filepath.Join(repoRoot(t), "scripts", "bump-version.sh")
	full := append([]string{"--root", testRoot}, args...)
	return runScript(t, script, full...)
}

func runScript(t *testing.T, script string, args ...string) (int, string) {
	t.Helper()
	cmdArgs := append([]string{script}, args...)
	cmd := exec.Command("bash", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), string(out)
	}
	t.Fatalf("invoke %s: %v", script, err)
	return -1, ""
}

// writeCanonical writes a minimal internal/version/version.go under root
// declaring `const Value = "<ver>"`.
func writeCanonical(t *testing.T, root, ver string) {
	t.Helper()
	dir := filepath.Join(root, "internal", "version")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "package version\n\nconst Value = \"" + ver + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, "version.go"), []byte(body), 0o644); err != nil {
		t.Fatalf("write version.go: %v", err)
	}
}

// writeVHistory writes docs/version-history.md verbatim under root.
func writeVHistory(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, "docs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "version-history.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write version-history.md: %v", err)
	}
}

// readFile reads p, failing the test on error.
func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

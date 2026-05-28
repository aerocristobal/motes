// Package ci_test exercises CI infrastructure: the workflow YAML declaration
// (workflow_test.go) and the SHA-pinning lint script (this file).
//
// No production Go code lives in internal/ci/ — it exists solely to host
// these integration tests so `scripts/lint-actions-pinning.sh` and
// `.github/workflows/ci.yml` are validated under `go test ./...`. Mirrors
// the internal/githook test-only package pattern.
//
// Story: STORY-CIHYG-001 — CI hygiene patterns.
package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// repoRoot walks up from this test file's directory until it finds the
// repo root (the directory containing scripts/lint-actions-pinning.sh).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "lint-actions-pinning.sh")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root from " + file)
		}
		dir = parent
	}
}

// runLint invokes the lint script against testRoot and returns the exit
// code along with combined stdout+stderr.
func runLint(t *testing.T, testRoot string) (int, string) {
	t.Helper()
	script := filepath.Join(repoRoot(t), "scripts", "lint-actions-pinning.sh")
	cmd := exec.Command("bash", script, testRoot)
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

// writeWorkflow stages a fixture workflow file under <root>/.github/workflows/ci.yml.
func writeWorkflow(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write ci.yml: %v", err)
	}
}

// --- HAPPY PATH (Scenarios 4, 6) ---

func TestLint_PassesForShaWithTagComment(t *testing.T) {
	root := t.TempDir()
	writeWorkflow(t, root, `jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd # v6.0.2
      - uses: actions/setup-go@4a3601121dd01d1626a1e23e37211e3254c1c06c # v6.4.0
`)
	code, out := runLint(t, root)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output:\n%s", code, out)
	}
}

// --- ERROR PATH (Scenario 5) ---

func TestLint_FailsForTagPin(t *testing.T) {
	root := t.TempDir()
	writeWorkflow(t, root, `jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`)
	code, out := runLint(t, root)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d; output:\n%s", code, out)
	}
	if !strings.Contains(out, "ci.yml") {
		t.Errorf("expected output to mention ci.yml; output:\n%s", out)
	}
	if !strings.Contains(out, "actions/checkout@v4") {
		t.Errorf("expected output to mention the offending uses: ref; output:\n%s", out)
	}
}

func TestLint_ReportsFileAndLineNumber(t *testing.T) {
	root := t.TempDir()
	writeWorkflow(t, root, `jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`)
	code, out := runLint(t, root)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d; output:\n%s", code, out)
	}
	if !regexp.MustCompile(`ci\.yml:\d+:`).MatchString(out) {
		t.Errorf("expected file:line in output; got:\n%s", out)
	}
}

// --- BOUNDARY (Scenario 6: first-party gets the same treatment) ---

func TestLint_FailsForFirstPartyTagPin(t *testing.T) {
	root := t.TempDir()
	writeWorkflow(t, root, `jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
`)
	code, out := runLint(t, root)
	if code != 1 {
		t.Fatalf("expected exit 1 (first-party must be SHA-pinned too), got %d; output:\n%s", code, out)
	}
}

// --- ERROR: tag comment required alongside the SHA ---

func TestLint_FailsForShaWithoutTagComment(t *testing.T) {
	root := t.TempDir()
	writeWorkflow(t, root, `jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd
`)
	code, out := runLint(t, root)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d; output:\n%s", code, out)
	}
	if !strings.Contains(out, "missing tag comment") {
		t.Errorf("expected 'missing tag comment' in output; got:\n%s", out)
	}
}

// --- OPERATIONAL: no workflow files present ---

func TestLint_ExitsTwoWhenNoWorkflowFiles(t *testing.T) {
	root := t.TempDir()
	// No .github/workflows/ at all.
	code, out := runLint(t, root)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d; output:\n%s", code, out)
	}
}

func TestLint_ExitsTwoWhenWorkflowsDirIsEmpty(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".github", "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	code, out := runLint(t, root)
	if code != 2 {
		t.Fatalf("expected exit 2 for empty workflows dir, got %d; output:\n%s", code, out)
	}
}

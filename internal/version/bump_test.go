// Exec tests for scripts/bump-version.sh. Mirrors STORY-VERSIONS-001
// Scenarios 3, 4, 5, 6 plus Q5 (pre-release acceptance).
package version_test

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// gitRun runs `git <args>` in dir, failing the test on non-zero exit.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// gitInit makes root a git repo with a stable identity and no signing.
func gitInit(t *testing.T, root string) {
	t.Helper()
	gitRun(t, root, "init", "-q", "-b", "master")
	gitRun(t, root, "config", "user.email", "test@example.invalid")
	gitRun(t, root, "config", "user.name", "Test")
	gitRun(t, root, "config", "commit.gpgsign", "false")
}

func gitCommitCount(t *testing.T, dir string) int {
	t.Helper()
	out := strings.TrimSpace(gitRun(t, dir, "rev-list", "--count", "HEAD"))
	n, err := strconv.Atoi(out)
	if err != nil {
		t.Fatalf("parse commit count %q: %v", out, err)
	}
	return n
}

// setupBumpRepo creates a fresh git repo with canonical=ver and a minimal
// docs/version-history.md whose head bullet matches ver.
func setupBumpRepo(t *testing.T, ver string) string {
	t.Helper()
	root := t.TempDir()
	gitInit(t, root)
	writeCanonical(t, root, ver)
	writeVHistory(t, root, "# History\n\n## Version History\n\n- **v"+ver+"** — initial.\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "initial")
	return root
}

// --- HAPPY PATH (Scenario 3) ---

func TestBump_RewritesEveryTrackedLocation(t *testing.T) {
	root := setupBumpRepo(t, "0.4.37")
	code, out := runBump(t, root, "0.4.38")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output:\n%s", code, out)
	}
	canon := readFile(t, filepath.Join(root, "internal", "version", "version.go"))
	if !strings.Contains(canon, `const Value = "0.4.38"`) {
		t.Errorf("canonical not rewritten:\n%s", canon)
	}
	vh := readFile(t, filepath.Join(root, "docs", "version-history.md"))
	if !strings.Contains(vh, "- **v0.4.38**") {
		t.Errorf("version-history did not gain v0.4.38 bullet:\n%s", vh)
	}
	if !strings.Contains(vh, "- **v0.4.37**") {
		t.Errorf("version-history lost the previous entry:\n%s", vh)
	}
	// New bullet must appear BEFORE the previous head bullet.
	if strings.Index(vh, "- **v0.4.38**") > strings.Index(vh, "- **v0.4.37**") {
		t.Errorf("new bullet was not inserted at the head:\n%s", vh)
	}
}

// After a bump, the check script must pass against the same tree.

func TestBump_FollowedByCheckPasses(t *testing.T) {
	root := setupBumpRepo(t, "0.4.37")
	if code, out := runBump(t, root, "0.4.38"); code != 0 {
		t.Fatalf("bump failed: %d\n%s", code, out)
	}
	if code, out := runCheck(t, root); code != 0 {
		t.Fatalf("check after bump failed: %d\n%s", code, out)
	}
}

// --- ALT (Scenario 4) ---

func TestBump_CommitCreatesExactlyOneCommitWithNoTag(t *testing.T) {
	root := setupBumpRepo(t, "0.4.37")
	before := gitCommitCount(t, root)
	code, out := runBump(t, root, "0.4.38", "--commit")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output:\n%s", code, out)
	}
	if got := gitCommitCount(t, root) - before; got != 1 {
		t.Errorf("expected exactly 1 new commit, got %d", got)
	}
	msg := gitRun(t, root, "log", "-1", "--format=%B")
	if !strings.Contains(msg, "0.4.38") {
		t.Errorf("commit message does not mention 0.4.38: %q", msg)
	}
	if tags := strings.TrimSpace(gitRun(t, root, "tag", "--list")); tags != "" {
		t.Errorf("expected no tags after --commit; got %q", tags)
	}
	if status := strings.TrimSpace(gitRun(t, root, "status", "--porcelain")); status != "" {
		t.Errorf("expected clean working tree after --commit; got:\n%s", status)
	}
	// Scenario 4 final clause: the commit's diff contains every updated
	// location and nothing else. `git show --name-only` lists exactly the
	// files in the latest commit.
	changed := strings.Fields(gitRun(t, root, "show", "--name-only", "--format=", "HEAD"))
	want := map[string]bool{
		"internal/version/version.go": true,
		"docs/version-history.md":     true,
	}
	if len(changed) != len(want) {
		t.Fatalf("expected commit to touch exactly %d files; got %d: %v", len(want), len(changed), changed)
	}
	for _, f := range changed {
		if !want[f] {
			t.Errorf("unexpected file in commit diff: %s", f)
		}
	}
}

// --- ERROR PATH (Scenario 5) ---

func TestBump_NonSemverRejectedTreeUnchanged(t *testing.T) {
	root := setupBumpRepo(t, "0.4.37")
	code, out := runBump(t, root, "banana")
	if code == 0 {
		t.Fatalf("expected non-zero exit for non-semver, got 0; output:\n%s", out)
	}
	if !strings.Contains(out, "semver") && !strings.Contains(out, "MAJOR.MINOR.PATCH") {
		t.Errorf("expected semver-related error message; got:\n%s", out)
	}
	canon := readFile(t, filepath.Join(root, "internal", "version", "version.go"))
	if !strings.Contains(canon, `const Value = "0.4.37"`) {
		t.Errorf("working tree changed despite rejection:\n%s", canon)
	}
	if status := strings.TrimSpace(gitRun(t, root, "status", "--porcelain")); status != "" {
		t.Errorf("git status not clean after rejection: %q", status)
	}
}

// --- BUSINESS RULE (Scenario 6) ---

func TestBump_DowngradeRejectedByDefault(t *testing.T) {
	root := setupBumpRepo(t, "0.4.37")
	code, out := runBump(t, root, "0.4.36")
	if code == 0 {
		t.Fatalf("expected non-zero exit for downgrade, got 0; output:\n%s", out)
	}
	if !strings.Contains(out, "downgrade") && !strings.Contains(out, "--allow-downgrade") {
		t.Errorf("expected downgrade-related error; got:\n%s", out)
	}
	canon := readFile(t, filepath.Join(root, "internal", "version", "version.go"))
	if !strings.Contains(canon, `const Value = "0.4.37"`) {
		t.Errorf("working tree changed despite rejection:\n%s", canon)
	}
}

func TestBump_DowngradeOverridable(t *testing.T) {
	root := setupBumpRepo(t, "0.4.37")
	code, out := runBump(t, root, "0.4.36", "--allow-downgrade")
	if code != 0 {
		t.Fatalf("expected exit 0 with --allow-downgrade, got %d; output:\n%s", code, out)
	}
	canon := readFile(t, filepath.Join(root, "internal", "version", "version.go"))
	if !strings.Contains(canon, `const Value = "0.4.36"`) {
		t.Errorf("canonical not downgraded:\n%s", canon)
	}
}

// --- Q5: pre-release accepted ---

func TestBump_PreReleaseAccepted(t *testing.T) {
	root := setupBumpRepo(t, "0.4.37")
	code, out := runBump(t, root, "0.5.0-beta.1")
	if code != 0 {
		t.Fatalf("expected exit 0 for pre-release, got %d; output:\n%s", code, out)
	}
	canon := readFile(t, filepath.Join(root, "internal", "version", "version.go"))
	if !strings.Contains(canon, `const Value = "0.5.0-beta.1"`) {
		t.Errorf("pre-release not written:\n%s", canon)
	}
}

// --- ERROR: no-op (same version) rejected ---

func TestBump_RejectsNoOp(t *testing.T) {
	root := setupBumpRepo(t, "0.4.37")
	code, out := runBump(t, root, "0.4.37")
	if code == 0 {
		t.Fatalf("expected non-zero for no-op bump, got 0; output:\n%s", out)
	}
}

// --- ERROR: missing canonical file is exit 2 (validation) ---

func TestBump_MissingCanonicalFileExitsTwo(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	writeVHistory(t, root, "## Version History\n\n- **v0.4.37** — x.\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "initial")
	code, out := runBump(t, root, "0.4.38")
	if code != 2 {
		t.Fatalf("expected exit 2, got %d; output:\n%s", code, out)
	}
}

// Exec tests for scripts/check-versions.sh. Mirrors STORY-VERSIONS-001
// Scenarios 1, 2, 7, and operational/missing-file behavior.
package version_test

import (
	"regexp"
	"strings"
	"testing"
)

// --- HAPPY PATH (Scenario 1) ---

func TestCheck_PassesWhenHeadEqualsCanonical(t *testing.T) {
	root := t.TempDir()
	writeCanonical(t, root, "0.4.37")
	writeVHistory(t, root, `# History

Intro prose.

## Version History

- **v0.4.37** — current.
- **v0.4.36** — older.
`)
	code, out := runCheck(t, root)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output:\n%s", code, out)
	}
	if !strings.Contains(out, "canonical=0.4.37") {
		t.Errorf("expected canonical reported in success output; got:\n%s", out)
	}
}

// --- ERROR PATH (Scenario 2) ---

func TestCheck_FailsWhenHeadDriftsFromCanonical(t *testing.T) {
	root := t.TempDir()
	writeCanonical(t, root, "0.4.37")
	writeVHistory(t, root, `## Version History

- **v0.4.13** — stale head.
`)
	code, out := runCheck(t, root)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d; output:\n%s", code, out)
	}
	if !strings.Contains(out, "docs/version-history.md") {
		t.Errorf("expected output to mention docs/version-history.md; got:\n%s", out)
	}
	if !strings.Contains(out, "expected 0.4.37") {
		t.Errorf("expected 'expected 0.4.37'; got:\n%s", out)
	}
	if !strings.Contains(out, "found 0.4.13") {
		t.Errorf("expected 'found 0.4.13'; got:\n%s", out)
	}
	if !regexp.MustCompile(`docs/version-history\.md:\d+:`).MatchString(out) {
		t.Errorf("expected file:line: format in output; got:\n%s", out)
	}
}

// --- BOUNDARY (Scenario 7) ---
// Historical entries below the head are not policed: only the first
// matching bullet after the heading is compared.

func TestCheck_HistoricalEntriesNotPoliced(t *testing.T) {
	root := t.TempDir()
	writeCanonical(t, root, "0.4.37")
	writeVHistory(t, root, `## Version History

- **v0.4.37** — current.
- **v0.4.36** — older.
- **v0.4.0** — much older (would be a "drift" if every entry were checked).
`)
	code, out := runCheck(t, root)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output:\n%s", code, out)
	}
}

// --- PRE-RELEASE accepted in regex (Q5) ---

func TestCheck_PreReleaseHeadIsRecognized(t *testing.T) {
	root := t.TempDir()
	writeCanonical(t, root, "0.5.0-beta.1")
	writeVHistory(t, root, `## Version History

- **v0.5.0-beta.1** — pre-release.
- **v0.4.37** — prior.
`)
	code, out := runCheck(t, root)
	if code != 0 {
		t.Fatalf("expected exit 0 for pre-release head; got %d; output:\n%s", code, out)
	}
}

// --- OPERATIONAL: canonical file missing ---

func TestCheck_ExitsTwoWhenCanonicalFileMissing(t *testing.T) {
	root := t.TempDir()
	// only the history file exists
	writeVHistory(t, root, "## Version History\n\n- **v0.4.37** — x.\n")
	code, out := runCheck(t, root)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d; output:\n%s", code, out)
	}
	if !strings.Contains(out, "missing canonical file") {
		t.Errorf("expected 'missing canonical file' in error; got:\n%s", out)
	}
}

// --- ERROR: tracked file missing ---

func TestCheck_FailsWhenTrackedFileMissing(t *testing.T) {
	root := t.TempDir()
	writeCanonical(t, root, "0.4.37")
	// no docs/version-history.md
	code, out := runCheck(t, root)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d; output:\n%s", code, out)
	}
	if !strings.Contains(out, "tracked file not found") {
		t.Errorf("expected 'tracked file not found'; got:\n%s", out)
	}
}

// --- ERROR: heading missing in tracked file ---

func TestCheck_FailsWhenHeadingMissing(t *testing.T) {
	root := t.TempDir()
	writeCanonical(t, root, "0.4.37")
	writeVHistory(t, root, "# Some Other Document\n\nNo Version History section here.\n")
	code, out := runCheck(t, root)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d; output:\n%s", code, out)
	}
	if !strings.Contains(out, "heading 'Version History' not found") {
		t.Errorf("expected heading-missing error; got:\n%s", out)
	}
}

// --- LIVE: the real repo passes against itself ---

func TestCheck_LiveRepoSelfConsistent(t *testing.T) {
	code, out := runCheck(t, repoRoot(t))
	if code != 0 {
		t.Fatalf("live repo failed the version-consistency check: code=%d output:\n%s", code, out)
	}
}

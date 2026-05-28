package ci_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// workflow is the minimal subset of the GitHub Actions workflow schema
// STORY-CIHYG-001 constrains. Other keys are left to the schema's discretion.
type workflow struct {
	Concurrency struct {
		Group            string `yaml:"group"`
		CancelInProgress *bool  `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
}

// loadCIWorkflow parses the live repo-root .github/workflows/ci.yml. The
// tests deliberately read the real file rather than a fixture: if the file
// goes missing or loses its concurrency block, CI should fail.
func loadCIWorkflow(t *testing.T) (workflow, []byte) {
	t.Helper()
	path := filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}
	var wf workflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("parse ci.yml: %v", err)
	}
	return wf, raw
}

// --- Scenario 1: concurrency cancellation declaration ---

func TestConcurrency_HasGroupAndCancelInProgress(t *testing.T) {
	wf, _ := loadCIWorkflow(t)
	if wf.Concurrency.Group == "" {
		t.Fatal("concurrency.group must be non-empty")
	}
	if wf.Concurrency.CancelInProgress == nil || !*wf.Concurrency.CancelInProgress {
		t.Fatal("concurrency.cancel-in-progress must be the boolean true")
	}
}

// --- Scenarios 2, 3: group key keeps independent runs in distinct groups ---

func TestConcurrency_GroupKeyDistinguishesPRsFromBranches(t *testing.T) {
	wf, _ := loadCIWorkflow(t)
	mustContain := []string{
		"github.workflow",                  // distinct workflows do not cancel each other
		"github.event_name",                // push vs pull_request do not collapse (Scenario 3)
		"github.event.pull_request.number", // per-PR uniqueness (Scenario 2)
		"github.ref",                       // per-branch fallback for non-PR events
	}
	for _, needle := range mustContain {
		if !strings.Contains(wf.Concurrency.Group, needle) {
			t.Errorf("concurrency.group missing %q: got %q", needle, wf.Concurrency.Group)
		}
	}
}

// --- Scenario 4 / 6: live ci.yml itself is SHA-pinned with a tag comment ---
//
// The fixture-driven tests in lint_actions_test.go exercise the lint script
// against synthetic workflows; this test guards against drift in the real
// file even if a future contributor were to bypass the lint job.
func TestWorkflow_AllUsesLinesArePinned(t *testing.T) {
	_, raw := loadCIWorkflow(t)
	// Match `uses:` lines and capture the value plus any trailing comment.
	pinRe := regexp.MustCompile(`@[a-f0-9]{40}\s+#\s+\S`)
	usesRe := regexp.MustCompile(`(?m)^\s*-?\s*uses:\s+(.+)$`)

	matches := usesRe.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatal("expected at least one uses: line in ci.yml")
	}
	for _, m := range matches {
		value := strings.TrimSpace(m[1])
		// Strip surrounding quotes if present.
		value = strings.Trim(value, `"'`)
		// Local composite actions are exempt from SHA-pinning (none today,
		// but keep the test in lockstep with the lint script).
		if strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") {
			continue
		}
		if !pinRe.MatchString(value) {
			t.Errorf("uses: value not SHA-pinned with tag comment: %q", value)
		}
	}
}

package githook_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"gopkg.in/yaml.v3"
)

// precommitConfigPath returns the absolute path to .pre-commit-config.yaml.
func precommitConfigPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), ".pre-commit-config.yaml")
}

type precommitHook struct {
	ID   string   `yaml:"id"`
	Args []string `yaml:"args"`
}

type precommitRepo struct {
	Repo  string          `yaml:"repo"`
	Rev   string          `yaml:"rev"`
	Hooks []precommitHook `yaml:"hooks"`
}

type precommitConfig struct {
	Repos []precommitRepo `yaml:"repos"`
}

func loadPrecommitConfig(t *testing.T) precommitConfig {
	t.Helper()
	b, err := os.ReadFile(precommitConfigPath(t))
	if err != nil {
		t.Fatalf("read .pre-commit-config.yaml: %v", err)
	}
	var cfg precommitConfig
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("parse .pre-commit-config.yaml: %v", err)
	}
	return cfg
}

// --- STRUCTURE (Scenario 7) ---

func TestPrecommitConfig_Exists(t *testing.T) {
	if _, err := os.Stat(precommitConfigPath(t)); err != nil {
		t.Fatalf(".pre-commit-config.yaml must be at repo root (STORY-BR-21): %v", err)
	}
}

func TestPrecommitConfig_AllReposHavePinnedRev(t *testing.T) {
	cfg := loadPrecommitConfig(t)
	if len(cfg.Repos) == 0 {
		t.Fatal(".pre-commit-config.yaml has no repos")
	}
	for _, r := range cfg.Repos {
		if r.Rev == "" {
			t.Errorf("repo %q has no pinned rev (Scenario 7: every repo entry must have a non-empty rev)", r.Repo)
		}
		if len(r.Hooks) == 0 {
			t.Errorf("repo %q declares no hooks", r.Repo)
		}
	}
}

// --- HOOK PRESENCE (Scenarios 1-5) ---

func TestPrecommitConfig_RequiredHooksPresent(t *testing.T) {
	cfg := loadPrecommitConfig(t)
	present := map[string]bool{}
	for _, r := range cfg.Repos {
		for _, h := range r.Hooks {
			present[h.ID] = true
		}
	}
	required := []string{
		"golangci-lint",           // Scenario 1
		"trailing-whitespace",     // Scenario 2
		"end-of-file-fixer",       // Scenario 3
		"check-yaml",              // Scenario 4
		"check-added-large-files", // Scenario 5
	}
	for _, id := range required {
		if !present[id] {
			t.Errorf("required hook %q is missing from .pre-commit-config.yaml", id)
		}
	}
}

// --- DRIFT GUARD ---

// TestPrecommitConfig_GolangciLintMatchesShellHook fails when the
// golangci-lint version pinned in .pre-commit-config.yaml drifts from
// the @vX.Y.Z pin in .githooks/pre-commit. The two files are equivalent
// contributor paths (STORY-BR-02 vs STORY-BR-21); they MUST stay in sync.
func TestPrecommitConfig_GolangciLintMatchesShellHook(t *testing.T) {
	cfg := loadPrecommitConfig(t)

	var configRev string
	for _, r := range cfg.Repos {
		for _, h := range r.Hooks {
			if h.ID == "golangci-lint" {
				configRev = r.Rev
			}
		}
	}
	if configRev == "" {
		t.Fatal("golangci-lint hook not found in .pre-commit-config.yaml")
	}

	shellHook, err := os.ReadFile(filepath.Join(repoRoot(t), ".githooks", "pre-commit"))
	if err != nil {
		t.Fatalf("read .githooks/pre-commit: %v", err)
	}
	m := regexp.MustCompile(`golangci-lint@(v\d+\.\d+\.\d+)`).FindStringSubmatch(string(shellHook))
	if m == nil {
		t.Fatal(".githooks/pre-commit does not contain a `golangci-lint@vX.Y.Z` pin")
	}
	shellRev := m[1]

	if configRev != shellRev {
		t.Errorf("golangci-lint version drift: .pre-commit-config.yaml pins %q but .githooks/pre-commit pins %q — bump both together", configRev, shellRev)
	}
}

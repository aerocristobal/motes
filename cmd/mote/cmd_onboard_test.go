// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/core"
)

func TestMigrateClaudeSettings_BdPrime(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	input := `{
  "hooks": {
    "SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "bd prime"}]}],
    "PreCompact": [{"matcher": "", "hooks": [{"type": "command", "command": "bd prime"}]}]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	migrated, err := migrateClaudeSettings(dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if migrated != 2 {
		t.Errorf("expected 2 migrated hooks, got %d", migrated)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "mote prime") {
		t.Error("expected settings to contain 'mote prime' after migration")
	}
	if strings.Contains(content, "bd prime") {
		t.Error("expected settings to NOT contain 'bd prime' after migration")
	}
}

func TestMigrateClaudeSettings_BdSync(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	input := `{
  "hooks": {
    "SessionEnd": [{"matcher": "", "hooks": [{"type": "command", "command": "bd sync"}]}]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	migrated, err := migrateClaudeSettings(dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if migrated != 1 {
		t.Errorf("expected 1 migrated hook, got %d", migrated)
	}

	data, _ := os.ReadFile(settingsPath)
	if !strings.Contains(string(data), "mote session-end") {
		t.Error("expected settings to contain 'mote session-end' after migration")
	}
}

func TestMigrateClaudeSettings_NoBdHooks(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	input := `{
  "hooks": {
    "SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "mote prime"}]}]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	migrated, err := migrateClaudeSettings(dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if migrated != 0 {
		t.Errorf("expected 0 migrated hooks, got %d", migrated)
	}

	// Verify file was not modified
	data, _ := os.ReadFile(settingsPath)
	if string(data) != input {
		t.Error("expected settings file to be unchanged when no bd hooks found")
	}
}

func TestMigrateClaudeSettings_DryRun(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	input := `{
  "hooks": {
    "SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "bd prime"}]}]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	migrated, err := migrateClaudeSettings(dir, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if migrated != 1 {
		t.Errorf("expected 1 detected hook, got %d", migrated)
	}

	// Verify file was NOT modified (dry run)
	data, _ := os.ReadFile(settingsPath)
	if string(data) != input {
		t.Error("dry run should not modify settings file")
	}
}

func TestMigrateClaudeSettings_NoFile(t *testing.T) {
	dir := t.TempDir()

	migrated, err := migrateClaudeSettings(dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if migrated != 0 {
		t.Errorf("expected 0 migrated hooks for missing file, got %d", migrated)
	}
}

// --- ensureClaudeHooks tests ---

func TestEnsureClaudeHooks_CreatesNew(t *testing.T) {
	dir := t.TempDir()

	if err := ensureClaudeHooks(dir, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	hooks := settings["hooks"].(map[string]interface{})
	if !hookEventHasCommand(hooks, "SessionStart", claudeAgentKindPrefix+"mote prime --hook --mode=startup") {
		t.Error("expected SessionStart hook with startup mode (kind-prefixed)")
	}
	if !hookEventHasCommand(hooks, "PreCompact", claudeAgentKindPrefix+"mote prime --hook --mode=compact") {
		t.Error("expected PreCompact hook with compact mode (kind-prefixed)")
	}
	if !hookEventHasCommand(hooks, "UserPromptSubmit", claudeAgentKindPrefix+"mote prompt-context") {
		t.Error("expected UserPromptSubmit hook with mote prompt-context (kind-prefixed)")
	}
}

func TestEnsureClaudeHooks_MergesExisting(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	existing := `{
  "hooks": {
    "SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "echo hello"}]}]
  },
  "other_key": true
}`
	os.WriteFile(settingsPath, []byte(existing), 0644)

	if err := ensureClaudeHooks(dir, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(settingsPath)
	var settings map[string]interface{}
	json.Unmarshal(data, &settings)

	// Verify other_key preserved
	if settings["other_key"] != true {
		t.Error("expected other_key to be preserved")
	}

	hooks := settings["hooks"].(map[string]interface{})

	// SessionStart should have 5 entries (original + 4 mote prime modes)
	entries := hooks["SessionStart"].([]interface{})
	if len(entries) != 5 {
		t.Errorf("expected 5 SessionStart entries, got %d", len(entries))
	}

	// PreCompact should be added
	if !hookEventHasCommand(hooks, "PreCompact", claudeAgentKindPrefix+"mote prime --hook --mode=compact") {
		t.Error("expected PreCompact hook to be added")
	}

	// Original hook should still be there
	if !hookEventHasCommand(hooks, "SessionStart", "echo hello") {
		t.Error("expected original SessionStart hook to be preserved")
	}
}

func TestEnsureClaudeHooks_Idempotent(t *testing.T) {
	dir := t.TempDir()

	// First call
	ensureClaudeHooks(dir, false)
	data1, _ := os.ReadFile(filepath.Join(dir, "settings.json"))

	// Second call
	ensureClaudeHooks(dir, false)
	data2, _ := os.ReadFile(filepath.Join(dir, "settings.json"))

	if string(data1) != string(data2) {
		t.Error("expected idempotent behavior — file changed on second call")
	}
}

// --- ensureMoteSkills tests ---

func TestEnsureMoteSkills_CreatesNew(t *testing.T) {
	dir := t.TempDir()

	if err := ensureMoteSkills(dir, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, name := range []string{"mote-capture", "mote-retrieve"} {
		path := filepath.Join(dir, ".claude", "skills", name, "SKILL.md")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected %s to be created", path)
		}
		data, _ := os.ReadFile(path)
		if len(data) == 0 {
			t.Errorf("expected non-empty skill file for %s", name)
		}
	}
}

func TestEnsureMoteSkills_Idempotent(t *testing.T) {
	dir := t.TempDir()

	ensureMoteSkills(dir, false)

	capturePath := filepath.Join(dir, ".claude", "skills", "mote-capture", "SKILL.md")
	data1, _ := os.ReadFile(capturePath)

	ensureMoteSkills(dir, false)
	data2, _ := os.ReadFile(capturePath)

	if string(data1) != string(data2) {
		t.Error("expected skill content to be unchanged on second call")
	}
}

// --- cleanup flag test ---

func TestOnboard_CleanupFlag(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Create a fake .beads directory
	cwd, _ := os.Getwd()
	beadsDir := filepath.Join(cwd, ".beads")
	os.MkdirAll(beadsDir, 0755)
	issueData := `{"id":"test-1","title":"Test issue","status":"open","priority":1,"issue_type":"task"}`
	os.WriteFile(filepath.Join(beadsDir, "issues.jsonl"), []byte(issueData), 0644)

	onboardDryRun = false
	onboardGlobal = false
	onboardCleanup = true
	onboardIncludeClosed = false
	onboardFrom = "beads"
	defer func() {
		onboardCleanup = false
		onboardFrom = ""
	}()

	if err := runOnboard(onboardCmd, nil); err != nil {
		t.Fatalf("onboard --cleanup: %v", err)
	}

	if dirExists(beadsDir) {
		t.Error("expected .beads/ to be removed with --cleanup")
	}
}

// --- detectSources tests ---

func TestDetectSources_WithBeadsAndMD(t *testing.T) {
	dir := t.TempDir()

	// Create MEMORY.md
	os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte("# Test\nsome content"), 0644)

	// Create .beads/issues.jsonl
	beadsDir := filepath.Join(dir, ".beads")
	os.MkdirAll(beadsDir, 0755)
	issues := `{"id":"1","title":"Open issue","status":"open","priority":1,"issue_type":"task"}
{"id":"2","title":"Closed issue","status":"closed","priority":2,"issue_type":"bug"}`
	os.WriteFile(filepath.Join(beadsDir, "issues.jsonl"), []byte(issues), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	d := detectSources(dir)

	if d.memoryMDPath == "" {
		t.Error("expected MEMORY.md to be detected")
	}
	if len(d.beadsIssues) != 2 {
		t.Errorf("expected 2 beads issues, got %d", len(d.beadsIssues))
	}
	if d.openBeads != 1 {
		t.Errorf("expected 1 open bead, got %d", d.openBeads)
	}
	if d.closedBeads != 1 {
		t.Errorf("expected 1 closed bead, got %d", d.closedBeads)
	}
}

func TestDetectSources_Empty(t *testing.T) {
	dir := t.TempDir()

	d := detectSources(dir)

	if d.memoryMDPath != "" {
		t.Error("expected no MEMORY.md")
	}
	if len(d.beadsIssues) != 0 {
		t.Error("expected no beads issues")
	}
	if d.memoryDirExists {
		t.Error("expected .memory/ to not exist")
	}
	if d.claudeHasMotes {
		t.Error("expected CLAUDE.md to not have motes")
	}
}

// --- buildMenu tests ---

func TestBuildMenu_AllDetected(t *testing.T) {
	d := sourceDetection{
		beadsIssues:  []beadsIssue{{ID: "1", Status: "open"}},
		openBeads:    1,
		memoryMDPath: "/tmp/MEMORY.md",
		ghAvailable:  true,
	}

	opts := buildMenu(d)

	if len(opts) != 4 {
		t.Fatalf("expected 4 options, got %d", len(opts))
	}
	if opts[0].source != sourceMarkdown {
		t.Errorf("expected first option to be markdown, got %s", opts[0].source)
	}
	if opts[1].source != sourceBeads {
		t.Errorf("expected second option to be beads, got %s", opts[1].source)
	}
	if opts[2].source != sourceGithub {
		t.Errorf("expected third option to be github, got %s", opts[2].source)
	}
	if opts[3].source != sourceFresh {
		t.Errorf("expected last option to be fresh, got %s", opts[3].source)
	}
}

func TestBuildMenu_NoneDetected(t *testing.T) {
	d := sourceDetection{}

	opts := buildMenu(d)

	if len(opts) != 1 {
		t.Fatalf("expected 1 option (fresh only), got %d", len(opts))
	}
	if opts[0].source != sourceFresh {
		t.Errorf("expected only option to be fresh, got %s", opts[0].source)
	}
}

// --- promptSelection tests ---

func TestPromptSelection_Valid(t *testing.T) {
	opts := []menuOption{
		{source: sourceMarkdown, label: "Markdown", description: "desc1"},
		{source: sourceBeads, label: "Beads", description: "desc2"},
		{source: sourceFresh, label: "Fresh", description: "desc3"},
	}

	chosen, err := promptSelection(strings.NewReader("2\n"), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chosen.source != sourceBeads {
		t.Errorf("expected beads, got %s", chosen.source)
	}
}

func TestPromptSelection_OutOfRange(t *testing.T) {
	opts := []menuOption{
		{source: sourceFresh, label: "Fresh", description: "desc"},
	}

	_, err := promptSelection(strings.NewReader("5\n"), opts)
	if err == nil {
		t.Error("expected error for out-of-range choice")
	}
	if !strings.Contains(err.Error(), "invalid choice") {
		t.Errorf("expected 'invalid choice' in error, got: %v", err)
	}
}

func TestPromptSelection_NonNumeric(t *testing.T) {
	opts := []menuOption{
		{source: sourceFresh, label: "Fresh", description: "desc"},
	}

	_, err := promptSelection(strings.NewReader("abc\n"), opts)
	if err == nil {
		t.Error("expected error for non-numeric input")
	}
	if !strings.Contains(err.Error(), "not a number") {
		t.Errorf("expected 'not a number' in error, got: %v", err)
	}
}

// --- promptRepo tests ---

func TestPromptRepo_Valid(t *testing.T) {
	repo, err := promptRepo(strings.NewReader("owner/repo\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo != "owner/repo" {
		t.Errorf("expected owner/repo, got %s", repo)
	}
}

func TestPromptRepo_Invalid(t *testing.T) {
	_, err := promptRepo(strings.NewReader("noslash\n"))
	if err == nil {
		t.Error("expected error for invalid repo format")
	}
	if !strings.Contains(err.Error(), "invalid repo format") {
		t.Errorf("expected 'invalid repo format' in error, got: %v", err)
	}
}

// --- runMigrateMarkdown test ---

func TestRunMigrateMarkdown(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	cwd, _ := os.Getwd()
	root := filepath.Join(cwd, ".memory")

	// Create a MEMORY.md with sections
	mdPath := filepath.Join(cwd, "MEMORY.md")
	content := "## Decisions\nWe chose Go for speed.\n\n## Lessons\nAlways test first.\n"
	os.WriteFile(mdPath, []byte(content), 0644)

	mm := newTestMoteManager(root)

	created, err := runMigrateMarkdown(mm, mdPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created == 0 {
		t.Error("expected at least 1 mote created")
	}

	// Check original was archived
	if _, err := os.Stat(mdPath); !os.IsNotExist(err) {
		t.Error("expected MEMORY.md to be archived (renamed)")
	}
}

// --- runMigrateBeads test ---

func TestRunMigrateBeads(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	cwd, _ := os.Getwd()
	root := filepath.Join(cwd, ".memory")
	mm := newTestMoteManager(root)

	issues := []beadsIssue{
		{ID: "b1", Title: "Open task", Status: "open", Priority: 1, IssueType: "task", Description: "do stuff"},
		{ID: "b2", Title: "Closed bug", Status: "closed", Priority: 2, IssueType: "bug", Description: "was broken"},
	}

	// Without includeClosed
	created, err := runMigrateBeads(mm, issues, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created != 1 {
		t.Errorf("expected 1 created (open only), got %d", created)
	}

	// With includeClosed (b1 already imported, only b2 new)
	created2, err := runMigrateBeads(mm, issues, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created2 != 1 {
		t.Errorf("expected 1 created (closed only, open already imported), got %d", created2)
	}
}

// --- --from flag tests ---

func TestOnboard_FromFresh(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	onboardFrom = "fresh"
	onboardDryRun = false
	onboardGlobal = false
	onboardIncludeClosed = false
	onboardCleanup = false
	defer func() { onboardFrom = "" }()

	if err := runOnboard(onboardCmd, nil); err != nil {
		t.Fatalf("onboard --from=fresh: %v", err)
	}
}

func TestOnboard_FromInvalid(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	onboardFrom = "bogus"
	onboardDryRun = false
	onboardGlobal = false
	defer func() { onboardFrom = "" }()

	err := runOnboard(onboardCmd, nil)
	if err == nil {
		t.Fatal("expected error for --from=bogus")
	}
	if !strings.Contains(err.Error(), "invalid --from value") {
		t.Errorf("expected 'invalid --from value' in error, got: %v", err)
	}
}

// newTestMoteManager is a helper to create a MoteManager for the given root.
func newTestMoteManager(root string) *core.MoteManager {
	return core.NewMoteManager(root)
}

// --- Codex hooks tests ---

func TestEnsureCodexHooks_CreatesNew(t *testing.T) {
	codexDir := t.TempDir()
	if err := ensureCodexHooks(codexDir, false); err != nil {
		t.Fatalf("ensureCodexHooks: %v", err)
	}

	hooksPath := filepath.Join(codexDir, "hooks.json")
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("hooks.json should be created: %v", err)
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("hooks.json should be valid JSON: %v", err)
	}
	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		t.Fatal("hooks key missing from settings")
	}
	for _, evt := range []string{"SessionStart", "UserPromptSubmit", "Stop"} {
		if _, ok := hooks[evt]; !ok {
			t.Errorf("expected %s hook, missing", evt)
		}
	}

	// Confirm Stop hook carries the explicit 600s timeout (signals the long
	// session-end runtime to anyone reading the file)
	stopEntries := hooks["Stop"].([]interface{})
	stopHook := stopEntries[0].(map[string]interface{})["hooks"].([]interface{})[0].(map[string]interface{})
	if stopHook["timeout"] != float64(600) {
		t.Errorf("Stop hook timeout: got %v, want 600", stopHook["timeout"])
	}
}

func TestEnsureCodexHooks_Idempotent(t *testing.T) {
	codexDir := t.TempDir()
	if err := ensureCodexHooks(codexDir, false); err != nil {
		t.Fatalf("first install: %v", err)
	}
	first, _ := os.ReadFile(filepath.Join(codexDir, "hooks.json"))

	// Second run should produce identical content
	if err := ensureCodexHooks(codexDir, false); err != nil {
		t.Fatalf("second install: %v", err)
	}
	second, _ := os.ReadFile(filepath.Join(codexDir, "hooks.json"))

	if string(first) != string(second) {
		t.Errorf("ensureCodexHooks not idempotent\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestEnsureCodexHooks_PreservesUserConfig(t *testing.T) {
	codexDir := t.TempDir()
	hooksPath := filepath.Join(codexDir, "hooks.json")
	// Pre-existing user config with an unrelated hook
	preexisting := `{
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "/usr/local/bin/audit-bash.sh"}]}
    ]
  }
}`
	if err := os.WriteFile(hooksPath, []byte(preexisting), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ensureCodexHooks(codexDir, false); err != nil {
		t.Fatalf("ensureCodexHooks: %v", err)
	}

	data, _ := os.ReadFile(hooksPath)
	if !strings.Contains(string(data), "audit-bash.sh") {
		t.Error("preexisting PreToolUse hook should survive ensureCodexHooks")
	}
	if !strings.Contains(string(data), "mote prime") {
		t.Error("mote hooks should be added alongside existing ones")
	}
}

func TestEnsureCodexFeatureFlag_CreatesConfig(t *testing.T) {
	codexDir := t.TempDir()
	if err := ensureCodexFeatureFlag(codexDir, false); err != nil {
		t.Fatalf("ensureCodexFeatureFlag: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(codexDir, "config.toml"))
	if err != nil {
		t.Fatalf("config.toml should be created: %v", err)
	}
	if !strings.Contains(string(data), "codex_hooks = true") {
		t.Errorf("config.toml missing feature flag:\n%s", data)
	}
}

func TestEnsureCodexFeatureFlag_AppendsToExistingConfig(t *testing.T) {
	codexDir := t.TempDir()
	configPath := filepath.Join(codexDir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[user]\nname = \"someone\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ensureCodexFeatureFlag(codexDir, false); err != nil {
		t.Fatalf("ensureCodexFeatureFlag: %v", err)
	}
	data, _ := os.ReadFile(configPath)
	if !strings.Contains(string(data), "[user]") {
		t.Error("existing [user] section should survive")
	}
	if !strings.Contains(string(data), "codex_hooks = true") {
		t.Error("feature flag should be appended")
	}
}

func TestEnsureCodexFeatureFlag_NoOpWhenAlreadySet(t *testing.T) {
	codexDir := t.TempDir()
	configPath := filepath.Join(codexDir, "config.toml")
	original := "[features]\ncodex_hooks = true\n"
	if err := os.WriteFile(configPath, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ensureCodexFeatureFlag(codexDir, false); err != nil {
		t.Fatalf("ensureCodexFeatureFlag: %v", err)
	}
	data, _ := os.ReadFile(configPath)
	if string(data) != original {
		t.Errorf("file should be unchanged when flag already set\ngot:\n%s", data)
	}
}

// --- Dual-path skills test ---

func TestEnsureMoteSkills_InstallsAtAgentsPathWhenCodexEnabled(t *testing.T) {
	home := t.TempDir()
	// Pre-create ~/.codex/ to trigger the auto-detect path
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := ensureMoteSkills(home, false); err != nil {
		t.Fatalf("ensureMoteSkills: %v", err)
	}

	// Both locations should have the mote-capture skill
	for _, expected := range []string{
		filepath.Join(home, ".claude", "skills", "mote-capture", "SKILL.md"),
		filepath.Join(home, ".agents", "skills", "mote-capture", "SKILL.md"),
	} {
		if _, err := os.Stat(expected); err != nil {
			t.Errorf("expected skill at %s: %v", expected, err)
		}
	}
}

func TestEnsureMoteSkills_ClaudeOnlyWhenCodexAbsent(t *testing.T) {
	home := t.TempDir()
	// No ~/.codex/, no --codex flag → should not write to ~/.agents/skills
	onboardCodex = false

	if err := ensureMoteSkills(home, false); err != nil {
		t.Fatalf("ensureMoteSkills: %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "mote-capture", "SKILL.md")); err != nil {
		t.Errorf("Claude skill should be installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "mote-capture", "SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("agents skill should NOT be installed when codex is disabled (got err=%v)", err)
	}
}

func TestEnsureMoteSkills_RespectsExplicitCodexFlag(t *testing.T) {
	home := t.TempDir()
	// No ~/.codex/ but --codex flag set → still install at .agents/skills
	onboardCodex = true
	defer func() { onboardCodex = false }()

	if err := ensureMoteSkills(home, false); err != nil {
		t.Fatalf("ensureMoteSkills: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "mote-capture", "SKILL.md")); err != nil {
		t.Errorf("agents skill should be installed when --codex flag set: %v", err)
	}
}

// --- Gemini CLI settings tests ---

func TestEnsureGeminiSettings_CreatesNew(t *testing.T) {
	geminiDir := t.TempDir()
	if err := ensureGeminiSettings(geminiDir, false); err != nil {
		t.Fatalf("ensureGeminiSettings: %v", err)
	}

	settingsPath := filepath.Join(geminiDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json should be created: %v", err)
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json should be valid JSON: %v", err)
	}

	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		t.Fatal("hooks key missing from settings")
	}
	for _, evt := range []string{"SessionStart", "BeforeAgent", "SessionEnd"} {
		if _, ok := hooks[evt]; !ok {
			t.Errorf("expected %s hook, missing", evt)
		}
	}

	// SessionEnd must carry the explicit 300000ms timeout — Gemini's 60s
	// default would kill the heavy mote session-end --hook flush.
	endEntries := hooks["SessionEnd"].([]interface{})
	endHook := endEntries[0].(map[string]interface{})["hooks"].([]interface{})[0].(map[string]interface{})
	if endHook["timeout"] != float64(300000) {
		t.Errorf("SessionEnd timeout: got %v, want 300000 (ms)", endHook["timeout"])
	}

	// context.fileName must include both GEMINI.md and AGENTS.md
	ctx, _ := settings["context"].(map[string]interface{})
	if ctx == nil {
		t.Fatal("context key missing")
	}
	fileNames, _ := ctx["fileName"].([]interface{})
	if !stringInArray(fileNames, "GEMINI.md") {
		t.Errorf("context.fileName missing GEMINI.md: %v", fileNames)
	}
	if !stringInArray(fileNames, "AGENTS.md") {
		t.Errorf("context.fileName missing AGENTS.md: %v", fileNames)
	}
}

func TestEnsureGeminiSettings_Idempotent(t *testing.T) {
	geminiDir := t.TempDir()
	if err := ensureGeminiSettings(geminiDir, false); err != nil {
		t.Fatalf("first install: %v", err)
	}
	first, _ := os.ReadFile(filepath.Join(geminiDir, "settings.json"))

	if err := ensureGeminiSettings(geminiDir, false); err != nil {
		t.Fatalf("second install: %v", err)
	}
	second, _ := os.ReadFile(filepath.Join(geminiDir, "settings.json"))

	if string(first) != string(second) {
		t.Errorf("ensureGeminiSettings not idempotent\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestEnsureGeminiSettings_PreservesUserHooks(t *testing.T) {
	geminiDir := t.TempDir()
	settingsPath := filepath.Join(geminiDir, "settings.json")
	preexisting := `{
  "hooks": {
    "BeforeTool": [
      {"matcher": "write_.*", "hooks": [{"name": "audit", "type": "command", "command": "/usr/local/bin/audit.sh"}]}
    ]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(preexisting), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ensureGeminiSettings(geminiDir, false); err != nil {
		t.Fatalf("ensureGeminiSettings: %v", err)
	}

	data, _ := os.ReadFile(settingsPath)
	if !strings.Contains(string(data), "audit.sh") {
		t.Error("preexisting BeforeTool audit hook should survive")
	}
	if !strings.Contains(string(data), "mote prime") {
		t.Error("mote hooks should be added alongside existing ones")
	}
}

func TestEnsureGeminiSettings_PreservesUserContextFileNames(t *testing.T) {
	geminiDir := t.TempDir()
	settingsPath := filepath.Join(geminiDir, "settings.json")
	preexisting := `{"context": {"fileName": ["MY_RULES.md"]}}`
	if err := os.WriteFile(settingsPath, []byte(preexisting), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ensureGeminiSettings(geminiDir, false); err != nil {
		t.Fatalf("ensureGeminiSettings: %v", err)
	}

	data, _ := os.ReadFile(settingsPath)
	for _, want := range []string{"MY_RULES.md", "GEMINI.md", "AGENTS.md"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("context.fileName missing %q after merge:\n%s", want, data)
		}
	}
}

func TestEnsureGeminiSettings_SkillsPathsArePreserved(t *testing.T) {
	// A user-defined SessionStart hook that ALREADY runs mote prime
	// should not be duplicated (idempotency edge case).
	geminiDir := t.TempDir()
	settingsPath := filepath.Join(geminiDir, "settings.json")
	preexisting := `{
  "hooks": {
    "SessionStart": [
      {"matcher": "startup|resume|clear", "hooks": [{"name": "mote-prime", "type": "command", "command": "mote prime --hook --mode=startup", "timeout": 60000}]}
    ]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(preexisting), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ensureGeminiSettings(geminiDir, false); err != nil {
		t.Fatalf("ensureGeminiSettings: %v", err)
	}
	data, _ := os.ReadFile(settingsPath)
	count := strings.Count(string(data), `"mote prime --hook --mode=startup"`)
	if count != 1 {
		t.Errorf("mote-prime command should appear exactly once after re-install, got %d", count)
	}
}

func TestStringInArray(t *testing.T) {
	tests := []struct {
		arr    []interface{}
		needle string
		want   bool
	}{
		{[]interface{}{"a", "b", "c"}, "b", true},
		{[]interface{}{"a", "b"}, "z", false},
		{[]interface{}{}, "a", false},
		{nil, "a", false},
		{[]interface{}{1, "a"}, "a", true}, // mixed types: only string match counts
	}
	for _, tt := range tests {
		if got := stringInArray(tt.arr, tt.needle); got != tt.want {
			t.Errorf("stringInArray(%v, %q) = %v, want %v", tt.arr, tt.needle, got, tt.want)
		}
	}
}

// --- Shared agents-skills install condition tests ---

func TestEnsureMoteSkills_AgentsPathOnGeminiOnly(t *testing.T) {
	home := t.TempDir()
	// Create ~/.gemini/ to trigger geminiEnabled(); no ~/.codex/
	if err := os.MkdirAll(filepath.Join(home, ".gemini"), 0755); err != nil {
		t.Fatal(err)
	}
	onboardCodex = false
	onboardGemini = false

	if err := ensureMoteSkills(home, false); err != nil {
		t.Fatalf("ensureMoteSkills: %v", err)
	}
	for _, expected := range []string{
		filepath.Join(home, ".claude", "skills", "mote-capture", "SKILL.md"),
		filepath.Join(home, ".agents", "skills", "mote-capture", "SKILL.md"),
	} {
		if _, err := os.Stat(expected); err != nil {
			t.Errorf("expected skill at %s: %v", expected, err)
		}
	}
}

func TestAgentsSkillsEnabled_FlagsAndAutoDetect(t *testing.T) {
	tests := []struct {
		name       string
		codexFlag  bool
		geminiFlag bool
		hasCodex   bool
		hasGemini  bool
		want       bool
	}{
		{"none", false, false, false, false, false},
		{"codex flag only", true, false, false, false, true},
		{"gemini flag only", false, true, false, false, true},
		{"both flags", true, true, false, false, true},
		{"codex auto-detect", false, false, true, false, true},
		{"gemini auto-detect", false, false, false, true, true},
		{"both auto-detect", false, false, true, true, true},
		{"flag + autodetect", true, false, false, true, true},
	}
	defer func() {
		onboardCodex = false
		onboardGemini = false
	}()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			onboardCodex = tt.codexFlag
			onboardGemini = tt.geminiFlag
			if tt.hasCodex {
				os.MkdirAll(filepath.Join(home, ".codex"), 0755)
			}
			if tt.hasGemini {
				os.MkdirAll(filepath.Join(home, ".gemini"), 0755)
			}
			if got := agentsSkillsEnabled(home); got != tt.want {
				t.Errorf("agentsSkillsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

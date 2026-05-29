// SPDX-License-Identifier: MIT
// STORY-PLUGINS-001 Scenarios 5 & 6 — release-time structural validation of
// the vendored plugin manifests. No public JSON schema for Claude Code /
// Codex plugin manifests exists at time of writing (Q3), so we assert
// structurally: required fields present, name matches "mote", versions are
// semver and equal to internal/version.Value, and the skill copies under
// plugins/mote/skills/ are byte-identical to the canonical skills/ files.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"motes/internal/version"
)

// repoRootFromCmdMote walks up from this test file's directory to the repo
// root (the directory containing go.mod). Mirrors the pattern in
// internal/version/scripts_helpers_test.go.
func repoRootFromCmdMote(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root from " + file)
		}
		dir = parent
	}
}

// semverRE accepts MAJOR.MINOR.PATCH with optional -prerelease and +build.
// Matches the regex used by scripts/bump-version.sh, kept in sync deliberately.
var semverRE = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

func loadManifest(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return m
}

// --- Scenario 5: structural validation ---

func TestClaudePluginManifest_HasRequiredFields(t *testing.T) {
	root := repoRootFromCmdMote(t)
	m := loadManifest(t, filepath.Join(root, "plugins", "mote", ".claude-plugin", "plugin.json"))
	if name, _ := m["name"].(string); name != "mote" {
		t.Errorf("plugin.json#name: got %q, want %q", name, "mote")
	}
	if desc, _ := m["description"].(string); desc == "" {
		t.Errorf("plugin.json#description must be a non-empty string")
	}
	v, _ := m["version"].(string)
	if !semverRE.MatchString(v) {
		t.Errorf("plugin.json#version: %q does not match semver regex", v)
	}
	// hooks field must point at the bundled hooks.json so the marketplace
	// install supplies the lifecycle hooks without re-running mote onboard.
	if hooks, _ := m["hooks"].(string); hooks != "./hooks/hooks.json" {
		t.Errorf("plugin.json#hooks: got %q, want %q", hooks, "./hooks/hooks.json")
	}
	// hooks.json must exist where plugin.json declares it.
	if _, err := os.Stat(filepath.Join(root, "plugins", "mote", ".claude-plugin", "hooks", "hooks.json")); err != nil {
		t.Errorf("declared hooks file is missing: %v", err)
	}
}

func TestCodexPluginManifest_HasRequiredFields(t *testing.T) {
	root := repoRootFromCmdMote(t)
	m := loadManifest(t, filepath.Join(root, "plugins", "mote", ".codex-plugin", "plugin.json"))
	if name, _ := m["name"].(string); name != "mote" {
		t.Errorf("codex plugin.json#name: got %q, want %q", name, "mote")
	}
	if desc, _ := m["description"].(string); desc == "" {
		t.Errorf("codex plugin.json#description must be a non-empty string")
	}
	v, _ := m["version"].(string)
	if !semverRE.MatchString(v) {
		t.Errorf("codex plugin.json#version: %q does not match semver regex", v)
	}
}

func TestMarketplaceManifest_PluginEntryWellFormed(t *testing.T) {
	root := repoRootFromCmdMote(t)
	m := loadManifest(t, filepath.Join(root, "plugins", "mote", ".claude-plugin", "marketplace.json"))
	if name, _ := m["name"].(string); name == "" {
		t.Errorf("marketplace.json#name must be a non-empty string")
	}
	plugins, ok := m["plugins"].([]interface{})
	if !ok || len(plugins) == 0 {
		t.Fatalf("marketplace.json#plugins must be a non-empty array")
	}
	entry, _ := plugins[0].(map[string]interface{})
	if entry == nil {
		t.Fatalf("marketplace.json#plugins[0] must be an object")
	}
	if name, _ := entry["name"].(string); name != "mote" {
		t.Errorf("plugins[0].name: got %q, want %q", name, "mote")
	}
	v, _ := entry["version"].(string)
	if !semverRE.MatchString(v) {
		t.Errorf("plugins[0].version: %q does not match semver regex", v)
	}
	src, _ := entry["source"].(map[string]interface{})
	if src == nil {
		t.Fatalf("plugins[0].source must be an object")
	}
	if path, _ := src["path"].(string); path != "plugins/mote" {
		t.Errorf("plugins[0].source.path: got %q, want %q", path, "plugins/mote")
	}
}

// hooks.json structure must mirror what desiredHooks() writes to settings.json
// so the marketplace install is behaviourally equivalent.
func TestClaudePluginHooks_StructureMirrorsDesiredHooks(t *testing.T) {
	root := repoRootFromCmdMote(t)
	m := loadManifest(t, filepath.Join(root, "plugins", "mote", ".claude-plugin", "hooks", "hooks.json"))
	hooks, _ := m["hooks"].(map[string]interface{})
	if hooks == nil {
		t.Fatal("hooks.json#hooks must be an object")
	}
	for _, evt := range []string{"SessionStart", "PreCompact", "UserPromptSubmit", "Stop"} {
		entries, _ := hooks[evt].([]interface{})
		if len(entries) == 0 {
			t.Errorf("hooks.json missing entries for event %q", evt)
		}
	}
	// SessionStart must declare all four matchers (startup, resume, compact, clear)
	// — the differentiated set that desiredHooks() writes.
	sess, _ := hooks["SessionStart"].([]interface{})
	gotMatchers := map[string]bool{}
	for _, raw := range sess {
		em, _ := raw.(map[string]interface{})
		if m, ok := em["matcher"].(string); ok {
			gotMatchers[m] = true
		}
	}
	for _, want := range []string{"startup", "resume", "compact", "clear"} {
		if !gotMatchers[want] {
			t.Errorf("hooks.json SessionStart missing matcher %q", want)
		}
	}
}

// --- Scenario 6: version lock-step ---

func TestPluginVersion_MatchesBinaryVersion(t *testing.T) {
	root := repoRootFromCmdMote(t)
	want := version.Value

	for _, rel := range []string{
		filepath.Join(".claude-plugin", "plugin.json"),
		filepath.Join(".codex-plugin", "plugin.json"),
	} {
		path := filepath.Join(root, "plugins", "mote", rel)
		m := loadManifest(t, path)
		if got, _ := m["version"].(string); got != want {
			t.Errorf("%s#version: got %q, want %q (canonical)", rel, got, want)
		}
	}

	// marketplace.json#plugins[0].version must also be locked.
	mkt := loadManifest(t, filepath.Join(root, "plugins", "mote", ".claude-plugin", "marketplace.json"))
	plugins, _ := mkt["plugins"].([]interface{})
	if len(plugins) > 0 {
		entry, _ := plugins[0].(map[string]interface{})
		if got, _ := entry["version"].(string); got != want {
			t.Errorf("marketplace.json plugins[0].version: got %q, want %q (canonical)", got, want)
		}
	}
}

// --- byte-identity: plugin skills must match canonical skills ---

func TestPluginSkills_ByteIdenticalToCanonical(t *testing.T) {
	root := repoRootFromCmdMote(t)
	for _, name := range []string{"mote-capture", "mote-plan", "mote-retrieve", "mote-subagent"} {
		canonical, err := os.ReadFile(filepath.Join(root, "skills", name, "SKILL.md"))
		if err != nil {
			t.Fatalf("read canonical %s: %v", name, err)
		}
		copyData, err := os.ReadFile(filepath.Join(root, "plugins", "mote", "skills", name, "SKILL.md"))
		if err != nil {
			t.Fatalf("read plugin copy of %s: %v", name, err)
		}
		if !bytes.Equal(canonical, copyData) {
			t.Errorf("plugin skill copy diverges from canonical: %s\nCanonical is the source of truth — re-run `cp skills/%s/SKILL.md plugins/mote/skills/%s/SKILL.md` to resync.",
				name, name, name)
		}
	}
}

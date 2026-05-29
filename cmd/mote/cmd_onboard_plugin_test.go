// SPDX-License-Identifier: MIT
// Unit tests for STORY-PLUGINS-001 plugin-presence detectors.
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeClaudePluginManifest places a `.claude-plugin/plugin.json` under
// `<home>/.claude/plugins/<dir>/` with the given top-level name. Helper for
// the plugin-detection tests below.
func writeClaudePluginManifest(t *testing.T, home, pluginDir, name string) {
	t.Helper()
	manifestDir := filepath.Join(home, ".claude", "plugins", pluginDir, ".claude-plugin")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	body := `{"name":"` + name + `","version":"0.4.39","description":"x"}`
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write plugin.json: %v", err)
	}
}

// writeClaudePluginManifestAt places a `.claude-plugin/plugin.json` at the
// given absolute path (relative to home). Used for the marketplace-layout
// test where the manifest lives several levels deep.
func writeClaudePluginManifestAt(t *testing.T, manifestDir, name string) {
	t.Helper()
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	body := `{"name":"` + name + `","version":"0.4.39","description":"x"}`
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write plugin.json: %v", err)
	}
}

// writeCodexPluginManifest mirrors writeClaudePluginManifest under
// `<home>/.codex/plugins/<dir>/.codex-plugin/`.
func writeCodexPluginManifest(t *testing.T, home, pluginDir, name string) {
	t.Helper()
	manifestDir := filepath.Join(home, ".codex", "plugins", pluginDir, ".codex-plugin")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	body := `{"name":"` + name + `","version":"0.4.39"}`
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write plugin.json: %v", err)
	}
}

// --- claudePluginInstalled ---

func TestClaudePluginInstalled_TrueWhenManifestPresent(t *testing.T) {
	home := t.TempDir()
	writeClaudePluginManifest(t, home, "mote", "mote")
	if !claudePluginInstalled(home) {
		t.Errorf("expected claudePluginInstalled=true for present manifest with name=mote")
	}
}

// Real Claude Code marketplaces install plugins one layer deeper, under
// ~/.claude/plugins/marketplaces/<name>/plugins/<plugin>/.claude-plugin/.
// The detector must find them at that depth too.
func TestClaudePluginInstalled_TrueWhenInMarketplaceLayout(t *testing.T) {
	home := t.TempDir()
	deep := filepath.Join(home, ".claude", "plugins", "marketplaces", "official", "plugins", "mote", ".claude-plugin")
	writeClaudePluginManifestAt(t, deep, "mote")
	if !claudePluginInstalled(home) {
		t.Errorf("expected detection at marketplace depth (.../marketplaces/.../plugins/...)")
	}
}

func TestClaudePluginInstalled_FalseWhenDirAbsent(t *testing.T) {
	home := t.TempDir()
	if claudePluginInstalled(home) {
		t.Errorf("expected claudePluginInstalled=false when ~/.claude/plugins/ is absent")
	}
}

func TestClaudePluginInstalled_FalseWhenNameDiffers(t *testing.T) {
	home := t.TempDir()
	writeClaudePluginManifest(t, home, "other-plugin", "other-plugin")
	if claudePluginInstalled(home) {
		t.Errorf("expected claudePluginInstalled=false when no manifest names 'mote'")
	}
}

func TestClaudePluginInstalled_FalseOnMalformedManifest(t *testing.T) {
	home := t.TempDir()
	manifestDir := filepath.Join(home, ".claude", "plugins", "mote", ".claude-plugin")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if claudePluginInstalled(home) {
		t.Errorf("expected claudePluginInstalled=false on malformed JSON (must not panic, must not match)")
	}
}

// An empty plugins/ directory with no plugin.json files should not match.
func TestClaudePluginInstalled_FalseWhenNoManifests(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude", "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	if claudePluginInstalled(home) {
		t.Errorf("expected claudePluginInstalled=false for empty plugins/ dir")
	}
}

// --- codexPluginInstalled (mirrors Claude pair) ---

func TestCodexPluginInstalled_TrueWhenManifestPresent(t *testing.T) {
	home := t.TempDir()
	writeCodexPluginManifest(t, home, "mote", "mote")
	if !codexPluginInstalled(home) {
		t.Errorf("expected codexPluginInstalled=true for present manifest with name=mote")
	}
}

func TestCodexPluginInstalled_FalseWhenDirAbsent(t *testing.T) {
	home := t.TempDir()
	if codexPluginInstalled(home) {
		t.Errorf("expected codexPluginInstalled=false when ~/.codex/plugins/ is absent")
	}
}

func TestCodexPluginInstalled_FalseWhenNameDiffers(t *testing.T) {
	home := t.TempDir()
	writeCodexPluginManifest(t, home, "other", "other")
	if codexPluginInstalled(home) {
		t.Errorf("expected codexPluginInstalled=false when no manifest names 'mote'")
	}
}

func TestCodexPluginInstalled_FalseOnMalformedManifest(t *testing.T) {
	home := t.TempDir()
	manifestDir := filepath.Join(home, ".codex", "plugins", "mote", ".codex-plugin")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if codexPluginInstalled(home) {
		t.Errorf("expected codexPluginInstalled=false on malformed JSON")
	}
}

// --- ensureMoteSkillsGated ---

// With neither plugin present and Codex enabled, both ~/.claude/skills/ and
// ~/.agents/skills/ are written — matches the pre-PLUGINS-001 behaviour.
func TestEnsureMoteSkillsGated_NoPluginsBothPaths(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	defer func() { onboardCodex = false }()

	if err := ensureMoteSkillsGated(home, false, false, false); err != nil {
		t.Fatalf("ensureMoteSkillsGated: %v", err)
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

// With the Claude plugin present, ~/.claude/skills/ is skipped.
func TestEnsureMoteSkillsGated_ClaudePluginSkipsClaudePath(t *testing.T) {
	home := t.TempDir()
	if err := ensureMoteSkillsGated(home, true, false, false); err != nil {
		t.Fatalf("ensureMoteSkillsGated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "mote-capture", "SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("~/.claude/skills/mote-capture should NOT be installed when Claude plugin present (err=%v)", err)
	}
}

// With the Codex plugin present and ONLY Codex enabled, ~/.agents/skills/ is
// skipped. Claude side is unaffected.
func TestEnsureMoteSkillsGated_CodexPluginSkipsAgentsPathWhenOnlyCodex(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	defer func() { onboardCodex = false }()

	if err := ensureMoteSkillsGated(home, false, true, false); err != nil {
		t.Fatalf("ensureMoteSkillsGated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "mote-capture", "SKILL.md")); err != nil {
		t.Errorf("Claude path should still be installed when only Codex plugin present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "mote-capture", "SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("~/.agents/skills should NOT be installed when only Codex consumer is plugin-served (err=%v)", err)
	}
}

// With the Codex plugin present but Gemini ALSO enabled, ~/.agents/skills/
// must still be written — Gemini has no plugin path (Q7 non-goal).
func TestEnsureMoteSkillsGated_GeminiForcesAgentsPathEvenWithCodexPlugin(t *testing.T) {
	home := t.TempDir()
	// Both ~/.codex/ and ~/.gemini/ present → both auto-enabled.
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".gemini"), 0o755); err != nil {
		t.Fatal(err)
	}
	defer func() {
		onboardCodex = false
		onboardGemini = false
	}()

	if err := ensureMoteSkillsGated(home, false, true, false); err != nil {
		t.Fatalf("ensureMoteSkillsGated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "mote-capture", "SKILL.md")); err != nil {
		t.Errorf("~/.agents/skills must remain installed when Gemini is enabled, even if Codex plugin present: %v", err)
	}
}

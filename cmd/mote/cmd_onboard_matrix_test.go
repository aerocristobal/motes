// SPDX-License-Identifier: MIT
// STORY-PLUGINS-001 Scenario 7 — onboard integration decision matrix.
//
// Each row mirrors one Examples line from the Gherkin Scenario Outline. The
// test exercises the same decision logic that runCommonSetup applies and
// asserts which dotfile writes did/did not occur per host.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnboard_DecisionMatrix(t *testing.T) {
	rows := []struct {
		name              string
		claudePluginThere bool
		codexPluginThere  bool
		geminiPresent     bool
		// Expected post-condition flags. nil-able for non-applicable cells:
		// "wantClaudeHooks" means ~/.claude/settings.json gained mote hook entries.
		// "wantClaudeSkills" means ~/.claude/skills/mote-*/ was created.
		// "wantCodexHooks" means ~/.codex/hooks.json gained mote hook entries.
		// "wantGeminiSettings" means ~/.gemini/settings.json was created.
		// "wantAgentsSkills" means ~/.agents/skills/mote-*/ was created.
		wantClaudeHooks    bool
		wantClaudeSkills   bool
		wantCodexHooks     bool
		wantGeminiSettings bool
		wantAgentsSkills   bool
	}{
		{
			name:              "claude=present, codex=present, gemini=present",
			claudePluginThere: true, codexPluginThere: true, geminiPresent: true,
			wantClaudeHooks: false, wantClaudeSkills: false,
			wantCodexHooks:     false,
			wantGeminiSettings: true,
			wantAgentsSkills:   true, // Gemini still needs ~/.agents/skills/
		},
		{
			name:              "claude=present, codex=absent, gemini=absent",
			claudePluginThere: true, codexPluginThere: false, geminiPresent: false,
			wantClaudeHooks: false, wantClaudeSkills: false,
			wantCodexHooks:     false, // codex not enabled
			wantGeminiSettings: false, // gemini not enabled
			wantAgentsSkills:   false,
		},
		{
			name:              "claude=absent, codex=present, gemini=present",
			claudePluginThere: false, codexPluginThere: true, geminiPresent: true,
			wantClaudeHooks: true, wantClaudeSkills: true,
			wantCodexHooks:     false, // codex plugin suppresses
			wantGeminiSettings: true,
			wantAgentsSkills:   true, // gemini forces it
		},
		{
			name:              "claude=absent, codex=absent, gemini=absent",
			claudePluginThere: false, codexPluginThere: false, geminiPresent: false,
			wantClaudeHooks: true, wantClaudeSkills: true,
			wantCodexHooks:     false, // codex not enabled
			wantGeminiSettings: false,
			wantAgentsSkills:   false,
		},
		{
			name:              "claude=absent, codex=absent, gemini=present",
			claudePluginThere: false, codexPluginThere: false, geminiPresent: true,
			wantClaudeHooks: true, wantClaudeSkills: true,
			wantCodexHooks:     false, // codex not enabled
			wantGeminiSettings: true,
			wantAgentsSkills:   true, // gemini triggers it
		},
	}

	// Save/restore package-level flag state mutated by codexEnabled/geminiEnabled.
	origCodex := onboardCodex
	origGemini := onboardGemini
	t.Cleanup(func() {
		onboardCodex = origCodex
		onboardGemini = origGemini
	})

	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			onboardCodex = false
			onboardGemini = false
			home := t.TempDir()

			// Arrange: plugin presence
			if r.claudePluginThere {
				writeClaudePluginManifest(t, home, "mote", "mote")
			}
			if r.codexPluginThere {
				writeCodexPluginManifest(t, home, "mote", "mote")
				// codex plugin presence implies codex is in use
				if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if r.geminiPresent {
				if err := os.MkdirAll(filepath.Join(home, ".gemini"), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			// Act: replay the same gating logic that runCommonSetup applies.
			claudePlugin := claudePluginInstalled(home)
			codexPlugin := codexPluginInstalled(home)
			claudeDir := filepath.Join(home, ".claude")
			if !claudePlugin {
				if err := ensureClaudeHooks(claudeDir, false); err != nil {
					t.Fatalf("ensureClaudeHooks: %v", err)
				}
			}
			if codexEnabled(home) && !codexPlugin {
				if err := ensureCodexHooks(filepath.Join(home, ".codex"), false); err != nil {
					t.Fatalf("ensureCodexHooks: %v", err)
				}
			}
			if geminiEnabled(home) {
				if err := ensureGeminiSettings(filepath.Join(home, ".gemini"), false); err != nil {
					t.Fatalf("ensureGeminiSettings: %v", err)
				}
			}
			if err := ensureMoteSkillsGated(home, claudePlugin, codexPlugin, false); err != nil {
				t.Fatalf("ensureMoteSkillsGated: %v", err)
			}

			// Assert: per-host write effects match expected.
			gotClaudeHooks := claudeSettingsHasMoteHook(t, claudeDir)
			if gotClaudeHooks != r.wantClaudeHooks {
				t.Errorf("claude hooks: got %v, want %v", gotClaudeHooks, r.wantClaudeHooks)
			}

			gotClaudeSkills := dirExists(filepath.Join(home, ".claude", "skills", "mote-capture"))
			if gotClaudeSkills != r.wantClaudeSkills {
				t.Errorf("claude skills: got %v, want %v", gotClaudeSkills, r.wantClaudeSkills)
			}

			gotCodexHooks := codexHooksHasMoteHook(t, filepath.Join(home, ".codex"))
			if gotCodexHooks != r.wantCodexHooks {
				t.Errorf("codex hooks: got %v, want %v", gotCodexHooks, r.wantCodexHooks)
			}

			gotGemini := false
			if _, err := os.Stat(filepath.Join(home, ".gemini", "settings.json")); err == nil {
				gotGemini = true
			}
			if gotGemini != r.wantGeminiSettings {
				t.Errorf("gemini settings: got %v, want %v", gotGemini, r.wantGeminiSettings)
			}

			gotAgents := dirExists(filepath.Join(home, ".agents", "skills", "mote-capture"))
			if gotAgents != r.wantAgentsSkills {
				t.Errorf("agents skills: got %v, want %v", gotAgents, r.wantAgentsSkills)
			}
		})
	}
}

// claudeSettingsHasMoteHook returns true if ~/.claude/settings.json exists and
// contains a hook command starting with "MOTE_AGENT_KIND=claude mote".
func claudeSettingsHasMoteHook(t *testing.T, claudeDir string) bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		return false
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return false
	}
	hooks, _ := settings["hooks"].(map[string]interface{})
	for _, raw := range hooks {
		entries, _ := raw.([]interface{})
		for _, e := range entries {
			em, _ := e.(map[string]interface{})
			hl, _ := em["hooks"].([]interface{})
			for _, h := range hl {
				hm, _ := h.(map[string]interface{})
				if cmd, ok := hm["command"].(string); ok && strings.Contains(cmd, "MOTE_AGENT_KIND=claude mote") {
					return true
				}
			}
		}
	}
	return false
}

// codexHooksHasMoteHook returns true if ~/.codex/hooks.json exists and contains
// a hook command starting with "MOTE_AGENT_KIND=codex mote".
func codexHooksHasMoteHook(t *testing.T, codexDir string) bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(codexDir, "hooks.json"))
	if err != nil {
		return false
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return false
	}
	hooks, _ := settings["hooks"].(map[string]interface{})
	for _, raw := range hooks {
		entries, _ := raw.([]interface{})
		for _, e := range entries {
			em, _ := e.(map[string]interface{})
			hl, _ := em["hooks"].([]interface{})
			for _, h := range hl {
				hm, _ := h.(map[string]interface{})
				if cmd, ok := hm["command"].(string); ok && strings.Contains(cmd, "MOTE_AGENT_KIND=codex mote") {
					return true
				}
			}
		}
	}
	return false
}

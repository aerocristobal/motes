// SPDX-License-Identifier: MIT
// STORY-ADAPRIME-001 — MCP-server detection across three host config files.
package prime_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/prime"
)

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// Scenario 1 — Claude host declares mote.
func TestDetectMCPServer_TrueWhenClaudeSettingsDeclaresMote(t *testing.T) {
	home := t.TempDir()
	writeJSON(t, filepath.Join(home, ".claude", "settings.json"), map[string]any{
		"mcpServers": map[string]any{
			"mote": map[string]any{"command": "mote-mcp"},
		},
	})
	if !prime.DetectMCPServer(home) {
		t.Fatal("expected true when mote MCP server declared in claude settings")
	}
}

// Scenario 1 / Q2 — Codex host declares mote (no claude settings).
func TestDetectMCPServer_TrueWhenCodexSettingsDeclaresMote(t *testing.T) {
	home := t.TempDir()
	writeJSON(t, filepath.Join(home, ".codex", "settings.json"), map[string]any{
		"mcpServers": map[string]any{
			"mote": map[string]any{"command": "mote-mcp"},
		},
	})
	if !prime.DetectMCPServer(home) {
		t.Fatal("expected true when mote MCP server declared in codex settings")
	}
}

// Scenario 1 / Q2 — Gemini host declares mote (no claude or codex settings).
func TestDetectMCPServer_TrueWhenGeminiSettingsDeclaresMote(t *testing.T) {
	home := t.TempDir()
	writeJSON(t, filepath.Join(home, ".gemini", "settings.json"), map[string]any{
		"mcpServers": map[string]any{
			"mote": map[string]any{"command": "mote-mcp"},
		},
	})
	if !prime.DetectMCPServer(home) {
		t.Fatal("expected true when mote MCP server declared in gemini settings")
	}
}

// Scenario 2 — no settings files at all.
func TestDetectMCPServer_FalseWhenNoSettingsFiles(t *testing.T) {
	home := t.TempDir()
	if prime.DetectMCPServer(home) {
		t.Fatal("expected false when no settings files exist")
	}
}

// Scenario 2 variant — settings present but only OTHER servers declared.
func TestDetectMCPServer_FalseWhenSettingsHasOtherServers(t *testing.T) {
	home := t.TempDir()
	writeJSON(t, filepath.Join(home, ".claude", "settings.json"), map[string]any{
		"mcpServers": map[string]any{
			"filesystem": map[string]any{"command": "fs-mcp"},
			"github":     map[string]any{"command": "gh-mcp"},
		},
	})
	if prime.DetectMCPServer(home) {
		t.Fatal("expected false when only non-mote servers declared")
	}
}

// Scenario 2 variant — settings present, no mcpServers key at all.
func TestDetectMCPServer_FalseWhenNoMcpServersKey(t *testing.T) {
	home := t.TempDir()
	writeJSON(t, filepath.Join(home, ".claude", "settings.json"), map[string]any{
		"hooks":  map[string]any{},
		"theme":  "dark",
		"unused": []any{1, 2, 3},
	})
	if prime.DetectMCPServer(home) {
		t.Fatal("expected false when no mcpServers key present")
	}
}

// Scenario 7 — malformed JSON returns false (silent fallback).
func TestDetectMCPServer_FalseOnMalformedSettings(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if prime.DetectMCPServer(home) {
		t.Fatal("expected false on malformed JSON, not a panic")
	}
}

// Scenario 7 variant — JSON array at top level (unexpected shape).
func TestDetectMCPServer_FalseWhenTopLevelIsArray(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`[1,2,3]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if prime.DetectMCPServer(home) {
		t.Fatal("expected false when top-level is an array, not an object")
	}
}

// ANY positive hit short-circuits — even when earlier paths are absent.
func TestDetectMCPServer_AnyHostPositiveHitWins(t *testing.T) {
	home := t.TempDir()
	// No claude file, no codex file — only gemini declares.
	writeJSON(t, filepath.Join(home, ".gemini", "settings.json"), map[string]any{
		"mcpServers": map[string]any{
			"mote": map[string]any{"command": "mote-mcp"},
		},
	})
	if !prime.DetectMCPServer(home) {
		t.Fatal("expected true when only gemini settings declares mote")
	}
}

// Scenario 7 last line — --debug surface gets a path-naming warning for
// the malformed file (DetectMCPServerVerbose returns it; the cmd layer
// gates on the --debug flag).
func TestDetectMCPServerVerbose_ReturnsWarningOnMalformed(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	detected, warnings := prime.DetectMCPServerVerbose(home)
	if detected {
		t.Fatal("expected false on malformed JSON")
	}
	if len(warnings) == 0 {
		t.Fatal("expected at least one warning naming the broken path")
	}
	if !strings.Contains(warnings[0], p) {
		t.Errorf("warning should name the broken path %q; got: %q", p, warnings[0])
	}
}

// Missing files are NOT a warning — only unexpected read or parse errors are.
func TestDetectMCPServerVerbose_NoWarningsWhenNoFiles(t *testing.T) {
	home := t.TempDir()
	detected, warnings := prime.DetectMCPServerVerbose(home)
	if detected {
		t.Fatal("expected false when no files exist")
	}
	if len(warnings) != 0 {
		t.Errorf("expected zero warnings for missing files; got %d: %v", len(warnings), warnings)
	}
}

// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Per-agent global shims point at the canonical ~/.motes/MOTES.md cross-agent
// knowledge index. Each agent gets either:
//   1. A managed section inside its global context file (Claude, Codex), OR
//   2. A separate motes-owned file registered via the agent's settings
//      (Gemini — avoids collision with the agent's own save_memory writes).

const (
	shimBeginMarker = "<!-- BEGIN motes-managed -->"
	shimEndMarker   = "<!-- END motes-managed -->"

	claudeGlobalShimText = `## Motes Memory

Read ` + "`~/.motes/MOTES.md`" + ` at session start to load the shared cross-agent
knowledge index. Each entry links to a mote in ` + "`~/.motes/nodes/`" + `.

This is **distinct from Claude's auto-memory** (` + "`~/.claude/memory/MEMORY.md`" + `),
which the harness manages automatically as locally-scoped thread carry-over.
The motes index is the long-lived, cross-agent shared layer.`

	codexGlobalShimText = `## Motes Memory

Read ` + "`~/.motes/MOTES.md`" + ` for the shared cross-agent knowledge graph.
Each entry links to a mote in ` + "`~/.motes/nodes/`" + `.

This is **distinct from Codex's local Memories feature**
(` + "`~/.codex/memories/`" + `, opt-in via ` + "`[features] memories = true`" + `).
MOTES is shared across agents and scoped to long-lived knowledge; Codex
Memories carry per-thread context locally. They serve different roles — both
are useful, neither replaces the other.`

	geminiShimFilename = "motes-shim.md"
	geminiShimText     = `# Motes Memory

@~/.motes/MOTES.md

The above include is the shared cross-agent knowledge graph (` + "`~/.motes/nodes/`" + `).
Distinct from facts saved into ` + "`~/.gemini/GEMINI.md`" + ` via Gemini's ` + "`save_memory`" + `
tool — those are local recall; MOTES is the long-lived shared layer.`
)

// ensureManagedSection ensures a managed section exists in a markdown file,
// delimited by HTML comment markers. If the markers are absent, the section
// is appended (with a leading blank line). If present, the body between
// markers is replaced. User-authored content outside the markers is preserved.
func ensureManagedSection(path, content string, dryRun bool) (changed bool, err error) {
	if dryRun {
		fmt.Printf("  Would update managed section in %s\n", path)
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, fmt.Errorf("create parent dir: %w", err)
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read %s: %w", path, err)
	}

	managedBlock := shimBeginMarker + "\n" + content + "\n" + shimEndMarker

	var updated string
	if len(existing) == 0 {
		updated = managedBlock + "\n"
	} else {
		body := string(existing)
		begin := strings.Index(body, shimBeginMarker)
		end := strings.Index(body, shimEndMarker)
		if begin >= 0 && end > begin {
			// Replace existing block.
			before := body[:begin]
			after := body[end+len(shimEndMarker):]
			updated = before + managedBlock + after
		} else {
			// Append, ensuring single trailing newline before the block.
			trimmed := strings.TrimRight(body, "\n")
			updated = trimmed + "\n\n" + managedBlock + "\n"
		}
		if updated == body {
			return false, nil
		}
	}

	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

func ensureClaudeGlobalShim(home string, dryRun bool) error {
	path := filepath.Join(home, ".claude", "CLAUDE.md")
	changed, err := ensureManagedSection(path, claudeGlobalShimText, dryRun)
	if err != nil {
		return err
	}
	if changed && !dryRun {
		fmt.Println("Updated managed section in ~/.claude/CLAUDE.md")
	}
	return nil
}

func ensureCodexGlobalShim(home string, dryRun bool) error {
	path := filepath.Join(home, ".codex", "AGENTS.md")
	changed, err := ensureManagedSection(path, codexGlobalShimText, dryRun)
	if err != nil {
		return err
	}
	if changed && !dryRun {
		fmt.Println("Updated managed section in ~/.codex/AGENTS.md")
	}
	return nil
}

// ensureGeminiGlobalShim writes a motes-owned file under ~/.gemini/ and adds
// it to the context.fileName array in settings.json. ~/.gemini/GEMINI.md is
// never touched by motes — Gemini's save_memory tool owns that file.
func ensureGeminiGlobalShim(home string, dryRun bool) error {
	geminiDir := filepath.Join(home, ".gemini")
	shimPath := filepath.Join(geminiDir, geminiShimFilename)

	if dryRun {
		fmt.Printf("  Would write %s and register it in settings.json context.fileName\n", shimPath)
		return nil
	}

	if err := os.MkdirAll(geminiDir, 0755); err != nil {
		return fmt.Errorf("create ~/.gemini: %w", err)
	}

	desired := []byte(geminiShimText + "\n")
	existing, err := os.ReadFile(shimPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", shimPath, err)
	}
	if !bytesEqual(existing, desired) {
		if err := os.WriteFile(shimPath, desired, 0644); err != nil {
			return fmt.Errorf("write %s: %w", shimPath, err)
		}
		fmt.Printf("Wrote %s\n", shimPath)
	}

	// Register in settings.json context.fileName.
	settingsPath := filepath.Join(geminiDir, "settings.json")
	if err := registerGeminiContextFile(settingsPath, geminiShimFilename); err != nil {
		return fmt.Errorf("register context file: %w", err)
	}
	return nil
}

// registerGeminiContextFile adds filename to context.fileName in settings.json
// if not already present. Idempotent. Preserves all other settings.
func registerGeminiContextFile(settingsPath, filename string) error {
	var settings map[string]any
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse settings.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if settings == nil {
		settings = map[string]any{}
	}

	context, _ := settings["context"].(map[string]any)
	if context == nil {
		context = map[string]any{}
	}
	rawList, _ := context["fileName"].([]any)
	for _, v := range rawList {
		if s, ok := v.(string); ok && s == filename {
			return nil // already present
		}
	}
	rawList = append(rawList, filename)
	context["fileName"] = rawList
	settings["context"] = context

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(settingsPath, append(out, '\n'), 0644)
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"motes/claudehooks"
	"motes/internal/core"
)

// preToolUseScript pairs an installed script's filename with its embedded
// content from the claudehooks package. The filename also forms the basis
// of the command path written into settings.json.
type preToolUseScript struct {
	name    string
	content []byte
}

func preToolUseScripts() []preToolUseScript {
	return []preToolUseScript{
		{"block-interactive-cmds.sh", claudehooks.BlockInteractiveCmds},
		{"block-gh-watch.sh", claudehooks.BlockGhWatch},
		{"block-mote-rm.sh", claudehooks.BlockMoteRm},
	}
}

// desiredPreToolUseHooks returns the PreToolUse[Bash] hookSpecs that mote
// onboard registers in ~/.claude/settings.json. The command path uses $HOME
// so the shell expands at execution time — the literal value is identical
// regardless of which user runs onboard.
func desiredPreToolUseHooks() []hookSpec {
	var specs []hookSpec
	for _, s := range preToolUseScripts() {
		specs = append(specs, hookSpec{
			event:   "PreToolUse",
			matcher: "Bash",
			command: "$HOME/.claude/hooks/" + s.name,
		})
	}
	return specs
}

// installPreToolUseScripts writes the embedded safety hook scripts to
// <claudeDir>/hooks/<name>.sh with mode 0755. Idempotent: skips files whose
// content is already up to date.
func installPreToolUseScripts(claudeDir string, dryRun bool) error {
	hooksDir := filepath.Join(claudeDir, "hooks")

	for _, s := range preToolUseScripts() {
		target := filepath.Join(hooksDir, s.name)

		existing, err := os.ReadFile(target)
		if err == nil && bytes.Equal(existing, s.content) {
			continue
		}

		action := "installed"
		if err == nil {
			action = "updated"
		}

		if dryRun {
			fmt.Printf("  Would install hook script: %s\n", s.name)
			continue
		}

		if err := os.MkdirAll(hooksDir, 0755); err != nil {
			return fmt.Errorf("create hooks dir: %w", err)
		}
		if err := core.AtomicWrite(target, s.content, 0755); err != nil {
			return fmt.Errorf("write hook %s: %w", s.name, err)
		}
		fmt.Printf("  %s hook script: %s\n", action, s.name)
	}
	return nil
}

// ensurePreToolUseHooks installs the safety hook scripts under
// <claudeDir>/hooks/ and registers their PreToolUse[Bash] entries in
// <claudeDir>/settings.json. Idempotent.
func ensurePreToolUseHooks(claudeDir string, dryRun bool) error {
	if err := installPreToolUseScripts(claudeDir, dryRun); err != nil {
		return err
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")

	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		settings = map[string]interface{}{}
	} else if err != nil {
		return fmt.Errorf("read settings.json: %w", err)
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse settings.json: %w", err)
		}
	}

	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
	}

	var installed []string
	for _, spec := range desiredPreToolUseHooks() {
		if hookEventHasMatcherCommand(hooks, spec.event, spec.matcher, spec.command) {
			continue
		}
		entry := map[string]interface{}{
			"matcher": spec.matcher,
			"hooks": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": spec.command,
				},
			},
		}
		existing, _ := hooks[spec.event].([]interface{})
		hooks[spec.event] = append(existing, entry)
		installed = append(installed, spec.command)
	}

	if len(installed) == 0 {
		return nil
	}

	if dryRun {
		for _, cmd := range installed {
			fmt.Printf("  Would register PreToolUse[Bash]: %s\n", cmd)
		}
		return nil
	}

	settings["hooks"] = hooks
	newData, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	newData = append(newData, '\n')

	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("create claude dir: %w", err)
	}
	if err := core.AtomicWrite(settingsPath, newData, 0644); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}

	for _, cmd := range installed {
		fmt.Printf("  registered PreToolUse[Bash]: %s\n", cmd)
	}
	return nil
}

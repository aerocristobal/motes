// SPDX-License-Identifier: MIT
package prime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// hostSettingsPaths lists the per-host config files (relative to the
// user's HOME directory) that are checked for an `mcpServers.mote` entry.
// Mote integrates with three agent hosts; ANY positive hit short-circuits
// detection (the user has wired mote-MCP into at least one host).
var hostSettingsPaths = []string{
	".claude/settings.json",
	".codex/settings.json",
	".gemini/settings.json",
}

// DetectMCPServer reports whether at least one supported agent-host
// settings file under homeDir declares an `mcpServers.mote` entry.
//
// The contract is silent-by-default (matching STORY-BR-23-4): missing
// files, unreadable files, malformed JSON, and unexpected shapes all
// return false without surfacing anything to stderr. Use
// DetectMCPServerVerbose if you need the diagnostic warnings, e.g. for
// the `--debug` surface (Scenario 7 last line).
func DetectMCPServer(homeDir string) bool {
	detected, _ := DetectMCPServerVerbose(homeDir)
	return detected
}

// DetectMCPServerVerbose returns the same detection result as
// DetectMCPServer plus a human-readable warning per host path that
// failed to parse. Callers that pass --debug surface the warnings to
// stderr; default callers discard them.
//
// Note: a missing settings file is NOT a warning (os.IsNotExist) — it is
// the common case. Only unexpected read or JSON-parse failures populate
// the warnings slice.
func DetectMCPServerVerbose(homeDir string) (bool, []string) {
	var warnings []string
	for _, rel := range hostSettingsPaths {
		path := filepath.Join(homeDir, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				warnings = append(warnings, fmt.Sprintf("read %s: %v", path, err))
			}
			continue
		}
		var top map[string]interface{}
		if err := json.Unmarshal(data, &top); err != nil {
			warnings = append(warnings, fmt.Sprintf("parse %s: %v", path, err))
			continue
		}
		servers, ok := top["mcpServers"].(map[string]interface{})
		if !ok {
			// Either the key is missing or it isn't an object (e.g. a
			// JSON array). Treat both as "no mote MCP server declared"
			// per Scenario 7; this is the common case for hosts that
			// don't use mcpServers at all.
			continue
		}
		if _, ok := servers["mote"]; ok {
			return true, warnings
		}
	}
	return false, warnings
}

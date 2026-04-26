// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"fmt"
	"os"
)

// ResolveAgentID returns an identifier for the current agent/process.
// Uses MOTE_AGENT_ID env var if set, otherwise falls back to hostname-PID.
//
// This is a per-session/per-process identifier used for access tracking. It
// is distinct from ResolveAgentKind, which classifies the agent family
// (claude/codex/gemini) for config overrides.
func ResolveAgentID() string {
	if id := os.Getenv("MOTE_AGENT_ID"); id != "" {
		return id
	}
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}

// ResolveAgentKind returns the family identifier of the running agent —
// "claude", "codex", "gemini", a custom value, or "" when unknown. Set by
// each agent's installed hooks (e.g. MOTE_AGENT_KIND=codex). Used by
// LoadConfig to apply per-agent provider overrides from
// dream.provider.per_agent.
func ResolveAgentKind() string {
	return os.Getenv("MOTE_AGENT_KIND")
}

// SPDX-License-Identifier: AGPL-3.0-or-later
package core

// Valid enum values for mote fields.
var (
	ValidTypes    = []string{"task", "decision", "lesson", "context", "question", "constellation", "anchor", "explore"}
	ValidStatuses = []string{"active", "in_progress", "completed", "archived", "deprecated"}
	ValidOrigins  = []string{"normal", "failure", "revert", "hotfix", "discovery"}
	ValidSizes    = []string{"xs", "s", "m", "l", "xl"}

	// KnowledgeTypes are mote types that default to global storage (~/.motes/nodes/).
	// Task, context, constellation, and anchor types remain project-local — context is
	// inherently session-bound and was the dominant source of global-layer pollution.
	KnowledgeTypes = map[string]bool{
		"decision": true, "lesson": true, "explore": true,
		"question": true,
	}

	// PromotableTypes are the types `mote promote` will accept. Context is excluded
	// because session-bound notes don't belong in cross-project memory.
	PromotableTypes = map[string]bool{
		"decision": true, "lesson": true, "explore": true, "question": true,
	}
)

// IsLive reports whether a mote status represents "live" (not-yet-done) work.
// Live statuses block their dependents and surface in live-work views such as
// `mote prime`. The "ready to pick up" filter (`--ready`) is narrower: it
// requires exactly "active" so that a task already in flight isn't re-offered.
func IsLive(status string) bool {
	return status == "active" || status == "in_progress"
}

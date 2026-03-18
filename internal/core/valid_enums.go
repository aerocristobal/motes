package core

// Valid enum values for mote fields.
var (
	ValidTypes    = []string{"task", "decision", "lesson", "context", "question", "constellation", "anchor", "explore"}
	ValidStatuses = []string{"active", "completed", "archived", "deprecated"}
	ValidOrigins  = []string{"normal", "failure", "revert", "hotfix", "discovery"}
	ValidSizes    = []string{"xs", "s", "m", "l", "xl"}
)

package compliance

// controlMapping defines a NIST 800-53 control and how motes implements it.
type controlMapping struct {
	ControlID   string
	Description string
}

// controlMappings returns the hardcoded control-to-implementation mappings.
func controlMappings() []controlMapping {
	return []controlMapping{
		{
			ControlID: "si-10",
			Description: "Input Validation (SI-10): All mote inputs are validated through " +
				"internal/security/validation.go functions including ValidateEnum, ValidateTag, " +
				"ValidateWeight, ValidateBodySize, and ValidatePathComponent. These enforce " +
				"allowlists, length limits, and character restrictions on all user-supplied data " +
				"before it reaches storage.",
		},
		{
			ControlID: "sc-28",
			Description: "Protection of Information at Rest (SC-28): Mote data is persisted " +
				"via AtomicWrite with explicit file permissions (0644). Soft-delete moves motes " +
				"to a trash/ directory rather than permanent deletion, preserving audit trails. " +
				"All file operations use atomic write-rename patterns to prevent data corruption.",
		},
		{
			ControlID: "ac-3",
			Description: "Access Enforcement (AC-3): Agent identity is resolved via " +
				"ResolveAgentID() and recorded in CreatedBy/ModifiedBy fields on every mote. " +
				"All mutating operations (create, update, delete, link, unlink) are logged to " +
				"an append-only audit.jsonl file with operation type, mote ID, agent ID, and " +
				"timestamp for non-repudiation.",
		},
	}
}

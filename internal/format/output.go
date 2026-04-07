// SPDX-License-Identifier: AGPL-3.0-or-later
package format

import (
	"fmt"
	"strings"
)

// Header prints a formatted section header.
func Header(title string) string {
	return fmt.Sprintf("=== %s ===", title)
}

// Field formats a key-value pair for terminal output.
func Field(key, value string) string {
	return fmt.Sprintf("  %-16s %s", key+":", value)
}

// TagList formats tags as a comma-separated string.
func TagList(tags []string) string {
	if len(tags) == 0 {
		return "(none)"
	}
	return strings.Join(tags, ", ")
}

// Truncate shortens a string to maxLen, adding "..." if truncated.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

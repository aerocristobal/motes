// SPDX-License-Identifier: MIT
package format

import "os"

// StatusIcon returns a single-character glyph for a mote lifecycle status.
// When ascii is true, it returns ASCII-only fallbacks suitable for terminals
// without UTF-8 support or for log piping.
func StatusIcon(status string, ascii bool) string {
	if ascii {
		switch status {
		case "active":
			return "o"
		case "in_progress":
			return "p"
		case "completed":
			return "x"
		case "archived":
			return "."
		case "deprecated":
			return "-"
		}
		return "?"
	}
	switch status {
	case "active":
		return "○"
	case "in_progress":
		return "◐"
	case "completed":
		return "✓"
	case "archived":
		return "●"
	case "deprecated":
		return "❄"
	}
	return "?"
}

// IconASCIIFromEnv reports whether NO_UNICODE is set to a truthy value,
// signaling that callers should use ASCII icons instead of Unicode.
func IconASCIIFromEnv() bool {
	v := os.Getenv("NO_UNICODE")
	return v != "" && v != "0" && v != "false"
}

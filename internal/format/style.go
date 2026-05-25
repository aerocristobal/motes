// SPDX-License-Identifier: MIT
package format

import (
	"os"
	"regexp"

	"golang.org/x/term"
)

// IsClosed reports whether a status counts as "closed" for rendering purposes.
// Closed motes are muted in human-readable output; live statuses (active,
// in_progress) render normally. Mirrors core.IsLive on the inverse axis but
// kept here so the format package has no dependency on core.
func IsClosed(status string) bool {
	switch status {
	case "completed", "archived", "deprecated":
		return true
	}
	return false
}

// IsTTY reports whether the given file descriptor is connected to a terminal.
// Honors MOTE_FORCE_TTY=1 as an internal test-only override (matches the
// MOTE_GLOBAL_ROOT precedent in internal/core/safety.go) — never document.
func IsTTY(fd uintptr) bool {
	if os.Getenv("MOTE_FORCE_TTY") == "1" {
		return true
	}
	return term.IsTerminal(int(fd))
}

// ShouldColor decides whether to emit ANSI escapes given the TTY state and
// the --no-color flag. The NO_COLOR env var (any non-empty value) overrides
// TTY detection. See https://no-color.org.
func ShouldColor(isTTY bool, noColorFlag bool) bool {
	if noColorFlag {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return isTTY
}

// Muted wraps s in dim+reset ANSI escapes when enabled, or returns s unchanged.
// Callers MUST pad/align the raw string before calling Muted so that ANSI
// escapes (which have zero visible width) don't disturb column alignment.
func Muted(s string, enabled bool) string {
	if !enabled {
		return s
	}
	return "\x1b[2m" + s + "\x1b[0m"
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// StripANSI removes CSI escape sequences. Used by tests to verify that ANSI
// wrapping has not disturbed column alignment.
func StripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

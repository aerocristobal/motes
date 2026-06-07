// SPDX-License-Identifier: MIT
package format

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"

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

// Background is the inferred terminal background brightness, used by
// AdaptiveColor to pick between light- and dark-variant hex values.
type Background int

const (
	// BackgroundDark is the default when COLORFGBG is absent or unparseable.
	BackgroundDark Background = iota
	// BackgroundLight is reported when COLORFGBG's background field is in
	// the high-color range (8..15).
	BackgroundLight
)

// AdaptiveColor is a pair of 24-bit hex colors keyed by background
// brightness. It is the type underlying every semantic token below.
type AdaptiveColor struct {
	Light string // 24-bit hex e.g. "#86b300"
	Dark  string // 24-bit hex e.g. "#c2d94c"
}

// Render wraps s in an ANSI 24-bit foreground sequence using the variant
// chosen by detectBackground(). When enabled is false (NO_COLOR, non-TTY,
// --no-color, --json) it returns s unchanged. Unparseable hex values fall
// through to the bare string rather than emitting a malformed escape.
func (a AdaptiveColor) Render(s string, enabled bool) string {
	if !enabled {
		return s
	}
	hex := a.Dark
	if detectBackground() == BackgroundLight {
		hex = a.Light
	}
	r, g, b, ok := parseHex(hex)
	if !ok {
		return s
	}
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", r, g, b, s)
}

// Semantic tokens. Pass/Warn/Fail hex values are pinned by the story
// (Ayu palette, verbatim from beads-recommendations §23.13). Accent/Muted/
// Command picks are Ayu Sublime/VS Code values selected during Three Amigos
// for STORY-COLOR-001.
var (
	PassColor    = AdaptiveColor{Light: "#86b300", Dark: "#c2d94c"}
	WarnColor    = AdaptiveColor{Light: "#f2ae49", Dark: "#ffb454"}
	FailColor    = AdaptiveColor{Light: "#f07171", Dark: "#f07178"}
	AccentColor  = AdaptiveColor{Light: "#399ee6", Dark: "#59c2ff"}
	MutedColor   = AdaptiveColor{Light: "#828c99", Dark: "#5c6773"}
	CommandColor = AdaptiveColor{Light: "#55b4d4", Dark: "#95e6cb"}
)

// Pass, Warn, Fail, Accent, Command are convenience renderers that consult
// package-global color state (set once by SetColorEnabled from main.go).
// Use these in code paths that do not already have a `useColor` bool in
// scope — e.g. simple stdout prints in cmd_doctor.go.
func Pass(s string) string    { return PassColor.Render(s, currentEnabled()) }
func Warn(s string) string    { return WarnColor.Render(s, currentEnabled()) }
func Fail(s string) string    { return FailColor.Render(s, currentEnabled()) }
func Accent(s string) string  { return AccentColor.Render(s, currentEnabled()) }
func Command(s string) string { return CommandColor.Render(s, currentEnabled()) }

// Muted wraps s in the SGR dim attribute when enabled, or returns s
// unchanged. The 2-arg signature and the literal "\x1b[2m" escape preserve
// the byte-for-byte STORY-MUTED-001 contract that downstream snapshot tests
// in cmd/mote pin on (TestLs_ForcedTTY_MutesClosedRows et al.).
//
// Note: Muted is intentionally implemented with the dim attribute rather
// than routed through MutedColor.Render. Dim composes multiplicatively with
// the terminal's existing foreground — it always reads as "secondary"
// regardless of background or color scheme. The MutedColor AdaptiveColor
// below documents what a hex-based Muted *would* look like, and is the
// value future renderers should reach for if they need an explicit fill
// (e.g. a header badge background); it is not what this function emits.
//
// Callers MUST pad/align the raw string before calling Muted so that ANSI
// escapes (which have zero visible width) don't disturb column alignment.
// The TestMuted_ColumnAlignment invariant guards this.
func Muted(s string, enabled bool) string {
	if !enabled {
		return s
	}
	return "\x1b[2m" + s + "\x1b[0m"
}

var (
	colorEnabled atomic.Bool
	testForceOff atomic.Bool
)

// SetColorEnabled sets the package-level color flag consulted by the
// 1-arg convenience tokens (Pass/Warn/Fail/Accent/Command). Called once
// from cmd/mote/main.go's PersistentPreRunE after flag parsing. Tokens
// called before this runs (e.g. during init) default to off.
func SetColorEnabled(enabled bool) {
	colorEnabled.Store(enabled)
}

// SetNoColorForTest forces currentEnabled() to return false when disabled
// is true, regardless of SetColorEnabled. Used by tests to exercise the
// suppression path deterministically without depending on env var ordering.
func SetNoColorForTest(disabled bool) {
	testForceOff.Store(disabled)
}

func currentEnabled() bool {
	if testForceOff.Load() {
		return false
	}
	return colorEnabled.Load()
}

// detectBackground parses COLORFGBG and returns BackgroundLight when the
// background field is in the rxvt/urxvt high-color range (8..15). The
// convention is shared by lipgloss and editorconfig: low color indices are
// dark, high indices are light. Absent or unparseable values default to
// BackgroundDark — safer to over-darken than to wash out an unintended
// light variant on a dark terminal.
func detectBackground() Background {
	fgbg := os.Getenv("COLORFGBG")
	if fgbg == "" {
		return BackgroundDark
	}
	fields := strings.Split(fgbg, ";")
	if len(fields) < 2 {
		return BackgroundDark
	}
	bg, err := strconv.Atoi(strings.TrimSpace(fields[len(fields)-1]))
	if err != nil {
		return BackgroundDark
	}
	if bg >= 8 && bg <= 15 {
		return BackgroundLight
	}
	return BackgroundDark
}

func parseHex(s string) (r, g, b int, ok bool) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int((n >> 16) & 0xff), int((n >> 8) & 0xff), int(n & 0xff), true
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// StripANSI removes CSI escape sequences. Used by tests to verify that ANSI
// wrapping has not disturbed column alignment.
func StripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

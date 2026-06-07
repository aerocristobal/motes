// SPDX-License-Identifier: MIT
package format

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// HeaderInput is the data RenderHeader needs. Kept as primitives so the
// format package does not import core (would create a cycle —
// core/ready_explanation.go already imports format).
type HeaderInput struct {
	ID     string
	Status string
	Title  string
	Weight float64
}

// HeaderMode selects pretty (ANSI + Unicode + right-aligned padding) or
// plain (ASCII + `  |  ` separator + no padding) rendering. STORY-HDRZ-001
// scenarios 1, 4 cover pretty; scenario 5 covers plain.
type HeaderMode int

const (
	// HeaderPretty produces ANSI-styled output right-aligned to width.
	HeaderPretty HeaderMode = iota
	// HeaderPlain produces colorless, no-padding output with `  |  ` zones.
	HeaderPlain
)

// PlainSeparator is the literal two-space-pipe-two-space joiner between the
// left and right zones in plain mode. Exported so tests in cmd/mote can
// reference the same literal without re-defining it.
const PlainSeparator = "  |  "

const (
	prettyEllipsis = "…"
	plainEllipsis  = "..."
	prettySepIcon  = "·"
	plainSepIcon   = "-"
)

// RenderHeader returns the two-zone header line for a mote — used by
// `mote show` first line and each row of `mote ls`.
//
//	Pretty: "<icon> <Accent(ID)> · <title>" + padding + "[<icon> <STATUS> w<weight>]"
//	Plain:  "<icon> <ID> - <title>  |  [<icon> <STATUS> w<weight>]"
//
// Closed motes (per IsClosed) drop the " w<weight>" segment from the right
// zone AND, in pretty mode with useColor, wrap the entire line in Muted.
// Plain mode never emits ANSI regardless of color flags.
//
// width is the visible-column target for pretty mode. Plain mode ignores
// it. A pretty title that does not fit is truncated with a single-glyph
// ellipsis so the right zone is always preserved at the right edge (STORY-HDRZ-001
// Scenario 4: "no information is dropped from the right zone").
func RenderHeader(in HeaderInput, width int, mode HeaderMode, ascii bool, useColor bool) string {
	closed := IsClosed(in.Status)
	statusUpper := strings.ToUpper(in.Status)

	if mode == HeaderPlain {
		icon := StatusIcon(in.Status, true)
		var right string
		if closed {
			right = fmt.Sprintf("[%s %s]", icon, statusUpper)
		} else {
			right = fmt.Sprintf("[%s %s w%.1f]", icon, statusUpper, in.Weight)
		}
		left := fmt.Sprintf("%s %s %s %s", icon, in.ID, plainSepIcon, in.Title)
		return left + PlainSeparator + right
	}

	// Pretty mode.
	icon := StatusIcon(in.Status, ascii)
	var right string
	if closed {
		right = fmt.Sprintf("[%s %s]", icon, statusUpper)
	} else {
		right = fmt.Sprintf("[%s %s w%.1f]", icon, statusUpper, in.Weight)
	}

	if width <= 0 {
		width = 80
	}

	// Visible prefix budget: "<icon> <id> · " (no ANSI bytes yet).
	leftPrefix := fmt.Sprintf("%s %s %s ", icon, in.ID, prettySepIcon)
	leftPrefixWidth := visibleWidth(leftPrefix)
	rightWidth := visibleWidth(right)
	// Reserve at least 1 col of padding between zones — never zero or the
	// eye loses the boundary even when widths exactly add up.
	titleBudget := width - leftPrefixWidth - rightWidth - 1
	if titleBudget < 1 {
		titleBudget = 1
	}
	titleVisible := truncateLeftZone(in.Title, titleBudget, false)

	leftVisible := leftPrefix + titleVisible
	leftVisibleWidth := visibleWidth(leftVisible)
	padding := width - leftVisibleWidth - rightWidth
	if padding < 1 {
		padding = 1
	}

	// Apply ANSI styling AFTER all width math (UI_PHILOSOPHY column-alignment
	// invariant: pad raw, wrap second).
	idStyled := AccentColor.Render(in.ID, useColor)
	leftStyled := fmt.Sprintf("%s %s %s %s", icon, idStyled, prettySepIcon, titleVisible)

	line := leftStyled + strings.Repeat(" ", padding) + right

	if closed && useColor {
		line = Muted(line, true)
	}
	return line
}

// truncateLeftZone returns title truncated to a visible-column budget. If
// truncation occurs, a trailing ellipsis (`…` for pretty, `...` for plain)
// replaces the dropped runes. A budget that cannot fit even the ellipsis
// returns the leading prefix of the title verbatim — losing visible info
// is preferable to producing a broken multi-byte sequence.
func truncateLeftZone(title string, budget int, plain bool) string {
	if budget <= 0 {
		return ""
	}
	runes := []rune(title)
	if len(runes) <= budget {
		return title
	}
	glyph := prettyEllipsis
	glyphWidth := 1
	if plain {
		glyph = plainEllipsis
		glyphWidth = 3
	}
	if budget <= glyphWidth {
		return string(runes[:budget])
	}
	keep := budget - glyphWidth
	return string(runes[:keep]) + glyph
}

// TerminalWidth returns the column count of the controlling terminal. The
// MOTE_FORCE_WIDTH env var overrides detection (test-only, undocumented —
// mirrors MOTE_FORCE_TTY in style.go). When nothing reports a width, 80.
func TerminalWidth() int {
	if v := os.Getenv("MOTE_FORCE_WIDTH"); v != "" {
		if w, err := strconv.Atoi(v); err == nil && w > 0 {
			return w
		}
	}
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}

// visibleWidth returns the column-cell width of s ignoring ANSI escapes.
// Each rune counts as one cell — adequate for the BMP glyphs mote ever
// emits (`○ ◐ ✓ ● ❄ · …`) but not a wcwidth substitute for arbitrary text.
func visibleWidth(s string) int {
	return utf8.RuneCountInString(StripANSI(s))
}

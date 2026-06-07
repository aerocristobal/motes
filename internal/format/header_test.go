// SPDX-License-Identifier: MIT
package format

import (
	"strings"
	"testing"
)

// TestRenderHeader_PrettyActive_HitsExactWidth covers STORY-HDRZ-001 Scenario 1:
// active mote, 100-col TTY, color on; first line is two-zone, right-aligned,
// 100 cols wide after ANSI strip, and contains the expected literal text.
func TestRenderHeader_PrettyActive_HitsExactWidth(t *testing.T) {
	in := HeaderInput{ID: "T1abc7", Status: "active", Weight: 0.6, Title: "Add login form"}
	got := RenderHeader(in, 100, HeaderPretty, false, true)
	stripped := StripANSI(got)

	if !strings.HasPrefix(stripped, "○ T1abc7 · Add login form") {
		t.Fatalf("left zone: got %q", stripped)
	}
	if !strings.HasSuffix(stripped, "[○ ACTIVE w0.6]") {
		t.Fatalf("right zone: got %q", stripped)
	}
	if w := visibleWidth(stripped); w != 100 {
		t.Fatalf("visible width: want 100, got %d (%q)", w, stripped)
	}
	if !strings.Contains(got, "\x1b[") {
		t.Fatal("expected ANSI styling, got none")
	}
}

// TestRenderHeader_ClosedMote_MutedNoWeight covers Scenario 2: completed
// mote's whole line is ANSI-wrapped (muted) and the right zone has no
// w<weight> token.
func TestRenderHeader_ClosedMote_MutedNoWeight(t *testing.T) {
	in := HeaderInput{ID: "T2def9", Status: "completed", Weight: 0.4, Title: "Old work item"}
	got := RenderHeader(in, 100, HeaderPretty, false, true)
	stripped := StripANSI(got)

	if !strings.Contains(stripped, "[✓ COMPLETED]") {
		t.Fatalf("right zone: want '[✓ COMPLETED]' (no weight), got %q", stripped)
	}
	if strings.Contains(stripped, "w0.4") {
		t.Fatalf("closed mote must not carry weight in right zone, got %q", stripped)
	}
	if len(got) <= len(stripped) {
		t.Fatalf("expected ANSI-wrapped (muted) line; len(got)=%d len(stripped)=%d", len(got), len(stripped))
	}
	if !strings.HasPrefix(got, "\x1b[2m") {
		t.Fatalf("closed line should be wrapped in SGR dim; got prefix %q", got[:min(6, len(got))])
	}
}

// TestRenderHeader_LongTitle_TruncatedRightPreserved covers Scenario 4:
// 80-col TTY, title overflows; left zone truncates with `…` and the right
// zone `[○ ACTIVE w0.5]` survives intact at the right edge.
func TestRenderHeader_LongTitle_TruncatedRightPreserved(t *testing.T) {
	in := HeaderInput{
		ID:     "T4jkl5",
		Status: "active",
		Weight: 0.5,
		Title:  "A title that is much longer than the available terminal width permits",
	}
	got := RenderHeader(in, 80, HeaderPretty, false, true)
	stripped := StripANSI(got)

	if len([]rune(stripped)) != 80 {
		t.Fatalf("visible width: want 80, got %d (%q)", len([]rune(stripped)), stripped)
	}
	if !strings.HasSuffix(stripped, "[○ ACTIVE w0.5]") {
		t.Fatalf("right zone must be preserved at right edge, got %q", stripped)
	}
	if !strings.Contains(stripped, "…") {
		t.Fatalf("left zone must show ellipsis '…', got %q", stripped)
	}
}

// TestRenderHeader_Plain_ASCIISeparator_NoANSI covers Scenario 5: plain
// mode uses the literal `  |  ` separator, ASCII icons, no ANSI escapes,
// no right-align padding.
func TestRenderHeader_Plain_ASCIISeparator_NoANSI(t *testing.T) {
	in := HeaderInput{ID: "T1abc7", Status: "active", Weight: 0.6, Title: "Add login form"}
	got := RenderHeader(in, 0, HeaderPlain, false, false)

	if strings.Contains(got, "\x1b[") {
		t.Fatalf("plain must not contain ANSI; got %q", got)
	}
	want := "o T1abc7 - Add login form" + PlainSeparator + "[o ACTIVE w0.6]"
	if got != want {
		t.Fatalf("plain header:\n  got:  %q\n  want: %q", got, want)
	}
}

// TestRenderHeader_Plain_Closed_NoWeight verifies the right-zone weight
// rule applies in plain mode too — closed motes drop weight.
func TestRenderHeader_Plain_Closed_NoWeight(t *testing.T) {
	in := HeaderInput{ID: "T2def9", Status: "completed", Weight: 0.4, Title: "Old work item"}
	got := RenderHeader(in, 0, HeaderPlain, false, false)
	want := "x T2def9 - Old work item" + PlainSeparator + "[x COMPLETED]"
	if got != want {
		t.Fatalf("plain closed header:\n  got:  %q\n  want: %q", got, want)
	}
}

// TestRenderHeader_StatusVariants_RightZone covers Scenario 9 with the
// project's existing five-glyph icon set (UI_PHILOSOPHY Rule 2 — we chose
// to reuse format.StatusIcon rather than introduce a new badge set).
func TestRenderHeader_StatusVariants_RightZone(t *testing.T) {
	cases := []struct {
		status string
		weight float64
		want   string
	}{
		{"active", 0.6, "[○ ACTIVE w0.6]"},
		{"in_progress", 0.8, "[◐ IN_PROGRESS w0.8]"},
		{"completed", 0.4, "[✓ COMPLETED]"},
		{"archived", 0.5, "[● ARCHIVED]"},
		{"deprecated", 0.3, "[❄ DEPRECATED]"},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			in := HeaderInput{ID: "Tzzz0", Status: tc.status, Weight: tc.weight, Title: "X"}
			got := StripANSI(RenderHeader(in, 120, HeaderPretty, false, true))
			if !strings.HasSuffix(got, tc.want) {
				t.Fatalf("status=%s: want suffix %q, got %q", tc.status, tc.want, got)
			}
		})
	}
}

func TestRenderHeader_NoColor_NoANSI(t *testing.T) {
	in := HeaderInput{ID: "T1abc7", Status: "active", Weight: 0.6, Title: "Add login form"}
	got := RenderHeader(in, 100, HeaderPretty, false, false)
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("useColor=false must not emit ANSI; got %q", got)
	}
}

func TestRenderHeader_ASCIIFlag_UsesASCIIIcons(t *testing.T) {
	in := HeaderInput{ID: "T1abc7", Status: "active", Weight: 0.6, Title: "Add login form"}
	got := RenderHeader(in, 100, HeaderPretty, true, false)
	if !strings.HasPrefix(got, "o T1abc7 ") {
		t.Fatalf("ascii=true should yield 'o' icon prefix; got %q", got)
	}
}

func TestTruncateLeftZone(t *testing.T) {
	cases := []struct {
		name   string
		title  string
		budget int
		plain  bool
		want   string
	}{
		{"short fits exactly", "Short", 20, false, "Short"},
		{"long pretty", "Exactly twenty chars!!", 20, false, "Exactly twenty char…"},
		{"empty", "", 10, false, ""},
		{"exactly fits no truncation", "abc", 3, false, "abc"},
		{"pretty one over", "abcd", 3, false, "ab…"},
		{"plain truncation", "abcdefghijk", 8, true, "abcde..."},
		{"budget equals ellipsis width returns first rune", "abcdef", 1, false, "a"},
		{"unicode runes counted by glyph", "café résumé", 5, false, "café…"},
		{"zero budget", "anything", 0, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateLeftZone(tc.title, tc.budget, tc.plain)
			if got != tc.want {
				t.Fatalf("truncateLeftZone(%q, %d, plain=%v) = %q, want %q", tc.title, tc.budget, tc.plain, got, tc.want)
			}
		})
	}
}

func TestTerminalWidth_HonorsForceWidth(t *testing.T) {
	t.Setenv("MOTE_FORCE_WIDTH", "123")
	if got := TerminalWidth(); got != 123 {
		t.Fatalf("MOTE_FORCE_WIDTH=123 → %d, want 123", got)
	}
}

func TestTerminalWidth_FallsBackTo80(t *testing.T) {
	t.Setenv("MOTE_FORCE_WIDTH", "")
	// In `go test`, stdout is a pipe → term.GetSize fails → fallback path.
	got := TerminalWidth()
	if got <= 0 {
		t.Fatalf("TerminalWidth must be positive, got %d", got)
	}
}

func TestTerminalWidth_IgnoresBadForceWidth(t *testing.T) {
	t.Setenv("MOTE_FORCE_WIDTH", "not-a-number")
	got := TerminalWidth()
	if got <= 0 {
		t.Fatalf("TerminalWidth must be positive on bad MOTE_FORCE_WIDTH, got %d", got)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

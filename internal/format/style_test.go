// SPDX-License-Identifier: MIT
package format

import (
	"strings"
	"testing"
)

func TestIsClosed(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"active", false},
		{"in_progress", false},
		{"completed", true},
		{"archived", true},
		{"deprecated", true},
		{"", false},
		{"unknown", false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := IsClosed(tt.status); got != tt.want {
				t.Errorf("IsClosed(%q): want %v, got %v", tt.status, tt.want, got)
			}
		})
	}
}

func TestShouldColor_Matrix(t *testing.T) {
	tests := []struct {
		name        string
		isTTY       bool
		noColorEnv  string // value of NO_COLOR; "" means unset
		noColorFlag bool
		want        bool
	}{
		{"tty_no_env_no_flag", true, "", false, true},
		{"non_tty", false, "", false, false},
		{"tty_with_no_color_env_1", true, "1", false, false},
		{"tty_with_no_color_env_anything", true, "anything", false, false},
		{"tty_with_no_color_env_unset", true, "", false, true},
		{"tty_with_no_color_flag", true, "", true, false},
		{"non_tty_with_no_color_flag", false, "", true, false},
		{"flag_beats_env_off_combo", true, "1", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tt.noColorEnv)
			got := ShouldColor(tt.isTTY, tt.noColorFlag)
			if got != tt.want {
				t.Errorf("ShouldColor(isTTY=%v, noColorFlag=%v, NO_COLOR=%q): want %v, got %v",
					tt.isTTY, tt.noColorFlag, tt.noColorEnv, tt.want, got)
			}
		})
	}
}

func TestMuted_AnsiWrapping(t *testing.T) {
	got := Muted("hello", true)
	if !strings.HasPrefix(got, "\x1b[2m") {
		t.Errorf("muted should begin with dim escape; got %q", got)
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("muted should end with reset escape; got %q", got)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("muted should preserve payload; got %q", got)
	}
}

func TestMuted_Passthrough(t *testing.T) {
	if got := Muted("hello", false); got != "hello" {
		t.Errorf("muted with color off: want passthrough; got %q", got)
	}
	if got := Muted("", true); got != "\x1b[2m\x1b[0m" {
		t.Errorf("muted empty string with color on: got %q", got)
	}
}

func TestMuted_ColumnAlignment(t *testing.T) {
	// ANSI escapes have zero visible width. Stripping them must leave the
	// original padded string intact so column alignment is preserved when a
	// renderer wraps a pre-padded row.
	raw := "proj-T1ABC                task            completed     0.50      Refactor router"
	wrapped := Muted(raw, true)
	if stripped := StripANSI(wrapped); stripped != raw {
		t.Errorf("stripped ANSI does not match raw.\nraw:      %q\nstripped: %q", raw, stripped)
	}
}

func TestStripANSI_RemovesCSI(t *testing.T) {
	in := "\x1b[2mfoo\x1b[0m bar \x1b[31mbaz\x1b[0m"
	want := "foo bar baz"
	if got := StripANSI(in); got != want {
		t.Errorf("StripANSI: want %q, got %q", want, got)
	}
}

func TestStripANSI_NoChangeWhenNoEscapes(t *testing.T) {
	in := "plain text, no escapes"
	if got := StripANSI(in); got != in {
		t.Errorf("StripANSI on plain text: want %q, got %q", in, got)
	}
}

func TestIsTTY_HonorsForceEnv(t *testing.T) {
	// In `go test` the process stdout is typically not a TTY. The override
	// must report true regardless so tests can deterministically exercise
	// the colored path.
	t.Setenv("MOTE_FORCE_TTY", "1")
	// Using a deliberately invalid fd (999) to prove the override short-circuits
	// before calling term.IsTerminal — i.e. we do not depend on the runtime
	// fd being a real terminal.
	if !IsTTY(999) {
		t.Error("IsTTY should return true when MOTE_FORCE_TTY=1 even for non-terminal fd")
	}
}

func TestIsTTY_ForceEnvUnsetFallsThroughToReal(t *testing.T) {
	t.Setenv("MOTE_FORCE_TTY", "")
	// fd 999 is not a terminal — the real check should return false.
	if IsTTY(999) {
		t.Error("IsTTY(999) should be false without MOTE_FORCE_TTY")
	}
}

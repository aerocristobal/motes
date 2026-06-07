// SPDX-License-Identifier: MIT
package format

import (
	"fmt"
	"strconv"
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

// ---- STORY-COLOR-001 — semantic token tests ----

// containsRGB checks whether s contains the SGR substring "38;2;R;G;B"
// corresponding to the given hex string ("#RRGGBB" or "RRGGBB").
func containsRGB(s, hex string) bool {
	h := strings.TrimPrefix(hex, "#")
	n, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return false
	}
	r := (n >> 16) & 0xff
	g := (n >> 8) & 0xff
	b := n & 0xff
	return strings.Contains(s, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
}

// enableColorForTest sets up the package-global gate so the 1-arg
// convenience tokens (Pass/Warn/Fail/Accent/Command) render in their
// colored form. Restores prior state on test cleanup.
func enableColorForTest(t *testing.T) {
	t.Helper()
	SetColorEnabled(true)
	SetNoColorForTest(false)
	t.Cleanup(func() {
		SetColorEnabled(false)
		SetNoColorForTest(false)
	})
}

func TestAdaptiveColor_Render_BareWhenDisabled(t *testing.T) {
	c := AdaptiveColor{Light: "#86b300", Dark: "#c2d94c"}
	if got := c.Render("payload", false); got != "payload" {
		t.Errorf("Render disabled: want bare 'payload', got %q", got)
	}
}

func TestAdaptiveColor_Render_FallsThroughOnBadHex(t *testing.T) {
	c := AdaptiveColor{Light: "not-a-hex", Dark: "also-bad"}
	if got := c.Render("payload", true); got != "payload" {
		t.Errorf("Render bad hex: want bare 'payload' (fail-safe), got %q", got)
	}
}

// Scenario 1: tokens emit ANSI on color-capable TTY, dark terminal default.
func TestTokens_EmitANSI_OnDarkTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("COLORFGBG", "15;0") // dark background
	t.Setenv("MOTE_FORCE_TTY", "1")
	enableColorForTest(t)

	cases := []struct {
		name    string
		token   func(string) string
		wantHex string
	}{
		{"Pass", Pass, "#c2d94c"},
		{"Warn", Warn, "#ffb454"},
		{"Fail", Fail, "#f07178"},
		{"Accent", Accent, "#59c2ff"},
		{"Command", Command, "#95e6cb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.token("payload")
			if !strings.HasPrefix(got, "\x1b[") {
				t.Fatalf("%s: want CSI-prefixed string, got %q", tc.name, got)
			}
			if !strings.HasSuffix(got, "\x1b[0m") {
				t.Fatalf("%s: want reset suffix, got %q", tc.name, got)
			}
			if !containsRGB(got, tc.wantHex) {
				t.Fatalf("%s: want 24-bit color %s in %q", tc.name, tc.wantHex, got)
			}
			if stripped := StripANSI(got); stripped != "payload" {
				t.Fatalf("%s: round-trip failed, got %q want %q", tc.name, stripped, "payload")
			}
		})
	}
}

// Scenario 2: NO_COLOR, --no-color, and non-TTY all suppress ANSI.
func TestTokens_NoColor_FullySuppressed(t *testing.T) {
	tokens := map[string]func(string) string{
		"Pass":    Pass,
		"Warn":    Warn,
		"Fail":    Fail,
		"Accent":  Accent,
		"Command": Command,
	}
	matrix := []struct {
		name        string
		noColor     string // NO_COLOR env value
		forceTTY    string // MOTE_FORCE_TTY env value
		noColorFlag bool
	}{
		{"NO_COLOR=1 + TTY", "1", "1", false},
		{"--no-color + TTY", "", "1", true},
		{"NO_COLOR=1 + no TTY", "1", "", false},
		{"no TTY, no flags", "", "", false},
		{"--no-color + no TTY", "", "", true},
		{"NO_COLOR=1 + --no-color", "1", "1", true},
	}
	for _, m := range matrix {
		for name, tok := range tokens {
			t.Run(m.name+"/"+name, func(t *testing.T) {
				t.Setenv("NO_COLOR", m.noColor)
				t.Setenv("MOTE_FORCE_TTY", m.forceTTY)
				// Simulate main.go's PersistentPreRunE: feed useColorOutput
				// equivalent into the package-global gate.
				enabled := ShouldColor(IsTTY(999) || m.forceTTY == "1", m.noColorFlag)
				SetColorEnabled(enabled)
				SetNoColorForTest(false)
				t.Cleanup(func() {
					SetColorEnabled(false)
					SetNoColorForTest(false)
				})

				got := tok("payload")
				if got != "payload" {
					t.Fatalf("%s/%s: want bare string, got %q", m.name, name, got)
				}
				if StripANSI(got) != "payload" {
					t.Fatalf("%s/%s: StripANSI should be a no-op on bare string", m.name, name)
				}
			})
		}
	}
}

// Scenario 3: light-terminal users get the Light variant hex.
func TestPass_AdaptiveColor_LightVariant(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("COLORFGBG", "0;15") // light background
	t.Setenv("MOTE_FORCE_TTY", "1")
	enableColorForTest(t)

	got := Pass("ok")
	if !containsRGB(got, "#86b300") {
		t.Fatalf("light variant: want #86b300 (RGB 134;179;0) in %q", got)
	}
	if containsRGB(got, "#c2d94c") {
		t.Fatalf("light variant: should NOT contain dark hex #c2d94c, got %q", got)
	}
}

// Scenario 4: dark-terminal users get the Dark variant hex.
func TestPass_AdaptiveColor_DarkVariant(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("COLORFGBG", "15;0") // dark background
	t.Setenv("MOTE_FORCE_TTY", "1")
	enableColorForTest(t)

	got := Pass("ok")
	if !containsRGB(got, "#c2d94c") {
		t.Fatalf("dark variant: want #c2d94c (RGB 194;217;76) in %q", got)
	}
	if containsRGB(got, "#86b300") {
		t.Fatalf("dark variant: should NOT contain light hex #86b300, got %q", got)
	}
}

func TestDetectBackground_COLORFGBG_Parsing(t *testing.T) {
	cases := []struct {
		env  string
		want Background
	}{
		{"", BackgroundDark},          // absent → default dark
		{"15;0", BackgroundDark},      // dark bg
		{"7;0", BackgroundDark},       // dark bg
		{"0;15", BackgroundLight},     // light bg
		{"15;15", BackgroundLight},    // light bg
		{"garbage", BackgroundDark},   // unparseable → default dark
		{"1;default", BackgroundDark}, // unparseable bg field
		{"0;7", BackgroundDark},       // bg=7 still in dark range
		{"0;8", BackgroundLight},      // bg=8 first light index
	}
	for _, tc := range cases {
		t.Run("COLORFGBG="+tc.env, func(t *testing.T) {
			t.Setenv("COLORFGBG", tc.env)
			if got := detectBackground(); got != tc.want {
				t.Fatalf("COLORFGBG=%q: got %v, want %v", tc.env, got, tc.want)
			}
		})
	}
}

func TestSetNoColorForTest_OverridesGlobalEnabled(t *testing.T) {
	SetColorEnabled(true)
	SetNoColorForTest(true)
	t.Cleanup(func() {
		SetColorEnabled(false)
		SetNoColorForTest(false)
	})
	if got := Pass("ok"); got != "ok" {
		t.Errorf("SetNoColorForTest(true) should suppress color even when SetColorEnabled(true); got %q", got)
	}
	SetNoColorForTest(false)
	t.Setenv("MOTE_FORCE_TTY", "1")
	if got := Pass("ok"); got == "ok" {
		t.Errorf("after SetNoColorForTest(false) and SetColorEnabled(true), Pass should re-color; got bare %q", got)
	}
}

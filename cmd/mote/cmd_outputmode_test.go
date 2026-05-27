// SPDX-License-Identifier: MIT
//
// STORY-PLAIN-001 — exhaustive coverage for the outputMode() decision helper.
// The helper is the single source of truth for the mode selection across every
// read command, so it gets its own table-driven test rather than redundant
// per-command coverage.
package main

import (
	"errors"
	"strings"
	"testing"
)

func resetModeFlags(t *testing.T) {
	t.Helper()
	plainFlag = false
	prettyFlag = false
	noColorFlag = false
	t.Cleanup(func() {
		plainFlag = false
		prettyFlag = false
		noColorFlag = false
	})
}

func TestOutputMode_NoFlags_ReturnsModeAuto(t *testing.T) {
	resetModeFlags(t)
	mode, err := outputMode(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != ModeAuto {
		t.Fatalf("mode = %v, want ModeAuto", mode)
	}
}

func TestOutputMode_JSONFlag_ReturnsModeJSON(t *testing.T) {
	resetModeFlags(t)
	mode, err := outputMode(true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != ModeJSON {
		t.Fatalf("mode = %v, want ModeJSON", mode)
	}
}

func TestOutputMode_PlainFlag_ReturnsModePlain(t *testing.T) {
	resetModeFlags(t)
	plainFlag = true
	mode, err := outputMode(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != ModePlain {
		t.Fatalf("mode = %v, want ModePlain", mode)
	}
}

func TestOutputMode_PrettyFlag_ReturnsModePretty(t *testing.T) {
	resetModeFlags(t)
	prettyFlag = true
	mode, err := outputMode(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != ModePretty {
		t.Fatalf("mode = %v, want ModePretty", mode)
	}
}

func TestOutputMode_MutexCombinations(t *testing.T) {
	cases := []struct {
		name                string
		json, plain, pretty bool
		wantPicked          []string
	}{
		{"json+plain", true, true, false, []string{"--json", "--plain"}},
		{"json+pretty", true, false, true, []string{"--json", "--pretty"}},
		{"plain+pretty", false, true, true, []string{"--pretty", "--plain"}},
		{"json+plain+pretty", true, true, true, []string{"--json", "--pretty", "--plain"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetModeFlags(t)
			plainFlag = tc.plain
			prettyFlag = tc.pretty
			_, err := outputMode(tc.json)
			if err == nil {
				t.Fatalf("expected mutex error, got nil")
			}
			var ec *exitCodeError
			if !errors.As(err, &ec) {
				t.Fatalf("expected *exitCodeError, got %T: %v", err, err)
			}
			if ec.code != 2 {
				t.Errorf("exit code = %d, want 2", ec.code)
			}
			msg := err.Error()
			for _, flag := range tc.wantPicked {
				if !strings.Contains(msg, flag) {
					t.Errorf("error message must name %q; got %q", flag, msg)
				}
			}
		})
	}
}

func TestUseColorOutput_NoColorFlag_AlwaysFalse(t *testing.T) {
	resetModeFlags(t)
	noColorFlag = true
	prettyFlag = true // even with --pretty, --no-color wins
	t.Setenv("MOTE_FORCE_TTY", "1")
	if useColorOutput() {
		t.Fatal("useColorOutput must return false when --no-color is set, even with --pretty")
	}
}

func TestUseColorOutput_PrettyForcesOnNonTTY(t *testing.T) {
	resetModeFlags(t)
	prettyFlag = true
	t.Setenv("MOTE_FORCE_TTY", "")
	if !useColorOutput() {
		t.Fatal("useColorOutput must return true on non-TTY when --pretty is set")
	}
}

func TestUseColorOutput_AutoNonTTY_NoColor(t *testing.T) {
	resetModeFlags(t)
	t.Setenv("MOTE_FORCE_TTY", "")
	if useColorOutput() {
		t.Fatal("useColorOutput must return false on non-TTY without --pretty")
	}
}

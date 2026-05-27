// SPDX-License-Identifier: MIT
//
// STORY-PLAIN-001 — coverage for `mote show <id> --plain` across default,
// --short, --long, and --execution-only content selectors.
package main

import (
	"strings"
	"testing"

	"motes/internal/core"
)

func TestShowPlain_KeyValueLines_NoTufteChrome(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	seeded, err := mm.Create("decision", "important decision", core.CreateOpts{Tags: []string{"alpha", "beta"}})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	resetModeFlags(t)
	plainFlag = true
	resetShowFlags()
	defer resetShowFlags()

	stdout, _ := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"show", seeded.ID, "--plain"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("plain show emitted ANSI: %q", stdout)
	}
	if strings.Contains(stdout, "=== ") || strings.Contains(stdout, "--- ") {
		t.Errorf("plain show emitted Tufte chrome: %q", stdout)
	}

	// Must include at minimum: id, type, status, title, tag (one per tag), weight, origin.
	mustHave := []string{
		"id: " + seeded.ID,
		"type: decision",
		"status: active",
		"title: important decision",
		"tag: alpha",
		"tag: beta",
		"weight: ",
		"origin: ",
	}
	for _, want := range mustHave {
		if !strings.Contains(stdout, want) {
			t.Errorf("plain show missing %q; full output:\n%s", want, stdout)
		}
	}

	// `format.Field` pads keys to 16 chars with `%-16s`. Plain must NOT use it,
	// so no line should contain the run of spaces that padding produces.
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "    ") {
			t.Errorf("plain show emitted padded line: %q", line)
		}
	}
}

func TestShowShortPlain_SingleLine_NoIcon_NoANSI(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	seeded, err := mm.Create("task", "short plain target", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	resetModeFlags(t)
	plainFlag = true
	resetShowFlags()
	showShort = true
	defer resetShowFlags()

	stdout, _ := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"show", seeded.ID, "--short", "--plain"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("--short --plain leaked ANSI: %q", stdout)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("--short --plain must emit one line; got %d lines: %q", len(lines), stdout)
	}
	if !strings.HasPrefix(lines[0], seeded.ID+" ") {
		t.Errorf("first whitespace token must be mote id; got %q", lines[0])
	}
	if !strings.Contains(lines[0], "[task]") {
		t.Errorf("--short --plain must include [type]; got %q", lines[0])
	}
}

func TestShowExecutionOnlyPlain_EmitsKeyValueLines(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	seeded, err := mm.Create("task", "exec only", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	model := "opus"
	emode := "delegated"
	if err := mm.Update(seeded.ID, core.UpdateOpts{
		ExecutionSuggestedModel: &model,
		ExecutionMode:           &emode,
	}); err != nil {
		t.Fatalf("set execution metadata: %v", err)
	}

	resetModeFlags(t)
	plainFlag = true
	resetShowFlags()
	showExecutionOnly = true
	defer resetShowFlags()

	stdout, _ := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"show", seeded.ID, "--execution-only", "--plain"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if strings.Contains(stdout, "{") {
		t.Errorf("--execution-only --plain must NOT emit JSON; got %q", stdout)
	}
	if !strings.Contains(stdout, "id: "+seeded.ID) {
		t.Errorf("--execution-only --plain must emit id: line; got %q", stdout)
	}
	if !strings.Contains(stdout, "execution_suggested_model: opus") {
		t.Errorf("--execution-only --plain missing execution_suggested_model; got %q", stdout)
	}
	if !strings.Contains(stdout, "execution_mode: delegated") {
		t.Errorf("--execution-only --plain missing execution_mode; got %q", stdout)
	}
}

func TestShowLongPlain_IncludesInternalState(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	seeded, err := mm.Create("task", "long plain", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	resetModeFlags(t)
	plainFlag = true
	resetShowFlags()
	showLong = true
	defer resetShowFlags()

	stdout, _ := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"show", seeded.ID, "--long", "--plain"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if strings.Contains(stdout, "--- internal state ---") {
		t.Errorf("--long --plain must NOT emit Tufte section header; got %q", stdout)
	}
	if !strings.Contains(stdout, "audit_log_path: ") {
		t.Errorf("--long --plain must include audit_log_path; got %q", stdout)
	}
	if !strings.Contains(stdout, "audit_log_entries_count: ") {
		t.Errorf("--long --plain must include audit_log_entries_count; got %q", stdout)
	}
}

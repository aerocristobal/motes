// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// STORY-BR-23-2 — Truncation directive prepended to every successful
// `mote prime` output, plus persistence of the full body to
// .memory/last_prime.txt for agents whose host has truncated the preview.

const wantDirectivePrefix = "[mote prime]"

func firstNonBlankLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			return line
		}
	}
	return ""
}

// --- Scenario 1, 4, 5: directive begins text output across modes (and empty project). ---

func TestPrime_TextOutput_BeginsWithDirective(t *testing.T) {
	tests := []struct {
		name   string
		seeded bool
		mode   string
	}{
		{"empty project, startup mode", false, "startup"},
		{"empty project, resume mode", false, "resume"},
		{"empty project, compact mode", false, "compact"},
		{"populated project, startup mode", true, "startup"},
		{"populated project, resume mode", true, "resume"},
		{"populated project, compact mode", true, "compact"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, cleanup := setupIntegrationTest(t)
			defer cleanup()

			if tt.seeded {
				seedMotes(t, root, []moteSpec{
					{Type: "task", Title: "Active task", Tags: []string{"topic"}, Weight: 0.5},
				})
			}

			origMode := primeMode
			primeMode = tt.mode
			defer func() { primeMode = origMode }()

			output := captureStdout(func() {
				if err := primeCmd.RunE(primeCmd, nil); err != nil {
					t.Fatalf("prime: %v", err)
				}
			})

			first := firstNonBlankLine(output)
			if !strings.HasPrefix(first, wantDirectivePrefix) {
				t.Errorf("first non-blank line of stdout does not start with %q\nfirst line: %q\nfull output:\n%s",
					wantDirectivePrefix, first, output)
			}
		})
	}
}

// --- Scenario 2: JSON output carries the directive as a top-level field. ---

func TestPrime_JSONOutput_IncludesTruncationNotice(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "JSON task", Tags: []string{"json"}, Weight: 0.5},
	})

	primeJSON = true
	defer func() { primeJSON = false }()

	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime --json: %v", err)
		}
	})

	// Scenario 2: the *full* output must parse as valid JSON — no text prelude.
	var parsed PrimeOutput
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("full --json output is not valid JSON: %v\noutput:\n%s", err, output)
	}
	if parsed.TruncationNotice != truncationDirective {
		t.Errorf("TruncationNotice mismatch\n got:  %q\n want: %q",
			parsed.TruncationNotice, truncationDirective)
	}
	// Sanity: existing field still serialized.
	if parsed.ActiveTasks == nil {
		t.Error("ActiveTasks should still be present (even if empty slice)")
	}
}

// --- Scenario 2 (empty project): --json on a project with no active tasks
// must still emit valid JSON with truncation_notice (not text fallback). ---

func TestPrime_JSONOutput_EmptyProject_StillValidJSON(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	primeJSON = true
	defer func() { primeJSON = false }()

	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime --json on empty: %v", err)
		}
	})

	var parsed PrimeOutput
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("empty-project --json output is not valid JSON: %v\noutput:\n%s", err, output)
	}
	if parsed.TruncationNotice != truncationDirective {
		t.Errorf("TruncationNotice mismatch on empty project")
	}
}

// --- Scenario 3: Hook envelope wraps body whose first non-blank line is the directive. ---

func TestPrime_HookOutput_DirectiveInsideAdditionalContext(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Hook task", Tags: []string{"hook"}, Weight: 0.5},
	})

	primeHook = true
	defer func() { primeHook = false }()

	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime --hook: %v", err)
		}
	})

	var env struct {
		AdditionalContext string `json:"additionalContext"`
	}
	idx := strings.Index(output, "{")
	if idx < 0 {
		t.Fatalf("no JSON found in --hook output:\n%s", output)
	}
	if err := json.Unmarshal([]byte(output[idx:]), &env); err != nil {
		t.Fatalf("invalid hook envelope JSON: %v\noutput:\n%s", err, output)
	}
	first := firstNonBlankLine(env.AdditionalContext)
	if !strings.HasPrefix(first, wantDirectivePrefix) {
		t.Errorf("additionalContext does not begin with directive\nfirst line: %q\nadditionalContext:\n%s",
			first, env.AdditionalContext)
	}
}

// --- Scenario 5 (focused): empty project still emits the directive followed by canonical fallback. ---

func TestPrime_EmptyProject_StillEmitsDirective(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	output := captureStdout(func() {
		if err := primeCmd.RunE(primeCmd, nil); err != nil {
			t.Fatalf("prime on empty project: %v", err)
		}
	})

	if !strings.HasPrefix(firstNonBlankLine(output), wantDirectivePrefix) {
		t.Errorf("directive missing from empty-project output:\n%s", output)
	}
	// Empty project triggers the "No active tasks. Showing top motes by weight:" fallback.
	if !strings.Contains(output, "No active tasks") {
		t.Errorf("expected 'No active tasks' canonical message after directive, got:\n%s", output)
	}
}

// --- Scenario 6: directive text is the exact, stable constant downstream parsers can pin. ---

func TestPrime_DirectiveText_IsExactConstant(t *testing.T) {
	want := "[mote prime] If this output is truncated by your host, " +
		"read the full persisted output at .memory/last_prime.txt before continuing; " +
		"it may contain project memories and session rules not visible in the preview."
	if truncationDirective != want {
		t.Errorf("truncationDirective drift detected\n got:  %q\n want: %q", truncationDirective, want)
	}
}

// --- Persistence support: .memory/last_prime.txt is written on every successful prime. ---
//
// Across all three invocation modes (text, JSON, hook), the directive must
// appear in the persisted body — as the first non-blank line for text/hook
// and inside the JSON envelope for --json (where stdout otherwise carries a
// pre-existing prelude of text section headers before the JSON block).
func TestPrime_PersistsLastPrimeFile(t *testing.T) {
	tests := []struct {
		name      string
		setFlags  func()
		resetFlag func()
	}{
		{
			name:      "text mode",
			setFlags:  func() {},
			resetFlag: func() {},
		},
		{
			name:      "json mode",
			setFlags:  func() { primeJSON = true },
			resetFlag: func() { primeJSON = false },
		},
		{
			name:      "hook mode",
			setFlags:  func() { primeHook = true },
			resetFlag: func() { primeHook = false },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, cleanup := setupIntegrationTest(t)
			defer cleanup()

			seedMotes(t, root, []moteSpec{
				{Type: "task", Title: "Persist task", Tags: []string{"persist"}, Weight: 0.5},
			})

			tt.setFlags()
			defer tt.resetFlag()

			_ = captureStdout(func() {
				if err := primeCmd.RunE(primeCmd, nil); err != nil {
					t.Fatalf("prime: %v", err)
				}
			})

			path := filepath.Join(root, "last_prime.txt")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("last_prime.txt not written at %s: %v", path, err)
			}
			if len(data) == 0 {
				t.Fatal("last_prime.txt is empty")
			}
			if !strings.Contains(string(data), wantDirectivePrefix) {
				t.Errorf("last_prime.txt missing directive %q\nbody:\n%s",
					wantDirectivePrefix, string(data))
			}
		})
	}
}

// SPDX-License-Identifier: MIT
//
// STORY-PLAIN-001 — coverage for `mote stats --plain`. Q5 confirms one-fact-
// per-line keyed exactly to the StatsOutput JSON field names.
package main

import (
	"strings"
	"testing"

	"motes/internal/core"
)

func TestStatsPlain_OneFactPerLine_KeyNamesMatchJSON(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	if _, err := mm.Create("task", "stats plain target", core.CreateOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	resetModeFlags(t)
	plainFlag = true
	statsJSON = false
	statsDecayPreview = false
	defer func() { statsJSON = false }()

	stdout, _ := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"stats", "--plain"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("plain stats emitted ANSI: %q", stdout)
	}
	if strings.Contains(stdout, "=== ") {
		t.Errorf("plain stats emitted Tufte header; got %q", stdout)
	}
	// Verify the core key set matches the JSON field names exactly.
	mustHave := []string{
		"total_motes: ",
		"status_active: ",
		"accessed_7d: ",
		"accessed_30d: ",
		"accessed_90d: ",
		"never_accessed: ",
		"total_tags: ",
		"overloaded_tags: ",
		"singleton_tags: ",
		"contradictions: ",
		"pending_visions: ",
		"created_7d: ",
		"created_30d: ",
		"created_90d: ",
		"deprecated_7d: ",
		"deprecated_30d: ",
		"deprecated_90d: ",
		"net_growth_7d: ",
		"net_growth_30d: ",
		"net_growth_90d: ",
	}
	for _, want := range mustHave {
		if !strings.Contains(stdout, want) {
			t.Errorf("plain stats missing key %q; full output:\n%s", want, stdout)
		}
	}
	for _, line := range strings.Split(stdout, "\n") {
		if line == "" {
			continue
		}
		if !strings.Contains(line, ": ") {
			t.Errorf("plain stats line lacks key: value form: %q", line)
		}
	}
}

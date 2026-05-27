// SPDX-License-Identifier: MIT
//
// STORY-PLAIN-001 — coverage for `mote search <query> --plain`.
package main

import (
	"strings"
	"testing"

	"motes/internal/core"
)

func TestSearchPlain_FirstTokenIsMoteID_NoChrome(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	seeded, err := mm.Create("decision", "rare-search-keyword-qqq plain target", core.CreateOpts{})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	all, _ := mm.ReadAllWithGlobal()
	_ = rebuildMoteBM25(root, all)

	resetModeFlags(t)
	plainFlag = true
	searchJSON = false
	defer func() { searchJSON = false }()

	stdout, _ := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"search", "rare-search-keyword-qqq", "--plain"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("plain search emitted ANSI: %q", stdout)
	}
	if strings.Contains(stdout, "SCORE") {
		t.Errorf("plain search emitted header row: %q", stdout)
	}
	if strings.Contains(stdout, "---") {
		t.Errorf("plain search emitted separator: %q", stdout)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) < 1 {
		t.Fatal("expected at least one search result")
	}
	first := strings.SplitN(lines[0], " ", 2)[0]
	if first != seeded.ID {
		t.Errorf("first token = %q, want mote id %q", first, seeded.ID)
	}
}

// SPDX-License-Identifier: MIT
//
// STORY-PLAIN-001 — coverage for `mote context <topic> --plain`.
package main

import (
	"strings"
	"testing"

	"motes/internal/core"
)

func TestContextPlain_OneResultPerLine_NoChrome(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(root)
	if _, err := mm.Create("decision", "deciduous-tree rare-context-keyword-zzz", core.CreateOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	all, err := mm.ReadAllWithGlobal()
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if err := rebuildMoteBM25(root, all); err != nil {
		t.Fatalf("rebuild bm25: %v", err)
	}
	im := core.NewIndexManager(root)
	if err := im.Rebuild(all); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}

	resetModeFlags(t)
	plainFlag = true
	contextJSON = false
	defer func() { contextJSON = false }()

	stdout, _ := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"context", "rare-context-keyword-zzz", "--plain"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("plain context emitted ANSI: %q", stdout)
	}
	if strings.Contains(stdout, "SCORE") {
		t.Errorf("plain context emitted header row: %q", stdout)
	}
	if strings.Contains(stdout, "---") || strings.Contains(stdout, "===") {
		t.Errorf("plain context emitted separator: %q", stdout)
	}
	// Must have at least one result line, and the first token must be a mote id.
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("plain context must emit at least one result line")
	}
	first := strings.SplitN(lines[0], " ", 2)[0]
	if !strings.Contains(first, "-") {
		t.Errorf("first token does not look like a mote id: %q", first)
	}
}

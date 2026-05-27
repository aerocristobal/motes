// SPDX-License-Identifier: MIT
//
// STORY-PLAIN-001 — coverage for `mote memories --plain`.
package main

import (
	"strings"
	"testing"

	"motes/internal/core"
)

func TestMemoriesPlain_KeyColonValue_NoPadding(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	store := core.NewMemoryStore(root)
	if _, err := store.Put("alpha", "first body", "test", core.PutOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := store.Put("longer-key", "second body content here", "test", core.PutOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	resetModeFlags(t)
	plainFlag = true
	memoriesJSON = false
	defer func() { memoriesJSON = false }()

	stdout, _ := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"memories", "--plain"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), stdout)
	}
	for _, line := range lines {
		if strings.Contains(line, "  ") {
			t.Errorf("plain memories line contains padding: %q", line)
		}
		if !strings.Contains(line, ": ") {
			t.Errorf("plain memories line not key: value: %q", line)
		}
	}
	if !strings.HasPrefix(lines[0], "alpha:") {
		t.Errorf("expected first line to begin with key 'alpha:'; got %q", lines[0])
	}
}

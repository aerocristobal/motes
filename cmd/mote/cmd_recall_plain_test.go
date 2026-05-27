// SPDX-License-Identifier: MIT
//
// STORY-PLAIN-001 — sanity coverage for `mote recall --plain`. Recall's output
// is already raw plain text (just the body + newline), so --plain is a no-op
// for the actual rendering. The test exists to verify that the persistent
// flag is accepted on the command without error, and that the body output
// is byte-identical to the no-flag form.
package main

import (
	"testing"

	"motes/internal/core"
)

func TestRecallPlain_AcceptedAsNoOp(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	store := core.NewMemoryStore(root)
	if _, err := store.Put("the-key", "the body", "test", core.PutOpts{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	resetModeFlags(t)

	// Baseline: no flag.
	plainFlag = false
	baseline, _ := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"recall", "the-key"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("recall (baseline): %v", err)
		}
	})

	// With --plain.
	plainFlag = true
	withFlag, _ := captureBothStreams(t, func() {
		rootCmd.SetArgs([]string{"recall", "the-key", "--plain"})
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("recall (--plain): %v", err)
		}
	})

	if baseline != withFlag {
		t.Fatalf("recall --plain must be byte-identical to no-flag form;\n baseline=%q\n withFlag=%q", baseline, withFlag)
	}
}

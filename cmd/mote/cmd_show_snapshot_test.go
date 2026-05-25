// SPDX-License-Identifier: MIT
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Scenario 1: default `mote show` output is byte-stable against a golden fixture.
//
// Regenerate the fixture with: UPDATE_GOLDEN=1 go test ./cmd/mote/ -run TestShow_DefaultOutput_ByteStableAgainstSnapshot
//
// The fixture uses createDeterministicMote so timestamps and IDs are stable;
// the snapshot is therefore byte-identical across releases unless the renderer
// or default field set changes intentionally.
func TestShow_DefaultOutput_ByteStableAgainstSnapshot(t *testing.T) {
	// Resolve the golden path BEFORE setupIntegrationTest, which chdirs into a
	// tempdir. Otherwise relative writes/reads target the wrong directory.
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	goldenPath := filepath.Join(origCwd, "testdata", "show_default.golden")

	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	createDeterministicMote(t, root, "proj-T1ABC", "Add login flow")

	resetShowFlags()
	defer resetShowFlags()

	got := captureStdout(func() {
		if err := showCmd.RunE(showCmd, []string{"proj-T1ABC"}); err != nil {
			t.Fatalf("runShow: %v", err)
		}
	})

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("regenerated %s (%d bytes)", goldenPath, len(got))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run UPDATE_GOLDEN=1 to create): %v", err)
	}
	if got != string(want) {
		t.Errorf("default output drifted from snapshot.\n--- want (%d bytes) ---\n%s\n--- got (%d bytes) ---\n%s",
			len(want), string(want), len(got), got)
	}
}

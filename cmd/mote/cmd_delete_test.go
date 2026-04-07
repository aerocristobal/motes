// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/core"
)

func TestDeleteCommand(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Create a mote
	mm := core.NewMoteManager(memDir)
	m, err := mm.Create("lesson", "Delete test", core.CreateOpts{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Run delete command
	output := captureStdout(func() {
		rootCmd.SetArgs([]string{"delete", m.ID})
		rootCmd.Execute()
	})

	if !strings.Contains(output, "Deleted") {
		t.Errorf("expected 'Deleted' in output, got: %s", output)
	}
	if !strings.Contains(output, m.ID) {
		t.Errorf("expected mote ID in output, got: %s", output)
	}

	// Verify mote is in trash
	trashPath := filepath.Join(memDir, "trash", m.ID+".md")
	if _, err := os.Stat(trashPath); err != nil {
		t.Errorf("mote should exist in trash: %v", err)
	}

	// Verify mote is not in nodes
	nodePath := filepath.Join(memDir, "nodes", m.ID+".md")
	if _, err := os.Stat(nodePath); !os.IsNotExist(err) {
		t.Error("mote should not exist in nodes after delete")
	}
}

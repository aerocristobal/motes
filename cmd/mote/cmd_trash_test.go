package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"motes/internal/core"
)

func TestTrashListCommand(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Empty trash
	output := captureStdout(func() {
		rootCmd.SetArgs([]string{"trash", "list"})
		rootCmd.Execute()
	})
	if !strings.Contains(output, "empty") {
		t.Errorf("expected 'empty' for empty trash, got: %s", output)
	}

	// Delete a mote, then list
	mm := core.NewMoteManager(memDir)
	im := core.NewIndexManager(memDir)
	im.Load()
	m, _ := mm.Create("lesson", "Trashed mote", core.CreateOpts{})
	mm.Delete(m.ID, im)

	output = captureStdout(func() {
		rootCmd.SetArgs([]string{"trash", "list"})
		rootCmd.Execute()
	})
	if !strings.Contains(output, m.ID) {
		t.Errorf("expected mote ID in trash list, got: %s", output)
	}
}

func TestTrashRestoreCommand(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(memDir)
	im := core.NewIndexManager(memDir)
	im.Load()
	m, _ := mm.Create("decision", "Restore me", core.CreateOpts{})
	mm.Delete(m.ID, im)

	output := captureStdout(func() {
		rootCmd.SetArgs([]string{"trash", "restore", m.ID})
		rootCmd.Execute()
	})

	if !strings.Contains(output, "Restored") {
		t.Errorf("expected 'Restored' in output, got: %s", output)
	}

	// Verify mote is back in nodes
	nodePath := filepath.Join(memDir, "nodes", m.ID+".md")
	if _, err := os.Stat(nodePath); err != nil {
		t.Errorf("mote should exist in nodes after restore: %v", err)
	}
}

func TestTrashPurgeAllCommand(t *testing.T) {
	memDir, cleanup := setupIntegrationTest(t)
	defer cleanup()

	mm := core.NewMoteManager(memDir)
	im := core.NewIndexManager(memDir)
	im.Load()
	m, _ := mm.Create("lesson", "Purge me", core.CreateOpts{})
	mm.Delete(m.ID, im)

	output := captureStdout(func() {
		rootCmd.SetArgs([]string{"trash", "purge", "--all"})
		rootCmd.Execute()
	})

	if !strings.Contains(output, "Permanently deleted") {
		t.Errorf("expected 'Permanently deleted' in output, got: %s", output)
	}

	// Verify mote is gone from trash
	trashPath := filepath.Join(memDir, "trash", m.ID+".md")
	if _, err := os.Stat(trashPath); !os.IsNotExist(err) {
		t.Error("mote should be permanently deleted from trash")
	}
}

func TestTrashRetentionCommand(t *testing.T) {
	_, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Show default retention
	output := captureStdout(func() {
		rootCmd.SetArgs([]string{"trash", "retention"})
		rootCmd.Execute()
	})
	if !strings.Contains(output, "30") {
		t.Errorf("expected default retention of 30 days, got: %s", output)
	}

	// Set retention
	output = captureStdout(func() {
		rootCmd.SetArgs([]string{"trash", "retention", "14"})
		rootCmd.Execute()
	})
	if !strings.Contains(output, "14") {
		t.Errorf("expected retention set to 14, got: %s", output)
	}
}

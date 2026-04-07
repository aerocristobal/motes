// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"strings"
	"testing"

	"motes/internal/core"
)

func TestLinkDryRun_Symmetric(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "decision", Title: "Decision A", Tags: []string{"test"}},
		{Type: "decision", Title: "Decision B", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	motes, _ := mm.List(core.ListFilters{Type: "decision"})
	if len(motes) < 2 {
		t.Fatal("need 2 motes")
	}
	idA, idB := motes[0].ID, motes[1].ID

	linkDryRun = true
	defer func() { linkDryRun = false }()

	output := captureStdout(func() {
		linkCmd.RunE(linkCmd, []string{idA, "relates_to", idB})
	})

	if !strings.Contains(output, "[dry-run]") {
		t.Errorf("expected [dry-run] prefix, got:\n%s", output)
	}
	if !strings.Contains(output, "add link") {
		t.Errorf("expected 'add link' effect, got:\n%s", output)
	}
	if !strings.Contains(output, "add symmetric link") {
		t.Errorf("expected 'add symmetric link' effect, got:\n%s", output)
	}

	// Verify nothing was written
	mA, _ := mm.Read(idA)
	if len(mA.RelatesTo) > 0 {
		t.Error("dry-run should not have written links")
	}
}

func TestLinkDryRun_Inverse(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Task A", Tags: []string{"test"}},
		{Type: "task", Title: "Task B", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	idA, idB := motes[0].ID, motes[1].ID

	linkDryRun = true
	defer func() { linkDryRun = false }()

	output := captureStdout(func() {
		linkCmd.RunE(linkCmd, []string{idA, "depends_on", idB})
	})

	if !strings.Contains(output, "add inverse link") {
		t.Errorf("expected 'add inverse link' effect, got:\n%s", output)
	}

	// Verify nothing was written
	mA, _ := mm.Read(idA)
	if len(mA.DependsOn) > 0 {
		t.Error("dry-run should not have written links")
	}
}

func TestLinkDryRun_AutoDeprecate(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "decision", Title: "New Decision", Tags: []string{"test"}},
		{Type: "decision", Title: "Old Decision", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	motes, _ := mm.List(core.ListFilters{Type: "decision"})
	idNew, idOld := motes[0].ID, motes[1].ID

	linkDryRun = true
	defer func() { linkDryRun = false }()

	output := captureStdout(func() {
		linkCmd.RunE(linkCmd, []string{idNew, "supersedes", idOld})
	})

	if !strings.Contains(output, "auto-deprecate target") {
		t.Errorf("expected 'auto-deprecate target' effect, got:\n%s", output)
	}

	// Verify target not deprecated
	mOld, _ := mm.Read(idOld)
	if mOld.Status == "deprecated" {
		t.Error("dry-run should not have deprecated target")
	}
}

func TestLinkDryRun_AlreadyLinked(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "decision", Title: "Decision A", Tags: []string{"test"}},
		{Type: "decision", Title: "Decision B", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	im.Load()
	motes, _ := mm.List(core.ListFilters{Type: "decision"})
	idA, idB := motes[0].ID, motes[1].ID

	// Actually create the link first
	mm.Link(idA, "relates_to", idB, im)

	linkDryRun = true
	defer func() { linkDryRun = false }()

	output := captureStdout(func() {
		linkCmd.RunE(linkCmd, []string{idA, "relates_to", idB})
	})

	if !strings.Contains(output, "no-op") {
		t.Errorf("expected 'no-op' for already-linked motes, got:\n%s", output)
	}
}

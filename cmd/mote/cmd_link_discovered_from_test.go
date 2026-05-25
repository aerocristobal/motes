// SPDX-License-Identifier: MIT
package main

import (
	"strings"
	"testing"

	"motes/internal/core"
)

// STORY-DISC-001 Scenario 1: linkCmd writes A's frontmatter, leaves B's alone,
// and prints "Linked <A> --discovered_from--> <B>".
func TestLink_DiscoveredFrom_HappyPath(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Source", Tags: []string{"test"}},
		{Type: "task", Title: "Follow-up", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	if len(motes) < 2 {
		t.Fatal("need 2 motes")
	}
	idSource, idFollowup := motes[0].ID, motes[1].ID

	output := captureStdout(func() {
		_ = linkCmd.RunE(linkCmd, []string{idFollowup, "discovered_from", idSource})
	})

	expected := "Linked " + idFollowup + " --discovered_from--> " + idSource
	if !strings.Contains(output, expected) {
		t.Errorf("missing %q in output:\n%s", expected, output)
	}

	mFollowup, _ := mm.Read(idFollowup)
	if len(mFollowup.DiscoveredFrom) != 1 || mFollowup.DiscoveredFrom[0] != idSource {
		t.Errorf("follow-up DiscoveredFrom = %v, want [%s]", mFollowup.DiscoveredFrom, idSource)
	}

	mSource, _ := mm.Read(idSource)
	if len(mSource.DiscoveredFrom) != 0 {
		t.Errorf("source frontmatter must not gain DiscoveredFrom, got %v", mSource.DiscoveredFrom)
	}
}

// STORY-DISC-001 Scenario 4: --dry-run prints the preview line and does not
// persist anything to disk.
func TestLink_DiscoveredFrom_DryRun_NoWrite(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Source", Tags: []string{"test"}},
		{Type: "task", Title: "Follow-up", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	idSource, idFollowup := motes[0].ID, motes[1].ID

	linkDryRun = true
	defer func() { linkDryRun = false }()

	output := captureStdout(func() {
		_ = linkCmd.RunE(linkCmd, []string{idFollowup, "discovered_from", idSource})
	})

	if !strings.Contains(output, "[dry-run]") {
		t.Errorf("expected [dry-run] prefix, got:\n%s", output)
	}
	if !strings.Contains(output, idFollowup+" --discovered_from--> "+idSource) {
		t.Errorf("dry-run line missing expected edge, got:\n%s", output)
	}

	mFollowup, _ := mm.Read(idFollowup)
	if len(mFollowup.DiscoveredFrom) != 0 {
		t.Errorf("dry-run must not persist; got DiscoveredFrom=%v", mFollowup.DiscoveredFrom)
	}
}

// STORY-DISC-001 Scenario 5: linking to a non-existent target returns an error
// and does not modify the source.
func TestLink_DiscoveredFrom_TargetMissing_Errors(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Follow-up", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	idFollowup := motes[0].ID

	err := linkCmd.RunE(linkCmd, []string{idFollowup, "discovered_from", "motes-ZZZNOTREAL"})
	if err == nil {
		t.Fatal("expected error when target mote does not exist")
	}

	mFollowup, _ := mm.Read(idFollowup)
	if len(mFollowup.DiscoveredFrom) != 0 {
		t.Errorf("source must be unchanged on error; got DiscoveredFrom=%v", mFollowup.DiscoveredFrom)
	}
}

// STORY-DISC-001 Scenario 6: path-traversal-shaped IDs are rejected by
// security.ValidateMoteID before any I/O.
func TestLink_DiscoveredFrom_InvalidID_Errors(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Follow-up", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	idFollowup := motes[0].ID

	err := linkCmd.RunE(linkCmd, []string{idFollowup, "discovered_from", "../../etc/passwd"})
	if err == nil {
		t.Fatal("expected error when target ID contains path traversal")
	}
	if !strings.Contains(err.Error(), "invalid target ID") {
		t.Errorf("expected 'invalid target ID' in error, got: %v", err)
	}

	mFollowup, _ := mm.Read(idFollowup)
	if len(mFollowup.DiscoveredFrom) != 0 {
		t.Errorf("source must be unchanged on invalid ID; got DiscoveredFrom=%v", mFollowup.DiscoveredFrom)
	}
}

// STORY-DISC-001 Scenario 8: unlink clears both frontmatter and reverse index
// edge; on next save the YAML omits the field entirely.
func TestUnlink_DiscoveredFrom_RemovesEdge_CLI(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Source", Tags: []string{"test"}},
		{Type: "task", Title: "Follow-up", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	_, _ = im.Load()
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	idSource, idFollowup := motes[0].ID, motes[1].ID

	_ = mm.Link(idFollowup, "discovered_from", idSource, im)

	output := captureStdout(func() {
		_ = unlinkCmd.RunE(unlinkCmd, []string{idFollowup, "discovered_from", idSource})
	})

	if !strings.Contains(output, "Unlinked") {
		t.Errorf("expected 'Unlinked' in output, got:\n%s", output)
	}

	mFollowup, _ := mm.Read(idFollowup)
	if len(mFollowup.DiscoveredFrom) != 0 {
		t.Errorf("follow-up DiscoveredFrom should be empty after unlink, got %v", mFollowup.DiscoveredFrom)
	}

	idx, _ := im.Load()
	for _, e := range idx.Edges {
		if e.EdgeType == "discovered_ref" || e.EdgeType == "discovered_from" {
			t.Errorf("index still has %s edge %s->%s after unlink", e.EdgeType, e.Source, e.Target)
		}
	}
}

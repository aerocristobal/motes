// SPDX-License-Identifier: MIT
package main

import (
	"strings"
	"testing"

	"motes/internal/core"
)

// STORY-DISC-001 Scenario 2: `mote context motes-AAA` includes a "Discovered"
// section listing motes-BBB. The reverse edge is read from the in-memory
// index (motes-AAA's frontmatter stays untouched).
func TestContext_ShowsDiscoveredSection_ForSingleID(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Source provenance task", Tags: []string{"provtest"}},
		{Type: "task", Title: "Follow-up race in retry loop", Tags: []string{"provtest"}},
	})

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	_, _ = im.Load()
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	idSource, idFollowup := motes[0].ID, motes[1].ID
	_ = mm.Link(idFollowup, "discovered_from", idSource, im)

	all, _ := mm.ReadAllParallel()
	_ = im.Rebuild(all)

	output := captureStdout(func() {
		_ = contextCmd.RunE(contextCmd, []string{idSource})
	})

	if !strings.Contains(output, "Discovered") {
		t.Errorf("context for source must contain 'Discovered' section, got:\n%s", output)
	}
	if !strings.Contains(output, idFollowup) {
		t.Errorf("context for source must list follow-up ID %s, got:\n%s", idFollowup, output)
	}
}

// Silent when the mote has no incoming discovered_ref edges.
func TestContext_NoDiscoveredSection_WhenNoChildren(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Lone task provenance", Tags: []string{"provtest"}},
	})

	mm := core.NewMoteManager(root)
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	idLone := motes[0].ID

	output := captureStdout(func() {
		_ = contextCmd.RunE(contextCmd, []string{idLone})
	})

	if strings.Contains(output, "Discovered while working") {
		t.Errorf("context must not emit Discovered section when no children, got:\n%s", output)
	}
}

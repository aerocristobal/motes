// SPDX-License-Identifier: MIT
package main

import (
	"strings"
	"testing"

	"motes/internal/core"
)

// STORY-DISC-001 Scenario 2 (show side): `mote show` for the discovered child
// renders its `discovered_from` row; `mote show` for the source mote renders a
// `discovered` row read from reverse index edges.
func TestShow_DiscoveredFromAndDiscoveredSections(t *testing.T) {
	root, cleanup := setupIntegrationTest(t)
	defer cleanup()

	seedMotes(t, root, []moteSpec{
		{Type: "task", Title: "Source task", Tags: []string{"test"}},
		{Type: "task", Title: "Follow-up race in retry loop", Tags: []string{"test"}},
	})

	mm := core.NewMoteManager(root)
	im := core.NewIndexManager(root)
	_, _ = im.Load()
	motes, _ := mm.List(core.ListFilters{Type: "task"})
	idSource, idFollowup := motes[0].ID, motes[1].ID
	_ = mm.Link(idFollowup, "discovered_from", idSource, im)

	// Rebuild index from frontmatter so the reverse edge is visible to show.
	all, _ := mm.ReadAllParallel()
	_ = im.Rebuild(all)

	childOutput := captureStdout(func() {
		_ = showCmd.RunE(showCmd, []string{idFollowup})
	})
	if !strings.Contains(childOutput, "discovered_from") {
		t.Errorf("show on child must list discovered_from, got:\n%s", childOutput)
	}
	if !strings.Contains(childOutput, idSource) {
		t.Errorf("show on child must reference source ID %s, got:\n%s", idSource, childOutput)
	}

	parentOutput := captureStdout(func() {
		_ = showCmd.RunE(showCmd, []string{idSource})
	})
	if !strings.Contains(parentOutput, "discovered") {
		t.Errorf("show on source must list 'discovered' children, got:\n%s", parentOutput)
	}
	if !strings.Contains(parentOutput, idFollowup) {
		t.Errorf("show on source must reference follow-up ID %s, got:\n%s", idFollowup, parentOutput)
	}
}

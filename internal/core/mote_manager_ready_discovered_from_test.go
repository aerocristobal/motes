// SPDX-License-Identifier: MIT
package core

import "testing"

// STORY-DISC-001 Scenario 3: a mote whose only link to an unfinished mote is
// discovered_from must still appear as ready. Readiness only consults
// depends_on, so discovered_from is non-blocking by construction.
func TestList_Ready_IgnoresDiscoveredFromEdges(t *testing.T) {
	_, mm, im := setupTestLink(t)

	source, _ := mm.Create("task", "Source still active", CreateOpts{})
	_ = mm.Update(source.ID, UpdateOpts{Status: StringPtr("in_progress")})

	followup, _ := mm.Create("task", "Discovered follow-up", CreateOpts{})

	if err := mm.Link(followup.ID, "discovered_from", source.ID, im); err != nil {
		t.Fatal(err)
	}

	motes, err := mm.List(ListFilters{Ready: true})
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, m := range motes {
		if m.ID == followup.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("follow-up should be ready despite source being in_progress; got %d ready motes", len(motes))
	}
}

// Regression guard: adding discovered_from must not weaken depends_on's
// blocking effect.
func TestList_Ready_StillRespectsDependsOn(t *testing.T) {
	_, mm, im := setupTestLink(t)

	blocker, _ := mm.Create("task", "Blocker", CreateOpts{})
	blocked, _ := mm.Create("task", "Blocked", CreateOpts{})

	_ = mm.Link(blocked.ID, "depends_on", blocker.ID, im)

	motes, err := mm.List(ListFilters{Ready: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range motes {
		if m.ID == blocked.ID {
			t.Errorf("blocked task must NOT be ready while its depends_on target is active")
		}
	}
}

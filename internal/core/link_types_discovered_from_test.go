// SPDX-License-Identifier: MIT
package core

import "testing"

// STORY-DISC-001: discovered_from is registered with index-only reverse edge
// discovered_ref (mirroring builds_on/built_by_ref). Source-side asymmetric,
// no inverse frontmatter field, no auto-deprecate.
func TestValidLinkTypes_DiscoveredFromRegistered(t *testing.T) {
	behavior, ok := ValidLinkTypes["discovered_from"]
	if !ok {
		t.Fatal("discovered_from must be registered in ValidLinkTypes")
	}
	if behavior.Symmetric {
		t.Error("discovered_from must not be symmetric")
	}
	if behavior.InverseType != "" {
		t.Errorf("discovered_from must not write an inverse to frontmatter, got %q", behavior.InverseType)
	}
	if behavior.IndexReverse != "discovered_ref" {
		t.Errorf("expected IndexReverse=discovered_ref, got %q", behavior.IndexReverse)
	}
	if behavior.AutoDeprecate {
		t.Error("discovered_from must not auto-deprecate target")
	}
}

func TestGetLinkSlice_DiscoveredFrom(t *testing.T) {
	m := &Mote{DiscoveredFrom: []string{"motes-AAA", "motes-BBB"}}
	got := GetLinkSlice(m, "discovered_from")
	if len(got) != 2 || got[0] != "motes-AAA" || got[1] != "motes-BBB" {
		t.Errorf("GetLinkSlice(discovered_from) = %v, want [motes-AAA motes-BBB]", got)
	}
}

func TestSetLinkSlice_DiscoveredFrom(t *testing.T) {
	m := &Mote{}
	SetLinkSlice(m, "discovered_from", []string{"motes-AAA"})
	if len(m.DiscoveredFrom) != 1 || m.DiscoveredFrom[0] != "motes-AAA" {
		t.Errorf("SetLinkSlice(discovered_from) did not assign field; got %v", m.DiscoveredFrom)
	}
}
